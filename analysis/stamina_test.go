package analysis

import (
	"testing"

	"github.com/paralin/s2replay"
)

// TestRunbackStaminaLatchKeepsZeroAndAbsenceDistinct checks resource provenance in facts.
func TestRunbackStaminaLatchKeepsZeroAndAbsenceDistinct(t *testing.T) {
	// Supply one observed zero and one missing latch on a real pawn projection.
	pawn := runbackPawn(100, 92, 1)
	pawn.HasStaminaLatchValue = true
	pawn.StaminaLatchValueTick = 80
	facts, err := buildRunbackFacts([]s2replay.EntitySample{pawn}, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hero := facts.Heroes[0]
	if !hero.StaminaLatchValue.Present || hero.StaminaLatchValue.Value != 0 {
		t.Fatal("observed zero lost")
	}
	if hero.StaminaLatchValue.SourceTick != 80 || hero.StaminaLatchValue.FreshnessTicks != 20 {
		t.Fatal("latch provenance lost")
	}
	if hero.StaminaLatchTime.Present || hero.StaminaLatchTime.MissingReason != RunbackMissingNotInSample {
		t.Fatal("absent latch time fabricated")
	}
}
