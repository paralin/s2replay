package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"flag"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/paralin/s2replay"
	"github.com/paralin/s2replay/analysis"
)

// runAnalyze parses analyze flags and streams the replay analysis to
// standard output.
func runAnalyze(args []string) error {
	// Define the command's flags and suppress the standard flag package output.
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "jsonl", "analysis output format: jsonl or pb")
	combatGap := fs.Float64("combat-gap", -1, "maximum seconds between selected combat events; negative disables windows")
	combatEvents := fs.String("combat-events", "", "comma-separated event types to include in combat windows")

	// Parse the command line before validating its requested output.
	if err := fs.Parse(args); err != nil {
		return analyzeUsageError{}
	}
	if *format != "jsonl" && *format != "pb" {
		return errors.New("unsupported analyze format " + strconvQuote(*format))
	}
	if fs.NArg() != 1 {
		return analyzeUsageError{}
	}

	// Resolve the optional combat windows before reading the replay.
	var opts analysis.AnalyzeOptions
	if *combatGap >= 0 {
		include, err := combatEventFilter(*combatEvents)
		if err != nil {
			return err
		}
		opts.CombatWindows = &analysis.CombatWindowOptions{MaxGap: *combatGap, Include: include}
	}
	return analyzeStream(fs.Arg(0), os.Stdout, *format, opts)
}

// analyzeStream parses one replay, reading it from disk as it parses, and
// writes its analysis records.
func analyzeStream(path string, out io.Writer, format string, opts analysis.AnalyzeOptions) error {
	return withParser(path, func(p *s2replay.Parser) error {
		return writeAnalysis(p, out, format, opts)
	})
}

// writeAnalysis writes each analysis record as it closes: jsonl writes one
// JSON object per line, and pb writes length-delimited AnalysisRecord frames,
// one varint byte count followed by that many message bytes.
func writeAnalysis(src analysis.EventSource, out io.Writer, format string, opts analysis.AnalyzeOptions) error {
	w := bufio.NewWriterSize(out, 512*1024)
	var frame []byte
	err := analysis.Analyze(src, opts, func(rec *analysis.AnalysisRecord) error {
		if format == "pb" {
			size := rec.SizeVT()
			frame = binary.AppendUvarint(frame[:0], uint64(size))
			n := len(frame)
			frame = slices.Grow(frame, size)[:n+size]
			if _, err := rec.MarshalToSizedBufferVT(frame[n:]); err != nil {
				return err
			}
			_, err := w.Write(frame)
			return err
		}
		line, err := rec.MarshalJSON()
		if err != nil {
			return err
		}
		if _, err := w.Write(line); err != nil {
			return err
		}
		return w.WriteByte('\n')
	})
	if err != nil {
		return err
	}
	return w.Flush()
}

// combatEventFilter builds an event predicate from the supported filter names.
func combatEventFilter(filter string) (func(s2replay.Event) bool, error) {
	// Normalize the caller's filter before interpreting its event names.
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil, nil
	}

	// Validate and collect each requested event type.
	allowed := map[s2replay.EventType]bool{}
	for raw := range strings.SplitSeq(filter, ",") {
		name := strings.TrimSpace(raw)
		switch s2replay.EventType(name) {
		case s2replay.EventDamage, s2replay.EventModifier, s2replay.EventPurchase, s2replay.EventEntitySample:
			allowed[s2replay.EventType(name)] = true
		default:
			return nil, errors.New("unsupported combat event type " + strconvQuote(name))
		}
	}

	// Return a predicate that accepts only the requested event types.
	return func(ev s2replay.Event) bool {
		return allowed[ev.Type]
	}, nil
}

// strconvQuote quotes a string for inclusion in an error message.
func strconvQuote(s string) string {
	return strconv.Quote(s)
}

// analyzeUsageError reports invalid analyze command arguments.
type analyzeUsageError struct{}

// Error returns the analyze command usage message.
func (analyzeUsageError) Error() string {
	return "usage: s2replay analyze [--format jsonl|pb] [--combat-gap seconds] [--combat-events damage,modifier] <demo.dem>"
}
