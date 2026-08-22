package s2replay

import (
	"io"
	"math"
	"slices"

	"github.com/paralin/s2replay/protocol"
)

// EventType identifies a record in the unified replay event stream.
type EventType string

const (
	// EventSchemaVersion is the schema version emitted with each replay event.
	EventSchemaVersion = 1

	// EventDamage identifies a damage event.
	EventDamage EventType = "damage"
	// EventModifier identifies a modifier lifecycle event.
	EventModifier EventType = "modifier"
	// EventPurchase identifies an item or ability ownership transition.
	EventPurchase EventType = "purchase"
	// EventEntitySample identifies a sampled entity state.
	EventEntitySample EventType = "entity_sample"
	// EventDamageSummary identifies an engine-compiled recent-damage summary.
	EventDamageSummary EventType = "damage_summary"
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
// OwnedItems is the player item set at event time when attribution is available.
type Event struct {
	SchemaVersion int                 `json:"schema_version"`
	Type          EventType           `json:"type"`
	Tick          uint32              `json:"tick"`
	GameTime      float64             `json:"game_time"`
	Entity        int32               `json:"entity"`
	PlayerSlot    int32               `json:"player_slot"`
	OwnedItems    []uint32            `json:"owned_items,omitempty"`
	Damage        *DamageEvent        `json:"damage,omitempty"`
	Modifier      *ModifierEvent      `json:"modifier,omitempty"`
	Purchase      *PurchaseEvent      `json:"purchase,omitempty"`
	EntitySample  *EntitySample       `json:"entity_sample,omitempty"`
	DamageSummary *DamageSummaryEvent `json:"damage_summary,omitempty"`
}

// DamageSummaryRecord is one engine-attributed damage entry in a
// RecentDamageSummary user message.
type DamageSummaryRecord struct {
	Damage         int32   `json:"damage"`
	Hits           int32   `json:"hits"`
	DamageType     uint32  `json:"damage_type"`
	HeroID         uint32  `json:"hero_id"`
	AbilityID      uint32  `json:"ability_id"`
	AttackerClass  uint32  `json:"attacker_class"`
	DamageAbsorbed float32 `json:"damage_absorbed"`
	IsKillingBlow  bool    `json:"is_killing_blow"`
	VictimHeroID   uint32  `json:"victim_hero_id"`
	PreDamage      float32 `json:"pre_damage"`
	CritDamage     float32 `json:"crit_damage"`
}

// DamageSummaryModifierRecord is one engine-attributed modifier entry in a
// RecentDamageSummary user message.
type DamageSummaryModifierRecord struct {
	AbilityID      uint32  `json:"ability_id"`
	ModifierTypeID uint32  `json:"modifier_type_id"`
	EntindexCaster int32   `json:"entindex_caster"`
	StartTime      float32 `json:"start_time"`
	EndTime        float32 `json:"end_time"`
	Debuff         bool    `json:"debuff"`
}

// DamageSummaryEvent is the engine-compiled recent-damage summary the game
// itself uses for death recap and scoreboard attribution.
type DamageSummaryEvent struct {
	Tick            uint32                        `json:"tick"`
	GameTime        float64                       `json:"game_time"`
	PlayerSlot      int32                         `json:"player_slot"`
	TotalDamage     int32                         `json:"total_damage"`
	LostGold        int32                         `json:"lost_gold"`
	StartTime       float32                       `json:"start_time"`
	EndTime         float32                       `json:"end_time"`
	DamageRecords   []DamageSummaryRecord         `json:"damage_records"`
	ModifierRecords []DamageSummaryModifierRecord `json:"modifier_records"`
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
	sanitizeEvent(&ev)
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

func sanitizeEvent(ev *Event) {
	ev.GameTime = finiteFloat64(ev.GameTime)
	if ev.Damage != nil {
		sanitizeDamageEvent(ev.Damage)
	}
	if ev.Modifier != nil {
		ev.Modifier.GameTime = finiteFloat64(ev.Modifier.GameTime)
		ev.Modifier.LastAppliedTime = finiteFloat32(ev.Modifier.LastAppliedTime)
		ev.Modifier.Duration = finiteFloat32(ev.Modifier.Duration)
	}
	if ev.Purchase != nil {
		ev.Purchase.GameTime = finiteFloat64(ev.Purchase.GameTime)
	}
	if ev.EntitySample != nil {
		sanitizeEntitySample(ev.EntitySample)
	}
}

func sanitizeDamageEvent(ev *DamageEvent) {
	ev.GameTime = finiteFloat64(ev.GameTime)
	ev.PreDamage = finiteFloat32(ev.PreDamage)
	ev.DamageAbsorbed = finiteFloat32(ev.DamageAbsorbed)
	ev.Effectiveness = finiteFloat32(ev.Effectiveness)
	ev.CritDamage = finiteFloat32(ev.CritDamage)
	ev.OriginX = finiteFloat32(ev.OriginX)
	ev.OriginY = finiteFloat32(ev.OriginY)
	ev.OriginZ = finiteFloat32(ev.OriginZ)
	ev.DamageDirectionX = finiteFloat32(ev.DamageDirectionX)
	ev.DamageDirectionY = finiteFloat32(ev.DamageDirectionY)
	ev.DamageDirectionZ = finiteFloat32(ev.DamageDirectionZ)
}

func sanitizeEntitySample(sample *EntitySample) {
	sample.GameTime = finiteFloat64(sample.GameTime)
	if !isFiniteFloat32(sample.Health) || !isFiniteFloat32(sample.MaxHealth) {
		sample.Health = 0
		sample.MaxHealth = 0
		sample.HasHealth = false
	}
	if !isFiniteFloat32(sample.Shield) || !isFiniteFloat32(sample.MaxShield) {
		sample.Shield = 0
		sample.MaxShield = 0
		sample.HasShield = false
	}
	if !isFiniteFloat32(sample.PositionX) ||
		!isFiniteFloat32(sample.PositionY) ||
		!isFiniteFloat32(sample.PositionZ) {
		sample.PositionX = 0
		sample.PositionY = 0
		sample.PositionZ = 0
		sample.HasPosition = false
	}
}

func finiteFloat32(v float32) float32 {
	if !isFiniteFloat32(v) {
		return 0
	}
	return v
}

func finiteFloat64(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func isFiniteFloat32(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
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
	slices.Sort(out)
	return out
}

func normalizedTick(tick uint32) uint32 {
	if tick == PreGameTick {
		return 0
	}
	return tick
}
