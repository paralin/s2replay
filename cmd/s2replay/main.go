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
		}
	}

	fmt.Fprintf(os.Stderr, "s2replay %s\n", s2replay.Version)
	fmt.Fprintln(os.Stderr, "usage: s2replay [version|parse <demo.dem>]")
	os.Exit(2)
}
