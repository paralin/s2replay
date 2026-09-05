package analysis

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"testing"

	"github.com/paralin/s2replay"
	"github.com/paralin/s2replay/protocol"
)

func TestRunbackModifierProjectionBindsPawnAndPreservesTiming(t *testing.T) {
	decodePayload := func(data []byte) *protocol.CModifierTableEntry {
		t.Helper()
		if len(data) == 0 {
			t.Fatal("missing modifier payload")
		}
		entry := &protocol.CModifierTableEntry{}
		if err := entry.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
		return entry
	}
	assertPayloadEqual := func(want, got *protocol.CModifierTableEntry) {
		t.Helper()
		// NaNs compare unequal in generated EqualVT; compare source bits first.
		if want.Float3_ == nil || got.Float3_ == nil || math.Float32bits(*want.Float3_) != math.Float32bits(*got.Float3_) {
			t.Fatal("source NaN bits changed")
		}
		want, got = want.CloneVT(), got.CloneVT()
		want.Float3_, got.Float3_ = nil, nil
		if !got.EqualVT(want) {
			t.Fatal("decoded payloads differ")
		}
	}
	// Same entity index and player slot, different pawn serials.
	oldParent, newParent := uint32(7<<14|99), uint32(8<<14|99)
	serial1, serial2, serial3 := uint32(1), uint32(2), uint32(3)
	subclass, abilityClass := uint32(1344157725), uint32(77)
	ability, caster := uint32(6<<14|45), uint32(9<<14|46)
	duration, applied := float32(5), float32(10)
	stack := int32(2)
	truth, falsity := true, false
	customInt, zeroInt := int32(9), int32(0)
	nan, infinity := math.Float32frombits(0x7fc01234), float32(math.Inf(1))
	x, y, zeroFloat := float32(3), float32(4), float32(0)
	unknownWire := binary.AppendUvarint(nil, 1000<<3)
	unknownWire = binary.AppendUvarint(unknownWire, 7)
	demo := append([]byte("PBDEMS2\x00"), make([]byte, 8)...)
	tableName := "ActiveModifiers"
	appendTable := func(tick uint32, entries ...*protocol.CModifierTableEntry) {
		t.Helper()
		table := &protocol.CDemoStringTablesTableT{TableName: &tableName}
		for _, entry := range entries {
			data, err := entry.MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			if tick == 128 || tick == 160 {
				data = append(data, unknownWire...)
			}
			table.Items = append(table.Items, &protocol.CDemoStringTablesItemsT{Data: data})
		}
		data, err := (&protocol.CDemoStringTables{Tables: []*protocol.CDemoStringTablesTableT{table}}).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		demo = binary.AppendUvarint(demo, uint64(protocol.EDemoCommands_DEM_StringTables))
		demo = binary.AppendUvarint(demo, uint64(tick))
		demo = binary.AppendUvarint(demo, uint64(len(data)))
		demo = append(demo, data...)
	}
	old := &protocol.CModifierTableEntry{SerialNumber: &serial1, Parent: &oldParent, ModifierSubclass: &subclass, Duration: &duration, LastAppliedTime: &applied, Bool2_: &truth}
	appendTable(100, old, old)
	// Replacement must not inherit old duration; another old-pawn row remains open.
	unknown := &protocol.CModifierTableEntry{SerialNumber: &serial3, Parent: &newParent, ModifierSubclass: &subclass}
	appendTable(120, &protocol.CModifierTableEntry{SerialNumber: &serial2, Parent: &newParent, ModifierSubclass: &subclass}, old, unknown)
	appendTable(128, &protocol.CModifierTableEntry{SerialNumber: &serial2, Parent: &newParent, ModifierSubclass: &subclass, Duration: &duration, LastAppliedTime: &applied, Caster: &caster, Ability: &ability, AbilitySubclass: &abilityClass, Bool1_: &truth, Int1_: &customInt, Float3_: &nan, Float4_: &infinity, Vec1_: &protocol.CMsgVector{X: &x}})
	// Actual partial payload: preserve prior identity/timing, record latest observation.
	for range 2 {
		appendTable(160, &protocol.CModifierTableEntry{StackCount: &stack, Bool1_: &falsity, Int1_: &zeroInt, Float1_: &zeroFloat, Vec1_: &protocol.CMsgVector{Y: &y}})
	}
	parser, err := s2replay.NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}
	// This fixture contains only outer string-table commands, so NextMessage
	// reaches EOF after applying them; their typed events remain queued.
	if _, err := parser.NextMessage(); err != io.EOF {
		t.Fatalf("outer table stream: %v", err)
	}
	var events []s2replay.Event
	if err := consumeReplayEvents(parser, func(ev s2replay.Event) {
		if ev.Modifier == nil {
			return
		}
		// Slot attribution cannot distinguish these incarnations; the handle must.
		ev.PlayerSlot = 1
		events = append(events, ev)
		if ev.Tick == 120 && ev.Modifier.Transition == s2replay.ModifierAdd && ev.Modifier.TableIndex == 0 && (ev.Modifier.HasDuration || ev.Modifier.HasLastAppliedTime || decodePayload(ev.Modifier.PayloadProto).Bool2_ != nil) {
			t.Fatal("replacement inherited old timing or custom payload")
		}
	}); err != nil {
		t.Fatal(err)
	}

	timelines := Build(events)
	pawn := runbackPawn(200, 99, 1)
	pawn.EntitySerial = 8
	facts, err := buildRunbackFacts([]s2replay.EntitySample{pawn}, timelines, ReplaySourceIdentity{}, RunbackRequest{Tick: 200}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := facts.Heroes[0].Modifiers
	if len(rows) != 2 {
		t.Fatalf("exact-pawn modifiers: %+v", rows)
	}
	var timed, omitted *RunbackModifier
	for i := range rows {
		if rows[i].Parent != newParent {
			t.Fatalf("old pawn leaked: %+v", rows[i])
		}
		if rows[i].SerialNumber == serial2 {
			timed = &rows[i]
		} else {
			omitted = &rows[i]
		}
	}
	if timed == nil || omitted == nil {
		t.Fatalf("instance identity lost: %+v", rows)
	}
	if timed.TableIndex != 0 || !timed.HasSerialNumber || timed.StartTick != 120 || timed.LastObservedTick != 160 || timed.StackCount != 2 || timed.Caster != caster || timed.Ability != ability || timed.AbilitySubclass != abilityClass {
		t.Fatalf("timed identity: %+v", timed)
	}
	if !timed.HasDuration || timed.Duration != 5 || !timed.HasLastAppliedTime || timed.LastAppliedTime != 10 {
		t.Fatalf("timing: %+v", timed)
	}
	// At independently observed server time12, the retained timing yields3s;
	// demo StartTick is not substituted for server last-applied seconds.
	if remaining := timed.LastAppliedTime + timed.Duration - 12; remaining != 3 {
		t.Fatalf("remaining=%v", remaining)
	}
	if omitted.TableIndex != 2 || omitted.HasDuration || omitted.HasLastAppliedTime || !omitted.HasSerialNumber || omitted.LastObservedTick != 120 {
		t.Fatalf("unknown timing: %+v", omitted)
	}
	last := events[len(events)-1].ToProto()
	encoded, err := last.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	var decoded protocol.ReplayEvent
	if err := decoded.UnmarshalVT(encoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Modifier.HasSerialNumber || !decoded.Modifier.HasDuration || !decoded.Modifier.HasLastAppliedTime {
		t.Fatalf("wire presence lost: %+v", decoded.Modifier)
	}
	payload := decodePayload(timed.PayloadProto)
	if payload.Bool1_ == nil || *payload.Bool1_ || payload.Int1_ == nil || *payload.Int1_ != 0 || payload.Float1_ == nil || *payload.Float1_ != 0 || payload.Bool2_ != nil || payload.Float2_ != nil || payload.Vec1_ == nil || payload.Vec1_.X == nil || *payload.Vec1_.X != x || payload.Vec1_.Y == nil || *payload.Vec1_.Y != y || payload.Vec1_.Z != nil {
		t.Fatalf("custom presence/partial update lost: %v", payload)
	}
	if payload.Float3_ == nil || math.Float32bits(*payload.Float3_) != 0x7fc01234 || payload.Float4_ == nil || math.Float32bits(*payload.Float4_) != math.Float32bits(infinity) {
		t.Fatal("opaque source nonfinite float bits changed")
	}
	assertPayloadEqual(payload, decodePayload(decoded.Modifier.PayloadProto))
	wire, err := payload.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(wire, bytes.Repeat(unknownWire, 3)) {
		t.Fatal("unknown wire occurrences lost across parser/projection")
	}
	// JSON base64 preserves the binary payload, including unknowns and NaN bits.
	jsonBytes, err := json.Marshal(timed)
	if err != nil {
		t.Fatal(err)
	}
	var jsonRow RunbackModifier
	if err := json.Unmarshal(jsonBytes, &jsonRow); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(jsonRow.PayloadProto, timed.PayloadProto) {
		t.Fatal("JSON base64 changed payload bytes")
	}
	assertPayloadEqual(payload, decodePayload(jsonRow.PayloadProto))
	for _, ev := range events {
		if ev.Tick == 128 {
			prior := decodePayload(ev.Modifier.PayloadProto)
			if prior.Int1_ == nil || *prior.Int1_ != 9 || prior.Vec1_.Y != nil {
				t.Fatal("later partial updates mutated earlier event payload")
			}
		}
	}
	// Facts and wire projections own their byte slices independently.
	before := bytes.Clone(timed.PayloadProto)
	timed.PayloadProto[0] ^= 0xff
	for _, interval := range timelines.Modifiers.Modifiers {
		if interval.SerialNumber == serial2 && !bytes.Equal(interval.PayloadProto, before) {
			t.Fatal("facts payload aliases timeline state")
		}
	}
	last.Modifier.PayloadProto[0] ^= 0xff
	if !bytes.Equal(events[len(events)-1].Modifier.PayloadProto, before) {
		t.Fatal("wire payload aliases event state")
	}
}
