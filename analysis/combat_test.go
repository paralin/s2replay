package analysis

import (
	"testing"

	"github.com/paralin/s2replay"
)

func TestBuildCombatWindowsUsesCallerPolicy(t *testing.T) {
	events := []s2replay.Event{
		{Type: s2replay.EventDamage, Tick: 1, GameTime: 0},
		{Type: s2replay.EventPurchase, Tick: 2, GameTime: 2},
		{Type: s2replay.EventModifier, Tick: 3, GameTime: 4},
		{Type: s2replay.EventDamage, Tick: 4, GameTime: 13},
	}
	windows := BuildCombatWindows(events, CombatWindowOptions{
		MaxGap: 8,
		Include: func(ev s2replay.Event) bool {
			return ev.Type == s2replay.EventDamage || ev.Type == s2replay.EventModifier
		},
	})
	if len(windows) != 2 {
		t.Fatalf("combat windows: want 2, got %d: %+v", len(windows), windows)
	}
	assertCombatWindow(t, windows[0], 1, 3, 0, 4, 2, 0, 2)
	assertCombatWindow(t, windows[1], 4, 4, 13, 13, 1, 3, 3)
}

func TestBuildCombatWindowsBoundaryBehavior(t *testing.T) {
	events := []s2replay.Event{
		{Type: s2replay.EventDamage, Tick: 1, GameTime: 1},
		{Type: s2replay.EventDamage, Tick: 2, GameTime: 1},
		{Type: s2replay.EventDamage, Tick: 3, GameTime: 9},
		{Type: s2replay.EventDamage, Tick: 4, GameTime: 17.01},
	}
	windows := BuildCombatWindows(events, CombatWindowOptions{MaxGap: 8})
	if len(windows) != 2 {
		t.Fatalf("combat windows: want 2, got %d: %+v", len(windows), windows)
	}
	assertCombatWindow(t, windows[0], 1, 3, 1, 9, 3, 0, 2)
	assertCombatWindow(t, windows[1], 4, 4, 17.01, 17.01, 1, 3, 3)
}

func TestBuildCombatWindowsWithoutPolicyIncludesAllEvents(t *testing.T) {
	windows := BuildCombatWindows([]s2replay.Event{
		{Type: s2replay.EventPurchase, Tick: 1, GameTime: 1},
		{Type: s2replay.EventDamage, Tick: 2, GameTime: 2},
	}, CombatWindowOptions{MaxGap: 2})
	if len(windows) != 1 || windows[0].Events != 2 {
		t.Fatalf("default combat window policy should include all events: %+v", windows)
	}
}

func TestBuildCombatWindowsSummarizesReplayFacts(t *testing.T) {
	events := []s2replay.Event{
		{
			Type:       s2replay.EventEntitySample,
			Tick:       1,
			GameTime:   1,
			Entity:     200,
			PlayerSlot: 2,
			EntitySample: &s2replay.EntitySample{
				Entity: 200,
			},
		},
		{
			Type:       s2replay.EventDamage,
			Tick:       2,
			GameTime:   2,
			Entity:     100,
			PlayerSlot: 1,
			Damage: &s2replay.DamageEvent{
				Attacker: 100,
				Victim:   200,
			},
		},
		{
			Type:       s2replay.EventModifier,
			Tick:       3,
			GameTime:   3,
			Entity:     300,
			PlayerSlot: 1,
			Modifier: &s2replay.ModifierEvent{
				Parent: 400,
			},
		},
	}
	windows := BuildCombatWindows(events, CombatWindowOptions{
		MaxGap: 8,
		Include: func(ev s2replay.Event) bool {
			return ev.Type == s2replay.EventDamage || ev.Type == s2replay.EventModifier
		},
	})
	if len(windows) != 1 {
		t.Fatalf("combat windows: want 1, got %d: %+v", len(windows), windows)
	}
	window := windows[0]
	if window.Events != 2 || window.DamageEvents != 1 || window.ModifierEvents != 1 || window.EntitySamples != 0 {
		t.Fatalf("combat counts mismatch: %+v", window)
	}
	if !sameInt32Slice(window.PlayerSlots, []int32{1, 2}) {
		t.Fatalf("player slots: want [1 2], got %v", window.PlayerSlots)
	}
	if !sameInt32Slice(window.Entities, []int32{100, 200, 300}) {
		t.Fatalf("entities: want [100 200 300], got %v", window.Entities)
	}
	if !sameInt32Slice(window.DamageAttackers, []int32{100}) {
		t.Fatalf("damage attackers: want [100], got %v", window.DamageAttackers)
	}
	if !sameInt32Slice(window.DamageVictims, []int32{200}) {
		t.Fatalf("damage victims: want [200], got %v", window.DamageVictims)
	}
	if len(window.ModifierParents) != 1 || window.ModifierParents[0] != 400 {
		t.Fatalf("modifier parents: want [400], got %v", window.ModifierParents)
	}
}

func assertCombatWindow(t *testing.T, got CombatWindow, startTick uint32, endTick uint32, startTime float64, endTime float64, events int, firstIndex int, lastIndex int) {
	t.Helper()
	if got.StartTick != startTick || got.EndTick != endTick ||
		got.StartTime != startTime || got.EndTime != endTime ||
		got.Events != events || got.FirstEventIndex != firstIndex || got.LastEventIndex != lastIndex {
		t.Fatalf("combat window mismatch: got %+v", got)
	}
}

func sameInt32Slice(a []int32, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
