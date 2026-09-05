package s2replay

import (
	"bytes"
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
	// PayloadProto is a serialized CModifierTableEntry after same-instance merging.
	// JSON base64 preserves unknown fields and source float bits without interpretation.
	PayloadProto             []byte             `json:"payload_proto,omitempty"`
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
	entry   ModifierEvent
	payload *protocol.CModifierTableEntry
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
	prev, hadPrev := p.modifiers[item.index]
	removal := entry.GetEntryType() == protocol.MODIFIER_ENTRY_TYPE_MODIFIER_ENTRY_TYPE_REMOVED
	if removal && !hadPrev {
		return nil
	}
	// A present serial change authoritatively replaces the table occupant.
	// Missing serials are partial updates, not a replacement with serial zero.
	replacement := !removal && hadPrev && entry.SerialNumber != nil && prev.entry.HasSerialNumber && entry.GetSerialNumber() != prev.entry.SerialNumber
	if replacement {
		hadPrev = false
	}
	if hadPrev {
		// The generated decoder merges present fields, including nested messages,
		// and appends unknown wire occurrences. Clone to retain earlier snapshots.
		entry = prev.payload.CloneVT()
		if err := entry.UnmarshalVT(item.value); err != nil {
			return modifierDecodeError{index: item.index, err: err}
		}
	}
	// Serialize before publishing transitions so a codec error cannot leave a
	// replacement half-published. Exported consumers never borrow typed state.
	payloadProto, err := entry.MarshalVT()
	if err != nil {
		return modifierDecodeError{index: item.index, err: err}
	}
	if replacement {
		removed := prev.entry
		removed.Tick, removed.GameTime = tick, p.clock.GameTime()
		removed.Transition, removed.MatchedPrior = ModifierRemove, true
		p.pendingModifiers = append(p.pendingModifiers, removed)
		p.appendModifierEvent(removed)
	}
	ev := modifierEventFromEntry(tick, p.clock.GameTime(), item.index, entry)
	ev.PayloadProto = payloadProto
	ev.MatchedPrior = hadPrev
	switch {
	case removal:
		ev.Transition = ModifierRemove
		delete(p.modifiers, item.index)
		p.pendingModifiers = append(p.pendingModifiers, ev)
		p.appendModifierEvent(ev)
		return nil
	case hadPrev:
		ev.Transition = ModifierRefresh
	default:
		ev.Transition = ModifierAdd
	}
	state := modifierState{entry: ev, payload: entry}
	state.entry.PayloadProto = bytes.Clone(ev.PayloadProto)
	p.modifiers[item.index] = state
	p.pendingModifiers = append(p.pendingModifiers, ev)
	p.appendModifierEvent(ev)
	return nil
}

func (p *Parser) appendModifierEvent(ev ModifierEvent) {
	ev.PayloadProto = bytes.Clone(ev.PayloadProto)
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
