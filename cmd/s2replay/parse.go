package main

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/paralin/s2replay"
	"github.com/paralin/s2replay/protocol"
)

// runParse walks a demo container, prints its file header, and reports a
// monotonic tick / game-time stream. It is the Phase 2 container + clock proof.
func runParse(path string) error {
	demo, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p, err := s2replay.NewParser(demo)
	if err != nil {
		return err
	}

	counts := map[protocol.EDemoCommands]int{}
	var total int
	var lastTick uint32
	var maxTick uint32
	monotonic := true
	clock := p.Clock()

	for {
		cmd, err := p.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		total++
		counts[cmd.Kind]++

		if cmd.Kind == protocol.EDemoCommands_DEM_FileHeader {
			h := &protocol.CDemoFileHeader{}
			if err := h.UnmarshalVT(cmd.Payload); err != nil {
				return err
			}
			printHeader(h)
		}

		if cmd.Tick != s2replay.PreGameTick {
			if cmd.Tick < lastTick {
				monotonic = false
			}
			lastTick = cmd.Tick
			if cmd.Tick > maxTick {
				maxTick = cmd.Tick
			}
			if total%20000 == 0 {
				fmt.Printf("  command %d: tick %d  game_time %.2fs\n", total, clock.Tick(), clock.GameTime())
			}
		}
	}

	fmt.Printf("\ncommands: %d\n", total)
	fmt.Printf("last tick: %d  game_time: %.2fs  (tick_interval %.6fs)\n", maxTick, clock.GameTime(), clock.TickInterval())
	fmt.Printf("tick stream monotonic: %t\n", monotonic)
	printCounts(counts)
	return nil
}

// printHeader prints the human-readable demo file header fields.
func printHeader(h *protocol.CDemoFileHeader) {
	fmt.Println("demo file header:")
	fmt.Printf("  stamp:    %s\n", h.GetDemoFileStamp())
	fmt.Printf("  server:   %s\n", h.GetServerName())
	fmt.Printf("  map:      %s\n", h.GetMapName())
	fmt.Printf("  game_dir: %s\n", h.GetGameDirectory())
	fmt.Printf("  build:    %d\n", h.GetBuildNum())
	fmt.Printf("  addons:   %s\n", h.GetAddons())
}

// printCounts prints command-kind totals sorted by frequency.
func printCounts(counts map[protocol.EDemoCommands]int) {
	type row struct {
		kind protocol.EDemoCommands
		n    int
	}
	rows := make([]row, 0, len(counts))
	for k, n := range counts {
		rows = append(rows, row{k, n})
	}
	slices.SortFunc(rows, func(a, b row) int { return b.n - a.n })
	fmt.Println("command kinds:")
	for _, r := range rows {
		fmt.Printf("  %-28s %d\n", r.kind.String(), r.n)
	}
}
