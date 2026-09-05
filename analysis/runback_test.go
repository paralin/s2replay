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
	sample.PawnEntitySerial = pawn * 7
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
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
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
	_, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	var typed *RunbackError
	if !errors.As(err, &typed) || typed.Kind != RunbackErrorDuplicateSlot {
		t.Fatalf("error = %v, want duplicate slot", err)
	}
}

func TestBuildRunbackFactsRefusesOutOfRangeSlot(t *testing.T) {
	samples := []s2replay.EntitySample{runbackPawn(100, 92, MaxReplayParticipants)}
	_, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	var typed *RunbackError
	if !errors.As(err, &typed) || typed.Kind != RunbackErrorSlotOutOfRange {
		t.Fatalf("error = %v, want out of range slot", err)
	}
}

func TestBuildRunbackFactsRefusesDuplicateEntitySample(t *testing.T) {
	samples := []s2replay.EntitySample{runbackSample(100, 5, 9, "npc", -1), runbackSample(100, 5, 10, "npc", -1)}
	_, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
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
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
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
	a, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{SHA256: "abc"}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{SHA256: "abc"}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
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
	ability.OwnerEntitySerial = 92 * 7
	samples := []s2replay.EntitySample{runbackPawn(100, 92, 1), ability}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
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

func runbackMidBoss(tick uint32, entity int32) s2replay.EntitySample {
	sample := runbackSample(tick, entity, entity*3, RunbackMidBossClass, -1)
	sample.Team = 4
	sample.Health = 13195
	sample.MaxHealth = 13195
	return sample
}

func runbackTower(tick uint32, entity int32, team int32) s2replay.EntitySample {
	sample := runbackSample(tick, entity, entity*3, RunbackTowerClass, -1)
	sample.Team = team
	sample.Health = 1000
	sample.MaxHealth = 1000
	return sample
}

func runbackWalker(tick uint32, entity int32, team int32) s2replay.EntitySample {
	sample := runbackSample(tick, entity, entity*3, RunbackWalkerClass, -1)
	sample.Team = team
	sample.Health = 4000
	sample.MaxHealth = 4000
	return sample
}

// runbackObjectiveEvent mirrors packet.go's appendObjectiveEvent argument
// order: (kind, a=ObjectiveTeam, b=ObjectiveID, c=EntityType, d=BossesRemaining).
func runbackObjectiveEvent(tick uint32, kind string, a, b, c int32) s2replay.Event {
	return s2replay.Event{
		Type: s2replay.EventObjective, Tick: tick, Entity: -1, PlayerSlot: -1,
		Objective: &s2replay.ObjectiveEvent{
			Kind: kind, ObjectiveTeam: a, ObjectiveID: b, EntityType: c,
		},
	}
}

func TestBuildRunbackFactsObjectiveEntities(t *testing.T) {
	samples := []s2replay.EntitySample{
		runbackMidBoss(100, 2946),
		runbackTower(100, 422, 2),
		runbackTower(100, 429, 3),
		runbackWalker(100, 349, 2),
		runbackWalker(100, 350, 2),
		runbackPawn(100, 92, 1),
	}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	objectives := got.Objectives
	if objectives.MidBoss.EntityID != 2946 || objectives.MidBoss.ClassName != RunbackMidBossClass {
		t.Fatalf("mid boss: %+v", objectives.MidBoss)
	}
	if !objectives.MidBoss.Health.Present || objectives.MidBoss.Health.Value != 13195 {
		t.Fatalf("mid boss health: %+v", objectives.MidBoss.Health)
	}
	if !objectives.MidBoss.Alive.Alive || objectives.MidBoss.Alive.Basis != RunbackAliveHealthPositive {
		t.Fatalf("mid boss alive: %+v", objectives.MidBoss.Alive)
	}
	if len(objectives.Towers) != 2 || objectives.Towers[0].EntityID != 422 || objectives.Towers[1].EntityID != 429 {
		t.Fatalf("towers: %+v", objectives.Towers)
	}
	if objectives.Towers[0].Team.Value != 2 || objectives.Towers[1].Team.Value != 3 {
		t.Fatalf("tower teams: %+v", objectives.Towers)
	}
	if len(objectives.Walkers) != 2 || objectives.Walkers[0].EntityID != 349 {
		t.Fatalf("walkers: %+v", objectives.Walkers)
	}
}

func TestBuildRunbackFactsMidBossTypedMissing(t *testing.T) {
	samples := []s2replay.EntitySample{runbackPawn(100, 92, 1)}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	objectives := got.Objectives
	if objectives.MidBoss.EntityID != 0 || objectives.MidBoss.ClassName != "" {
		t.Fatalf("absent mid boss should be zero: %+v", objectives.MidBoss)
	}
	if objectives.MidBoss.Health.Present || objectives.MidBoss.Health.MissingReason != RunbackMissingNotInSample {
		t.Fatalf("absent mid boss health should be typed missing: %+v", objectives.MidBoss.Health)
	}
	if len(objectives.Towers) != 0 || len(objectives.Walkers) != 0 {
		t.Fatalf("absent towers/walkers should be empty: %+v", objectives)
	}
}

func TestBuildRunbackFactsRejuvenatorFromEvents(t *testing.T) {
	samples := []s2replay.EntitySample{runbackPawn(100, 92, 1)}
	events := []s2replay.Event{
		runbackObjectiveEvent(50, RunbackRejuvenatorEventKind, 0, 2, 3),
		runbackObjectiveEvent(80, RunbackRejuvenatorEventKind, 0, 3, 2),
	}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, events)
	if err != nil {
		t.Fatal(err)
	}
	rejuv := got.Objectives.Rejuvenator
	if rejuv.Status != RunbackRejuvenatorStatusSeen {
		t.Fatalf("rejuvenator status: %+v", rejuv)
	}
	if rejuv.Last == nil || rejuv.Last.Tick != 80 || rejuv.Last.EventType != 3 || rejuv.Last.UserTeam != 2 {
		t.Fatalf("last rejuvenator event: %+v", rejuv.Last)
	}
}

func TestBuildRunbackFactsRejuvenatorTypedMissing(t *testing.T) {
	samples := []s2replay.EntitySample{runbackPawn(100, 92, 1)}
	events := []s2replay.Event{runbackObjectiveEvent(50, "boss_killed", 0, 5, 3)}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, events)
	if err != nil {
		t.Fatal(err)
	}
	rejuv := got.Objectives.Rejuvenator
	if rejuv.Status != RunbackRejuvenatorStatusAbsent || rejuv.Last != nil {
		t.Fatalf("rejuvenator should be typed missing without rejuv events: %+v", rejuv)
	}
}

func TestBuildRunbackFactsRejuvenatorIgnoresFutureEvents(t *testing.T) {
	samples := []s2replay.EntitySample{runbackPawn(100, 92, 1)}
	events := []s2replay.Event{runbackObjectiveEvent(200, RunbackRejuvenatorEventKind, 0, 2, 3)}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, events)
	if err != nil {
		t.Fatal(err)
	}
	if got.Objectives.Rejuvenator.Status != RunbackRejuvenatorStatusAbsent {
		t.Fatalf("future rejuv event must not be observed: %+v", got.Objectives.Rejuvenator)
	}
}

func TestBuildRunbackFactsUnattributedTransient(t *testing.T) {
	idol := runbackSample(100, 3547, 946, "CCitadelItemPickupIdol", -1)
	idol.Team = 4
	idol.HasOwnerEntity = true
	idol.OwnerEntity = 16383
	idol.OwnerEntityTick = 100
	idol.PositionX, idol.PositionY, idol.PositionZ = 6584, 0, 144
	samples := []s2replay.EntitySample{runbackPawn(100, 92, 1), idol}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Objectives.Transients) != 1 {
		t.Fatalf("transients: %+v", got.Objectives.Transients)
	}
	transient := got.Objectives.Transients[0]
	if transient.EntityID != 3547 || transient.ClassName != "CCitadelItemPickupIdol" {
		t.Fatalf("transient identity: %+v", transient)
	}
	if !transient.OwnerEntity.Present || transient.OwnerEntity.Value != 16383 {
		t.Fatalf("transient owner: %+v", transient.OwnerEntity)
	}
	if transient.MissingReason != RunbackMissingOwnerUnattributed {
		t.Fatalf("transient missing reason: %+v", transient)
	}
	if transient.Team.Value != 4 || !transient.Team.Present {
		t.Fatalf("transient team: %+v", transient.Team)
	}
	// The unattributed item must not appear on any hero row.
	for _, hero := range got.Heroes {
		for _, item := range hero.Items {
			if item.EntityID == 3547 {
				t.Fatalf("unattributed item leaked to hero row: %+v", item)
			}
		}
	}
}

func TestBuildRunbackFactsOwnedItemNotTransient(t *testing.T) {
	item := runbackSample(100, 300, 21, "CCitadel_Item_WarpStone", -1)
	item.HasOwnerEntity = true
	item.OwnerEntity = 92
	item.OwnerEntitySerial = 92 * 7
	item.OwnerEntityTick = 100
	samples := []s2replay.EntitySample{runbackPawn(100, 92, 1), item}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Objectives.Transients) != 0 {
		t.Fatalf("owned item must not be a transient: %+v", got.Objectives.Transients)
	}
	if len(got.Heroes) != 1 || len(got.Heroes[0].Items) != 1 || got.Heroes[0].Items[0].EntityID != 300 {
		t.Fatalf("owned item should stay on hero row: %+v", got.Heroes)
	}
}

func TestBuildRunbackFactsTickProvenance(t *testing.T) {
	samples := []s2replay.EntitySample{runbackPawn(100, 92, 1)}
	provenance := RunbackTickProvenance{
		TickIntervalSeconds: RunbackFloat{Value: 0.015625, Present: true, SourceTick: 100},
		ServerStartTick:     RunbackInt{Value: 2220, Present: true, FreshnessTicks: 100},
	}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, provenance, nil)
	if err != nil {
		t.Fatal(err)
	}
	tp := got.TickProvenance
	if !tp.TickIntervalSeconds.Present || tp.TickIntervalSeconds.Value != 0.015625 {
		t.Fatalf("tick interval: %+v", tp.TickIntervalSeconds)
	}
	if !tp.ServerStartTick.Present || tp.ServerStartTick.Value != 2220 {
		t.Fatalf("server start tick: %+v", tp.ServerStartTick)
	}
}

func TestBuildRunbackFactsTickProvenanceTypedMissing(t *testing.T) {
	samples := []s2replay.EntitySample{runbackPawn(100, 92, 1)}
	got, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tp := got.TickProvenance
	if tp.TickIntervalSeconds.Present || tp.TickIntervalSeconds.MissingReason != RunbackMissingNoServerInfo {
		t.Fatalf("unknown interval must be typed missing: %+v", tp.TickIntervalSeconds)
	}
	if tp.ServerStartTick.Present || tp.ServerStartTick.MissingReason != RunbackMissingHeaderField {
		t.Fatalf("absent start tick must be typed missing: %+v", tp.ServerStartTick)
	}
}

func TestBuildRunbackFactsObjectivesDeterministicJSON(t *testing.T) {
	samples := []s2replay.EntitySample{
		runbackMidBoss(100, 2946),
		runbackTower(100, 429, 3),
		runbackTower(100, 422, 2),
		runbackWalker(100, 350, 2),
		runbackWalker(100, 349, 2),
		runbackPawn(100, 92, 1),
	}
	events := []s2replay.Event{runbackObjectiveEvent(80, RunbackRejuvenatorEventKind, 0, 3, 2)}
	provenance := RunbackTickProvenance{TickIntervalSeconds: RunbackFloat{Value: 0.015625, Present: true, SourceTick: 100}}
	a, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{SHA256: "abc"}, RunbackRequest{Tick: 100}, provenance, events)
	if err != nil {
		t.Fatal(err)
	}
	b, err := buildRunbackFacts(samples, Result{}, ReplaySourceIdentity{SHA256: "abc"}, RunbackRequest{Tick: 100}, provenance, events)
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
		t.Fatal("objective facts JSON is not deterministic")
	}
}

func TestOptInPinnedRunbackObjectives(t *testing.T) {
	path := os.Getenv("S2REPLAY_PINNED_DEMO")
	if path == "" {
		t.Skip("set S2REPLAY_PINNED_DEMO to run the pinned runback objectives")
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
		t.Fatal("pinned runback objective facts are not byte-identical across two extractions")
	}

	// Expected census observed from the pinned demo at tick 63280
	// (101514223.dem, sha256 b612e43f4055d4dde728c7eedbdd7ec38c3478ef90f33b870bfb29310b79194f).
	if a.Quality.SnapshotEntities != 2289 {
		t.Fatalf("pinned snapshot entities: got %d want 2289", a.Quality.SnapshotEntities)
	}
	if a.Quality.Heroes != 12 {
		t.Fatalf("pinned heroes: got %d want 12", a.Quality.Heroes)
	}
	objectives := a.Objectives
	if objectives.MidBoss.EntityID != 2946 || objectives.MidBoss.ClassName != RunbackMidBossClass {
		t.Fatalf("pinned mid boss: %+v", objectives.MidBoss)
	}
	if !objectives.MidBoss.Team.Present || objectives.MidBoss.Team.Value != 4 {
		t.Fatalf("pinned mid boss team: %+v", objectives.MidBoss.Team)
	}
	if !objectives.MidBoss.Health.Present || objectives.MidBoss.Health.Value != 13195 || objectives.MidBoss.MaxHealth.Value != 13195 {
		t.Fatalf("pinned mid boss health: %+v", objectives.MidBoss.Health)
	}
	if !objectives.MidBoss.Alive.Alive {
		t.Fatalf("pinned mid boss alive: %+v", objectives.MidBoss.Alive)
	}
	if len(objectives.Towers) != 8 {
		t.Fatalf("pinned towers: got %d want 8", len(objectives.Towers))
	}
	for _, tower := range objectives.Towers {
		if tower.ClassName != RunbackTowerClass || !tower.Alive.Alive {
			t.Fatalf("pinned tower row: %+v", tower)
		}
	}
	if len(objectives.Walkers) != 6 {
		t.Fatalf("pinned walkers: got %d want 6", len(objectives.Walkers))
	}
	for _, walker := range objectives.Walkers {
		if walker.ClassName != RunbackWalkerClass || !walker.Alive.Alive {
			t.Fatalf("pinned walker row: %+v", walker)
		}
	}
	// The pinned demo ends at the requested tick with no rejuv_status event
	// observed: the rejuvenator must be typed missing, not guessed.
	if objectives.Rejuvenator.Status != RunbackRejuvenatorStatusAbsent || objectives.Rejuvenator.Last != nil {
		t.Fatalf("pinned rejuvenator: %+v", objectives.Rejuvenator)
	}
	// Two unattributed item-class transients are active: the neutral gold
	// punchable item and the idol pickup.
	if len(objectives.Transients) != 2 {
		t.Fatalf("pinned transients: got %d want 2: %+v", len(objectives.Transients), objectives.Transients)
	}
	if objectives.Transients[0].EntityID != 3445 || objectives.Transients[0].ClassName != "CCitadelItemPunchableNeutralGold" {
		t.Fatalf("pinned transient 0: %+v", objectives.Transients[0])
	}
	if objectives.Transients[1].EntityID != 3547 || objectives.Transients[1].ClassName != "CCitadelItemPickupIdol" {
		t.Fatalf("pinned transient 1: %+v", objectives.Transients[1])
	}
	for _, transient := range objectives.Transients {
		if transient.MissingReason != RunbackMissingOwnerUnattributed {
			t.Fatalf("pinned transient reason: %+v", transient)
		}
	}
	// Tick provenance from the pinned demo: ServerInfo interval and header
	// start tick are both observed.
	if !a.TickProvenance.TickIntervalSeconds.Present || a.TickProvenance.TickIntervalSeconds.Value != 0.015625 {
		t.Fatalf("pinned tick interval: %+v", a.TickProvenance.TickIntervalSeconds)
	}
	if !a.TickProvenance.ServerStartTick.Present || a.TickProvenance.ServerStartTick.Value != 2220 {
		t.Fatalf("pinned server start tick: %+v", a.TickProvenance.ServerStartTick)
	}
}

func TestBuildRunbackFactsPreservesConcreteEquipmentIdentity(t *testing.T) {
	item := runbackSample(100, 300, 21, "CCitadel_Item_Empty", -1)
	item.HasOwnerEntity, item.OwnerEntity = true, 92
	item.OwnerEntitySerial = 92 * 7
	item.HasSubclassID, item.SubclassID, item.SubclassIDTick = true, 0xf1234567, 80
	item.HasAbilitySlot, item.AbilitySlot, item.AbilitySlotTick = true, 12, 81
	item.HasUpgradeInfo, item.UpgradeInfo, item.UpgradeInfoTick = true, 0, 82
	item.HasRemainingCharges, item.RemainingCharges, item.RemainingChargesTick = true, 2, 90
	item.HasCooldownEnd, item.CooldownEnd, item.CooldownEndTick = true, 14.5, 91
	ability := runbackSample(100, 301, 22, "CCitadel_Ability_PrimaryWeapon_Empty", -1)
	ability.HasOwnerEntity, ability.OwnerEntity = true, 92
	ability.OwnerEntitySerial = 92 * 7
	got, err := buildRunbackFacts([]s2replay.EntitySample{runbackPawn(100, 92, 1), item, ability}, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hero := got.Heroes[0]
	if len(hero.Items) != 1 || len(hero.Abilities) != 1 {
		t.Fatalf("equipment missing: %+v", hero)
	}
	id := hero.Items[0].SubclassID
	if !id.Present || id.Value != 0xf1234567 || id.SourceTick != 80 || id.FreshnessTicks != 20 {
		t.Fatalf("item identity lost: %+v", id)
	}
	itemState := hero.Items[0]
	if !itemState.Slot.Present || itemState.Slot.Value != 12 || itemState.Slot.SourceTick != 81 || itemState.Slot.FreshnessTicks != 19 || !itemState.UpgradeInfo.Present || itemState.UpgradeInfo.Value != 0 || itemState.UpgradeInfo.SourceTick != 82 {
		t.Fatalf("item slot or observed zero upgrades lost: %+v", itemState)
	}
	if !itemState.Charges.Present || itemState.Charges.Value != 2 || itemState.Charges.SourceTick != 90 || !itemState.CooldownEnd.Present || itemState.CooldownEnd.Value != 14.5 || itemState.CooldownEnd.SourceTick != 91 {
		t.Fatalf("active item state lost: %+v", itemState)
	}
	if hero.Abilities[0].Slot.Present || hero.Abilities[0].Slot.MissingReason != "m_eAbilitySlot_not_present" || hero.Abilities[0].UpgradeInfo.Present || hero.Abilities[0].UpgradeInfo.MissingReason != "m_nUpgradeInfo_not_present" {
		t.Fatalf("missing ability state fabricated: %+v", hero.Abilities[0])
	}
	missing := hero.Abilities[0].SubclassID
	if missing.Present || missing.MissingReason != "m_nSubclassID_not_present" {
		t.Fatalf("missing identity fabricated: %+v", missing)
	}
}

func TestBuildRunbackFactsRejectsReusedOwnershipGenerations(t *testing.T) {
	pawn := runbackPawn(100, 92, 1)
	item := runbackSample(100, 300, 21, "CCitadel_Item_WarpStone", -1)
	item.HasOwnerEntity = true
	item.OwnerEntity = pawn.Entity
	item.OwnerEntitySerial = pawn.EntitySerial - 1
	ability := item
	ability.Entity = 301
	ability.ClassName = "CCitadel_Ability_Dash"
	controller := runbackController(100, 10, pawn.Entity)
	controller.PawnEntitySerial = pawn.EntitySerial - 1
	facts, err := buildRunbackFacts([]s2replay.EntitySample{pawn, item, ability, controller}, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Heroes) != 1 || len(facts.Heroes[0].Items) != 0 || len(facts.Heroes[0].Abilities) != 0 || facts.Heroes[0].NetWorth.Present {
		t.Fatalf("stale generation attached: %+v", facts.Heroes)
	}
	if len(facts.Objectives.Transients) != 1 || facts.Objectives.Transients[0].EntityID != item.Entity {
		t.Fatalf("unattributed item lost: %+v", facts.Objectives.Transients)
	}
}
