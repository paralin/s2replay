package analysis

import (
	"bytes"
	"cmp"

	"github.com/paralin/s2replay"
)

// ModifierTimeline stores modifier lifecycle intervals.
type ModifierTimeline struct {
	Modifiers []ModifierInterval `json:"modifiers"`
}

// ModifierInterval is a closed or replay-end-open modifier lifecycle interval.
type ModifierInterval struct {
	// PayloadProto retains opaque CModifierTableEntry binary state, including
	// presence and unknown fields. JSON encodes these bytes as base64.
	PayloadProto       []byte  `json:"payload_proto,omitempty"`
	StartTick          uint32  `json:"start_tick"`
	EndTick            uint32  `json:"end_tick"`
	StartTime          float64 `json:"start_time"`
	EndTime            float64 `json:"end_time"`
	LastObservedTick   uint32  `json:"last_observed_tick"`
	SerialNumber       uint32  `json:"serial_number"`
	HasSerialNumber    bool    `json:"has_serial_number"`
	Caster             uint32  `json:"caster"`
	Duration           float32 `json:"duration"`
	HasDuration        bool    `json:"has_duration"`
	LastAppliedTime    float32 `json:"last_applied_time"`
	HasLastAppliedTime bool    `json:"has_last_applied_time"`
	TableIndex         int32   `json:"table_index"`
	EntityID           int32   `json:"entity_id"`
	PlayerSlot         int32   `json:"player_slot"`
	Parent             uint32  `json:"parent"`
	ModifierSubclass   uint32  `json:"modifier_subclass"`
	SourceID           uint32  `json:"source_id"`
	Ability            uint32  `json:"ability"`
	StackCount         int32   `json:"stack_count"`
	Refreshes          int     `json:"refreshes"`
	Open               bool    `json:"open"`
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
	active.PayloadProto = bytes.Clone(ev.Modifier.PayloadProto)
	active.LastObservedTick = ev.Tick
	active.SerialNumber, active.HasSerialNumber = ev.Modifier.SerialNumber, ev.Modifier.HasSerialNumber
	active.Caster = ev.Modifier.Caster
	active.Duration, active.HasDuration = ev.Modifier.Duration, ev.Modifier.HasDuration
	active.LastAppliedTime, active.HasLastAppliedTime = ev.Modifier.LastAppliedTime, ev.Modifier.HasLastAppliedTime
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
	active.PayloadProto = bytes.Clone(ev.Modifier.PayloadProto)
	active.LastObservedTick = ev.Tick
	active.SerialNumber, active.HasSerialNumber = ev.Modifier.SerialNumber, ev.Modifier.HasSerialNumber
	active.Caster = ev.Modifier.Caster
	active.Duration, active.HasDuration = ev.Modifier.Duration, ev.Modifier.HasDuration
	active.LastAppliedTime, active.HasLastAppliedTime = ev.Modifier.LastAppliedTime, ev.Modifier.HasLastAppliedTime
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
		PayloadProto:     bytes.Clone(ev.Modifier.PayloadProto),
		StartTick:        ev.Tick,
		LastObservedTick: ev.Tick,
		SerialNumber:     ev.Modifier.SerialNumber, HasSerialNumber: ev.Modifier.HasSerialNumber,
		Caster:   ev.Modifier.Caster,
		Duration: ev.Modifier.Duration, HasDuration: ev.Modifier.HasDuration,
		LastAppliedTime: ev.Modifier.LastAppliedTime, HasLastAppliedTime: ev.Modifier.HasLastAppliedTime,
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
