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

const analysisSchemaVersion = 2

type analysisOutput struct {
	SchemaVersion int                     `json:"schema_version"`
	Analysis      analysis.Result         `json:"analysis"`
	CombatWindows []analysis.CombatWindow `json:"combat_windows,omitempty"`
}

func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "json", "analysis output format")
	combatGap := fs.Float64("combat-gap", -1, "maximum seconds between selected combat events; negative disables windows")
	combatEvents := fs.String("combat-events", "", "comma-separated event types to include in combat windows")
	if err := fs.Parse(args); err != nil {
		return analyzeUsageError{}
	}
	if *format != "json" {
		return errors.New("unsupported analyze format " + strconvQuote(*format))
	}
	if fs.NArg() != 1 {
		return analyzeUsageError{}
	}
	return analyzeJSON(fs.Arg(0), os.Stdout, *combatGap, *combatEvents)
}

func analyzeJSON(path string, out io.Writer, combatGap float64, combatEvents string) error {
	demo, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p, err := s2replay.NewParser(demo)
	if err != nil {
		return err
	}
	events, err := p.CollectEvents(0)
	if err != nil {
		return err
	}
	result, err := analysisOutputFromEvents(events, combatGap, combatEvents)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func analysisOutputFromEvents(events []s2replay.Event, combatGap float64, combatEvents string) (analysisOutput, error) {
	out := analysisOutput{
		SchemaVersion: analysisSchemaVersion,
		Analysis:      analysis.Build(events),
	}
	if combatGap < 0 {
		return out, nil
	}
	include, err := combatEventFilter(combatEvents)
	if err != nil {
		return analysisOutput{}, err
	}
	out.CombatWindows = analysis.BuildCombatWindows(events, analysis.CombatWindowOptions{
		MaxGap:  combatGap,
		Include: include,
	})
	return out, nil
}

func combatEventFilter(filter string) (func(s2replay.Event) bool, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil, nil
	}
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
	return func(ev s2replay.Event) bool {
		return allowed[ev.Type]
	}, nil
}

func strconvQuote(s string) string {
	return strconv.Quote(s)
}

type analyzeUsageError struct{}

func (analyzeUsageError) Error() string {
	return "usage: s2replay analyze --format json [--combat-gap seconds] [--combat-events damage,modifier] <demo.dem>"
}
