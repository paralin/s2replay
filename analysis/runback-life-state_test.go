package analysis

import (
	"testing"

	"github.com/paralin/s2replay"
)

// TestRunbackWorldLifeStateOverridesPositiveHealth prevents reviving dead transport remnants.
func TestRunbackWorldLifeStateOverridesPositiveHealth(t *testing.T) {
	for _, state := range []uint32{0, 1, 2} {
		// Keep health positive while varying the independently recorded lifecycle.
		sample := runbackSample(100, 7, 3, "CNPC_Trooper", -1)
		sample.Health = 1
		sample.LifeState = state
		sample.HasLifeState = true
		sample.LifeStateTick = 80
		facts, err := buildRunbackFacts([]s2replay.EntitySample{sample}, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
		if err != nil {
			t.Fatal(err)
		}

		// Preserve the world row and health while publishing the native alive verdict.
		if len(facts.WorldEntities) != 1 {
			t.Fatalf("world rows: got %d, want 1", len(facts.WorldEntities))
		}
		row := facts.WorldEntities[0]
		if row.Health.Value != 1 {
			t.Fatalf("health changed: %+v", row.Health)
		}
		want := RunbackAlive{Alive: state == 0, Basis: RunbackAliveLifeState, SourceTick: 80, FreshnessTicks: 20}
		if row.Alive != want {
			t.Fatalf("life state %d: got %+v, want %+v", state, row.Alive, want)
		}
	}
}

// TestRunbackLifeStateAbsentPreservesFallback keeps health and activity evidence usable.
func TestRunbackLifeStateAbsentPreservesFallback(t *testing.T) {
	for _, sample := range []s2replay.EntitySample{
		{HasHealth: true, Health: 1, HealthTick: 80},
		{HasHealth: true, Health: 0, HealthTick: 80},
		{},
	} {
		want := RunbackAlive{Alive: true, Basis: RunbackAliveActive, SourceTick: 100}
		if sample.HasHealth {
			want = RunbackAlive{Alive: sample.Health > 0, Basis: RunbackAliveHealthPositive, SourceTick: 80, FreshnessTicks: 20}
		}
		if got := runbackAlive(&sample, 100); got != want {
			t.Fatalf("fallback: got %+v, want %+v", got, want)
		}
	}
}
