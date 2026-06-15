package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/paralin/s2replay"
)

func runEmit(args []string) error {
	fs := flag.NewFlagSet("emit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "jsonl", "event output format")
	if err := fs.Parse(args); err != nil {
		return emitUsageError{}
	}
	if *format != "jsonl" {
		return fmt.Errorf("unsupported emit format %q", *format)
	}
	if fs.NArg() != 1 {
		return emitUsageError{}
	}
	return emitJSONL(fs.Arg(0), os.Stdout)
}

func emitJSONL(path string, out io.Writer) error {
	demo, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p, err := s2replay.NewParser(demo)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	for {
		ev, err := p.NextEvent()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := enc.Encode(ev); err != nil {
			return err
		}
	}
}

type emitUsageError struct{}

func (emitUsageError) Error() string {
	return "usage: s2replay emit --format jsonl <demo.dem>"
}
