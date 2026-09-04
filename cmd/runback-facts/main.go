// Command runback-facts extracts a timestamp's replay state with parser provenance.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/paralin/s2replay/analysis"
)

func main() {
	demoPath := flag.String("demo", "", "Deadlock replay file")
	tick := flag.Uint("tick", 0, "Exact replay tick to reconstruct")
	flag.Parse()
	if *demoPath == "" || *tick == 0 || uint64(*tick) > uint64(^uint32(0)) || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: runback-facts -demo replay.dem -tick 63280 > facts.json")
		os.Exit(2)
	}
	if err := extract(*demoPath, uint32(*tick)); err != nil {
		fmt.Fprintln(os.Stderr, "runback-facts:", err)
		os.Exit(1)
	}
}

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
