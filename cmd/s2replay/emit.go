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
	format := fs.String("format", "jsonl", "event output format: jsonl or pb")
	if err := fs.Parse(args); err != nil {
		return emitUsageError{}
	}
	switch *format {
	case "jsonl":
		if fs.NArg() != 1 {
			return emitUsageError{}
		}
		return emitJSONL(fs.Arg(0), os.Stdout)
	case "pb":
		if fs.NArg() != 1 {
			return emitUsageError{}
		}
		return emitPB(fs.Arg(0), os.Stdout)
	default:
		return fmt.Errorf("unsupported emit format %q", *format)
	}
}

// emitPB writes the event stream as length-delimited ReplayEvent protobuf
// frames: one varint byte count followed by that many message bytes.
func emitPB(path string, out io.Writer) error {
	demo, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p, err := s2replay.NewParser(demo)
	if err != nil {
		return err
	}
	buf := make([]byte, 0, 64*1024)
	for {
		ev, err := p.NextEvent()
		if errors.Is(err, io.EOF) {
			_, err = out.Write(buf)
			return err
		}
		if err != nil {
			return err
		}
		buf, err = ev.EmitProto(buf)
		if err != nil {
			return err
		}
		if len(buf) >= 512*1024 {
			if _, err := out.Write(buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
	}
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
	return "usage: s2replay emit [--format jsonl|pb] <demo.dem>"
}
