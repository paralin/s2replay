package analysis

import "github.com/paralin/s2replay"

// InventoryTimeline stores player item ownership intervals.
type InventoryTimeline struct {
	Players map[int32][]InventoryInterval `json:"players"`
}

// InventoryInterval is a half-open player item-set interval.
type InventoryInterval struct {
	StartTick  uint32   `json:"start_tick"`
	EndTick    uint32   `json:"end_tick"`
	StartTime  float64  `json:"start_time"`
	EndTime    float64  `json:"end_time"`
	OwnedItems []uint32 `json:"owned_items"`
}

func (b *builder) acceptPurchase(ev s2replay.Event) {
	if ev.PlayerSlot < 0 {
		b.result.Quality.MissingPlayerSlotEvents++
		return
	}
	ownedItems := cloneItemSet(ev.OwnedItems)
	b.playerItems[ev.PlayerSlot] = ownedItems
	intervals := b.result.Inventory.Players[ev.PlayerSlot]
	if len(intervals) > 0 {
		last := &intervals[len(intervals)-1]
		if sameItemSet(last.OwnedItems, ownedItems) {
			return
		}
		last.EndTick = ev.Tick
		last.EndTime = ev.GameTime
	}
	intervals = append(intervals, InventoryInterval{
		StartTick:  ev.Tick,
		EndTick:    ev.Tick,
		StartTime:  ev.GameTime,
		EndTime:    ev.GameTime,
		OwnedItems: ownedItems,
	})
	b.result.Inventory.Players[ev.PlayerSlot] = intervals
	b.result.Quality.InventoryTransitions++
}
