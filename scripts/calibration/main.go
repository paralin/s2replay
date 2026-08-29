// Command calibration extracts a pro-calibration movement dataset from
// s2replay analyze JSON dumps.
//
// Usage:
//
//	go run ./scripts/calibration --outdir DIR [--downsample N] <analysis.json>...
//
// Each input is the whole-file JSON produced by
//
//	s2replay analyze --format json <demo.dem>
//
// Inputs run several gigabytes, so the tool never materializes them: it walks
// the document with an encoding/json token stream and decodes one entity
// sample at a time out of .analysis.entities.players. Peak RSS stays at a few
// tens of megabytes regardless of input size.
//
// Per player it emits downsampled feature rows (default every 4th tick,
// ~16 Hz at Deadlock's 64 tick rate) to <outdir>/<demo-id>-features.jsonl:
//
//	{"demo":...,"player_slot":...,"tick":...,"game_time":...,
//	 "x":..,"y":..,"z":..,"vx":..,"vy":..,"vz":..,"speed":..,
//	 "grounded":true|false}
//
// Velocity is a backward finite difference between consecutive raw samples;
// time deltas come from game_time, not an assumed tick rate. The grounded
// flag is a heuristic proxy: true when the sample's altitude is within
// groundedTolerance units of the trailing-minimum altitude over the previous
// baselineWindow seconds of that player's own track.
//
// Alongside the rows it writes <outdir>/<demo-id>-summary.json with
// per-player top-speed distributions, jump counts (grounded -> airborne
// transitions), and a vertical-gain profile (cumulative upward movement per
// game minute).
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	downsampleDefault = 4    // emit every Nth tick (~16 Hz)
	baselineWindow    = 3.0  // seconds of history for the ground-level minimum
	groundedTolerance = 24.0 // units above the trailing minimum still "grounded"
)

type entitySample struct {
	Tick        uint32  `json:"tick"`
	GameTime    float64 `json:"game_time"`
	PlayerSlot  int32   `json:"player_slot"`
	PositionX   float32 `json:"position_x"`
	PositionY   float32 `json:"position_y"`
	PositionZ   float32 `json:"position_z"`
	HasPosition bool    `json:"has_position"`
}

type featureRow struct {
	Demo       string  `json:"demo"`
	PlayerSlot int32   `json:"player_slot"`
	Tick       uint32  `json:"tick"`
	GameTime   float64 `json:"game_time"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Z          float64 `json:"z"`
	VX         float64 `json:"vx"`
	VY         float64 `json:"vy"`
	VZ         float64 `json:"vz"`
	Speed      float64 `json:"speed"`
	Grounded   bool    `json:"grounded"`
}

type playerSummary struct {
	SamplesRaw       int                `json:"samples_raw"`
	SamplesNoPos     int                `json:"samples_no_position"`
	DuplicateTicks   int                `json:"duplicate_ticks_skipped"`
	RowsEmitted      int                `json:"rows_emitted"`
	Jumps            int                `json:"jumps"`
	MaxSpeed         float64            `json:"max_speed_units_per_s"`
	MaxAltitude      float64            `json:"max_altitude_above_ground_units"`
	SpeedPercentiles map[string]float64 `json:"speed_percentiles_units_per_s"`
	GainPerMinute    map[string]float64 `json:"vertical_gain_by_minute_units"`
}

type summary struct {
	Demo       string                    `json:"demo"`
	SourceFile string                    `json:"source_file"`
	Downsample int                       `json:"downsample_tick_interval"`
	Players    map[string]*playerSummary `json:"players"`
}

type zPoint struct {
	t float64
	z float64
}

// tracker accumulates per-player motion state while samples stream past.
type tracker struct {
	sum         playerSummary
	prevTick    uint32
	prevTime    float64
	prevX       float64
	prevY       float64
	prevZ       float64
	hasPrev     bool
	zHist       []zPoint // recent altitude history for the ground baseline
	wasGrounded bool
	speeds      []float64
}

func main() {
	outdir := flag.String("outdir", ".", "output directory")
	ds := flag.Int("downsample", downsampleDefault, "emit every Nth tick")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: calibration --outdir DIR [--downsample N] <analysis.json>...")
		os.Exit(2)
	}
	if err := os.MkdirAll(*outdir, 0o755); err != nil {
		fatal(err)
	}
	for _, path := range flag.Args() {
		if err := process(path, *outdir, *ds); err != nil {
			fatal(fmt.Errorf("%s: %w", path, err))
		}
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "calibration: %v\n", err)
	os.Exit(1)
}

func demoID(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func expect(dec *json.Decoder, delim rune) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := tok.(json.Delim)
	if !ok || rune(d) != delim {
		return fmt.Errorf("expected %q, got %v", delim, tok)
	}
	return nil
}

func skipValue(dec *json.Decoder) error {
	var v json.RawMessage
	return dec.Decode(&v)
}

func process(path, outdir string, ds int) error {
	id := demoID(path)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	rowsPath := filepath.Join(outdir, id+"-features.jsonl")
	rowsF, err := os.Create(rowsPath)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(rowsF, 1<<20)
	enc := json.NewEncoder(w)
	sum := summary{
		Demo: id, SourceFile: path, Downsample: ds,
		Players: map[string]*playerSummary{},
	}

	trackers := map[string]*tracker{}
	err = walkDocument(json.NewDecoder(bufio.NewReaderSize(f, 1<<20)),
		id, ds, enc, trackers)
	sum.Players = make(map[string]*playerSummary, len(trackers))
	for slot, tr := range trackers {
		sum.Players[slot] = &tr.sum
	}

	flushErr := w.Flush()
	closeErr := rowsF.Close()
	if err != nil {
		return err
	}
	if flushErr != nil {
		return flushErr
	}
	if closeErr != nil {
		return closeErr
	}
	finalize(trackers)
	sb, err := json.MarshalIndent(&sum, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outdir, id+"-summary.json"), sb, 0o644)
}

// walkDocument descends root -> analysis -> entities -> players, decoding
// every sample array. It stops once players is exhausted; modifiers, quality,
// and combat_windows are skipped without decoding.
func walkDocument(dec *json.Decoder, id string, ds int, enc *json.Encoder,
	trackers map[string]*tracker,
) error {
	if err := expect(dec, '{'); err != nil {
		return err
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		if keyTok.(string) == "analysis" {
			return walkAnalysis(dec, id, ds, enc, trackers)
		}
		if err := skipValue(dec); err != nil {
			return err
		}
	}
	return fmt.Errorf("no analysis object found")
}

func walkAnalysis(dec *json.Decoder, id string, ds int, enc *json.Encoder,
	trackers map[string]*tracker,
) error {
	if err := expect(dec, '{'); err != nil {
		return err
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		if keyTok.(string) == "entities" {
			return walkEntities(dec, id, ds, enc, trackers)
		}
		if err := skipValue(dec); err != nil {
			return err
		}
	}
	return fmt.Errorf("no entities object found")
}

func walkEntities(dec *json.Decoder, id string, ds int, enc *json.Encoder,
	trackers map[string]*tracker,
) error {
	if err := expect(dec, '{'); err != nil {
		return err
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		if keyTok.(string) == "players" {
			return walkPlayers(dec, id, ds, enc, trackers)
		}
		if err := skipValue(dec); err != nil {
			return err
		}
	}
	return fmt.Errorf("no players object found")
}

func walkPlayers(dec *json.Decoder, id string, ds int, enc *json.Encoder,
	trackers map[string]*tracker,
) error {
	if err := expect(dec, '{'); err != nil {
		return err
	}
	for dec.More() {
		slotTok, err := dec.Token()
		if err != nil {
			return err
		}
		slot := slotTok.(string)
		tr := &tracker{}
		if err := expect(dec, '['); err != nil {
			return err
		}
		for dec.More() {
			var s entitySample
			if err := dec.Decode(&s); err != nil {
				return fmt.Errorf("player %s sample: %w", slot, err)
			}
			tr.observe(s, id, slot, ds, enc)
		}
		if err := expect(dec, ']'); err != nil {
			return err
		}
		trackers[slot] = tr
	}
	return expect(dec, '}')
}

func (tr *tracker) observe(s entitySample, id, slot string, ds int, enc *json.Encoder) {
	tr.sum.SamplesRaw++
	if !s.HasPosition {
		tr.sum.SamplesNoPos++
		return
	}
	if tr.hasPrev && s.Tick == tr.prevTick {
		tr.sum.DuplicateTicks++
		return // duplicate snapshot at the same tick; keep the first
	}

	x, y, z := float64(s.PositionX), float64(s.PositionY), float64(s.PositionZ)

	var vx, vy, vz, speed float64
	dt := s.GameTime - tr.prevTime
	if tr.hasPrev && dt > 1e-6 {
		vx = (x - tr.prevX) / dt
		vy = (y - tr.prevY) / dt
		vz = (z - tr.prevZ) / dt
		speed = math.Hypot(vx, vy)
	} else if tr.hasPrev {
		tr.hasPrev = false // degenerate delta; re-anchor on the next sample
	}

	// Grounded proxy: within tolerance of the trailing-minimum altitude.
	tr.zHist = append(tr.zHist, zPoint{t: s.GameTime, z: z})
	cut := s.GameTime - baselineWindow
	i := 0
	for i < len(tr.zHist)-1 && tr.zHist[i].t < cut {
		i++
	}
	tr.zHist = tr.zHist[i:]
	minZ := tr.zHist[0].z
	for _, zp := range tr.zHist {
		if zp.z < minZ {
			minZ = zp.z
		}
	}
	grounded := z-minZ <= groundedTolerance
	if tr.hasPrev && tr.wasGrounded && !grounded {
		tr.sum.Jumps++
	}
	tr.wasGrounded = grounded

	minute := strconv.Itoa(int(s.GameTime / 60))
	if tr.sum.GainPerMinute == nil {
		tr.sum.GainPerMinute = map[string]float64{}
	}
	if vz > 0 {
		tr.sum.GainPerMinute[minute] += vz * dt
	}
	if alt := z - minZ; alt > tr.sum.MaxAltitude {
		tr.sum.MaxAltitude = alt
	}

	tr.prevTick, tr.prevTime, tr.prevX, tr.prevY, tr.prevZ = s.Tick, s.GameTime, x, y, z
	tr.hasPrev = true

	if speed > tr.sum.MaxSpeed {
		tr.sum.MaxSpeed = speed
	}
	if s.Tick%uint32(ds) == 0 {
		row := featureRow{
			Demo: id, PlayerSlot: s.PlayerSlot, Tick: s.Tick, GameTime: s.GameTime,
			X: x, Y: y, Z: z, VX: vx, VY: vy, VZ: vz, Speed: speed, Grounded: grounded,
		}
		if err := enc.Encode(&row); err != nil {
			fatal(err)
		}
		tr.sum.RowsEmitted++
		tr.speeds = append(tr.speeds, speed)
	}
}

func finalize(trackers map[string]*tracker) {
	for _, tr := range trackers {
		ps := &tr.sum
		sort.Float64s(tr.speeds)
		ps.SpeedPercentiles = quantiles(tr.speeds)
		tr.speeds = nil
		if ps.GainPerMinute == nil {
			ps.GainPerMinute = map[string]float64{}
		}
	}
}

func quantiles(sorted []float64) map[string]float64 {
	out := map[string]float64{}
	if len(sorted) == 0 {
		return out
	}
	pick := func(q float64) float64 {
		idx := max(int(math.Ceil(q*float64(len(sorted))))-1, 0)
		return sorted[idx]
	}
	out["p50"] = pick(0.50)
	out["p90"] = pick(0.90)
	out["p99"] = pick(0.99)
	out["max"] = sorted[len(sorted)-1]
	return out
}
