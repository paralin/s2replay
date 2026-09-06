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
	// downsampleDefault emits one row every fourth tick, or about 16 Hz.
	downsampleDefault = 4
	// baselineWindow is the history duration used for the ground-level minimum.
	baselineWindow = 3.0
	// groundedTolerance is the altitude above the trailing minimum still treated as grounded.
	groundedTolerance = 24.0
)

// entitySample is one decoded player position sample from an analysis dump.
type entitySample struct {
	// Tick is the replay tick containing this sample.
	Tick uint32 `json:"tick"`
	// GameTime is the in-game timestamp for this sample.
	GameTime float64 `json:"game_time"`
	// PlayerSlot identifies the player represented by this sample.
	PlayerSlot int32 `json:"player_slot"`
	// PositionX is the player's x coordinate when HasPosition is true.
	PositionX float32 `json:"position_x"`
	// PositionY is the player's y coordinate when HasPosition is true.
	PositionY float32 `json:"position_y"`
	// PositionZ is the player's z coordinate when HasPosition is true.
	PositionZ float32 `json:"position_z"`
	// HasPosition reports whether the position fields contain an observation.
	HasPosition bool `json:"has_position"`
}

// featureRow is one downsampled movement feature emitted for a player.
type featureRow struct {
	// Demo identifies the replay that produced this row.
	Demo string `json:"demo"`
	// PlayerSlot identifies the player represented by this row.
	PlayerSlot int32 `json:"player_slot"`
	// Tick is the replay tick containing this row.
	Tick uint32 `json:"tick"`
	// GameTime is the in-game timestamp for this row.
	GameTime float64 `json:"game_time"`
	// X is the player's x coordinate.
	X float64 `json:"x"`
	// Y is the player's y coordinate.
	Y float64 `json:"y"`
	// Z is the player's z coordinate.
	Z float64 `json:"z"`
	// VX is the player's x velocity.
	VX float64 `json:"vx"`
	// VY is the player's y velocity.
	VY float64 `json:"vy"`
	// VZ is the player's z velocity.
	VZ float64 `json:"vz"`
	// Speed is the player's horizontal speed.
	Speed float64 `json:"speed"`
	// Grounded is the trailing-minimum altitude heuristic result.
	Grounded bool `json:"grounded"`
}

// playerSummary contains aggregate movement statistics for one player.
type playerSummary struct {
	// SamplesRaw counts all decoded samples for the player.
	SamplesRaw int `json:"samples_raw"`
	// SamplesNoPos counts decoded samples without a position.
	SamplesNoPos int `json:"samples_no_position"`
	// DuplicateTicks counts samples skipped because their tick was already seen.
	DuplicateTicks int `json:"duplicate_ticks_skipped"`
	// RowsEmitted counts feature rows written for the player.
	RowsEmitted int `json:"rows_emitted"`
	// Jumps counts grounded-to-airborne transitions.
	Jumps int `json:"jumps"`
	// MaxSpeed is the greatest horizontal speed observed.
	MaxSpeed float64 `json:"max_speed_units_per_s"`
	// MaxAltitude is the greatest altitude above the trailing ground baseline.
	MaxAltitude float64 `json:"max_altitude_above_ground_units"`
	// SpeedPercentiles contains selected speed percentile values.
	SpeedPercentiles map[string]float64 `json:"speed_percentiles_units_per_s"`
	// GainPerMinute contains cumulative upward movement grouped by game minute.
	GainPerMinute map[string]float64 `json:"vertical_gain_by_minute_units"`
}

// summary is the aggregate JSON document written beside feature rows.
type summary struct {
	// Demo identifies the replay that produced this summary.
	Demo string `json:"demo"`
	// SourceFile is the path of the input analysis dump.
	SourceFile string `json:"source_file"`
	// Downsample is the tick interval used for emitted feature rows.
	Downsample int `json:"downsample_tick_interval"`
	// Players maps player slots to their aggregate movement statistics.
	Players map[string]*playerSummary `json:"players"`
}

// zPoint records an altitude sample used to compute the trailing baseline.
type zPoint struct {
	// t is the game time of the altitude sample.
	t float64
	// z is the altitude at the sample time.
	z float64
}

// tracker accumulates per-player motion state while samples stream past.
type tracker struct {
	// sum contains aggregate statistics for the tracked player.
	sum playerSummary
	// prevTick is the most recent accepted sample tick.
	prevTick uint32
	// prevTime is the most recent accepted sample game time.
	prevTime float64
	// prevX is the most recent accepted x coordinate.
	prevX float64
	// prevY is the most recent accepted y coordinate.
	prevY float64
	// prevZ is the most recent accepted z coordinate.
	prevZ float64
	// hasPrev reports whether the previous sample can provide a finite difference.
	hasPrev bool
	// zHist contains recent altitude history for the ground baseline.
	zHist []zPoint
	// wasGrounded is the previous sample's grounded heuristic result.
	wasGrounded bool
	// speeds contains emitted horizontal speeds for percentile calculation.
	speeds []float64
}

// main parses calibration flags and processes each analysis dump.
func main() {
	// Parse the output directory and downsampling interval.
	outdir := flag.String("outdir", ".", "output directory")
	ds := flag.Int("downsample", downsampleDefault, "emit every Nth tick")
	flag.Parse()

	// Require at least one input analysis dump.
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: calibration --outdir DIR [--downsample N] <analysis.json>...")
		os.Exit(2)
	}

	// Create the output directory before processing inputs.
	if err := os.MkdirAll(*outdir, 0o755); err != nil {
		fatal(err)
	}

	// Process each input dump into feature rows and a summary.
	for _, path := range flag.Args() {
		if err := process(path, *outdir, *ds); err != nil {
			fatal(fmt.Errorf("%s: %w", path, err))
		}
	}
}

// fatal reports an unrecoverable command error and exits unsuccessfully.
func fatal(err error) {
	fmt.Fprintf(os.Stderr, "calibration: %v\n", err)
	os.Exit(1)
}

// demoID returns the input filename without its extension.
func demoID(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// expect consumes and validates one JSON delimiter.
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

// skipValue consumes one JSON value without decoding its contents.
func skipValue(dec *json.Decoder) error {
	var v json.RawMessage
	return dec.Decode(&v)
}

// process streams one analysis dump into feature rows and an aggregate summary.
func process(path, outdir string, ds int) error {
	// Open the input analysis dump.
	id := demoID(path)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	// Create the feature-row output and its streaming encoder.
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

	// Stream samples into per-player trackers.
	trackers := map[string]*tracker{}
	err = walkDocument(json.NewDecoder(bufio.NewReaderSize(f, 1<<20)),
		id, ds, enc, trackers)

	// Materialize tracker summaries for the aggregate output.
	sum.Players = make(map[string]*playerSummary, len(trackers))
	for slot, tr := range trackers {
		sum.Players[slot] = &tr.sum
	}

	// Flush and close feature-row output before writing the summary.
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

	// Finalize percentile and vertical-gain fields.
	finalize(trackers)
	sb, err := json.MarshalIndent(&sum, "", "  ")
	if err != nil {
		return err
	}

	// Write the aggregate summary beside the feature rows.
	return os.WriteFile(filepath.Join(outdir, id+"-summary.json"), sb, 0o644)
}

// walkDocument descends root -> analysis -> entities -> players, decoding
// every sample array. It stops once players is exhausted; modifiers, quality,
// and combat_windows are skipped without decoding.
func walkDocument(dec *json.Decoder, id string, ds int, enc *json.Encoder,
	trackers map[string]*tracker,
) error {
	// Enter the root JSON object.
	if err := expect(dec, '{'); err != nil {
		return err
	}

	// Find the analysis object while skipping unrelated top-level fields.
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

// walkAnalysis descends through the analysis object to its entities field.
func walkAnalysis(dec *json.Decoder, id string, ds int, enc *json.Encoder,
	trackers map[string]*tracker,
) error {
	// Enter the analysis object.
	if err := expect(dec, '{'); err != nil {
		return err
	}

	// Find the entities object while skipping unrelated analysis fields.
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

// walkEntities descends through the entities object to its players field.
func walkEntities(dec *json.Decoder, id string, ds int, enc *json.Encoder,
	trackers map[string]*tracker,
) error {
	// Enter the entities object.
	if err := expect(dec, '{'); err != nil {
		return err
	}

	// Find the players object while skipping unrelated entity fields.
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

// walkPlayers decodes each player's sample array into a motion tracker.
func walkPlayers(dec *json.Decoder, id string, ds int, enc *json.Encoder,
	trackers map[string]*tracker,
) error {
	// Enter the players object.
	if err := expect(dec, '{'); err != nil {
		return err
	}

	// Decode each player's sample array and retain its tracker.
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

// observe updates motion estimates and emits a row at the downsampling interval.
func (tr *tracker) observe(s entitySample, id, slot string, ds int, enc *json.Encoder) {
	// Count the raw sample and ignore samples without positions.
	tr.sum.SamplesRaw++
	if !s.HasPosition {
		tr.sum.SamplesNoPos++
		return
	}

	// Keep the first sample for each tick and skip duplicate snapshots.
	if tr.hasPrev && s.Tick == tr.prevTick {
		tr.sum.DuplicateTicks++
		return // duplicate snapshot at the same tick; keep the first
	}

	// Convert the sample coordinates and compute a finite-difference velocity.
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

	// Compute grounded status from the trailing-minimum altitude.
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

	// Accumulate upward movement and the maximum altitude for this sample.
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

	// Make this sample the baseline for the next finite difference.
	tr.prevTick, tr.prevTime, tr.prevX, tr.prevY, tr.prevZ = s.Tick, s.GameTime, x, y, z
	tr.hasPrev = true

	// Track the maximum speed and emit rows at the requested interval.
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

// finalize computes percentile and empty-map defaults for each tracker.
func finalize(trackers map[string]*tracker) {
	// Sort each player's emitted speeds before calculating percentiles.
	for _, tr := range trackers {
		ps := &tr.sum
		sort.Float64s(tr.speeds)
		ps.SpeedPercentiles = quantiles(tr.speeds)
		tr.speeds = nil

		// Keep vertical-gain output present for players without upward movement.
		if ps.GainPerMinute == nil {
			ps.GainPerMinute = map[string]float64{}
		}
	}
}

// quantiles returns the selected percentile values from sorted speeds.
func quantiles(sorted []float64) map[string]float64 {
	// Return an empty result when no speeds were emitted.
	out := map[string]float64{}
	if len(sorted) == 0 {
		return out
	}

	// Select the nearest-rank value for each requested percentile.
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
