package s2replay

import (
	"io"
	"strconv"

	"github.com/paralin/s2replay/protocol"
)

type ModifierTransition string

const (
	ModifierAdd     ModifierTransition = "add"
	ModifierRemove  ModifierTransition = "remove"
	ModifierRefresh ModifierTransition = "refresh"
)

// ModifierEvent records an instance transition and the last observed payload.
// Presence distinguishes unknown serial/timing from explicit zero or indefinite duration.
type ModifierEvent struct {
	Tick                     uint32             `json:"tick"`
	GameTime                 float64            `json:"game_time"`
	Transition               ModifierTransition `json:"transition"`
	TableIndex               int32              `json:"table_index"`
	Parent                   uint32             `json:"parent"`
	HasSerialNumber          bool               `json:"has_serial_number"`
	SerialNumber             uint32             `json:"serial_number"`
	ModifierSubclass         uint32             `json:"modifier_subclass"`
	StackCount               int32              `json:"stack_count"`
	MaxStackCount            int32              `json:"max_stack_count"`
	HasLastAppliedTime       bool               `json:"has_last_applied_time"`
	LastAppliedTime          float32            `json:"last_applied_time"`
	HasDuration              bool               `json:"has_duration"`
	Duration                 float32            `json:"duration"`
	Caster                   uint32             `json:"caster"`
	Ability                  uint32             `json:"ability"`
	AuraProviderSerialNumber int32              `json:"aura_provider_serial_number"`
	AuraProviderEHandle      uint32             `json:"aura_provider_ehandle"`
	AbilitySubclass          uint32             `json:"ability_subclass"`
	InAuraRange              bool               `json:"in_aura_range"`
	MatchedPrior             bool               `json:"matched_prior"`
}

type modifierState struct {
	entry ModifierEvent
}

func (p *Parser) applyActiveModifierItem(tick uint32, item *stringTableItem) error {
	if item == nil || len(item.value) == 0 {
		return nil
	}
	if tick == PreGameTick {
		tick = 0
	}
	entry := &protocol.CModifierTableEntry{}
	if err := entry.UnmarshalVT(item.value); err != nil {
		return modifierDecodeError{index: item.index, err: err}
	}
	ev := modifierEventFromEntry(tick, p.clock.GameTime(), item.index, entry)
	prev, hadPrev := p.modifiers[item.index]
	if entry.GetEntryType() == protocol.MODIFIER_ENTRY_TYPE_MODIFIER_ENTRY_TYPE_REMOVED {
		if !hadPrev {
			return nil
		}
		ev.Transition = ModifierRemove
		ev.MatchedPrior = true
		ev = mergeModifierUpdate(ev, prev.entry, entry)
		delete(p.modifiers, item.index)
		p.pendingModifiers = append(p.pendingModifiers, ev)
		p.appendModifierEvent(ev)
		return nil
	}
	// A present serial change authoritatively replaces the table occupant.
	// Missing serials are partial updates, not a replacement with serial zero.
	hasSerial := entry.SerialNumber != nil
	if hadPrev && hasSerial && prev.entry.HasSerialNumber && ev.SerialNumber != prev.entry.SerialNumber {
		removed := prev.entry
		removed.Tick, removed.GameTime = ev.Tick, ev.GameTime
		removed.Transition, removed.MatchedPrior = ModifierRemove, true
		p.pendingModifiers = append(p.pendingModifiers, removed)
		p.appendModifierEvent(removed)
		hadPrev = false
	}
	if hadPrev {
		ev = mergeModifierUpdate(ev, prev.entry, entry)
		ev.Transition = ModifierRefresh
		ev.MatchedPrior = true
	} else {
		ev.Transition = ModifierAdd
	}
	p.modifiers[item.index] = modifierState{entry: ev}
	p.pendingModifiers = append(p.pendingModifiers, ev)
	p.appendModifierEvent(ev)
	return nil
}

func (p *Parser) appendModifierEvent(ev ModifierEvent) {
	entity := int32(ev.Parent & uint32(entityHandleMask))
	slot, ok := p.entityPlayerSlots[entity]
	if !ok {
		slot = -1
	}
	p.pendingEvents = append(p.pendingEvents, Event{
		Type:       EventModifier,
		Tick:       ev.Tick,
		GameTime:   ev.GameTime,
		Entity:     entity,
		PlayerSlot: slot,
		Modifier:   &ev,
	})
}

func modifierEventFromEntry(tick uint32, gameTime float64, tableIndex int32, entry *protocol.CModifierTableEntry) ModifierEvent {
	return ModifierEvent{
		Tick:                     tick,
		GameTime:                 gameTime,
		TableIndex:               tableIndex,
		Parent:                   entry.GetParent(),
		SerialNumber:             entry.GetSerialNumber(),
		HasSerialNumber:          entry.SerialNumber != nil,
		ModifierSubclass:         entry.GetModifierSubclass(),
		StackCount:               entry.GetStackCount(),
		MaxStackCount:            entry.GetMaxStackCount(),
		LastAppliedTime:          entry.GetLastAppliedTime(),
		HasLastAppliedTime:       entry.LastAppliedTime != nil,
		Duration:                 entry.GetDuration(),
		HasDuration:              entry.Duration != nil,
		Caster:                   entry.GetCaster(),
		Ability:                  entry.GetAbility(),
		AuraProviderSerialNumber: entry.GetAuraProviderSerialNumber(),
		AuraProviderEHandle:      entry.GetAuraProviderEhandle(),
		AbilitySubclass:          entry.GetAbilitySubclass(),
		InAuraRange:              entry.GetInAuraRange(),
	}
}

// mergeModifierUpdate retains omitted fields only within the same instance.
// Explicit zero and explicit indefinite duration (-1) remain observed values.
func mergeModifierUpdate(update, prior ModifierEvent, entry *protocol.CModifierTableEntry) ModifierEvent {
	if entry.Parent == nil {
		update.Parent = prior.Parent
	}
	if entry.SerialNumber == nil {
		update.SerialNumber = prior.SerialNumber
		update.HasSerialNumber = prior.HasSerialNumber
	}
	if entry.ModifierSubclass == nil {
		update.ModifierSubclass = prior.ModifierSubclass
	}
	if entry.StackCount == nil {
		update.StackCount = prior.StackCount
	}
	if entry.MaxStackCount == nil {
		update.MaxStackCount = prior.MaxStackCount
	}
	if entry.LastAppliedTime == nil {
		update.LastAppliedTime = prior.LastAppliedTime
		update.HasLastAppliedTime = prior.HasLastAppliedTime
	}
	if entry.Duration == nil {
		update.Duration = prior.Duration
		update.HasDuration = prior.HasDuration
	}
	if entry.Caster == nil {
		update.Caster = prior.Caster
	}
	if entry.Ability == nil {
		update.Ability = prior.Ability
	}
	if entry.AuraProviderSerialNumber == nil {
		update.AuraProviderSerialNumber = prior.AuraProviderSerialNumber
	}
	if entry.AuraProviderEhandle == nil {
		update.AuraProviderEHandle = prior.AuraProviderEHandle
	}
	if entry.AbilitySubclass == nil {
		update.AbilitySubclass = prior.AbilitySubclass
	}
	if entry.InAuraRange == nil {
		update.InAuraRange = prior.InAuraRange
	}
	return update
}

func (p *Parser) NextModifierEvent() (ModifierEvent, error) {
	for len(p.pendingModifiers) == 0 {
		if _, err := p.NextMessage(); err != nil {
			return ModifierEvent{}, err
		}
	}
	ev := p.pendingModifiers[0]
	copy(p.pendingModifiers, p.pendingModifiers[1:])
	p.pendingModifiers = p.pendingModifiers[:len(p.pendingModifiers)-1]
	return ev, nil
}

func (p *Parser) CollectModifierEvents(limit int) ([]ModifierEvent, error) {
	var events []ModifierEvent
	for limit <= 0 || len(events) < limit {
		ev, err := p.NextModifierEvent()
		if err == io.EOF {
			return events, nil
		}
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

type modifierDecodeError struct {
	index int32
	err   error
}

func (e modifierDecodeError) Error() string {
	return e.err.Error() + " string_table=ActiveModifiers index=" + strconv.Itoa(int(e.index))
}

func (e modifierDecodeError) Unwrap() error {
	return e.err
}
