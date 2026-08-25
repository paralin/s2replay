package s2replay

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestProbeAbilityChargeFields walks a real demo when S2R_PROBE_DEMO is set
// and prints every ability-entity class plus charge-like field paths.
// Scratch validation for the dash-charge sampler; skipped in normal runs.
func TestProbeAbilityChargeFields(t *testing.T) {
	demoPath := os.Getenv("S2R_PROBE_DEMO")
	if demoPath == "" {
		t.Skip("set S2R_PROBE_DEMO to run")
	}
	raw, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(raw)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	chargeEvents := 0
	dashSamples := 0
	slotsSeen := map[int32]bool{}
	var walkErr error
	for {
		ev, err := p.NextEvent()
		if err != nil {
			walkErr = err
			break
		}
		n++
		if ev.Type == EventAbilityCharges {
			chargeEvents++
			slotsSeen[ev.PlayerSlot] = true
			ac := ev.AbilityCharges
			if strings.Contains(ac.ClassName, "Dash") && dashSamples < 5 {
				dashSamples++
				t.Logf("dash charge: slot %d %s charges=%d t=%d", ev.PlayerSlot, ac.ClassName, ac.RemainingCharges, ac.Tick)
			}
		}
	}
	t.Logf("events: %d, ability_charges: %d, distinct slots: %v", n, chargeEvents, slotsSeen)
	t.Logf("commands: %d, last err: %v", n, walkErr)
	t.Logf("entities tracked: %d, entity creates: %d", len(p.entities), p.entityCreates)
	t.Logf("firstEntityError: %q", p.firstEntityError)
	skipped := make([]string, 0)
	for k := range p.skippedMessages {
		skipped = append(skipped, fmt.Sprintf("%v x%d", k, p.skippedMessages[k]))
	}
	sort.Strings(skipped)
	if len(skipped) > 10 {
		skipped = skipped[:10]
	}
	t.Logf("skipped: %v", skipped)
	type info struct {
		fields map[string]bool
		entity *Entity
	}
	classes := map[string]*info{}
	var collect func(s *serializer, prefix string, out map[string]bool, depth int)
	collect = func(s *serializer, prefix string, out map[string]bool, depth int) {
		if s == nil || depth > 8 {
			return
		}
		for _, f := range s.fields {
			name := prefix + f.varName
			out[name] = true
			collect(f.serializer, name+".", out, depth+1)
		}
	}
	seenClass := map[string]bool{}
	for _, e := range p.entities {
		if e == nil || e.class == nil || !strings.Contains(e.class.name, "Ability") {
			continue
		}
		if seenClass[e.class.name] {
			continue
		}
		seenClass[e.class.name] = true
		all := map[string]bool{}
		collect(e.class.serializer, "", all, 0)
		rec := &info{fields: map[string]bool{}, entity: e}
		for path := range all {
			lower := strings.ToLower(path)
			if strings.Contains(lower, "charge") || strings.Contains(lower, "dash") ||
				strings.Contains(lower, "cooldown") ||
				strings.Contains(lower, "owner") || strings.Contains(lower, "caster") ||
				strings.Contains(lower, "controllingplayer") || strings.Contains(lower, "hmajor") ||
				strings.Contains(lower, "howner") {
				rec.fields[path] = true
			}
		}
		classes[e.class.name] = rec
	}
	names := make([]string, 0, len(classes))
	for k := range classes {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		if len(classes[name].fields) == 0 {
			continue
		}
		fields := make([]string, 0)
		for f := range classes[name].fields {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		fmt.Printf("%s:\n", name)
		for _, f := range fields {
			fmt.Printf("  %s = %v\n", f, classes[name].entity.Get(f))
		}
	}
}
