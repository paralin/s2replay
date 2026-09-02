package analysis

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/paralin/s2replay"
)

func runbackSample(tick uint32, entity, serial int32, class string, slot int32) s2replay.EntitySample {
	return s2replay.EntitySample{
		Tick: tick, Entity: entity, EntitySerial: serial, ClassID: entity + 100,
		ClassName: class, PlayerSlot: slot, Health: 100, Shield: 25,
		HealthTick: tick, MaxHealthTick: tick, ShieldTick: tick, MaxShieldTick: tick,
		PositionXTick: tick, PositionYTick: tick, PositionZTick: tick,
		PositionX: 1, PositionY: 2, PositionZ: 3,
		HeroID: 42, Team: 2, HasHealth: true, HasShield: true, HasPosition: true,
		HasHeroID: true, HasTeam: true,
	}
}

func runbackPawn(tick uint32, entity int32, slot int32) s2replay.EntitySample {
	sample := runbackSample(tick, entity, entity*7, "CCitadelPlayerPawn", slot)
	sample.Level = 25
	sample.LevelTick = tick
	sample.HasLevel = true
	return sample
}

func runbackController(tick uint32, entity, pawn int32) s2replay.EntitySample {
	sample := runbackSample(tick, entity, entity*7, "CCitadelPlayerController", -1)
	sample.HasPawnEntity = true
	sample.PawnEntity = pawn
	sample.PawnEntityTick = tick
	sample.NetWorth = int32(1000 + entity)
	sample.NetWorthTick = tick
	sample.HasNetWorth = true
	sample.Deaths = 3
	sample.DeathsTick = tick
	sample.HasDeaths = true
	return sample
}

func TestBuildRunbackFactsAttributesHeroSlotsFromPawns(t *testing.T) {
	samples := []s2replay.EntitySample{
		runbackSample(100, 500, 9, "npc_dota_boss", -1),
		runbackController(100, 7, 92),
		runbackPawn(100, 92, 1),
		runbackPawn(100, 78, 12),
	}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Heroes) != 2 {
		t.Fatalf("heroes: got %d, want 2", len(got.Heroes))
	}
	if got.Heroes[0].PlayerSlot != 1 || got.Heroes[0].EntityID != 92 || !got.Heroes[0].Level.Present || got.Heroes[0].Level.Value != 25 {
		t.Fatalf("hero slot 1: %+v", got.Heroes[0])
	}
	if got.Heroes[0].NetWorth.Value != 1007 || !got.Heroes[0].NetWorth.Present {
		t.Fatalf("net worth: %+v", got.Heroes[0].NetWorth)
	}
	if got.Heroes[0].Scores.Deaths.Value != 3 || !got.Heroes[0].Scores.Deaths.Present {
		t.Fatalf("scores: %+v", got.Heroes[0].Scores)
	}
	if got.Heroes[1].PlayerSlot != 12 || got.Heroes[1].EntityID != 78 {
		t.Fatalf("hero slot 12: %+v", got.Heroes[1])
	}
	if got.Heroes[1].NetWorth.Present || got.Heroes[1].NetWorth.MissingReason != RunbackMissingNoEntity {
		t.Fatalf("controller-less net worth should be typed missing: %+v", got.Heroes[1].NetWorth)
	}
	if len(got.WorldEntities) != 1 || got.WorldEntities[0].ClassName != "npc_dota_boss" {
		t.Fatalf("world entities: %+v", got.WorldEntities)
	}
	if !got.WorldEntities[0].Alive.Alive || got.WorldEntities[0].Alive.Basis != RunbackAliveHealthPositive {
		t.Fatalf("alive: %+v", got.WorldEntities[0].Alive)
	}
}

func TestBuildRunbackFactsRefusesDuplicateSlot(t *testing.T) {
	samples := []s2replay.EntitySample{
		runbackPawn(100, 92, 1),
		runbackPawn(100, 78, 1),
	}
	_, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100})
	var typed *RunbackError
	if !errors.As(err, &typed) || typed.Kind != RunbackErrorDuplicateSlot {
		t.Fatalf("error = %v, want duplicate slot", err)
	}
}

func TestBuildRunbackFactsRefusesOutOfRangeSlot(t *testing.T) {
	samples := []s2replay.EntitySample{runbackPawn(100, 92, MaxReplayParticipants)}
	_, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100})
	var typed *RunbackError
	if !errors.As(err, &typed) || typed.Kind != RunbackErrorSlotOutOfRange {
		t.Fatalf("error = %v, want out of range slot", err)
	}
}

func TestBuildRunbackFactsRefusesDuplicateEntitySample(t *testing.T) {
	samples := []s2replay.EntitySample{runbackSample(100, 5, 9, "npc", -1), runbackSample(100, 5, 10, "npc", -1)}
	_, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100})
	var typed *RunbackError
	if !errors.As(err, &typed) || typed.Kind != RunbackErrorInvalidEntity {
		t.Fatalf("error = %v, want invalid entity", err)
	}
}

func TestBuildRunbackFactsOrderingIsDeterministic(t *testing.T) {
	samples := []s2replay.EntitySample{
		runbackController(100, 7, 92),
		runbackPawn(100, 92, 1),
		runbackPawn(100, 78, 12),
		runbackSample(100, 500, 9, "npc_tower", -1),
		runbackSample(100, 501, 8, "npc_tower", -1),
	}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Heroes[0].PlayerSlot, int32(1)) || got.WorldEntities[0].EntityID != 500 {
		t.Fatalf("ordering: heroes=%d world0=%d", got.Heroes[0].PlayerSlot, got.WorldEntities[0].EntityID)
	}
}

func TestBuildRunbackFactsDeterministicJSON(t *testing.T) {
	samples := []s2replay.EntitySample{
		runbackController(100, 7, 92),
		runbackPawn(100, 92, 1),
		runbackSample(100, 500, 9, "npc_boss", -1),
	}
	a, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{SHA256: "abc"}, RunbackRequest{Tick: 100})
	if err != nil {
		t.Fatal(err)
	}
	b, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{SHA256: "abc"}, RunbackRequest{Tick: 100})
	if err != nil {
		t.Fatal(err)
	}
	aj, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	bj, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(aj) != string(bj) {
		t.Fatal("runback facts JSON is not deterministic")
	}
}

func TestBuildRunbackFactsNonFiniteRefused(t *testing.T) {
	ability := runbackPawn(100, 50, -1)
	ability.ClassName = "CCitadel_Ability_Dash"
	ability.CooldownEnd = float32(math.NaN())
	ability.HasCooldownEnd = true
	ability.HasOwnerEntity = true
	ability.OwnerEntity = 92
	samples := []s2replay.EntitySample{runbackPawn(100, 92, 1), ability}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Heroes) != 1 || len(got.Heroes[0].Abilities) != 1 {
		t.Fatalf("abilities: %+v", got.Heroes)
	}
	if got.Heroes[0].Abilities[0].CooldownEnd.Present || got.Heroes[0].Abilities[0].CooldownEnd.MissingReason != RunbackMissingNonFinite {
		t.Fatalf("non-finite cooldown became present: %+v", got.Heroes[0].Abilities[0].CooldownEnd)
	}
}

func TestExtractRunbackFactsRejectsInvalidTick(t *testing.T) {
	_, err := ExtractRunbackFacts([]byte("junk"), RunbackRequest{Tick: 0})
	if err == nil {
		t.Fatal("expected refusal for tick 0")
	}
	var typed *RunbackError
	if !errors.As(err, &typed) || typed.Kind != RunbackErrorInvalidTick {
		t.Fatalf("error = %v, want invalid tick", err)
	}
}

func TestExtractRunbackFactsRejectsBadDemo(t *testing.T) {
	if _, err := ExtractRunbackFacts([]byte("junk"), RunbackRequest{Tick: 100}); err == nil {
		t.Fatal("expected refusal for bad demo")
	}
}

func TestOptInPinnedRunbackFacts(t *testing.T) {
	path := os.Getenv("S2REPLAY_PINNED_DEMO")
	if path == "" {
		t.Skip("set S2REPLAY_PINNED_DEMO to run the pinned runback facts")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	extract := func() RunbackFacts {
		facts, err := extractRunbackFactsWithBuild(b, RunbackRequest{Tick: 63280}, "fixture", true)
		if err != nil {
			t.Fatal(err)
		}
		return facts
	}
	a := extract()
	bf := extract()
	aj, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	bj, err := json.Marshal(bf)
	if err != nil {
		t.Fatal(err)
	}
	if string(aj) != string(bj) {
		t.Fatal("pinned runback facts are not byte-identical across two extractions")
	}
	if a.Quality.Heroes != 12 {
		t.Fatalf("pinned hero slots: got %d want 12", a.Quality.Heroes)
	}
	if len(a.Heroes) == 0 || a.Heroes[0].PlayerSlot < 1 {
		t.Fatalf("hero slot ordering: %+v", a.Heroes)
	}
	var slots []int32
	for _, hero := range a.Heroes {
		slots = append(slots, hero.PlayerSlot)
	}
	for i := 1; i < len(slots); i++ {
		if slots[i] <= slots[i-1] {
			t.Fatalf("slots not strictly increasing: %v", slots)
		}
	}
	if len(a.WorldEntities) == 0 {
		t.Fatal("pinned world entities are empty")
	}
	var foundBoss, foundTower bool
	for _, entity := range a.WorldEntities {
		if entity.ClassName == "npc_dota_boss" {
			foundBoss = true
		}
		if entity.Team.Present {
			foundTower = true
		}
	}
	_ = foundBoss
	_ = foundTower
	t.Logf("heroes=%d world=%d snapshot=%d", a.Quality.Heroes, a.Quality.WorldEntities, a.Quality.SnapshotEntities)
}
