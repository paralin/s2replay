package analysis

import (
	"testing"

	"github.com/paralin/s2replay"
)

// TestRunbackCameraPreservesPartialEvidence keeps a missing axis distinct from zero.
func TestRunbackCameraPreservesPartialEvidence(t *testing.T) {
	// Build a pawn with one observed camera axis, one missing axis, and one observed zero.
	pawn := runbackPawn(100, 92, 1)
	pawn.CameraAngles = [3]float32{4.21875, 0, 0}
	pawn.CameraAnglesTicks = [3]uint32{95, 0, 96}
	pawn.HasCameraAngles = [3]bool{true, false, true}
	facts, err := buildRunbackFacts([]s2replay.EntitySample{pawn}, Result{}, ReplaySourceIdentity{}, RunbackRequest{Tick: 100}, RunbackTickProvenance{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that observed camera values retain their provenance.
	camera := facts.Heroes[0].CameraAngles
	if !camera[0].Present || camera[0].Value != 4.21875 || camera[0].SourceTick != 95 || camera[0].FreshnessTicks != 5 {
		t.Fatal("camera provenance lost")
	}
	if camera[1].Present || camera[1].MissingReason != "m_angClientCamera_not_present" {
		t.Fatal("missing yaw fabricated")
	}
	if !camera[2].Present || camera[2].Value != 0 || camera[2].SourceTick != 96 {
		t.Fatal("observed zero roll lost")
	}
}
