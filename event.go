package s2replay

import (
	"io"
	"sort"

	"github.com/paralin/s2replay/protocol"
)

type EventType string

const (
	EventSchemaVersion = 1

	EventDamage       EventType = "damage"
	EventModifier     EventType = "modifier"
	EventPurchase     EventType = "purchase"
	EventEntitySample EventType = "entity_sample"
)

// PurchaseEvent is an item/ability ownership transition observed in the user
// message stream.
type PurchaseEvent struct {
	Tick       uint32  `json:"tick"`
	GameTime   float64 `json:"game_time"`
	PlayerSlot int32   `json:"player_slot"`
	UserID     int32   `json:"user_id"`
	AbilityID  uint32  `json:"ability_id"`
	Change     string  `json:"change"`
	Sell       bool    `json:"sell"`
	Quickbuy   bool    `json:"quickbuy"`
	Source     string  `json:"source"`
}

// Event is the unified typed stream used by downstream Deadlock analysis.
// OwnedItems is the attacker-side item set when attribution is available.
type Event struct {
	SchemaVersion int            `json:"schema_version"`
	Type          EventType      `json:"type"`
	Tick          uint32         `json:"tick"`
	GameTime      float64        `json:"game_time"`
	Entity        int32          `json:"entity"`
	PlayerSlot    int32          `json:"player_slot"`
	OwnedItems    []uint32       `json:"owned_items,omitempty"`
	Damage        *DamageEvent   `json:"damage,omitempty"`
	Modifier      *ModifierEvent `json:"modifier,omitempty"`
	Purchase      *PurchaseEvent `json:"purchase,omitempty"`
	EntitySample  *EntitySample  `json:"entity_sample,omitempty"`
}

// NextEvent returns the next unified typed event produced while walking the
// demo stream.
func (p *Parser) NextEvent() (Event, error) {
	for len(p.pendingEvents) == 0 {
		if _, err := p.NextMessage(); err != nil {
			return Event{}, err
		}
	}
	ev := p.pendingEvents[0]
	copy(p.pendingEvents, p.pendingEvents[1:])
	p.pendingEvents = p.pendingEvents[:len(p.pendingEvents)-1]
	if ev.SchemaVersion == 0 {
		ev.SchemaVersion = EventSchemaVersion
	}
	return ev, nil
}

// CollectEvents reads up to limit unified events. A non-positive limit reads
// the whole demo.
func (p *Parser) CollectEvents(limit int) ([]Event, error) {
	var events []Event
	for limit <= 0 || len(events) < limit {
		ev, err := p.NextEvent()
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

func (p *Parser) applyAbilitiesChanged(tick uint32, msg *protocol.CCitadelUserMsg_AbilitiesChanged) {
	slot := msg.GetPurchaserPlayerSlot()
	abilityID := msg.GetAbilityId()
	change := msg.GetChange()
	if slot >= 0 && abilityID != 0 {
		switch change {
		case protocol.CCitadelUserMsg_AbilitiesChanged_EPurchased,
			protocol.CCitadelUserMsg_AbilitiesChanged_ESwappedActivatedAbility:
			p.addPlayerItem(slot, abilityID)
		case protocol.CCitadelUserMsg_AbilitiesChanged_ESold:
			p.removePlayerItem(slot, abilityID)
		}
	}
	pending := PurchaseEvent{
		Tick:       normalizedTick(tick),
		GameTime:   p.clock.GameTime(),
		PlayerSlot: slot,
		UserID:     -1,
		AbilityID:  abilityID,
		Change:     change.String(),
		Source:     "abilities_changed",
	}
	p.pendingEvents = append(p.pendingEvents, Event{
		Type:       EventPurchase,
		Tick:       pending.Tick,
		GameTime:   pending.GameTime,
		Entity:     -1,
		PlayerSlot: slot,
		OwnedItems: p.playerItemSet(slot),
		Purchase:   &pending,
	})
}

func (p *Parser) applyItemPurchaseNotification(tick uint32, msg *protocol.CCitadelUserMessage_ItemPurchaseNotification) {
	slot := msg.GetUserid()
	abilityID := msg.GetAbilityId()
	if slot >= 0 && abilityID != 0 {
		if msg.GetSell() {
			p.removePlayerItem(slot, abilityID)
		} else {
			p.addPlayerItem(slot, abilityID)
		}
	}
	pending := PurchaseEvent{
		Tick:       normalizedTick(tick),
		GameTime:   p.clock.GameTime(),
		PlayerSlot: slot,
		UserID:     slot,
		AbilityID:  abilityID,
		Change:     "notification",
		Sell:       msg.GetSell(),
		Quickbuy:   msg.GetQuickbuy(),
		Source:     "item_purchase_notification",
	}
	p.pendingEvents = append(p.pendingEvents, Event{
		Type:       EventPurchase,
		Tick:       pending.Tick,
		GameTime:   pending.GameTime,
		Entity:     -1,
		PlayerSlot: slot,
		OwnedItems: p.playerItemSet(slot),
		Purchase:   &pending,
	})
}

func (p *Parser) appendDamageEvent(tick uint32, msg *protocol.CCitadelUserMessage_Damage) {
	damage := damageEventFromProto(normalizedTick(tick), p.clock.GameTime(), msg)
	slot, ok := p.entityPlayerSlots[damage.Attacker]
	ev := Event{
		Type:     EventDamage,
		Tick:     damage.Tick,
		GameTime: damage.GameTime,
		Entity:   damage.Attacker,
		Damage:   &damage,
	}
	if ok {
		ev.PlayerSlot = slot
		ev.OwnedItems = p.playerItemSet(slot)
	} else {
		ev.PlayerSlot = -1
	}
	p.pendingEvents = append(p.pendingEvents, ev)
}

func (p *Parser) addPlayerItem(slot int32, abilityID uint32) {
	items := p.playerItems[slot]
	if items == nil {
		items = make(map[uint32]struct{})
		p.playerItems[slot] = items
	}
	items[abilityID] = struct{}{}
}

func (p *Parser) removePlayerItem(slot int32, abilityID uint32) {
	items := p.playerItems[slot]
	if items == nil {
		return
	}
	delete(items, abilityID)
	if len(items) == 0 {
		delete(p.playerItems, slot)
	}
}

func (p *Parser) playerItemSet(slot int32) []uint32 {
	items := p.playerItems[slot]
	if len(items) == 0 {
		return nil
	}
	out := make([]uint32, 0, len(items))
	for abilityID := range items {
		out = append(out, abilityID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func normalizedTick(tick uint32) uint32 {
	if tick == PreGameTick {
		return 0
	}
	return tick
}
