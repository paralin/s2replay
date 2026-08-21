package analysis

import (
	"testing"

	"github.com/paralin/s2replay"
)

func TestBuildInventoryTimelineCoalescesItemSets(t *testing.T) {
	result := Build([]s2replay.Event{
		purchaseEvent(10, 1, []uint32{200, 100}),
		{Type: s2replay.EventDamage, Tick: 12, GameTime: 12, PlayerSlot: 1, OwnedItems: []uint32{100, 200}},
		purchaseEvent(20, 1, []uint32{100, 200}),
		purchaseEvent(30, 1, []uint32{300, 100}),
		purchaseEvent(40, 1, nil),
		{Type: s2replay.EventDamage, Tick: 50, GameTime: 50, PlayerSlot: 1},
	})
	intervals := result.Inventory.Players[1]
	if len(intervals) != 3 {
		t.Fatalf("inventory intervals: want 3, got %d: %+v", len(intervals), intervals)
	}
	assertInterval(t, intervals[0], 10, 30, []uint32{100, 200})
	assertInterval(t, intervals[1], 30, 40, []uint32{100, 300})
	assertInterval(t, intervals[2], 40, 50, []uint32{})
	if result.Quality.InventoryTransitions != 3 {
		t.Fatalf("inventory transitions: want 3, got %d", result.Quality.InventoryTransitions)
	}
}

func TestBuildEntityTimelineRetainsReplayFacts(t *testing.T) {
	result := Build([]s2replay.Event{
		{
			Type:       s2replay.EventEntitySample,
			Tick:       22,
			GameTime:   1.5,
			Entity:     44,
			PlayerSlot: 3,
			OwnedItems: []uint32{9, 2},
			EntitySample: &s2replay.EntitySample{
				ClassID:     8,
				ClassName:   "CCitadelPlayerPawn",
				Health:      700,
				MaxHealth:   900,
				Shield:      11,
				MaxShield:   30,
				PositionX:   1,
				PositionY:   2,
				PositionZ:   3,
				HasHealth:   true,
				HasShield:   true,
				HasPosition: true,
			},
		},
	})
	byPlayer := result.Entities.Players[3]
	byEntity := result.Entities.Entities[44]
	if len(byPlayer) != 1 || len(byEntity) != 1 {
		t.Fatalf("entity samples missing player/entity indexes: players=%+v entities=%+v", result.Entities.Players, result.Entities.Entities)
	}
	sample := byPlayer[0]
	if sample.EntityID != 44 || sample.PlayerSlot != 3 || sample.Health != 700 || sample.MaxHealth != 900 || !sample.HasPosition {
		t.Fatalf("entity sample did not retain replay facts: %+v", sample)
	}
	if !sameItemSet(sample.OwnedItems, []uint32{2, 9}) {
		t.Fatalf("entity sample owned items: want [2 9], got %v", sample.OwnedItems)
	}
	if result.Quality.EntitySamples != 1 {
		t.Fatalf("entity sample count: want 1, got %d", result.Quality.EntitySamples)
	}
}

func TestBuildEntityTimelineUsesInventoryState(t *testing.T) {
	result := Build([]s2replay.Event{
		purchaseEvent(10, 3, []uint32{20, 10}),
		{
			Type:       s2replay.EventEntitySample,
			Tick:       12,
			GameTime:   12,
			Entity:     44,
			PlayerSlot: 3,
			EntitySample: &s2replay.EntitySample{
				Entity:    44,
				ClassName: "CCitadelPlayerPawn",
				Health:    700,
				MaxHealth: 900,
				HasHealth: true,
			},
		},
	})
	samples := result.Entities.Players[3]
	if len(samples) != 1 {
		t.Fatalf("entity samples: want 1, got %d", len(samples))
	}
	if !sameItemSet(samples[0].OwnedItems, []uint32{10, 20}) {
		t.Fatalf("entity sample inventory: want [10 20], got %v", samples[0].OwnedItems)
	}
}

func TestBuildModifierTimelineClosesIntervals(t *testing.T) {
	result := Build([]s2replay.Event{
		modifierEvent(1, 7, s2replay.ModifierAdd, 10, 42, 1),
		modifierEvent(5, 7, s2replay.ModifierRefresh, 10, 42, 2),
		modifierEvent(9, 7, s2replay.ModifierRemove, 10, 42, 2),
		modifierEvent(10, 8, s2replay.ModifierRemove, 20, 99, 1),
		modifierEvent(11, 9, s2replay.ModifierAdd, 30, 111, 1),
		{Type: s2replay.EventDamage, Tick: 12, GameTime: 12},
	})
	intervals := result.Modifiers.Modifiers
	if len(intervals) != 2 {
		t.Fatalf("modifier intervals: want 2, got %d: %+v", len(intervals), intervals)
	}
	closed := intervals[0]
	if closed.StartTick != 1 || closed.EndTick != 9 || closed.TableIndex != 7 || closed.SourceID != 42 || closed.StackCount != 2 || closed.Refreshes != 1 || closed.Open {
		t.Fatalf("closed modifier interval mismatch: %+v", closed)
	}
	open := intervals[1]
	if open.StartTick != 11 || open.EndTick != 12 || open.TableIndex != 9 || open.SourceID != 111 || !open.Open {
		t.Fatalf("open modifier interval mismatch: %+v", open)
	}
	if result.Quality.UnmatchedModifierRemoves != 1 || result.Quality.OpenModifierIntervals != 1 {
		t.Fatalf("modifier quality mismatch: %+v", result.Quality)
	}
}

func purchaseEvent(tick uint32, slot int32, items []uint32) s2replay.Event {
	return s2replay.Event{
		Type:       s2replay.EventPurchase,
		Tick:       tick,
		GameTime:   float64(tick),
		PlayerSlot: slot,
		OwnedItems: items,
		Purchase: &s2replay.PurchaseEvent{
			Tick:       tick,
			GameTime:   float64(tick),
			PlayerSlot: slot,
		},
	}
}

func modifierEvent(tick uint32, tableIndex int32, transition s2replay.ModifierTransition, subclass uint32, sourceID uint32, stack int32) s2replay.Event {
	return s2replay.Event{
		Type:       s2replay.EventModifier,
		Tick:       tick,
		GameTime:   float64(tick),
		Entity:     int32(subclass),
		PlayerSlot: 2,
		Modifier: &s2replay.ModifierEvent{
			Tick:             tick,
			GameTime:         float64(tick),
			Transition:       transition,
			TableIndex:       tableIndex,
			Parent:           subclass,
			ModifierSubclass: subclass,
			AbilitySubclass:  sourceID,
			StackCount:       stack,
		},
	}
}

func assertInterval(t *testing.T, got InventoryInterval, start uint32, end uint32, items []uint32) {
	t.Helper()
	if got.StartTick != start || got.EndTick != end {
		t.Fatalf("interval ticks: want [%d,%d), got [%d,%d): %+v", start, end, got.StartTick, got.EndTick, got)
	}
	if !sameItemSet(got.OwnedItems, items) {
		t.Fatalf("interval items: want %v, got %v", items, got.OwnedItems)
	}
}
