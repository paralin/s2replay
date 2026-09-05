package analysis

import (
	"encoding/binary"
	"io"
	"testing"

	"github.com/paralin/s2replay"
	"github.com/paralin/s2replay/protocol"
)

func TestRunbackModifierProjectionBindsPawnAndPreservesTiming(t *testing.T) {
	// Same entity index and player slot, different pawn serials.
	oldParent, newParent := uint32(7<<14|99), uint32(8<<14|99)
	serial1, serial2, serial3 := uint32(1), uint32(2), uint32(3)
	subclass, abilityClass := uint32(1344157725), uint32(77)
	ability, caster := uint32(6<<14|45), uint32(9<<14|46)
	duration, applied := float32(5), float32(10)
	stack := int32(2)
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
	old := &protocol.CModifierTableEntry{SerialNumber: &serial1, Parent: &oldParent, ModifierSubclass: &subclass, Duration: &duration, LastAppliedTime: &applied}
	appendTable(100, old, old)
	// Replacement must not inherit old duration; another old-pawn row remains open.
	unknown := &protocol.CModifierTableEntry{SerialNumber: &serial3, Parent: &newParent, ModifierSubclass: &subclass}
	appendTable(120, &protocol.CModifierTableEntry{SerialNumber: &serial2, Parent: &newParent, ModifierSubclass: &subclass}, old, unknown)
	appendTable(128, &protocol.CModifierTableEntry{SerialNumber: &serial2, Parent: &newParent, ModifierSubclass: &subclass, Duration: &duration, LastAppliedTime: &applied, Caster: &caster, Ability: &ability, AbilitySubclass: &abilityClass})
	// Actual partial payload: preserve prior identity/timing, record latest observation.
	appendTable(160, &protocol.CModifierTableEntry{StackCount: &stack})
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
		if ev.Tick == 120 && ev.Modifier.Transition == s2replay.ModifierAdd && ev.Modifier.TableIndex == 0 && (ev.Modifier.HasDuration || ev.Modifier.HasLastAppliedTime) {
			t.Fatal("replacement inherited old timing")
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
}
