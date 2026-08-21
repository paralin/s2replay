package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paralin/s2replay"
)

func TestReplayAnalysisSmoke(t *testing.T) {
	demoPath := os.Getenv("S2REPLAY_TEST_DEM")
	if demoPath == "" {
		demoPath = filepath.Join(os.Getenv("HOME"), "repos/deadlock-replays/48345595.dem")
	}
	if _, err := os.Stat(demoPath); err != nil {
		t.Skipf("set S2REPLAY_TEST_DEM to a Deadlock .dem to run replay analysis smoke: %v", err)
	}

	demo, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s2replay.NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}
	events, err := p.CollectEvents(0)
	if err != nil {
		t.Fatal(err)
	}
	result := Build(events)
	if result.Quality.InventoryTransitions == 0 {
		t.Fatalf("inventory timeline has no transitions: quality=%+v events=%d", result.Quality, len(events))
	}
	if result.Quality.EntitySamples == 0 {
		t.Fatalf("entity timeline has no samples: quality=%+v events=%d", result.Quality, len(events))
	}
	if len(result.Modifiers.Modifiers) == 0 {
		t.Fatalf("modifier timeline has no intervals: quality=%+v events=%d", result.Quality, len(events))
	}
	if result.Quality.Events != len(events) {
		t.Fatalf("quality event count: want %d, got %d", len(events), result.Quality.Events)
	}
	t.Logf(
		"analysis smoke: events=%d inventory_transitions=%d entity_samples=%d modifier_intervals=%d open_modifiers=%d unmatched_removes=%d",
		len(events),
		result.Quality.InventoryTransitions,
		result.Quality.EntitySamples,
		len(result.Modifiers.Modifiers),
		result.Quality.OpenModifierIntervals,
		result.Quality.UnmatchedModifierRemoves,
	)
}
