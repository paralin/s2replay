package analysis

import (
	"testing"

	"github.com/paralin/s2replay"
)

func TestModifierReplacementStartsNewInterval(t *testing.T) {
	events := []s2replay.Event{
		modifierEvent(1, 7, s2replay.ModifierAdd, 99, 42, 1),
		modifierEvent(3, 7, s2replay.ModifierRemove, 99, 42, 1),
		modifierEvent(3, 7, s2replay.ModifierAdd, 99, 42, 1),
		modifierEvent(4, 7, s2replay.ModifierRefresh, 99, 42, 2),
		modifierEvent(5, 7, s2replay.ModifierRemove, 99, 42, 2),
	}
	intervals := Build(events).Modifiers.Modifiers
	if len(intervals) != 2 {
		t.Fatalf("intervals: %+v", intervals)
	}
	if intervals[0].StartTick != 1 || intervals[0].EndTick != 3 || intervals[0].Open || intervals[1].StartTick != 3 || intervals[1].EndTick != 5 || intervals[1].Open || intervals[1].Refreshes != 1 {
		t.Fatalf("replacement lifetimes: %+v", intervals)
	}
	active := runbackModifiers(Build(events[:4]), &s2replay.EntitySample{Entity: 99, EntitySerial: 0}, 4)
	if len(active) != 1 || active[0].StartTick != 3 || active[0].StackCount != 2 {
		t.Fatalf("active replacement: %+v", active)
	}
}
