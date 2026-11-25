// Package analysis derives replay-local timelines from s2replay events.
package analysis

import (
	"maps"
	"slices"

	"github.com/paralin/s2replay"
)

// Result is the replay-local analysis output.
type Result struct {
	Inventory InventoryTimeline `json:"inventory"`
	Entities  EntityTimeline    `json:"entities"`
	Modifiers ModifierTimeline  `json:"modifiers"`
	Quality   Quality           `json:"quality"`
}

// Quality reports missing or unclosed replay facts observed while building
// timelines.
type Quality struct {
	Events                     int `json:"events"`
	InventoryTransitions       int `json:"inventory_transitions"`
	EntitySamples              int `json:"entity_samples"`
	ModifierAdds               int `json:"modifier_adds"`
	ModifierRefreshes          int `json:"modifier_refreshes"`
	ModifierRemoves            int `json:"modifier_removes"`
	MissingPlayerSlotEvents    int `json:"missing_player_slot_events"`
	UnmatchedModifierRefreshes int `json:"unmatched_modifier_refreshes"`
	UnmatchedModifierRemoves   int `json:"unmatched_modifier_removes"`
	OpenModifierIntervals      int `json:"open_modifier_intervals"`
}

// Build derives deterministic timelines from a complete replay event slice.
func Build(events []s2replay.Event) Result {
	result := Result{
		Inventory: InventoryTimeline{Players: make(map[int32][]InventoryInterval)},
		Entities: EntityTimeline{
			Players:  make(map[int32][]EntitySample),
			Entities: make(map[int32][]EntitySample),
		},
	}
	b := newBuilder(builderSinks{
		inventory: func(slot int32, iv InventoryInterval) {
			result.Inventory.Players[slot] = append(result.Inventory.Players[slot], iv)
		},
		modifier: func(iv ModifierInterval) {
			result.Modifiers.Modifiers = append(result.Modifiers.Modifiers, iv)
		},
		sample: func(slot int32, sample EntitySample) {
			if slot >= 0 {
				result.Entities.Players[slot] = append(result.Entities.Players[slot], sample)
			}
			result.Entities.Entities[sample.EntityID] = append(result.Entities.Entities[sample.EntityID], sample)
		},
	})
	for _, ev := range events {
		b.accept(ev)
	}
	b.finish()
	slices.SortFunc(result.Modifiers.Modifiers, compareModifierInterval)
	result.Quality = b.quality
	return result
}

// builderSinks receive the builder's output as each piece closes, so a
// streaming caller holds only the intervals still open.
type builderSinks struct {
	// inventory receives a player's closed item-set interval, in time order
	// per player.
	inventory func(slot int32, iv InventoryInterval)
	// modifier receives each closed or abandoned modifier interval.
	modifier func(iv ModifierInterval)
	// sample receives each entity sample with its player slot. Nil skips
	// building samples.
	sample func(slot int32, sample EntitySample)
}

// builder folds replay events into timelines one event at a time.
type builder struct {
	out             builderSinks
	quality         Quality
	lastTick        uint32
	lastTime        float64
	playerItems     map[int32][]uint32
	openInventory   map[int32]InventoryInterval
	activeModifiers map[int32]ModifierInterval
}

func newBuilder(out builderSinks) *builder {
	return &builder{
		out:             out,
		playerItems:     make(map[int32][]uint32),
		openInventory:   make(map[int32]InventoryInterval),
		activeModifiers: make(map[int32]ModifierInterval),
	}
}

func (b *builder) accept(ev s2replay.Event) {
	b.quality.Events++
	b.lastTick = ev.Tick
	b.lastTime = ev.GameTime
	switch ev.Type {
	case s2replay.EventPurchase:
		b.acceptPurchase(ev)
	case s2replay.EventEntitySample:
		b.acceptEntitySample(ev)
	case s2replay.EventModifier:
		b.acceptModifier(ev)
	}
}

// finish closes every open interval at the last event, in slot and table
// index order so the output does not depend on map order.
func (b *builder) finish() {
	for _, slot := range slices.Sorted(maps.Keys(b.openInventory)) {
		iv := b.openInventory[slot]
		iv.EndTick, iv.EndTime = b.lastTick, b.lastTime
		b.out.inventory(slot, iv)
	}
	clear(b.openInventory)
	for _, index := range slices.Sorted(maps.Keys(b.activeModifiers)) {
		iv := b.activeModifiers[index]
		iv.EndTick, iv.EndTime, iv.Open = b.lastTick, b.lastTime, true
		b.out.modifier(iv)
		b.quality.OpenModifierIntervals++
	}
	clear(b.activeModifiers)
}

func cloneItemSet(items []uint32) []uint32 {
	if len(items) == 0 {
		return []uint32{}
	}
	out := slices.Clone(items)
	slices.Sort(out)
	return out
}

func sameItemSet(a, b []uint32) bool {
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
