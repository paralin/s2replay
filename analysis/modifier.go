package analysis

import (
	"cmp"

	"github.com/paralin/s2replay"
)

// ModifierTimeline stores modifier lifecycle intervals.
type ModifierTimeline struct {
	Modifiers []ModifierInterval `json:"modifiers"`
}

// ModifierInterval is a closed or replay-end-open modifier lifecycle interval.
type ModifierInterval struct {
	StartTick        uint32  `json:"start_tick"`
	EndTick          uint32  `json:"end_tick"`
	StartTime        float64 `json:"start_time"`
	EndTime          float64 `json:"end_time"`
	TableIndex       int32   `json:"table_index"`
	EntityID         int32   `json:"entity_id"`
	PlayerSlot       int32   `json:"player_slot"`
	Parent           uint32  `json:"parent"`
	ModifierSubclass uint32  `json:"modifier_subclass"`
	SourceID         uint32  `json:"source_id"`
	Ability          uint32  `json:"ability"`
	StackCount       int32   `json:"stack_count"`
	Refreshes        int     `json:"refreshes"`
	Open             bool    `json:"open"`
}

func (b *builder) acceptModifier(ev s2replay.Event) {
	if ev.Modifier == nil {
		return
	}
	switch ev.Modifier.Transition {
	case s2replay.ModifierAdd:
		b.addModifierInterval(ev)
		b.result.Quality.ModifierAdds++
	case s2replay.ModifierRefresh:
		b.refreshModifierInterval(ev)
		b.result.Quality.ModifierRefreshes++
	case s2replay.ModifierRemove:
		b.removeModifierInterval(ev)
		b.result.Quality.ModifierRemoves++
	}
}

func (b *builder) addModifierInterval(ev s2replay.Event) {
	if active, ok := b.activeModifiers[ev.Modifier.TableIndex]; ok {
		active.EndTick = ev.Tick
		active.EndTime = ev.GameTime
		active.Open = true
		b.result.Modifiers.Modifiers = append(b.result.Modifiers.Modifiers, active)
		b.result.Quality.OpenModifierIntervals++
	}
	b.activeModifiers[ev.Modifier.TableIndex] = modifierIntervalFromEvent(ev)
}

func (b *builder) refreshModifierInterval(ev s2replay.Event) {
	active, ok := b.activeModifiers[ev.Modifier.TableIndex]
	if !ok {
		b.result.Quality.UnmatchedModifierRefreshes++
		b.activeModifiers[ev.Modifier.TableIndex] = modifierIntervalFromEvent(ev)
		return
	}
	active.Parent = ev.Modifier.Parent
	active.EntityID = ev.Entity
	active.PlayerSlot = ev.PlayerSlot
	active.ModifierSubclass = ev.Modifier.ModifierSubclass
	active.SourceID = ev.Modifier.AbilitySubclass
	active.Ability = ev.Modifier.Ability
	active.StackCount = ev.Modifier.StackCount
	active.Refreshes++
	b.activeModifiers[ev.Modifier.TableIndex] = active
}

func (b *builder) removeModifierInterval(ev s2replay.Event) {
	active, ok := b.activeModifiers[ev.Modifier.TableIndex]
	if !ok {
		b.result.Quality.UnmatchedModifierRemoves++
		return
	}
	active.EndTick = ev.Tick
	active.EndTime = ev.GameTime
	active.Parent = ev.Modifier.Parent
	active.EntityID = ev.Entity
	active.PlayerSlot = ev.PlayerSlot
	active.ModifierSubclass = ev.Modifier.ModifierSubclass
	active.SourceID = ev.Modifier.AbilitySubclass
	active.Ability = ev.Modifier.Ability
	active.StackCount = ev.Modifier.StackCount
	delete(b.activeModifiers, ev.Modifier.TableIndex)
	b.result.Modifiers.Modifiers = append(b.result.Modifiers.Modifiers, active)
}

func modifierIntervalFromEvent(ev s2replay.Event) ModifierInterval {
	return ModifierInterval{
		StartTick:        ev.Tick,
		EndTick:          ev.Tick,
		StartTime:        ev.GameTime,
		EndTime:          ev.GameTime,
		TableIndex:       ev.Modifier.TableIndex,
		EntityID:         ev.Entity,
		PlayerSlot:       ev.PlayerSlot,
		Parent:           ev.Modifier.Parent,
		ModifierSubclass: ev.Modifier.ModifierSubclass,
		SourceID:         ev.Modifier.AbilitySubclass,
		Ability:          ev.Modifier.Ability,
		StackCount:       ev.Modifier.StackCount,
	}
}

func compareModifierInterval(a, b ModifierInterval) int {
	if n := cmp.Compare(a.StartTick, b.StartTick); n != 0 {
		return n
	}
	if n := cmp.Compare(a.EndTick, b.EndTick); n != 0 {
		return n
	}
	return cmp.Compare(a.TableIndex, b.TableIndex)
}
