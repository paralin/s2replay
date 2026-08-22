package analysis

import "github.com/paralin/s2replay"

// EntityTimeline stores health and position samples by player slot and entity.
type EntityTimeline struct {
	Players  map[int32][]EntitySample `json:"players"`
	Entities map[int32][]EntitySample `json:"entities"`
}

// EntitySample is a replay-local copy of a sampled entity state.
type EntitySample struct {
	Tick        uint32   `json:"tick"`
	GameTime    float64  `json:"game_time"`
	PlayerSlot  int32    `json:"player_slot"`
	EntityID    int32    `json:"entity_id"`
	ClassID     int32    `json:"class_id"`
	ClassName   string   `json:"class_name"`
	Health      float32  `json:"health"`
	MaxHealth   float32  `json:"max_health"`
	Shield      float32  `json:"shield"`
	MaxShield   float32  `json:"max_shield"`
	PositionX   float32  `json:"position_x"`
	PositionY   float32  `json:"position_y"`
	PositionZ   float32  `json:"position_z"`
	HeroID      uint32   `json:"hero_id,omitempty"`
	Team        int32    `json:"team,omitempty"`
	HasHealth   bool     `json:"has_health"`
	HasShield   bool     `json:"has_shield"`
	HasPosition bool     `json:"has_position"`
	HasHeroID   bool     `json:"has_hero_id"`
	HasTeam     bool     `json:"has_team"`
	OwnedItems  []uint32 `json:"owned_items"`
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
		Tick:        ev.Tick,
		GameTime:    ev.GameTime,
		PlayerSlot:  ev.PlayerSlot,
		EntityID:    ev.Entity,
		ClassID:     ev.EntitySample.ClassID,
		ClassName:   ev.EntitySample.ClassName,
		Health:      ev.EntitySample.Health,
		MaxHealth:   ev.EntitySample.MaxHealth,
		Shield:      ev.EntitySample.Shield,
		MaxShield:   ev.EntitySample.MaxShield,
		PositionX:   ev.EntitySample.PositionX,
		PositionY:   ev.EntitySample.PositionY,
		PositionZ:   ev.EntitySample.PositionZ,
		HeroID:      ev.EntitySample.HeroID,
		Team:        ev.EntitySample.Team,
		HasHealth:   ev.EntitySample.HasHealth,
		HasShield:   ev.EntitySample.HasShield,
		HasPosition: ev.EntitySample.HasPosition,
		HasHeroID:   ev.EntitySample.HasHeroID,
		HasTeam:     ev.EntitySample.HasTeam,
		OwnedItems:  ownedItems,
	}
	if ev.PlayerSlot >= 0 {
		b.result.Entities.Players[ev.PlayerSlot] = append(b.result.Entities.Players[ev.PlayerSlot], sample)
	}
	b.result.Entities.Entities[ev.Entity] = append(b.result.Entities.Entities[ev.Entity], sample)
	b.result.Quality.EntitySamples++
}
