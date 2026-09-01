package analysis

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/paralin/s2replay"
)

func censusEvent(tick uint32, entity, serial int32, class string, slot int32) s2replay.Event {
	return s2replay.Event{
		Type: s2replay.EventEntitySample, Tick: tick, Entity: entity, EntitySerial: serial, PlayerSlot: slot,
		EntitySample: &s2replay.EntitySample{
			Tick: tick, Entity: entity, EntitySerial: serial, ClassID: entity + 100,
			ClassName: class, Health: 100, Shield: 25, PositionX: 1, PositionY: 2, PositionZ: 3,
			HeroID: 42, Team: 2, HasHealth: true, HasShield: true, HasPosition: true,
			HasHeroID: true, HasTeam: true,
		},
	}
}

func TestBuildWorldCensusIncludesHeroesAndNonPlayerEntities(t *testing.T) {
	world := censusEvent(100, 5, 9, "npc_deadlock_boss_like_name", -1)
	world.EntitySample.HasHeroID = false
	world.EntitySample.HasTeam = false
	events := []s2replay.Event{
		censusEvent(100, 20, 3, "hero_amber", 4),
		world,
	}

	got, err := BuildWorldCensus(events, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entities) != 2 {
		t.Fatalf("entities: got %d, want 2", len(got.Entities))
	}
	if got.Entities[0].EntityID != 5 || got.Entities[0].EntitySerial != 9 || got.Entities[0].ClassName != "npc_deadlock_boss_like_name" {
		t.Fatalf("unexpected world entity evidence: %+v", got.Entities[0])
	}
	hero := got.Entities[1]
	if hero.EntityID != 20 || hero.EntitySerial != 3 || hero.ClassName != "hero_amber" || !hero.HeroID.Present || hero.HeroID.Value != 42 || !hero.Team.Present || hero.Team.Value != 2 {
		t.Fatalf("unexpected hero evidence: %+v", hero)
	}
	if got.Entities[0].HeroID.Present || got.Entities[0].Team.Present {
		t.Fatalf("world entity should preserve source presence, not player classification: %+v", got.Entities[0])
	}
}

func TestBuildWorldCensusOrderingIsDeterministic(t *testing.T) {
	a := []s2replay.Event{censusEvent(100, 20, 3, "hero", 4), censusEvent(100, 5, 9, "tower", -1)}
	b := []s2replay.Event{a[1], a[0]}
	gotA, err := BuildWorldCensus(a, 100)
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := BuildWorldCensus(b, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotA, gotB) {
		t.Fatalf("census depends on event order:\nA=%+v\nB=%+v", gotA, gotB)
	}
	if gotA.Entities[0].EntityID != 5 || gotA.Entities[1].EntityID != 20 {
		t.Fatalf("entity order: %+v", gotA.Entities)
	}
}

func TestBuildWorldCensusPreservesMissingFieldsAndFreshness(t *testing.T) {
	ev := censusEvent(96, 7, 2, "npc", -1)
	ev.EntitySample.HasHealth = false
	ev.EntitySample.HasShield = false
	ev.EntitySample.HasPosition = false
	ev.EntitySample.HasHeroID = false
	ev.EntitySample.HasTeam = false
	got, err := BuildWorldCensus([]s2replay.Event{ev}, 100)
	if err != nil {
		t.Fatal(err)
	}
	row := got.Entities[0]
	if row.Health.Present || row.Shield.Present || row.PositionX.Present || row.HeroID.Present || row.Team.Present {
		t.Fatalf("missing source fields became present: %+v", row)
	}
	if row.Health.SourceTick != 0 || row.Health.FreshnessTicks != 0 {
		t.Fatalf("absent health metadata: %+v", row.Health)
	}

	ev = censusEvent(96, 8, 2, "npc", -1)
	ev.EntitySample.PositionXTick, ev.EntitySample.PositionYTick, ev.EntitySample.PositionZTick = 90, 92, 94
	got, err = BuildWorldCensus([]s2replay.Event{ev}, 100)
	if err != nil {
		t.Fatal(err)
	}
	row = got.Entities[0]
	if !row.Health.Present || row.Health.SourceTick != 96 || row.Health.FreshnessTicks != 4 {
		t.Fatalf("health evidence: %+v", row.Health)
	}
	if row.PositionX.SourceTick != 90 || row.PositionX.FreshnessTicks != 10 {
		t.Fatalf("position evidence: %+v", row.PositionX)
	}
}

func TestBuildWorldCensusRejectsDuplicateEntityGeneration(t *testing.T) {
	a := censusEvent(100, 7, 2, "npc_a", -1)
	b := censusEvent(100, 7, 3, "npc_b", -1)
	_, err := BuildWorldCensus([]s2replay.Event{a, b}, 100)
	var typed *WorldCensusError
	if !errors.As(err, &typed) || typed.Kind != WorldCensusDuplicateGeneration {
		t.Fatalf("error = %v, want duplicate-generation typed error", err)
	}
}

func TestBuildWorldCensusRejectsNonFiniteData(t *testing.T) {
	ev := censusEvent(100, 7, 2, "npc", -1)
	ev.EntitySample.Health = float32(math.NaN())
	_, err := BuildWorldCensus([]s2replay.Event{ev}, 100)
	var typed *WorldCensusError
	if !errors.As(err, &typed) || typed.Kind != WorldCensusNonFinite {
		t.Fatalf("error = %v, want non-finite typed error", err)
	}
}

func TestBuildWorldCensusUsesFieldSourceTicks(t *testing.T) {
	ev := censusEvent(100, 7, 2, "npc", -1)
	ev.EntitySample.HealthTick = 88
	ev.EntitySample.ShieldTick = 92
	ev.EntitySample.HeroIDTick = 96
	ev.EntitySample.TeamTick = 97
	got, err := BuildWorldCensus([]s2replay.Event{ev}, 100)
	if err != nil {
		t.Fatal(err)
	}
	row := got.Entities[0]
	if row.Health.SourceTick != 88 || row.Health.FreshnessTicks != 12 {
		t.Fatalf("health freshness: %+v", row.Health)
	}
	if row.Shield.SourceTick != 92 || row.Shield.FreshnessTicks != 8 {
		t.Fatalf("shield freshness: %+v", row.Shield)
	}
	if row.HeroID.SourceTick != 96 || row.HeroID.FreshnessTicks != 4 {
		t.Fatalf("hero freshness: %+v", row.HeroID)
	}
	if row.Team.SourceTick != 97 || row.Team.FreshnessTicks != 3 {
		t.Fatalf("team freshness: %+v", row.Team)
	}
}

func TestBuildWorldCensusRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name  string
		event s2replay.Event
		kind  WorldCensusErrorKind
	}{
		{
			name:  "nil sample",
			event: s2replay.Event{Type: s2replay.EventEntitySample, Tick: 100, Entity: 7, EntitySerial: 2},
			kind:  WorldCensusInvalidEntity,
		},
		{
			name: "sample tick mismatch",
			event: func() s2replay.Event {
				ev := censusEvent(100, 7, 2, "npc", -1)
				ev.EntitySample.Tick = 99
				return ev
			}(),
			kind: WorldCensusInvalidSampleTick,
		},
		{
			name: "future source tick",
			event: func() s2replay.Event {
				ev := censusEvent(100, 7, 2, "npc", -1)
				ev.EntitySample.HealthTick = 101
				return ev
			}(),
			kind: WorldCensusInvalidSourceTick,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildWorldCensus([]s2replay.Event{tc.event}, 100)
			var typed *WorldCensusError
			if !errors.As(err, &typed) || typed.Kind != tc.kind {
				t.Fatalf("error = %v, want %s", err, tc.kind)
			}
		})
	}
}
