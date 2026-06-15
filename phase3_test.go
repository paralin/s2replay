package s2replay

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

const phase3GoldenPath = "testdata/phase3_damage_golden.json"

func TestPhase3DamageDecodeGolden(t *testing.T) {
	demoPath := os.Getenv("S2REPLAY_TEST_DEM")
	if demoPath == "" {
		demoPath = filepath.Join(os.Getenv("HOME"), "repos/deadlock-replays/48345595.dem")
	}
	if _, err := os.Stat(demoPath); err != nil {
		t.Skipf("set S2REPLAY_TEST_DEM to a Deadlock .dem to run decode gate: %v", err)
	}

	demo, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}
	events, err := collectBoundedDamage(p, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 8 {
		t.Fatalf("damage sample: want 8 events, got %d", len(events))
	}
	if !p.Clock().TickIntervalKnown() || p.Clock().TickInterval() <= 0 {
		t.Fatalf("tick interval was not decoded from ServerInfo: %.9f", p.Clock().TickInterval())
	}
	for i, ev := range events {
		if ev.Damage <= 0 {
			t.Fatalf("event %d damage is not sane: %+v", i, ev)
		}
		if ev.Effectiveness < 0 {
			t.Fatalf("event %d effectiveness is not sane: %+v", i, ev)
		}
		if ev.Attacker < 0 || ev.Victim < 0 {
			t.Fatalf("event %d missing entity context: %+v", i, ev)
		}
	}

	got := formatPhase3Golden(p.Clock().TickInterval(), events)
	if os.Getenv("S2REPLAY_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(phase3GoldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(phase3GoldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(phase3GoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatalf("damage golden mismatch; rerun with S2REPLAY_UPDATE_GOLDEN=1 after verifying the decoded sample")
	}
}

func collectBoundedDamage(p *Parser, limit int) ([]DamageEvent, error) {
	events := make([]DamageEvent, 0, limit)
	for len(events) < limit {
		ev, err := p.NextDamage()
		if err != nil {
			return events, err
		}
		if ev.VictimHealthMax > 0 && ev.VictimHealthNew >= 0 && ev.VictimHealthNew <= ev.VictimHealthMax {
			events = append(events, ev)
		}
	}
	return events, nil
}

func TestNextDamageReportsEOF(t *testing.T) {
	p, err := NewParser(buildDemo(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.NextDamage(); err != io.EOF {
		t.Fatalf("empty demo: want io.EOF, got %v", err)
	}
}

func formatPhase3Golden(interval float64, events []DamageEvent) []byte {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	buf.WriteString("  \"tick_interval\": ")
	buf.WriteString(strconv.FormatFloat(interval, 'f', 9, 64))
	buf.WriteString(",\n")
	buf.WriteString("  \"damage_events\": [\n")
	for i, ev := range events {
		if i > 0 {
			buf.WriteString(",\n")
		}
		writeDamageEventJSON(&buf, ev)
	}
	buf.WriteString("\n  ]\n")
	buf.WriteString("}\n")
	return buf.Bytes()
}

func writeDamageEventJSON(buf *bytes.Buffer, ev DamageEvent) {
	buf.WriteString("    {\n")
	writeJSONUint(buf, "tick", uint64(ev.Tick), true)
	writeJSONFloat64(buf, "game_time", ev.GameTime, true)
	writeJSONInt(buf, "damage", int64(ev.Damage), true)
	writeJSONFloat32(buf, "pre_damage", ev.PreDamage, true)
	writeJSONInt(buf, "victim_health_new", int64(ev.VictimHealthNew), true)
	writeJSONInt(buf, "victim_health_max", int64(ev.VictimHealthMax), true)
	writeJSONFloat32(buf, "damage_absorbed", ev.DamageAbsorbed, true)
	writeJSONFloat32(buf, "effectiveness", ev.Effectiveness, true)
	writeJSONFloat32(buf, "crit_damage", ev.CritDamage, true)
	writeJSONInt(buf, "hits", int64(ev.Hits), true)
	writeJSONInt(buf, "attacker", int64(ev.Attacker), true)
	writeJSONInt(buf, "victim", int64(ev.Victim), true)
	writeJSONInt(buf, "inflictor", int64(ev.Inflictor), true)
	writeJSONInt(buf, "ability_entity", int64(ev.AbilityEntity), true)
	writeJSONUint(buf, "ability_id", uint64(ev.AbilityID), true)
	writeJSONInt(buf, "damage_type", int64(ev.DamageType), true)
	writeJSONInt(buf, "citadel_damage_type", int64(ev.CitadelDamageType), true)
	writeJSONInt(buf, "attacking_object", int64(ev.AttackingObject), true)
	writeJSONInt(buf, "victim_shield_new", int64(ev.VictimShieldNew), true)
	writeJSONInt(buf, "victim_shield_max", int64(ev.VictimShieldMax), true)
	writeJSONInt(buf, "health_lost", int64(ev.HealthLost), true)
	writeJSONInt(buf, "hitgroup_id", int64(ev.HitgroupID), true)
	writeJSONBool(buf, "is_secondary_stat", ev.IsSecondaryStat, true)
	writeJSONFloat32(buf, "origin_x", ev.OriginX, true)
	writeJSONFloat32(buf, "origin_y", ev.OriginY, true)
	writeJSONFloat32(buf, "origin_z", ev.OriginZ, true)
	writeJSONFloat32(buf, "damage_direction_x", ev.DamageDirectionX, true)
	writeJSONFloat32(buf, "damage_direction_y", ev.DamageDirectionY, true)
	writeJSONFloat32(buf, "damage_direction_z", ev.DamageDirectionZ, true)
	writeJSONInt(buf, "server_tick", int64(ev.ServerTick), true)
	writeJSONUint(buf, "flags", ev.Flags, true)
	writeJSONUint(buf, "attacker_class", uint64(ev.AttackerClass), true)
	writeJSONUint(buf, "victim_class", uint64(ev.VictimClass), true)
	writeJSONInt(buf, "pre_damage_deprecated", int64(ev.PreDamageDeprecated), true)
	writeJSONInt(buf, "damage_absorbed_deprecated", int64(ev.DamageAbsorbedDeprecated), false)
	buf.WriteString("    }")
}

func writeJSONInt(buf *bytes.Buffer, name string, v int64, comma bool) {
	writeJSONName(buf, name)
	buf.WriteString(strconv.FormatInt(v, 10))
	writeJSONLineEnd(buf, comma)
}

func writeJSONUint(buf *bytes.Buffer, name string, v uint64, comma bool) {
	writeJSONName(buf, name)
	buf.WriteString(strconv.FormatUint(v, 10))
	writeJSONLineEnd(buf, comma)
}

func writeJSONBool(buf *bytes.Buffer, name string, v bool, comma bool) {
	writeJSONName(buf, name)
	buf.WriteString(strconv.FormatBool(v))
	writeJSONLineEnd(buf, comma)
}

func writeJSONFloat32(buf *bytes.Buffer, name string, v float32, comma bool) {
	writeJSONName(buf, name)
	buf.WriteString(strconv.FormatFloat(float64(v), 'f', 6, 32))
	writeJSONLineEnd(buf, comma)
}

func writeJSONFloat64(buf *bytes.Buffer, name string, v float64, comma bool) {
	writeJSONName(buf, name)
	buf.WriteString(strconv.FormatFloat(v, 'f', 6, 64))
	writeJSONLineEnd(buf, comma)
}

func writeJSONName(buf *bytes.Buffer, name string) {
	buf.WriteString("      \"")
	buf.WriteString(name)
	buf.WriteString("\": ")
}

func writeJSONLineEnd(buf *bytes.Buffer, comma bool) {
	if comma {
		buf.WriteByte(',')
	}
	buf.WriteByte('\n')
}
