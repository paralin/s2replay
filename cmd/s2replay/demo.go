package main

import (
	"context"
	"os"

	"github.com/paralin/s2replay"
)

// withParser opens the demo at path, streams it through a parser, and passes
// the parser to fn, closing the file when fn returns. The parser reads the file
// one command at a time, so memory does not grow with the demo's size.
func withParser(path string, fn func(*s2replay.Parser) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	p, err := s2replay.NewParserFromReader(context.Background(), f)
	if err != nil {
		return err
	}
	return fn(p)
}
