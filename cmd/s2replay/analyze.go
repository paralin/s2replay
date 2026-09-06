package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/paralin/s2replay"
	"github.com/paralin/s2replay/analysis"
)

// analysisSchemaVersion identifies the JSON shape emitted by the analyze command.
const analysisSchemaVersion = 2

// analysisOutput is the JSON document emitted by the analyze command.
type analysisOutput struct {
	// SchemaVersion identifies the JSON shape used by this document.
	SchemaVersion int `json:"schema_version"`
	// Analysis contains the replay-wide analysis results.
	Analysis analysis.Result `json:"analysis"`
	// CombatWindows contains selected combat event windows when requested.
	CombatWindows []analysis.CombatWindow `json:"combat_windows,omitempty"`
}

// runAnalyze parses analyze flags and writes the requested replay analysis.
func runAnalyze(args []string) error {
	// Define the command's flags and suppress the standard flag package output.
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "json", "analysis output format")
	combatGap := fs.Float64("combat-gap", -1, "maximum seconds between selected combat events; negative disables windows")
	combatEvents := fs.String("combat-events", "", "comma-separated event types to include in combat windows")

	// Parse the command line before validating its requested output.
	if err := fs.Parse(args); err != nil {
		return analyzeUsageError{}
	}

	// Reject output formats that this command cannot encode.
	if *format != "json" {
		return errors.New("unsupported analyze format " + strconvQuote(*format))
	}

	// Require exactly one replay input path.
	if fs.NArg() != 1 {
		return analyzeUsageError{}
	}

	// Run the analysis and write its JSON representation to standard output.
	return analyzeJSON(fs.Arg(0), os.Stdout, *combatGap, *combatEvents)
}

// analyzeJSON parses one replay and writes its analysis as indented JSON.
func analyzeJSON(path string, out io.Writer, combatGap float64, combatEvents string) error {
	// Read and parse the replay input.
	demo, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p, err := s2replay.NewParser(demo)
	if err != nil {
		return err
	}

	// Collect replay events for the analysis builders.
	events, err := p.CollectEvents(0)
	if err != nil {
		return err
	}

	// Build the requested analysis output.
	result, err := analysisOutputFromEvents(events, combatGap, combatEvents)
	if err != nil {
		return err
	}

	// Encode the output with stable indentation for command consumers.
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// analysisOutputFromEvents builds replay analysis and optional combat windows.
func analysisOutputFromEvents(events []s2replay.Event, combatGap float64, combatEvents string) (analysisOutput, error) {
	// Build the replay-wide analysis that is always present.
	out := analysisOutput{
		SchemaVersion: analysisSchemaVersion,
		Analysis:      analysis.Build(events),
	}

	// Omit combat windows when the caller leaves them disabled.
	if combatGap < 0 {
		return out, nil
	}

	// Resolve the requested event filter before building combat windows.
	include, err := combatEventFilter(combatEvents)
	if err != nil {
		return analysisOutput{}, err
	}

	// Build combat windows from the selected events.
	out.CombatWindows = analysis.BuildCombatWindows(events, analysis.CombatWindowOptions{
		MaxGap:  combatGap,
		Include: include,
	})
	return out, nil
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
	return "usage: s2replay analyze --format json [--combat-gap seconds] [--combat-events damage,modifier] <demo.dem>"
}
