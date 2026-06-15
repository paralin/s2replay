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

type ModifierEvent struct {
	Tick                     uint32             `json:"tick"`
	GameTime                 float64            `json:"game_time"`
	Transition               ModifierTransition `json:"transition"`
	TableIndex               int32              `json:"table_index"`
	Parent                   uint32             `json:"parent"`
	SerialNumber             uint32             `json:"serial_number"`
	ModifierSubclass         uint32             `json:"modifier_subclass"`
	StackCount               int32              `json:"stack_count"`
	MaxStackCount            int32              `json:"max_stack_count"`
	LastAppliedTime          float32            `json:"last_applied_time"`
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
		ev = mergeModifierRemove(ev, prev.entry)
		delete(p.modifiers, item.index)
		p.pendingModifiers = append(p.pendingModifiers, ev)
		p.appendModifierEvent(ev)
		return nil
	}
	if hadPrev {
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
		ModifierSubclass:         entry.GetModifierSubclass(),
		StackCount:               entry.GetStackCount(),
		MaxStackCount:            entry.GetMaxStackCount(),
		LastAppliedTime:          entry.GetLastAppliedTime(),
		Duration:                 entry.GetDuration(),
		Caster:                   entry.GetCaster(),
		Ability:                  entry.GetAbility(),
		AuraProviderSerialNumber: entry.GetAuraProviderSerialNumber(),
		AuraProviderEHandle:      entry.GetAuraProviderEhandle(),
		AbilitySubclass:          entry.GetAbilitySubclass(),
		InAuraRange:              entry.GetInAuraRange(),
	}
}

func mergeModifierRemove(remove, prior ModifierEvent) ModifierEvent {
	if remove.Parent == protocol.Default_CModifierTableEntry_Parent {
		remove.Parent = prior.Parent
	}
	if remove.SerialNumber == 0 {
		remove.SerialNumber = prior.SerialNumber
	}
	if remove.ModifierSubclass == 0 {
		remove.ModifierSubclass = prior.ModifierSubclass
	}
	if remove.StackCount == 0 {
		remove.StackCount = prior.StackCount
	}
	if remove.MaxStackCount == 0 {
		remove.MaxStackCount = prior.MaxStackCount
	}
	if remove.LastAppliedTime == 0 {
		remove.LastAppliedTime = prior.LastAppliedTime
	}
	if remove.Duration == protocol.Default_CModifierTableEntry_Duration {
		remove.Duration = prior.Duration
	}
	if remove.Caster == protocol.Default_CModifierTableEntry_Caster {
		remove.Caster = prior.Caster
	}
	if remove.Ability == protocol.Default_CModifierTableEntry_Ability {
		remove.Ability = prior.Ability
	}
	if remove.AuraProviderSerialNumber == 0 {
		remove.AuraProviderSerialNumber = prior.AuraProviderSerialNumber
	}
	if remove.AuraProviderEHandle == protocol.Default_CModifierTableEntry_AuraProviderEhandle {
		remove.AuraProviderEHandle = prior.AuraProviderEHandle
	}
	if remove.AbilitySubclass == 0 {
		remove.AbilitySubclass = prior.AbilitySubclass
	}
	return remove
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
