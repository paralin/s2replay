package analysis

import (
	"fmt"
	"math"
	"slices"

	"github.com/paralin/s2replay"
)

// WorldCensus is the direct entity evidence observed at one requested tick.
type WorldCensus struct {
	Tick     uint32              `json:"tick"`
	Entities []WorldCensusEntity `json:"entities"`
}

// WorldCensusFloat is a directly observed float and its source age.
type WorldCensusFloat struct {
	Value          float32 `json:"value"`
	Present        bool    `json:"present"`
	SourceTick     uint32  `json:"source_tick"`
	FreshnessTicks uint32  `json:"freshness_ticks"`
}

// WorldCensusUint is a directly observed unsigned integer and its source age.
type WorldCensusUint struct {
	Value          uint32 `json:"value"`
	Present        bool   `json:"present"`
	SourceTick     uint32 `json:"source_tick"`
	FreshnessTicks uint32 `json:"freshness_ticks"`
}

// WorldCensusInt is a directly observed signed integer and its source age.
type WorldCensusInt struct {
	Value          int32  `json:"value"`
	Present        bool   `json:"present"`
	SourceTick     uint32 `json:"source_tick"`
	FreshnessTicks uint32 `json:"freshness_ticks"`
}

// WorldCensusEntity is one observed entity generation. ClassName is source
// data only; this type intentionally has no inferred entity category.
type WorldCensusEntity struct {
	EntityID       int32  `json:"entity_id"`
	EntitySerial   int32  `json:"entity_serial"`
	ClassID        int32  `json:"class_id"`
	ClassName      string `json:"class_name"`
	SourceTick     uint32 `json:"source_tick"`
	FreshnessTicks uint32 `json:"freshness_ticks"`

	PositionX WorldCensusFloat `json:"position_x"`
	PositionY WorldCensusFloat `json:"position_y"`
	PositionZ WorldCensusFloat `json:"position_z"`
	Health    WorldCensusFloat `json:"health"`
	Shield    WorldCensusFloat `json:"shield"`
	HeroID    WorldCensusUint  `json:"hero_id"`
	Team      WorldCensusInt   `json:"team"`
}

// WorldCensusErrorKind identifies a refused census input.
type WorldCensusErrorKind string

const (
	WorldCensusDuplicateGeneration WorldCensusErrorKind = "duplicate_entity_generation"
	WorldCensusNonFinite           WorldCensusErrorKind = "non_finite_data"
	WorldCensusInvalidSourceTick   WorldCensusErrorKind = "invalid_source_tick"
)

// WorldCensusError reports why direct world evidence was refused.
type WorldCensusError struct {
	Kind         WorldCensusErrorKind
	Tick         uint32
	EntityID     int32
	EntitySerial int32
	Field        string
}

func (e *WorldCensusError) Error() string {
	return fmt.Sprintf("world census: %s tick=%d entity=%d serial=%d field=%s", e.Kind, e.Tick, e.EntityID, e.EntitySerial, e.Field)
}

// BuildWorldCensus selects the latest typed entity sample at or before tick for
// each entity index. Two samples for one index at the same source tick are
// refused because they make its generation ambiguous.
func BuildWorldCensus(events []s2replay.Event, tick uint32) (WorldCensus, error) {
	selected := make(map[int32]s2replay.Event)
	for _, event := range events {
		if event.Type != s2replay.EventEntitySample || event.EntitySample == nil || event.Tick > tick {
			continue
		}
		if err := validateWorldCensusEvent(event, tick); err != nil {
			return WorldCensus{}, err
		}
		if prior, ok := selected[event.Entity]; ok && prior.Tick == event.Tick {
			return WorldCensus{}, &WorldCensusError{
				Kind: WorldCensusDuplicateGeneration, Tick: event.Tick,
				EntityID: event.Entity, EntitySerial: event.EntitySample.EntitySerial,
			}
		}
		if prior, ok := selected[event.Entity]; !ok || event.Tick > prior.Tick {
			selected[event.Entity] = event
		}
	}

	out := WorldCensus{Tick: tick, Entities: make([]WorldCensusEntity, 0, len(selected))}
	for _, event := range selected {
		out.Entities = append(out.Entities, worldCensusEntity(event, tick))
	}
	slices.SortFunc(out.Entities, func(a, b WorldCensusEntity) int {
		if a.EntityID != b.EntityID {
			if a.EntityID < b.EntityID {
				return -1
			}
			return 1
		}
		if a.EntitySerial < b.EntitySerial {
			return -1
		}
		if a.EntitySerial > b.EntitySerial {
			return 1
		}
		if a.ClassID < b.ClassID {
			return -1
		}
		if a.ClassID > b.ClassID {
			return 1
		}
		return 0
	})
	return out, nil
}

func validateWorldCensusEvent(event s2replay.Event, requestedTick uint32) error {
	sample := event.EntitySample
	values := []struct {
		name  string
		value float64
	}{
		{"event.game_time", event.GameTime},
		{"sample.game_time", sample.GameTime},
		{"health", float64(sample.Health)},
		{"max_health", float64(sample.MaxHealth)},
		{"shield", float64(sample.Shield)},
		{"max_shield", float64(sample.MaxShield)},
		{"position_x", float64(sample.PositionX)},
		{"position_y", float64(sample.PositionY)},
		{"position_z", float64(sample.PositionZ)},
		{"facing_x", float64(sample.FacingX)},
		{"facing_y", float64(sample.FacingY)},
		{"facing_z", float64(sample.FacingZ)},
		{"velocity_x", float64(sample.VelocityX)},
		{"velocity_y", float64(sample.VelocityY)},
		{"velocity_z", float64(sample.VelocityZ)},
	}
	for _, value := range values {
		if math.IsNaN(value.value) || math.IsInf(value.value, 0) {
			return &WorldCensusError{Kind: WorldCensusNonFinite, Tick: event.Tick, EntityID: event.Entity, EntitySerial: sample.EntitySerial, Field: value.name}
		}
	}
	if sample.HasPosition {
		for _, field := range []struct {
			name string
			tick uint32
		}{
			{"position_x", censusPositionTick(sample.PositionXTick, event.Tick)},
			{"position_y", censusPositionTick(sample.PositionYTick, event.Tick)},
			{"position_z", censusPositionTick(sample.PositionZTick, event.Tick)},
		} {
			if field.tick > requestedTick {
				return &WorldCensusError{Kind: WorldCensusInvalidSourceTick, Tick: event.Tick, EntityID: event.Entity, EntitySerial: sample.EntitySerial, Field: field.name}
			}
		}
	}
	return nil
}

func worldCensusEntity(event s2replay.Event, requestedTick uint32) WorldCensusEntity {
	sample := event.EntitySample
	row := WorldCensusEntity{
		EntityID: event.Entity, EntitySerial: sample.EntitySerial, ClassID: sample.ClassID,
		ClassName: sample.ClassName, SourceTick: event.Tick,
		FreshnessTicks: requestedTick - event.Tick,
	}
	if sample.HasHealth {
		row.Health = censusFloat(sample.Health, event.Tick, requestedTick)
	}
	if sample.HasShield {
		row.Shield = censusFloat(sample.Shield, event.Tick, requestedTick)
	}
	if sample.HasPosition {
		row.PositionX = censusFloat(sample.PositionX, censusPositionTick(sample.PositionXTick, event.Tick), requestedTick)
		row.PositionY = censusFloat(sample.PositionY, censusPositionTick(sample.PositionYTick, event.Tick), requestedTick)
		row.PositionZ = censusFloat(sample.PositionZ, censusPositionTick(sample.PositionZTick, event.Tick), requestedTick)
	}
	if sample.HasHeroID {
		row.HeroID = WorldCensusUint{Value: sample.HeroID, Present: true, SourceTick: event.Tick, FreshnessTicks: requestedTick - event.Tick}
	}
	if sample.HasTeam {
		row.Team = WorldCensusInt{Value: sample.Team, Present: true, SourceTick: event.Tick, FreshnessTicks: requestedTick - event.Tick}
	}
	return row
}

func censusFloat(value float32, sourceTick, requestedTick uint32) WorldCensusFloat {
	return WorldCensusFloat{Value: value, Present: true, SourceTick: sourceTick, FreshnessTicks: requestedTick - sourceTick}
}

func censusPositionTick(sourceTick, eventTick uint32) uint32 {
	if sourceTick == 0 && eventTick != 0 {
		return eventTick
	}
	return sourceTick
}
