// Command runback-facts extracts the replay state at one exact tick, with
// parser provenance, and writes it as JSON on stdout.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/paralin/s2replay/analysis"
)

// main parses the command-line flags and extracts the facts for the requested
// tick.
func main() {
	// Parse the command-line flags.
	demoPath := flag.String("demo", "", "Deadlock replay file")
	tick := flag.Uint("tick", 0, "Exact replay tick to reconstruct")
	flag.Parse()

	// Validate the arguments and reject ticks beyond the uint32 range.
	if *demoPath == "" || *tick == 0 || uint64(*tick) > uint64(^uint32(0)) || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: runback-facts -demo replay.dem -tick 63280 > facts.json")
		os.Exit(2)
	}

	// Extract the facts for the tick and write them to stdout.
	if err := extract(*demoPath, uint32(*tick)); err != nil {
		fmt.Fprintln(os.Stderr, "runback-facts:", err)
		os.Exit(1)
	}
}

// extract reads the replay file, extracts the runback facts for tick, and
// writes them as JSON to stdout.
func extract(path string, tick uint32) error {
	demo, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read replay: %w", err)
	}
	facts, err := analysis.ExtractRunbackFacts(demo, analysis.RunbackRequest{Tick: tick})
	if err != nil {
		return fmt.Errorf("extract tick %d: %w", tick, err)
	}
	return json.NewEncoder(os.Stdout).Encode(facts)
}
