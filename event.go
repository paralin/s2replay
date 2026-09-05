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
	// EventPostMatch identifies the engine-compiled post-match summary.
	EventPostMatch EventType = "post_match"
	// EventControllerSample identifies a player controller scoreboard sample.
	EventControllerSample EventType = "controller_sample"
	// EventKillStreak identifies a kill streak update from the user message
	// stream.
	EventKillStreak EventType = "kill_streak"
	// EventStaminaConsumed identifies a stamina spend from the user message
	// stream.
	EventStaminaConsumed EventType = "stamina_consumed"
	// EventAbilityCharges identifies an ability charge-count change from the
	// entity stream.
	EventAbilityCharges EventType = "ability_charges"
	// EventObjective identifies a map objective lifecycle observation.
	EventObjective EventType = "objective_event"
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
	SchemaVersion    int                   `json:"schema_version"`
	Type             EventType             `json:"type"`
	Tick             uint32                `json:"tick"`
	GameTime         float64               `json:"game_time"`
	Entity           int32                 `json:"entity"`
	EntitySerial     int32                 `json:"entity_serial,omitempty"`
	PlayerSlot       int32                 `json:"player_slot"`
	OwnedItems       []uint32              `json:"owned_items,omitempty"`
	Damage           *DamageEvent          `json:"damage,omitempty"`
	Modifier         *ModifierEvent        `json:"modifier,omitempty"`
	Purchase         *PurchaseEvent        `json:"purchase,omitempty"`
	EntitySample     *EntitySample         `json:"entity_sample,omitempty"`
	DamageSummary    *DamageSummaryEvent   `json:"damage_summary,omitempty"`
	PostMatch        *PostMatchEvent       `json:"post_match,omitempty"`
	ControllerSample *ControllerSample     `json:"controller_sample,omitempty"`
	KillStreak       *KillStreakEvent      `json:"kill_streak,omitempty"`
	StaminaConsumed  *StaminaConsumedEvent `json:"stamina_consumed,omitempty"`
	AbilityCharges   *AbilityChargesEvent  `json:"ability_charges,omitempty"`
	Objective        *ObjectiveEvent       `json:"objective_event,omitempty"`
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

// KillStreakEvent is one kill streak update: a player pawn reached num_kills
// consecutive kills. First blood and streak ends are flagged.
type KillStreakEvent struct {
	Tick         uint32  `json:"tick"`
	GameTime     float64 `json:"game_time"`
	PlayerPawn   uint32  `json:"player_pawn"`
	NumKills     int32   `json:"num_kills"`
	IsFirstBlood bool    `json:"is_first_blood"`
	StreakEnded  bool    `json:"streak_ended"`
	Duration     float32 `json:"duration"`
}

// StaminaConsumedEvent is one stamina spend observed in the user message
// stream. Before/After/Max are the player's stamina bars at the spend.
type StaminaConsumedEvent struct {
	Tick           uint32  `json:"tick"`
	GameTime       float64 `json:"game_time"`
	EntindexTarget int32   `json:"entindex_target"`
	StaminaBefore  float32 `json:"stamina_before"`
	StaminaAfter   float32 `json:"stamina_after"`
	Drained        bool    `json:"drained"`
	StaminaMax     float32 `json:"stamina_max"`
}

// AbilityChargesEvent is one ability charge-count change observed on an
// ability entity (dash charges are the primary consumer). Emitted only when
// the count differs from the previously seen value for that entity.
type AbilityChargesEvent struct {
	Tick             uint32  `json:"tick"`
	GameTime         float64 `json:"game_time"`
	ClassName        string  `json:"class_name"`
	RemainingCharges int32   `json:"remaining_charges"`
}

// PostMatchObjective is one map objective in the post-match record.
type PostMatchObjective struct {
	LegacyObjectiveID     int32  `json:"legacy_objective_id"`
	TeamObjectiveID       int32  `json:"team_objective_id"`
	Team                  int32  `json:"team"`
	DestroyedTimeS        uint32 `json:"destroyed_time_s"`
	FirstDamageTimeS      uint32 `json:"first_damage_time_s"`
	CreepDamage           uint32 `json:"creep_damage"`
	CreepDamageMitigated  uint32 `json:"creep_damage_mitigated"`
	PlayerDamage          uint32 `json:"player_damage"`
	PlayerDamageMitigated uint32 `json:"player_damage_mitigated"`
	PlayerSpiritDamage    uint32 `json:"player_spirit_damage"`
}

// PostMatchPlayerItem is one item or ability purchase in the post-match
// record, with buy and sell times.
type PostMatchPlayerItem struct {
	ItemID          uint32 `json:"item_id"`
	GameTimeS       uint32 `json:"game_time_s"`
	SoldTimeS       uint32 `json:"sold_time_s"`
	UpgradeID       uint32 `json:"upgrade_id"`
	Flags           uint32 `json:"flags"`
	ImbuedAbilityID uint32 `json:"imbued_ability_id"`
	UpgradeInfo     uint32 `json:"upgrade_info"`
}

// PostMatchPlayerStat is one timed snapshot of a player's scoreboard state.
type PostMatchPlayerStat struct {
	TimeStampS        uint32 `json:"time_stamp_s"`
	NetWorth          uint32 `json:"net_worth"`
	Kills             uint32 `json:"kills"`
	Deaths            uint32 `json:"deaths"`
	Assists           uint32 `json:"assists"`
	Level             uint32 `json:"level"`
	LastHits          uint32 `json:"last_hits"`
	Denies            uint32 `json:"denies"`
	PlayerDamage      uint32 `json:"player_damage"`
	PlayerDamageTaken uint32 `json:"player_damage_taken"`
	PlayerHealing     uint32 `json:"player_healing"`
	CreepDamage       uint32 `json:"creep_damage"`
	NeutralDamage     uint32 `json:"neutral_damage"`
	BossDamage        uint32 `json:"boss_damage"`
	DamageAbsorbed    uint32 `json:"damage_absorbed"`
	DamageMitigated   uint32 `json:"damage_mitigated"`
	ShotsHit          uint32 `json:"shots_hit"`
	ShotsMissed       uint32 `json:"shots_missed"`
	WeaponPower       uint32 `json:"weapon_power"`
	TechPower         uint32 `json:"tech_power"`
}

// PostMatchPlayer is one player in the engine-compiled post-match summary.
type PostMatchPlayer struct {
	AccountID     uint32                `json:"account_id"`
	PlayerSlot    uint32                `json:"player_slot"`
	Team          int32                 `json:"team"`
	Kills         uint32                `json:"kills"`
	Deaths        uint32                `json:"deaths"`
	Assists       uint32                `json:"assists"`
	NetWorth      uint32                `json:"net_worth"`
	HeroID        uint32                `json:"hero_id"`
	LastHits      uint32                `json:"last_hits"`
	Denies        uint32                `json:"denies"`
	AbilityPoints uint32                `json:"ability_points"`
	Level         uint32                `json:"level"`
	Items         []PostMatchPlayerItem `json:"items"`
	Stats         []PostMatchPlayerStat `json:"stats"`
}

// PostMatchEvent is the engine-compiled post-match summary embedded in the
// demo. It is the authoritative record for final builds, buy and sell times,
// and scoreboard totals.
type PostMatchEvent struct {
	MatchID      uint64               `json:"match_id"`
	DurationS    uint32               `json:"duration_s"`
	MatchOutcome int32                `json:"match_outcome"`
	WinningTeam  int32                `json:"winning_team"`
	GameMode     int32                `json:"game_mode"`
	MatchMode    int32                `json:"match_mode"`
	Players      []PostMatchPlayer    `json:"players"`
	Objectives   []PostMatchObjective `json:"objectives"`
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
		if math.IsNaN(float64(ev.Modifier.LastAppliedTime)) || math.IsInf(float64(ev.Modifier.LastAppliedTime), 0) {
			ev.Modifier.HasLastAppliedTime = false
		}
		if math.IsNaN(float64(ev.Modifier.Duration)) || math.IsInf(float64(ev.Modifier.Duration), 0) {
			ev.Modifier.HasDuration = false
		}
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
	for axis, name := range []string{"camera_pitch", "camera_yaw", "camera_roll"} {
		if !isFiniteFloat32(sample.CameraAngles[axis]) {
			markInvalidField(&sample.InvalidFields, name)
			sample.CameraAngles[axis] = 0
			sample.HasCameraAngles[axis] = false
		}
	}

	if !isFiniteFloat64(sample.GameTime) {
		markInvalidField(&sample.InvalidFields, "game_time")
		sample.GameTime = 0
	}
	if !isFiniteFloat32(sample.Health) {
		markInvalidField(&sample.InvalidFields, "health")
		sample.Health = 0
		sample.HasHealth = false
	}
	if !isFiniteFloat32(sample.MaxHealth) {
		markInvalidField(&sample.InvalidFields, "max_health")
		sample.MaxHealth = 0
		sample.HasHealth = false
	}
	if !isFiniteFloat32(sample.Shield) {
		markInvalidField(&sample.InvalidFields, "shield")
		sample.Shield = 0
		sample.HasShield = false
	}
	if !isFiniteFloat32(sample.MaxShield) {
		markInvalidField(&sample.InvalidFields, "max_shield")
		sample.MaxShield = 0
		sample.HasShield = false
	}
	// Keep invalid timer evidence while making every exported float JSON-safe.
	for _, timer := range []struct {
		name    string
		value   *float32
		present *bool
	}{
		{"stamina_latch_time", &sample.StaminaLatchTime, &sample.HasStaminaLatchTime},
		{"stamina_latch_value", &sample.StaminaLatchValue, &sample.HasStaminaLatchValue},
		{"cooldown_start", &sample.CooldownStart, &sample.HasCooldownStart},
		{"cooldown_end", &sample.CooldownEnd, &sample.HasCooldownEnd},
		{"charge_recharge_start", &sample.ChargeRechargeStart, &sample.HasChargeRechargeStart},
		{"charge_recharge_end", &sample.ChargeRechargeEnd, &sample.HasChargeRechargeEnd},
	} {
		if !isFiniteFloat32(*timer.value) {
			markInvalidField(&sample.InvalidFields, timer.name)
			*timer.value = 0
			*timer.present = false
		}
	}
	positionInvalid := false
	facingInvalid := false
	velocityInvalid := false
	for _, field := range []struct {
		name  string
		value *float32
	}{
		{"position_x", &sample.PositionX},
		{"position_y", &sample.PositionY},
		{"position_z", &sample.PositionZ},
		{"facing_x", &sample.FacingX},
		{"facing_y", &sample.FacingY},
		{"facing_z", &sample.FacingZ},
		{"velocity_x", &sample.VelocityX},
		{"velocity_y", &sample.VelocityY},
		{"velocity_z", &sample.VelocityZ},
	} {
		name, value := field.name, field.value
		if !isFiniteFloat32(*value) {
			markInvalidField(&sample.InvalidFields, name)
			*value = 0
			switch name[0] {
			case 'p':
				positionInvalid = true
			case 'f':
				facingInvalid = true
			case 'v':
				velocityInvalid = true
			}
		}
	}
	if positionInvalid {
		sample.HasPosition = false
	}
	if facingInvalid {
		sample.HasFacing = false
		sample.HasFacingX, sample.HasFacingY, sample.HasFacingZ = false, false, false
	}
	if velocityInvalid {
		sample.HasVelocity = false
		sample.HasVelocityX, sample.HasVelocityY, sample.HasVelocityZ = false, false, false
	}
}

func markInvalidField(fields *[]string, name string) {
	if slices.Contains(*fields, name) {
		return
	}
	*fields = append(*fields, name)
}

func isFiniteFloat64(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
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
