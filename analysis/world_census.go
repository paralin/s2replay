package analysis

import (
	"fmt"
	"io"
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
	WorldCensusInvalidEntity       WorldCensusErrorKind = "invalid_entity"
	WorldCensusNonFinite           WorldCensusErrorKind = "non_finite_data"
	WorldCensusInvalidSourceTick   WorldCensusErrorKind = "invalid_source_tick"
	WorldCensusInvalidSampleTick   WorldCensusErrorKind = "invalid_sample_tick"
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

// WorldCensusEventSource is the parser seam used by bounded census extraction.
type WorldCensusEventSource interface {
	NextEvent() (s2replay.Event, error)
	SetEventMode(bool)
	SetWorldEntityMode(bool)
	ReleasePendingQueues()
}

// ExtractWorldCensus reads entity events through the requested tick and then
// builds a census. The parser's generic-entity mode is opt-in and all pending
// decoded queues are released before the call returns.
func ExtractWorldCensus(parser WorldCensusEventSource, tick uint32) (WorldCensus, error) {
	parser.SetEventMode(true)
	parser.SetWorldEntityMode(true)
	defer func() {
		parser.SetWorldEntityMode(false)
		parser.SetEventMode(false)
		parser.ReleasePendingQueues()
	}()

	events := make([]s2replay.Event, 0)
	for {
		event, err := parser.NextEvent()
		if err == io.EOF {
			break
		}
		if err != nil {
			return WorldCensus{}, err
		}
		if event.Tick != s2replay.PreGameTick && event.Tick > tick {
			break
		}
		events = append(events, event)
	}
	return BuildWorldCensus(events, tick)
}

// BuildWorldCensus selects the latest typed entity sample at or before tick for
// each entity index. Two samples for one index at the same source tick are
// refused because they make its generation ambiguous.
func BuildWorldCensus(events []s2replay.Event, tick uint32) (WorldCensus, error) {
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
	if sample.Entity != event.Entity || sample.EntitySerial < 0 || event.EntitySerial < 0 || event.EntitySerial != sample.EntitySerial {
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
		{"health", censusSourceTick(sample.HealthTick, event.Tick), sample.HasHealth},
		{"max_health", censusSourceTick(sample.MaxHealthTick, event.Tick), sample.HasHealth},
		{"shield", censusSourceTick(sample.ShieldTick, event.Tick), sample.HasShield},
		{"max_shield", censusSourceTick(sample.MaxShieldTick, event.Tick), sample.HasShield},
		{"hero_id", censusSourceTick(sample.HeroIDTick, event.Tick), sample.HasHeroID},
		{"team", censusSourceTick(sample.TeamTick, event.Tick), sample.HasTeam},
		{"position_x", censusPositionTick(sample.PositionXTick, event.Tick), sample.HasPosition},
		{"position_y", censusPositionTick(sample.PositionYTick, event.Tick), sample.HasPosition},
		{"position_z", censusPositionTick(sample.PositionZTick, event.Tick), sample.HasPosition},
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

func worldCensusEntity(event s2replay.Event, requestedTick uint32) WorldCensusEntity {
	sample := event.EntitySample
	row := WorldCensusEntity{
		EntityID: event.Entity, EntitySerial: sample.EntitySerial, ClassID: sample.ClassID,
		ClassName: sample.ClassName, SourceTick: event.Tick,
		FreshnessTicks: requestedTick - event.Tick,
	}
	if sample.HasHealth {
		row.Health = censusFloat(sample.Health, censusSourceTick(sample.HealthTick, event.Tick), requestedTick)
	}
	if sample.HasShield {
		row.Shield = censusFloat(sample.Shield, censusSourceTick(sample.ShieldTick, event.Tick), requestedTick)
	}
	if sample.HasPosition {
		row.PositionX = censusFloat(sample.PositionX, censusPositionTick(sample.PositionXTick, event.Tick), requestedTick)
		row.PositionY = censusFloat(sample.PositionY, censusPositionTick(sample.PositionYTick, event.Tick), requestedTick)
		row.PositionZ = censusFloat(sample.PositionZ, censusPositionTick(sample.PositionZTick, event.Tick), requestedTick)
	}
	if sample.HasHeroID {
		sourceTick := censusSourceTick(sample.HeroIDTick, event.Tick)
		row.HeroID = WorldCensusUint{Value: sample.HeroID, Present: true, SourceTick: sourceTick, FreshnessTicks: requestedTick - sourceTick}
	}
	if sample.HasTeam {
		sourceTick := censusSourceTick(sample.TeamTick, event.Tick)
		row.Team = WorldCensusInt{Value: sample.Team, Present: true, SourceTick: sourceTick, FreshnessTicks: requestedTick - sourceTick}
	}
	return row
}

func censusFloat(value float32, sourceTick, requestedTick uint32) WorldCensusFloat {
	return WorldCensusFloat{Value: value, Present: true, SourceTick: sourceTick, FreshnessTicks: requestedTick - sourceTick}
}

func censusSourceTick(sourceTick, eventTick uint32) uint32 {
	if sourceTick == 0 && eventTick != 0 {
		return eventTick
	}
	return sourceTick
}

func censusPositionTick(sourceTick, eventTick uint32) uint32 {
	if sourceTick == 0 && eventTick != 0 {
		return eventTick
	}
	return sourceTick
}
