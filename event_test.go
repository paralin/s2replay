package s2replay

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const attributedEventsGoldenPath = "testdata/attributed_events_golden.json"

func TestAttributedEventStreamGolden(t *testing.T) {
	demoPath := os.Getenv("S2REPLAY_TEST_DEM")
	if demoPath == "" {
		demoPath = filepath.Join(os.Getenv("HOME"), "repos/deadlock-replays/48345595.dem")
	}
	if _, err := os.Stat(demoPath); err != nil {
		t.Skipf("set S2REPLAY_TEST_DEM to a Deadlock .dem to run attributed event gate: %v", err)
	}

	demo, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}
	events, counts, attributedDamage, err := collectAttributedEventSample(p)
	if err != nil {
		t.Fatal(err)
	}
	if counts[EventPurchase] == 0 || counts[EventModifier] == 0 || counts[EventEntitySample] == 0 || counts[EventDamage] == 0 {
		t.Fatalf("event stream missing required type counts: %v", counts)
	}
	if attributedDamage < 8 {
		t.Fatalf("attributed hero damage events: want at least 8, got %d", attributedDamage)
	}
	if len(events) == 0 {
		t.Fatalf("event sample is empty")
	}

	got := formatAttributedEventsGolden(events, counts, attributedDamage)
	if os.Getenv("S2REPLAY_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(attributedEventsGoldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(attributedEventsGoldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(attributedEventsGoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatalf("attributed event golden mismatch; rerun with S2REPLAY_UPDATE_GOLDEN=1 after verifying the decoded sample")
	}
}

func collectAttributedEventSample(p *Parser) ([]Event, map[EventType]int, int, error) {
	var sample []Event
	counts := map[EventType]int{}
	selected := map[EventType]int{}
	attributedDamage := 0
	attributedHeroDamage := 0
	for {
		ev, err := p.NextEvent()
		if err == io.EOF {
			return sample, counts, attributedDamage, nil
		}
		if err != nil {
			return sample, counts, attributedDamage, err
		}
		counts[ev.Type]++
		switch ev.Type {
		case EventPurchase:
			if selected[EventPurchase] < 4 {
				sample = append(sample, ev)
				selected[EventPurchase]++
			}
		case EventModifier:
			if ev.Modifier != nil && ev.Modifier.Duration > 0 && selected[EventModifier] < 2 {
				sample = append(sample, ev)
				selected[EventModifier]++
			}
		case EventEntitySample:
			if ev.EntitySample != nil && ev.EntitySample.HasHealth && selected[EventEntitySample] < 2 {
				sample = append(sample, ev)
				selected[EventEntitySample]++
			}
		case EventDamage:
			if ev.Damage == nil || len(ev.OwnedItems) == 0 || ev.PlayerSlot < 0 {
				continue
			}
			attributedDamage++
			if entity := p.FindEntity(ev.Damage.Attacker); entity != nil && isLikelyHeroClass(entity.ClassName()) {
				attributedHeroDamage++
			}
			if selected[EventDamage] < 8 {
				sample = append(sample, ev)
				selected[EventDamage]++
			}
		}
		if selected[EventPurchase] >= 4 &&
			selected[EventModifier] >= 2 &&
			selected[EventEntitySample] >= 2 &&
			selected[EventDamage] >= 8 &&
			attributedHeroDamage > 0 {
			return sample, counts, attributedDamage, nil
		}
	}
}

func formatAttributedEventsGolden(events []Event, counts map[EventType]int, attributedDamage int) []byte {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	buf.WriteString("  \"counts\": {\n")
	writeJSONInt(&buf, "damage", int64(counts[EventDamage]), true)
	writeJSONInt(&buf, "modifier", int64(counts[EventModifier]), true)
	writeJSONInt(&buf, "purchase", int64(counts[EventPurchase]), true)
	writeJSONInt(&buf, "entity_sample", int64(counts[EventEntitySample]), true)
	writeJSONInt(&buf, "attributed_damage", int64(attributedDamage), false)
	buf.WriteString("  },\n")
	buf.WriteString("  \"events\": [\n")
	for i, ev := range events {
		if i > 0 {
			buf.WriteString(",\n")
		}
		writeAttributedEventJSON(&buf, ev)
	}
	buf.WriteString("\n  ]\n")
	buf.WriteString("}\n")
	return buf.Bytes()
}

func writeAttributedEventJSON(buf *bytes.Buffer, ev Event) {
	buf.WriteString("    {\n")
	writeJSONString(buf, "type", string(ev.Type), true)
	writeJSONUint(buf, "tick", uint64(ev.Tick), true)
	writeJSONFloat64(buf, "game_time", ev.GameTime, true)
	writeJSONInt(buf, "entity", int64(ev.Entity), true)
	writeJSONInt(buf, "player_slot", int64(ev.PlayerSlot), true)
	writeJSONUintArray(buf, "owned_items", ev.OwnedItems, true)
	switch ev.Type {
	case EventDamage:
		if ev.Damage != nil {
			writeJSONInt(buf, "damage", int64(ev.Damage.Damage), true)
			writeJSONInt(buf, "attacker", int64(ev.Damage.Attacker), true)
			writeJSONInt(buf, "victim", int64(ev.Damage.Victim), true)
			writeJSONUint(buf, "ability_id", uint64(ev.Damage.AbilityID), false)
		}
	case EventModifier:
		if ev.Modifier != nil {
			writeJSONString(buf, "transition", string(ev.Modifier.Transition), true)
			writeJSONUint(buf, "parent", uint64(ev.Modifier.Parent), true)
			writeJSONUint(buf, "ability", uint64(ev.Modifier.Ability), true)
			writeJSONFloat32(buf, "duration", ev.Modifier.Duration, false)
		}
	case EventPurchase:
		if ev.Purchase != nil {
			writeJSONInt(buf, "user_id", int64(ev.Purchase.UserID), true)
			writeJSONUint(buf, "ability_id", uint64(ev.Purchase.AbilityID), true)
			writeJSONString(buf, "change", ev.Purchase.Change, true)
			writeJSONBool(buf, "sell", ev.Purchase.Sell, true)
			writeJSONBool(buf, "quickbuy", ev.Purchase.Quickbuy, true)
			writeJSONString(buf, "source", ev.Purchase.Source, false)
		}
	case EventEntitySample:
		if ev.EntitySample != nil {
			writeJSONString(buf, "class_name", ev.EntitySample.ClassName, true)
			writeJSONFloat32(buf, "health", ev.EntitySample.Health, true)
			writeJSONFloat32(buf, "max_health", ev.EntitySample.MaxHealth, true)
			writeJSONBool(buf, "has_position", ev.EntitySample.HasPosition, false)
		}
	}
	buf.WriteString("    }")
}

func writeJSONUintArray(buf *bytes.Buffer, name string, values []uint32, comma bool) {
	writeJSONName(buf, name)
	buf.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(strconv.FormatUint(uint64(v), 10))
	}
	buf.WriteByte(']')
	writeJSONLineEnd(buf, comma)
}

func TestAttributionTrace(t *testing.T) {
	if os.Getenv("S2REPLAY_TRACE_ATTRIBUTION") != "1" {
		t.Skip("set S2REPLAY_TRACE_ATTRIBUTION=1 to trace player-slot attribution")
	}
	demoPath := os.Getenv("S2REPLAY_TEST_DEM")
	if demoPath == "" {
		demoPath = filepath.Join(os.Getenv("HOME"), "repos/deadlock-replays/48345595.dem")
	}
	demo, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}
	purchases := 0
	damage := 0
	attributed := 0
	for {
		ev, err := p.NextEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch ev.Type {
		case EventPurchase:
			purchases++
			if purchases <= 20 {
				t.Logf("purchase tick=%d slot=%d user=%d ability=%d change=%s sell=%v source=%s owned=%v", ev.Tick, ev.PlayerSlot, ev.Purchase.UserID, ev.Purchase.AbilityID, ev.Purchase.Change, ev.Purchase.Sell, ev.Purchase.Source, ev.OwnedItems)
			}
		case EventDamage:
			damage++
			if len(ev.OwnedItems) != 0 {
				attributed++
				if attributed <= 20 {
					t.Logf("damage tick=%d attacker=%d slot=%d owned=%v ability=%d damage=%d", ev.Tick, ev.Damage.Attacker, ev.PlayerSlot, ev.OwnedItems, ev.Damage.AbilityID, ev.Damage.Damage)
				}
			}
		}
	}
	t.Logf("totals purchases=%d damage=%d attributed=%d player_items=%v entity_slots=%v", purchases, damage, attributed, p.playerItems, p.entityPlayerSlots)
	for _, line := range traceHeroIdentityFields(p) {
		t.Log(line)
	}
	for _, line := range tracePlayerClassSchemas(p) {
		t.Log(line)
	}
}

func traceHeroIdentityFields(p *Parser) []string {
	var lines []string
	for _, e := range p.entities {
		if e == nil || e.class == nil || (!isLikelyHeroClass(e.class.name) && !strings.Contains(e.class.name, "PlayerController")) {
			continue
		}
		if name, ok := p.entityName(e); ok {
			lines = append(lines, "entity="+strconv.Itoa(int(e.index))+" class="+e.class.name+" entity_name="+name)
		}
		for _, field := range traceEntityIdentityFields(e) {
			lines = append(lines, "entity="+strconv.Itoa(int(e.index))+" class="+e.class.name+" "+field)
		}
	}
	sort.Strings(lines)
	return lines
}

func traceEntityIdentityFields(e *Entity) []string {
	var out []string
	var walk func(*fieldState, []int)
	walk = func(state *fieldState, prefix []int) {
		keys := make([]int, 0, len(state.values))
		for key := range state.values {
			keys = append(keys, key)
		}
		sort.Ints(keys)
		for _, key := range keys {
			v := state.values[key]
			path := append(append([]int(nil), prefix...), key)
			if child, ok := v.(*fieldState); ok {
				walk(child, path)
				continue
			}
			var fp fieldPath
			fp.last = len(path) - 1
			for i, part := range path {
				fp.path[i] = part
			}
			name := e.class.fieldName(fp)
			lower := strings.ToLower(name)
			if strings.Contains(lower, "player") ||
				strings.Contains(lower, "user") ||
				strings.Contains(lower, "slot") ||
				strings.Contains(lower, "pawn") ||
				strings.Contains(lower, "hero") {
				out = append(out, name+"="+strconvAny(v))
			}
		}
	}
	walk(e.state, nil)
	sort.Strings(out)
	return out
}

func tracePlayerClassSchemas(p *Parser) []string {
	var lines []string
	for _, class := range p.classesByID {
		if class == nil || class.serializer == nil {
			continue
		}
		className := strings.ToLower(class.name)
		if !strings.Contains(className, "player") && !strings.Contains(className, "citadel") {
			continue
		}
		for _, name := range traceSerializerFieldNames(class.serializer, nil) {
			lower := strings.ToLower(name)
			if strings.Contains(lower, "playerslot") ||
				strings.Contains(lower, "player_slot") ||
				strings.Contains(lower, "playerpawn") ||
				strings.Contains(lower, "pawn") ||
				strings.Contains(lower, "hero") ||
				strings.Contains(lower, "userid") ||
				strings.Contains(lower, "steam") {
				lines = append(lines, "schema class="+class.name+" field="+name)
			}
		}
	}
	sort.Strings(lines)
	return lines
}

func traceSerializerFieldNames(s *serializer, prefix []string) []string {
	var out []string
	for _, f := range s.fields {
		parts := append([]string(nil), prefix...)
		if f.sendNode != "" {
			parts = append(parts, strings.Split(f.sendNode, ".")...)
		}
		parts = append(parts, f.varName)
		switch f.model {
		case fieldModelFixedTable:
			if f.serializer != nil {
				out = append(out, traceSerializerFieldNames(f.serializer, parts)...)
				continue
			}
		case fieldModelVariableTable:
			if f.serializer != nil {
				withIndex := append(append([]string(nil), parts...), "0000")
				out = append(out, traceSerializerFieldNames(f.serializer, withIndex)...)
			}
		}
		out = append(out, strings.Join(parts, "."))
	}
	return out
}

func strconvAny(v any) string {
	switch x := v.(type) {
	case bool:
		return strconv.FormatBool(x)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', 6, 32)
	case string:
		return x
	default:
		return ""
	}
}
