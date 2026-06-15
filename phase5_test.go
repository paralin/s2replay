package s2replay

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const phase5GoldenPath = "testdata/phase5_modifier_events.json"

func TestPhase5ModifierLifecycleGolden(t *testing.T) {
	demoPath := os.Getenv("S2REPLAY_TEST_DEM")
	if demoPath == "" {
		demoPath = filepath.Join(os.Getenv("HOME"), "repos/deadlock-replays/48345595.dem")
	}
	if _, err := os.Stat(demoPath); err != nil {
		t.Skipf("set S2REPLAY_TEST_DEM to a Deadlock .dem to run modifier decode gate: %v", err)
	}

	demo, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}
	events, counts, err := collectCheckedModifierEvents(p, 24)
	if err != nil {
		t.Fatal(err)
	}
	if counts[ModifierAdd] == 0 || counts[ModifierRefresh] == 0 || counts[ModifierRemove] == 0 {
		t.Fatalf("modifier lifecycle counts missing transition: add=%d refresh=%d remove=%d events=%d", counts[ModifierAdd], counts[ModifierRefresh], counts[ModifierRemove], len(events))
	}
	if len(events) != 24 {
		t.Fatalf("modifier event sample: want 24, got %d", len(events))
	}

	got := formatPhase5Golden(events, counts)
	if os.Getenv("S2REPLAY_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(phase5GoldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(phase5GoldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(phase5GoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatalf("modifier golden mismatch; rerun with S2REPLAY_UPDATE_GOLDEN=1 after verifying the decoded sample")
	}
}

func collectCheckedModifierEvents(p *Parser, limit int) ([]ModifierEvent, map[ModifierTransition]int, error) {
	events := make([]ModifierEvent, 0, limit)
	counts := map[ModifierTransition]int{}
	active := make(map[int32]ModifierEvent)
	for len(events) < limit || counts[ModifierRemove] == 0 || counts[ModifierRefresh] == 0 {
		ev, err := p.NextModifierEvent()
		if err == io.EOF {
			return events, counts, nil
		}
		if err != nil {
			return events, counts, err
		}
		counts[ev.Transition]++
		switch ev.Transition {
		case ModifierAdd:
			active[ev.TableIndex] = ev
		case ModifierRefresh:
			if _, ok := active[ev.TableIndex]; !ok {
				return events, counts, errModifierRefreshWithoutAdd
			}
			active[ev.TableIndex] = ev
		case ModifierRemove:
			if _, ok := active[ev.TableIndex]; !ok || !ev.MatchedPrior {
				return events, counts, errModifierRemoveWithoutAdd
			}
			delete(active, ev.TableIndex)
		}
		if len(events) < limit {
			if ev.Duration > 0 || ev.Transition == ModifierRemove {
				events = append(events, ev)
			}
		}
	}
	return events, counts, nil
}

func formatPhase5Golden(events []ModifierEvent, counts map[ModifierTransition]int) []byte {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	buf.WriteString("  \"counts\": {\n")
	writeJSONInt(&buf, "add", int64(counts[ModifierAdd]), true)
	writeJSONInt(&buf, "refresh", int64(counts[ModifierRefresh]), true)
	writeJSONInt(&buf, "remove", int64(counts[ModifierRemove]), false)
	buf.WriteString("  },\n")
	buf.WriteString("  \"modifier_events\": [\n")
	for i, ev := range events {
		if i > 0 {
			buf.WriteString(",\n")
		}
		writeModifierEventJSON(&buf, ev)
	}
	buf.WriteString("\n  ]\n")
	buf.WriteString("}\n")
	return buf.Bytes()
}

func writeModifierEventJSON(buf *bytes.Buffer, ev ModifierEvent) {
	buf.WriteString("    {\n")
	writeJSONUint(buf, "tick", uint64(ev.Tick), true)
	writeJSONFloat64(buf, "game_time", ev.GameTime, true)
	writeJSONString(buf, "transition", string(ev.Transition), true)
	writeJSONInt(buf, "table_index", int64(ev.TableIndex), true)
	writeJSONUint(buf, "parent", uint64(ev.Parent), true)
	writeJSONUint(buf, "serial_number", uint64(ev.SerialNumber), true)
	writeJSONUint(buf, "modifier_subclass", uint64(ev.ModifierSubclass), true)
	writeJSONInt(buf, "stack_count", int64(ev.StackCount), true)
	writeJSONInt(buf, "max_stack_count", int64(ev.MaxStackCount), true)
	writeJSONFloat32(buf, "last_applied_time", ev.LastAppliedTime, true)
	writeJSONFloat32(buf, "duration", ev.Duration, true)
	writeJSONUint(buf, "caster", uint64(ev.Caster), true)
	writeJSONUint(buf, "ability", uint64(ev.Ability), true)
	writeJSONInt(buf, "aura_provider_serial_number", int64(ev.AuraProviderSerialNumber), true)
	writeJSONUint(buf, "aura_provider_ehandle", uint64(ev.AuraProviderEHandle), true)
	writeJSONUint(buf, "ability_subclass", uint64(ev.AbilitySubclass), true)
	writeJSONBool(buf, "in_aura_range", ev.InAuraRange, true)
	writeJSONBool(buf, "matched_prior", ev.MatchedPrior, false)
	buf.WriteString("    }")
}
