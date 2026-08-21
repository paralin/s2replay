// Package analysis derives replay-local timelines from s2replay events.
package analysis

import (
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
	b := newBuilder()
	for _, ev := range events {
		b.accept(ev)
	}
	return b.finalize()
}

type builder struct {
	result          Result
	lastTick        uint32
	lastTime        float64
	playerItems     map[int32][]uint32
	activeModifiers map[int32]ModifierInterval
}

func newBuilder() *builder {
	return &builder{
		result: Result{
			Inventory: InventoryTimeline{Players: make(map[int32][]InventoryInterval)},
			Entities: EntityTimeline{
				Players:  make(map[int32][]EntitySample),
				Entities: make(map[int32][]EntitySample),
			},
		},
		playerItems:     make(map[int32][]uint32),
		activeModifiers: make(map[int32]ModifierInterval),
	}
}

func (b *builder) accept(ev s2replay.Event) {
	b.result.Quality.Events++
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

func (b *builder) finalize() Result {
	for slot, intervals := range b.result.Inventory.Players {
		if len(intervals) == 0 {
			continue
		}
		intervals[len(intervals)-1].EndTick = b.lastTick
		intervals[len(intervals)-1].EndTime = b.lastTime
		b.result.Inventory.Players[slot] = intervals
	}
	for _, interval := range b.activeModifiers {
		interval.EndTick = b.lastTick
		interval.EndTime = b.lastTime
		interval.Open = true
		b.result.Modifiers.Modifiers = append(b.result.Modifiers.Modifiers, interval)
		b.result.Quality.OpenModifierIntervals++
	}
	slices.SortFunc(b.result.Modifiers.Modifiers, compareModifierInterval)
	return b.result
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
