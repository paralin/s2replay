package s2replay

import (
	"testing"

	"github.com/paralin/s2replay/protocol"
)

// TestDeadlockCoordFromCell verifies the cell-to-world reconstruction across the
// pre- and post-change coordinate bounds.
func TestDeadlockCoordFromCell(t *testing.T) {
	for _, tc := range []struct {
		cell, vec, maxCoord, want float32
	}{
		{32, 0, deadlockDefaultMaxCoord, 0}, // old origin at cell 32
		{64, 0, 32768, 0},                   // new origin at cell 64
		{10, 512, 16384, -10752},            // build-10854 minimap boundary
		{42, 512, 32768, -10752},            // build-10932 minimap boundary
		{66, 320, 32768, 1344},
	} {
		if got := deadlockCoordFromCell(tc.cell, tc.vec, tc.maxCoord); got != tc.want {
			t.Fatalf("cell %v vec %v maxCoord %v: got %v want %v",
				tc.cell, tc.vec, tc.maxCoord, got, tc.want)
		}
	}
}

// TestApplyServerInfoReadsMaxCoord verifies the server-announced coordinate bound
// is read from GameSessionConfiguration, and the default is kept when absent.
func TestApplyServerInfoReadsMaxCoord(t *testing.T) {
	maxCoord := float32(32768)
	p := &Parser{clock: newClock()}
	p.applyServerInfo(&protocol.CSVCMsg_ServerInfo{
		GameSessionConfig: &protocol.CSVCMsg_GameSessionConfiguration{MaxCoord: &maxCoord},
	})
	if p.maxCoord != 32768 {
		t.Fatalf("announced maxCoord: got %v want 32768", p.maxCoord)
	}

	// A replay without an announced bound keeps the default.
	p2 := &Parser{clock: newClock(), maxCoord: deadlockDefaultMaxCoord}
	p2.applyServerInfo(&protocol.CSVCMsg_ServerInfo{})
	if p2.maxCoord != deadlockDefaultMaxCoord {
		t.Fatalf("default maxCoord: got %v want %v", p2.maxCoord, deadlockDefaultMaxCoord)
	}
}
