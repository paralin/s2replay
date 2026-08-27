package analysis

import "github.com/paralin/s2replay"

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
	ClassID              int32    `json:"class_id"`
	ClassName            string   `json:"class_name"`
	Health               float32  `json:"health"`
	MaxHealth            float32  `json:"max_health"`
	Shield               float32  `json:"shield"`
	MaxShield            float32  `json:"max_shield"`
	PositionX            float32  `json:"position_x"`
	PositionY            float32  `json:"position_y"`
	PositionZ            float32  `json:"position_z"`
	PositionXTick        uint32   `json:"position_x_tick,omitempty"`
	PositionYTick        uint32   `json:"position_y_tick,omitempty"`
	PositionZTick        uint32   `json:"position_z_tick,omitempty"`
	PositionXSourceField string   `json:"position_x_source_field,omitempty"`
	PositionYSourceField string   `json:"position_y_source_field,omitempty"`
	PositionZSourceField string   `json:"position_z_source_field,omitempty"`
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
	Team                 int32    `json:"team,omitempty"`
	HasHealth            bool     `json:"has_health"`
	HasShield            bool     `json:"has_shield"`
	HasPosition          bool     `json:"has_position"`
	HasHeroID            bool     `json:"has_hero_id"`
	HasTeam              bool     `json:"has_team"`
	OwnedItems           []uint32 `json:"owned_items"`
}

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
		Tick:                 ev.Tick,
		GameTime:             ev.GameTime,
		PlayerSlot:           ev.PlayerSlot,
		EntityID:             ev.Entity,
		ClassID:              ev.EntitySample.ClassID,
		ClassName:            ev.EntitySample.ClassName,
		Health:               ev.EntitySample.Health,
		MaxHealth:            ev.EntitySample.MaxHealth,
		Shield:               ev.EntitySample.Shield,
		MaxShield:            ev.EntitySample.MaxShield,
		PositionX:            ev.EntitySample.PositionX,
		PositionY:            ev.EntitySample.PositionY,
		PositionZ:            ev.EntitySample.PositionZ,
		PositionXTick:        ev.EntitySample.PositionXTick,
		PositionYTick:        ev.EntitySample.PositionYTick,
		PositionZTick:        ev.EntitySample.PositionZTick,
		PositionXSourceField: ev.EntitySample.PositionXSourceField,
		PositionYSourceField: ev.EntitySample.PositionYSourceField,
		PositionZSourceField: ev.EntitySample.PositionZSourceField,
		FacingX:              ev.EntitySample.FacingX,
		FacingY:              ev.EntitySample.FacingY,
		FacingZ:              ev.EntitySample.FacingZ,
		VelocityX:            ev.EntitySample.VelocityX,
		VelocityY:            ev.EntitySample.VelocityY,
		VelocityZ:            ev.EntitySample.VelocityZ,
		FacingXTick:          ev.EntitySample.FacingXTick,
		FacingYTick:          ev.EntitySample.FacingYTick,
		FacingZTick:          ev.EntitySample.FacingZTick,
		VelocityXTick:        ev.EntitySample.VelocityXTick,
		VelocityYTick:        ev.EntitySample.VelocityYTick,
		VelocityZTick:        ev.EntitySample.VelocityZTick,
		HasFacing:            ev.EntitySample.HasFacing,
		HasFacingX:           ev.EntitySample.HasFacingX,
		HasFacingY:           ev.EntitySample.HasFacingY,
		HasFacingZ:           ev.EntitySample.HasFacingZ,
		HasVelocity:          ev.EntitySample.HasVelocity,
		HasVelocityX:         ev.EntitySample.HasVelocityX,
		HasVelocityY:         ev.EntitySample.HasVelocityY,
		HasVelocityZ:         ev.EntitySample.HasVelocityZ,
		HeroID:               ev.EntitySample.HeroID,
		Team:                 ev.EntitySample.Team,
		HasHealth:            ev.EntitySample.HasHealth,
		HasShield:            ev.EntitySample.HasShield,
		HasPosition:          ev.EntitySample.HasPosition,
		HasHeroID:            ev.EntitySample.HasHeroID,
		HasTeam:              ev.EntitySample.HasTeam,
		OwnedItems:           ownedItems,
	}
	if ev.PlayerSlot >= 0 {
		b.result.Entities.Players[ev.PlayerSlot] = append(b.result.Entities.Players[ev.PlayerSlot], sample)
	}
	b.result.Entities.Entities[ev.Entity] = append(b.result.Entities.Entities[ev.Entity], sample)
	b.result.Quality.EntitySamples++
}
