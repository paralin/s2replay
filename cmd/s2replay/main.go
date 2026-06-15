// Command s2replay parses Source 2 / Deadlock replays.
package main

import (
	"fmt"
	"os"

	"github.com/paralin/s2replay"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(s2replay.Version)
		return
	}

	fmt.Fprintf(os.Stderr, "s2replay %s\n", s2replay.Version)
	fmt.Fprintln(os.Stderr, "usage: s2replay version")
	os.Exit(2)
}
