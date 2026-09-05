package analysis

import (
	"errors"
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
	WorldCensusDuplicateGeneration  WorldCensusErrorKind = "duplicate_entity_generation"
	WorldCensusInvalidEntity        WorldCensusErrorKind = "invalid_entity"
	WorldCensusNonFinite            WorldCensusErrorKind = "non_finite_data"
	WorldCensusInvalidSourceTick    WorldCensusErrorKind = "invalid_source_tick"
	WorldCensusInvalidSampleTick    WorldCensusErrorKind = "invalid_sample_tick"
	WorldCensusInvalidRequestedTick WorldCensusErrorKind = "invalid_requested_tick"
)

// WorldCensusError reports why direct world evidence was refused.
type WorldCensusError struct {
	Kind         WorldCensusErrorKind
	Tick         uint32
	EntityID     int32
	EntitySerial int32
	Field        string
}

// Error returns the typed refusal message with the offending values.
func (e *WorldCensusError) Error() string {
	return fmt.Sprintf("world census: %s tick=%d entity=%d serial=%d field=%s", e.Kind, e.Tick, e.EntityID, e.EntitySerial, e.Field)
}

// WorldCensusSnapshotSource is the parser seam used by bounded census extraction.
type WorldCensusSnapshotSource interface {
	WorldEntitySnapshot(uint32) ([]s2replay.EntitySample, error)
}

// ExtractWorldCensus samples the parser-owned active entity state at tick.
// The parser advances only through commands at or before the requested tick.
func ExtractWorldCensus(parser WorldCensusSnapshotSource, tick uint32) (WorldCensus, error) {
	if tick == s2replay.PreGameTick {
		return WorldCensus{}, &WorldCensusError{Kind: WorldCensusInvalidRequestedTick, Tick: tick, Field: "tick"}
	}
	samples, err := parser.WorldEntitySnapshot(tick)
	if err != nil {
		var unavailable *s2replay.WorldSnapshotError
		if errors.As(err, &unavailable) {
			return WorldCensus{}, &WorldCensusError{Kind: WorldCensusInvalidRequestedTick, Tick: unavailable.RequestedTick, Field: "tick_not_observed"}
		}
		return WorldCensus{}, err
	}
	events := make([]s2replay.Event, 0, len(samples))
	for i := range samples {
		sample := &samples[i]
		events = append(events, s2replay.Event{
			Type: s2replay.EventEntitySample, Tick: tick, Entity: sample.Entity,
			EntitySerial: sample.EntitySerial, EntitySample: sample,
		})
	}
	return BuildWorldCensus(events, tick)
}

// BuildWorldCensus selects the latest typed entity sample at or before tick for
// each entity index. Same-generation samples at one source tick are coalesced
// in parser order; differing generations at that tick are refused.
func BuildWorldCensus(events []s2replay.Event, tick uint32) (WorldCensus, error) {
	if tick == s2replay.PreGameTick {
		return WorldCensus{}, &WorldCensusError{Kind: WorldCensusInvalidRequestedTick, Tick: tick, Field: "tick"}
	}
	selected := make(map[int32]s2replay.Event)
	for _, event := range events {
		if event.Type != s2replay.EventEntitySample || event.Tick > tick {
			continue
		}
		if event.EntitySample == nil {
			return WorldCensus{}, &WorldCensusError{Kind: WorldCensusInvalidEntity, Tick: event.Tick, EntityID: event.Entity, Field: "entity_sample"}
		}
		if err := validateWorldCensusEvent(event, tick); err != nil {
			return WorldCensus{}, err
		}
		if prior, ok := selected[event.Entity]; ok && prior.Tick == event.Tick {
			if prior.EntitySample.EntitySerial != event.EntitySample.EntitySerial {
				return WorldCensus{}, &WorldCensusError{
					Kind: WorldCensusDuplicateGeneration, Tick: event.Tick,
					EntityID: event.Entity, EntitySerial: event.EntitySample.EntitySerial,
				}
			}
			// Entity samples are cumulative. The later same-tick sample carries
			// the state observed after the earlier sample.
			selected[event.Entity] = event
			continue
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

// validateWorldCensusEvent refuses events with inconsistent identity,
// sample ticks, or non-finite and future-dated fields.
func validateWorldCensusEvent(event s2replay.Event, requestedTick uint32) error {
	sample := event.EntitySample
	if event.Entity < 0 || sample.Entity < 0 || sample.Entity != event.Entity || sample.EntitySerial < 0 || event.EntitySerial < 0 || event.EntitySerial != sample.EntitySerial {
		return &WorldCensusError{Kind: WorldCensusInvalidEntity, Tick: event.Tick, EntityID: event.Entity, EntitySerial: sample.EntitySerial}
	}
	if sample.Tick != event.Tick {
		return &WorldCensusError{Kind: WorldCensusInvalidSampleTick, Tick: event.Tick, EntityID: event.Entity, EntitySerial: sample.EntitySerial, Field: "tick"}
	}
	if len(sample.InvalidFields) != 0 {
		return &WorldCensusError{Kind: WorldCensusNonFinite, Tick: event.Tick, EntityID: event.Entity, EntitySerial: sample.EntitySerial, Field: sample.InvalidFields[0]}
	}
	values := []struct {
		name    string
		value   float64
		present bool
	}{
		{"event.game_time", event.GameTime, true},
		{"sample.game_time", sample.GameTime, true},
		{"health", float64(sample.Health), sample.HasHealth},
		{"max_health", float64(sample.MaxHealth), sample.HasHealth},
		{"shield", float64(sample.Shield), sample.HasShield},
		{"max_shield", float64(sample.MaxShield), sample.HasShield},
		{"position_x", float64(sample.PositionX), sample.HasPosition},
		{"position_y", float64(sample.PositionY), sample.HasPosition},
		{"position_z", float64(sample.PositionZ), sample.HasPosition},
		{"facing_x", float64(sample.FacingX), sample.HasFacingX || sample.HasFacing},
		{"facing_y", float64(sample.FacingY), sample.HasFacingY || sample.HasFacing},
		{"facing_z", float64(sample.FacingZ), sample.HasFacingZ || sample.HasFacing},
		{"velocity_x", float64(sample.VelocityX), sample.HasVelocityX || sample.HasVelocity},
		{"velocity_y", float64(sample.VelocityY), sample.HasVelocityY || sample.HasVelocity},
		{"velocity_z", float64(sample.VelocityZ), sample.HasVelocityZ || sample.HasVelocity},
	}
	for _, value := range values {
		if value.present && (math.IsNaN(value.value) || math.IsInf(value.value, 0)) {
			return &WorldCensusError{Kind: WorldCensusNonFinite, Tick: event.Tick, EntityID: event.Entity, EntitySerial: sample.EntitySerial, Field: value.name}
		}
	}
	fields := []struct {
		name    string
		tick    uint32
		present bool
	}{
		{"health", sample.HealthTick, sample.HasHealth},
		{"max_health", sample.MaxHealthTick, sample.HasHealth},
		{"shield", sample.ShieldTick, sample.HasShield},
		{"max_shield", sample.MaxShieldTick, sample.HasShield},
		{"hero_id", sample.HeroIDTick, sample.HasHeroID},
		{"team", sample.TeamTick, sample.HasTeam},
		{"position_x", sample.PositionXTick, sample.HasPosition},
		{"position_y", sample.PositionYTick, sample.HasPosition},
		{"position_z", sample.PositionZTick, sample.HasPosition},
	}
	for _, field := range fields {
		if field.present && field.tick > event.Tick {
			return &WorldCensusError{Kind: WorldCensusInvalidSourceTick, Tick: event.Tick, EntityID: event.Entity, EntitySerial: sample.EntitySerial, Field: field.name}
		}
		if field.present && field.tick > requestedTick {
			return &WorldCensusError{Kind: WorldCensusInvalidSourceTick, Tick: event.Tick, EntityID: event.Entity, EntitySerial: sample.EntitySerial, Field: field.name}
		}
	}
	return nil
}

// worldCensusEntity converts one entity sample event into a census row.
func worldCensusEntity(event s2replay.Event, requestedTick uint32) WorldCensusEntity {
	sample := event.EntitySample
	row := WorldCensusEntity{
		EntityID: event.Entity, EntitySerial: sample.EntitySerial, ClassID: sample.ClassID,
		ClassName: sample.ClassName, SourceTick: event.Tick,
		FreshnessTicks: requestedTick - event.Tick,
	}
	if sample.HasHealth {
		row.Health = censusFloat(sample.Health, sample.HealthTick, requestedTick)
	}
	if sample.HasShield {
		row.Shield = censusFloat(sample.Shield, sample.ShieldTick, requestedTick)
	}
	if sample.HasPosition {
		row.PositionX = censusFloat(sample.PositionX, sample.PositionXTick, requestedTick)
		row.PositionY = censusFloat(sample.PositionY, sample.PositionYTick, requestedTick)
		row.PositionZ = censusFloat(sample.PositionZ, sample.PositionZTick, requestedTick)
	}
	if sample.HasHeroID {
		row.HeroID = WorldCensusUint{Value: sample.HeroID, Present: true, SourceTick: sample.HeroIDTick, FreshnessTicks: requestedTick - sample.HeroIDTick}
	}
	if sample.HasTeam {
		row.Team = WorldCensusInt{Value: sample.Team, Present: true, SourceTick: sample.TeamTick, FreshnessTicks: requestedTick - sample.TeamTick}
	}
	return row
}

// censusFloat builds a present float record with its source age.
func censusFloat(value float32, sourceTick, requestedTick uint32) WorldCensusFloat {
	return WorldCensusFloat{Value: value, Present: true, SourceTick: sourceTick, FreshnessTicks: requestedTick - sourceTick}
}
