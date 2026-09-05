package analysis

import (
	"slices"

	"github.com/paralin/s2replay"
)

// EntityTimeline stores health and position samples by player slot and entity.
type EntityTimeline struct {
	Players  map[int32][]EntitySample `json:"players"`
	Entities map[int32][]EntitySample `json:"entities"`
}

// EntitySample is a replay-local copy of a sampled entity state.
type EntitySample struct {
	Tick                 uint32   `json:"tick"`
	GameTime             float64  `json:"game_time"`
	PlayerSlot           int32    `json:"player_slot"`
	EntityID             int32    `json:"entity_id"`
	EntitySerial         int32    `json:"entity_serial,omitempty"`
	ClassID              int32    `json:"class_id"`
	ClassName            string   `json:"class_name"`
	Health               float32  `json:"health"`
	MaxHealth            float32  `json:"max_health"`
	Shield               float32  `json:"shield"`
	MaxShield            float32  `json:"max_shield"`
	HealthTick           uint32   `json:"health_tick,omitempty"`
	MaxHealthTick        uint32   `json:"max_health_tick,omitempty"`
	ShieldTick           uint32   `json:"shield_tick,omitempty"`
	MaxShieldTick        uint32   `json:"max_shield_tick,omitempty"`
	PositionX            float32  `json:"position_x"`
	PositionY            float32  `json:"position_y"`
	PositionZ            float32  `json:"position_z"`
	PositionXTick        uint32   `json:"position_x_tick,omitempty"`
	PositionYTick        uint32   `json:"position_y_tick,omitempty"`
	PositionZTick        uint32   `json:"position_z_tick,omitempty"`
	PositionXSourceField string   `json:"position_x_source_field,omitempty"`
	PositionYSourceField string   `json:"position_y_source_field,omitempty"`
	PositionZSourceField string   `json:"position_z_source_field,omitempty"`
	FacingXSourceField   string   `json:"facing_x_source_field,omitempty"`
	FacingYSourceField   string   `json:"facing_y_source_field,omitempty"`
	FacingZSourceField   string   `json:"facing_z_source_field,omitempty"`
	VelocityXSourceField string   `json:"velocity_x_source_field,omitempty"`
	VelocityYSourceField string   `json:"velocity_y_source_field,omitempty"`
	VelocityZSourceField string   `json:"velocity_z_source_field,omitempty"`
	FacingX              float32  `json:"facing_x,omitempty"`
	FacingY              float32  `json:"facing_y,omitempty"`
	FacingZ              float32  `json:"facing_z,omitempty"`
	VelocityX            float32  `json:"velocity_x,omitempty"`
	VelocityY            float32  `json:"velocity_y,omitempty"`
	VelocityZ            float32  `json:"velocity_z,omitempty"`
	FacingXTick          uint32   `json:"facing_x_tick,omitempty"`
	FacingYTick          uint32   `json:"facing_y_tick,omitempty"`
	FacingZTick          uint32   `json:"facing_z_tick,omitempty"`
	VelocityXTick        uint32   `json:"velocity_x_tick,omitempty"`
	VelocityYTick        uint32   `json:"velocity_y_tick,omitempty"`
	VelocityZTick        uint32   `json:"velocity_z_tick,omitempty"`
	HasFacing            bool     `json:"has_facing,omitempty"`
	HasFacingX           bool     `json:"has_facing_x,omitempty"`
	HasFacingY           bool     `json:"has_facing_y,omitempty"`
	HasFacingZ           bool     `json:"has_facing_z,omitempty"`
	HasVelocity          bool     `json:"has_velocity,omitempty"`
	HasVelocityX         bool     `json:"has_velocity_x,omitempty"`
	HasVelocityY         bool     `json:"has_velocity_y,omitempty"`
	HasVelocityZ         bool     `json:"has_velocity_z,omitempty"`
	HeroID               uint32   `json:"hero_id,omitempty"`
	HeroIDTick           uint32   `json:"hero_id_tick,omitempty"`
	Team                 int32    `json:"team,omitempty"`
	TeamTick             uint32   `json:"team_tick,omitempty"`
	HasHealth            bool     `json:"has_health"`
	HasShield            bool     `json:"has_shield"`
	HasPosition          bool     `json:"has_position"`
	HasHeroID            bool     `json:"has_hero_id"`
	HasTeam              bool     `json:"has_team"`
	OwnedItems           []uint32 `json:"owned_items"`
	InvalidFields        []string `json:"invalid_fields,omitempty"`

	Level                uint32 `json:"level,omitempty"`
	LevelTick            uint32 `json:"level_tick,omitempty"`
	HasLevel             bool   `json:"has_level,omitempty"`
	OwnerEntitySerial    int32  `json:"owner_entity_serial,omitempty"`
	OwnerEntity          int32  `json:"owner_entity,omitempty"`
	OwnerEntityTick      uint32 `json:"owner_entity_tick,omitempty"`
	HasOwnerEntity       bool   `json:"has_owner_entity,omitempty"`
	PawnEntitySerial     int32  `json:"pawn_entity_serial,omitempty"`
	PawnEntity           int32  `json:"pawn_entity,omitempty"`
	PawnEntityTick       uint32 `json:"pawn_entity_tick,omitempty"`
	HasPawnEntity        bool   `json:"has_pawn_entity,omitempty"`
	NetWorth             int32  `json:"net_worth,omitempty"`
	NetWorthTick         uint32 `json:"net_worth_tick,omitempty"`
	HasNetWorth          bool   `json:"has_net_worth,omitempty"`
	RemainingCharges     int32  `json:"remaining_charges,omitempty"`
	RemainingChargesTick uint32 `json:"remaining_charges_tick,omitempty"`
	HasRemainingCharges  bool   `json:"has_remaining_charges,omitempty"`
	// CooldownStart records the start of the active cooldown interval in server seconds.
	CooldownStart float32 `json:"cooldown_start,omitempty"`
	// CooldownStartTick identifies the last replay update for CooldownStart.
	CooldownStartTick uint32 `json:"cooldown_start_tick,omitempty"`
	// HasCooldownStart distinguishes an observed zero from an absent timer.
	HasCooldownStart bool `json:"has_cooldown_start,omitempty"`

	// ChargeRechargeStart records the start of the charge recovery interval in server seconds.
	ChargeRechargeStart float32 `json:"charge_recharge_start,omitempty"`
	// ChargeRechargeStartTick identifies the last replay update for ChargeRechargeStart.
	ChargeRechargeStartTick uint32 `json:"charge_recharge_start_tick,omitempty"`
	// HasChargeRechargeStart distinguishes an observed zero from an absent timer.
	HasChargeRechargeStart bool `json:"has_charge_recharge_start,omitempty"`

	// ChargeRechargeEnd records the end of the charge recovery interval in server seconds.
	ChargeRechargeEnd float32 `json:"charge_recharge_end,omitempty"`
	// ChargeRechargeEndTick identifies the last replay update for ChargeRechargeEnd.
	ChargeRechargeEndTick uint32 `json:"charge_recharge_end_tick,omitempty"`
	// HasChargeRechargeEnd distinguishes an observed zero from an absent timer.
	HasChargeRechargeEnd bool `json:"has_charge_recharge_end,omitempty"`

	CooldownEnd     float32 `json:"cooldown_end,omitempty"`
	CooldownEndTick uint32  `json:"cooldown_end_tick,omitempty"`
	HasCooldownEnd  bool    `json:"has_cooldown_end,omitempty"`
	Deaths          int32   `json:"deaths,omitempty"`
	DeathsTick      uint32  `json:"deaths_tick,omitempty"`
	HasDeaths       bool    `json:"has_deaths,omitempty"`
	LastHits        int32   `json:"last_hits,omitempty"`
	LastHitsTick    uint32  `json:"last_hits_tick,omitempty"`
	HasLastHits     bool    `json:"has_last_hits,omitempty"`
	Denies          int32   `json:"denies,omitempty"`
	DeniesTick      uint32  `json:"denies_tick,omitempty"`
	HasDenies       bool    `json:"has_denies,omitempty"`
	KillStreak      int32   `json:"kill_streak,omitempty"`
	KillStreakTick  uint32  `json:"kill_streak_tick,omitempty"`
	HasKillStreak   bool    `json:"has_kill_streak,omitempty"`
	HeroDamage      int32   `json:"hero_damage,omitempty"`
	HeroDamageTick  uint32  `json:"hero_damage_tick,omitempty"`
	HasHeroDamage   bool    `json:"has_hero_damage,omitempty"`
}

// acceptEntitySample records one entity sample in the per-player and
// per-entity timelines.
func (b *builder) acceptEntitySample(ev s2replay.Event) {
	if ev.EntitySample == nil {
		return
	}
	if ev.PlayerSlot < 0 {
		b.result.Quality.MissingPlayerSlotEvents++
	}
	ownedItems := cloneItemSet(ev.OwnedItems)
	if len(ownedItems) == 0 && ev.PlayerSlot >= 0 {
		ownedItems = cloneItemSet(b.playerItems[ev.PlayerSlot])
	}
	sample := EntitySample{
		Tick:                    ev.Tick,
		GameTime:                ev.GameTime,
		PlayerSlot:              ev.PlayerSlot,
		EntityID:                ev.Entity,
		EntitySerial:            ev.EntitySample.EntitySerial,
		ClassID:                 ev.EntitySample.ClassID,
		ClassName:               ev.EntitySample.ClassName,
		Health:                  ev.EntitySample.Health,
		MaxHealth:               ev.EntitySample.MaxHealth,
		Shield:                  ev.EntitySample.Shield,
		MaxShield:               ev.EntitySample.MaxShield,
		HealthTick:              ev.EntitySample.HealthTick,
		MaxHealthTick:           ev.EntitySample.MaxHealthTick,
		ShieldTick:              ev.EntitySample.ShieldTick,
		MaxShieldTick:           ev.EntitySample.MaxShieldTick,
		PositionX:               ev.EntitySample.PositionX,
		PositionY:               ev.EntitySample.PositionY,
		PositionZ:               ev.EntitySample.PositionZ,
		PositionXTick:           ev.EntitySample.PositionXTick,
		PositionYTick:           ev.EntitySample.PositionYTick,
		PositionZTick:           ev.EntitySample.PositionZTick,
		PositionXSourceField:    ev.EntitySample.PositionXSourceField,
		PositionYSourceField:    ev.EntitySample.PositionYSourceField,
		PositionZSourceField:    ev.EntitySample.PositionZSourceField,
		FacingXSourceField:      ev.EntitySample.FacingXSourceField,
		FacingYSourceField:      ev.EntitySample.FacingYSourceField,
		FacingZSourceField:      ev.EntitySample.FacingZSourceField,
		VelocityXSourceField:    ev.EntitySample.VelocityXSourceField,
		VelocityYSourceField:    ev.EntitySample.VelocityYSourceField,
		VelocityZSourceField:    ev.EntitySample.VelocityZSourceField,
		FacingX:                 ev.EntitySample.FacingX,
		FacingY:                 ev.EntitySample.FacingY,
		FacingZ:                 ev.EntitySample.FacingZ,
		VelocityX:               ev.EntitySample.VelocityX,
		VelocityY:               ev.EntitySample.VelocityY,
		VelocityZ:               ev.EntitySample.VelocityZ,
		FacingXTick:             ev.EntitySample.FacingXTick,
		FacingYTick:             ev.EntitySample.FacingYTick,
		FacingZTick:             ev.EntitySample.FacingZTick,
		VelocityXTick:           ev.EntitySample.VelocityXTick,
		VelocityYTick:           ev.EntitySample.VelocityYTick,
		VelocityZTick:           ev.EntitySample.VelocityZTick,
		HasFacing:               ev.EntitySample.HasFacing,
		HasFacingX:              ev.EntitySample.HasFacingX,
		HasFacingY:              ev.EntitySample.HasFacingY,
		HasFacingZ:              ev.EntitySample.HasFacingZ,
		HasVelocity:             ev.EntitySample.HasVelocity,
		HasVelocityX:            ev.EntitySample.HasVelocityX,
		HasVelocityY:            ev.EntitySample.HasVelocityY,
		HasVelocityZ:            ev.EntitySample.HasVelocityZ,
		HeroID:                  ev.EntitySample.HeroID,
		HeroIDTick:              ev.EntitySample.HeroIDTick,
		Team:                    ev.EntitySample.Team,
		TeamTick:                ev.EntitySample.TeamTick,
		HasHealth:               ev.EntitySample.HasHealth,
		HasShield:               ev.EntitySample.HasShield,
		HasPosition:             ev.EntitySample.HasPosition,
		HasHeroID:               ev.EntitySample.HasHeroID,
		HasTeam:                 ev.EntitySample.HasTeam,
		OwnedItems:              ownedItems,
		InvalidFields:           slices.Clone(ev.EntitySample.InvalidFields),
		Level:                   ev.EntitySample.Level,
		LevelTick:               ev.EntitySample.LevelTick,
		HasLevel:                ev.EntitySample.HasLevel,
		OwnerEntity:             ev.EntitySample.OwnerEntity,
		OwnerEntitySerial:       ev.EntitySample.OwnerEntitySerial,
		OwnerEntityTick:         ev.EntitySample.OwnerEntityTick,
		HasOwnerEntity:          ev.EntitySample.HasOwnerEntity,
		PawnEntity:              ev.EntitySample.PawnEntity,
		PawnEntitySerial:        ev.EntitySample.PawnEntitySerial,
		PawnEntityTick:          ev.EntitySample.PawnEntityTick,
		HasPawnEntity:           ev.EntitySample.HasPawnEntity,
		NetWorth:                ev.EntitySample.NetWorth,
		NetWorthTick:            ev.EntitySample.NetWorthTick,
		HasNetWorth:             ev.EntitySample.HasNetWorth,
		RemainingCharges:        ev.EntitySample.RemainingCharges,
		RemainingChargesTick:    ev.EntitySample.RemainingChargesTick,
		HasRemainingCharges:     ev.EntitySample.HasRemainingCharges,
		CooldownStart:           ev.EntitySample.CooldownStart,
		CooldownStartTick:       ev.EntitySample.CooldownStartTick,
		HasCooldownStart:        ev.EntitySample.HasCooldownStart,
		ChargeRechargeStart:     ev.EntitySample.ChargeRechargeStart,
		ChargeRechargeStartTick: ev.EntitySample.ChargeRechargeStartTick,
		HasChargeRechargeStart:  ev.EntitySample.HasChargeRechargeStart,
		ChargeRechargeEnd:       ev.EntitySample.ChargeRechargeEnd,
		ChargeRechargeEndTick:   ev.EntitySample.ChargeRechargeEndTick,
		HasChargeRechargeEnd:    ev.EntitySample.HasChargeRechargeEnd,
		CooldownEnd:             ev.EntitySample.CooldownEnd,
		CooldownEndTick:         ev.EntitySample.CooldownEndTick,
		HasCooldownEnd:          ev.EntitySample.HasCooldownEnd,
		Deaths:                  ev.EntitySample.Deaths,
		DeathsTick:              ev.EntitySample.DeathsTick,
		HasDeaths:               ev.EntitySample.HasDeaths,
		LastHits:                ev.EntitySample.LastHits,
		LastHitsTick:            ev.EntitySample.LastHitsTick,
		HasLastHits:             ev.EntitySample.HasLastHits,
		Denies:                  ev.EntitySample.Denies,
		DeniesTick:              ev.EntitySample.DeniesTick,
		HasDenies:               ev.EntitySample.HasDenies,
		KillStreak:              ev.EntitySample.KillStreak,
		KillStreakTick:          ev.EntitySample.KillStreakTick,
		HasKillStreak:           ev.EntitySample.HasKillStreak,
		HeroDamage:              ev.EntitySample.HeroDamage,
		HeroDamageTick:          ev.EntitySample.HeroDamageTick,
		HasHeroDamage:           ev.EntitySample.HasHeroDamage,
	}
	if ev.PlayerSlot >= 0 {
		b.result.Entities.Players[ev.PlayerSlot] = append(b.result.Entities.Players[ev.PlayerSlot], sample)
	}
	b.result.Entities.Entities[ev.Entity] = append(b.result.Entities.Entities[ev.Entity], sample)
	b.result.Quality.EntitySamples++
}
