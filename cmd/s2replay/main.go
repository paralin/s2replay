// Command s2replay parses Source 2 / Deadlock replays.
package main

import (
	"fmt"
	"os"

	"github.com/paralin/s2replay"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version":
			fmt.Println(s2replay.Version)
			return
		case "parse":
			if len(os.Args) != 3 {
				fmt.Fprintln(os.Stderr, "usage: s2replay parse <demo.dem>")
				os.Exit(2)
			}
			if err := runParse(os.Args[2]); err != nil {
				fmt.Fprintf(os.Stderr, "s2replay: %v\n", err)
				os.Exit(1)
			}
			return
		case "emit":
			if err := runEmit(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "s2replay: %v\n", err)
				os.Exit(1)
			}
			if os.Getenv("K145_DEBUG") != "" {
				seen, found, heroes, classes, s5t, s5ids, s5cls := s2replay.Kind145DebugStats()
				fmt.Fprintf(os.Stderr, "k145 envelopes=%d entity-found=%d hero-samples=%d classes=%v\n",
					seen, found, heroes, classes)
				fmt.Fprintf(os.Stderr, "sub5 total=%d ids=%v classes=%v\n", s5t, s5ids, s5cls)
			}
			return
		case "analyze":
			if err := runAnalyze(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "s2replay: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	fmt.Fprintf(os.Stderr, "s2replay %s\n", s2replay.Version)
	fmt.Fprintln(os.Stderr, "usage: s2replay [version|parse <demo.dem>|emit --format jsonl <demo.dem>|analyze --format json <demo.dem>]")
	os.Exit(2)
}
