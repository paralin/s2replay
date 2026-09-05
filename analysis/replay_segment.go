package analysis

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"reflect"
	"slices"

	"github.com/paralin/s2replay"
	"github.com/paralin/s2replay/protocol"
)

// ReplaySegmentEvidenceSchemaVersion identifies the replay segment contract.
const ReplaySegmentEvidenceSchemaVersion = 1

// MaxReplaySegmentRows bounds materialized evidence for one request.
const MaxReplaySegmentRows = 1_000_000

// MaxReplayParticipants is the engine player-slot boundary.
const MaxReplayParticipants = 64

// ErrMissingReplayHeader indicates that no DEM_FileHeader was found.
var ErrMissingReplayHeader = errors.New("replay segment requires DEM_FileHeader")

// ReplayFieldQuality describes whether a scalar came directly from the replay.
type ReplayFieldQuality string

const (
	// ReplayFieldExact marks a value decoded from a networked replay field.
	ReplayFieldExact ReplayFieldQuality = "exact"
	// ReplayFieldMissing marks a field absent from the source sample.
	ReplayFieldMissing ReplayFieldQuality = "missing"
)

// ReplaySegmentRequest selects an inclusive tick range and optional lead-in.
type ReplaySegmentRequest struct {
	StartTick         uint32                     `json:"start_tick"`
	EndTick           uint32                     `json:"end_tick"`
	LeadInTicks       uint32                     `json:"lead_in_ticks"`
	ParticipantSlots  []int32                    `json:"participant_slots,omitempty"`
	ExpectedIdentity  *ReplayIdentityExpectation `json:"expected_identity,omitempty"`
	MaxFreshnessTicks *uint32                    `json:"max_freshness_ticks,omitempty"`
}

// ReplayIdentityExpectation is the content-addressed replay identity expected by a consumer.
type ReplayIdentityExpectation struct {
	SHA256    string  `json:"sha256,omitempty"`
	MatchID   *uint64 `json:"match_id,omitempty"`
	Game      string  `json:"game,omitempty"`
	Map       string  `json:"map,omitempty"`
	GameBuild int32   `json:"game_build,omitempty"`
}

// ReplayCorrespondenceStatus reports whether observed and expected identities agree.
type ReplayCorrespondenceStatus string

const (
	ReplayCorrespondencePending    ReplayCorrespondenceStatus = "pending"
	ReplayCorrespondenceMatched    ReplayCorrespondenceStatus = "matched"
	ReplayCorrespondenceMismatched ReplayCorrespondenceStatus = "mismatched"
)

// ReplayIdentityCorrespondence records explicit observed-to-expected identity matching.
type ReplayIdentityCorrespondence struct {
	Expected ReplayIdentityExpectation  `json:"expected"`
	Status   ReplayCorrespondenceStatus `json:"status"`
	Reason   string                     `json:"reason,omitempty"`
}

// ReplaySourceIdentity is immutable identity for the bytes and parser build.
type ReplaySourceIdentity struct {
	SHA256               string `json:"sha256"`
	MatchID              uint64 `json:"match_id"`
	Game                 string `json:"game"`
	Map                  string `json:"map"`
	GameBuild            int32  `json:"game_build"`
	Parser               string `json:"parser"`
	ParserRevision       string `json:"parser_revision"`
	VCSRevision          string `json:"vcs_revision,omitempty"`
	MatchIDMissingReason string `json:"match_id_missing_reason,omitempty"`
}

// ReplaySegmentRange records the requested range and its exact inclusive span.
type ReplaySegmentRange struct {
	RequestedStartTick   uint32 `json:"requested_start_tick"`
	RequestedEndTick     uint32 `json:"requested_end_tick"`
	LeadInStartTick      uint32 `json:"lead_in_start_tick"`
	RequestedLeadInTicks uint32 `json:"requested_lead_in_ticks"`
	LeadInTicks          uint32 `json:"lead_in_ticks"`
	ExactStartTick       uint32 `json:"exact_start_tick"`
	ExactEndTick         uint32 `json:"exact_end_tick"`
}

// ReplayParticipant identifies a player using replay-local historical identity.
type ReplayParticipant struct {
	PlayerSlot          int32               `json:"player_slot"`
	HeroID              uint32              `json:"hero_id,omitempty"`
	Team                int32               `json:"team,omitempty"`
	HasHeroID           bool                `json:"has_hero_id"`
	HasTeam             bool                `json:"has_team"`
	HistoricalEntityID  int32               `json:"historical_entity_id"`
	HistoricalEntityIDs []int32             `json:"historical_entity_ids"`
	Epochs              []ReplayEntityEpoch `json:"epochs"`
}

// ReplayEntityEpoch identifies one serial/index lifetime in the replay.
type ReplayEntityEpoch struct {
	EntityID        int32  `json:"entity_id"`
	Serial          int32  `json:"serial"`
	FirstSampleTick uint32 `json:"first_sample_tick"`
	LastSampleTick  uint32 `json:"last_sample_tick"`
}

// ReplayScalar is one source field with its source tick and age at row time.
type ReplayScalar struct {
	Value          float32            `json:"value"`
	SourceField    string             `json:"source_field"`
	SampleTick     uint32             `json:"sample_tick"`
	FreshnessTicks uint32             `json:"freshness_ticks"`
	Quality        ReplayFieldQuality `json:"quality"`
	MissingReason  string             `json:"missing_reason,omitempty"`
}

// ReplaySegmentRow is a dense source sample for one participant.
type ReplaySegmentRow struct {
	Tick         uint32   `json:"tick"`
	GameTime     *float64 `json:"game_time,omitempty"`
	LeadIn       bool     `json:"lead_in"`
	PlayerSlot   int32    `json:"player_slot"`
	EntityID     int32    `json:"entity_id"`
	EntitySerial int32    `json:"entity_serial"`

	PositionX ReplayScalar `json:"position_x"`
	PositionY ReplayScalar `json:"position_y"`
	PositionZ ReplayScalar `json:"position_z"`
	FacingX   ReplayScalar `json:"facing_x"`
	FacingY   ReplayScalar `json:"facing_y"`
	FacingZ   ReplayScalar `json:"facing_z"`
	VelocityX ReplayScalar `json:"velocity_x"`
	VelocityY ReplayScalar `json:"velocity_y"`
	VelocityZ ReplayScalar `json:"velocity_z"`
}

// ReplaySegmentRowKey identifies a requested participant row absent at an observed tick.
type ReplaySegmentRowKey struct {
	Tick       uint32 `json:"tick"`
	PlayerSlot int32  `json:"player_slot"`
}

// ReplaySegmentQuality summarizes source coverage without deriving motion.
type ReplaySegmentQuality struct {
	Rows                      int                   `json:"rows"`
	ObservedRows              int                   `json:"observed_rows"`
	MissingRows               int                   `json:"missing_rows"`
	LeadInRows                int                   `json:"lead_in_rows"`
	RequestedRows             int                   `json:"requested_rows"`
	Participants              int                   `json:"participants"`
	ExactFacingRows           int                   `json:"exact_facing_rows"`
	ExactVelocityRows         int                   `json:"exact_velocity_rows"`
	MissingFacingRows         int                   `json:"missing_facing_rows"`
	MissingVelocityRows       int                   `json:"missing_velocity_rows"`
	ExactStartPresent         bool                  `json:"exact_start_present"`
	ExactEndPresent           bool                  `json:"exact_end_present"`
	BoundaryMissingReason     string                `json:"boundary_missing_reason,omitempty"`
	MissingParticipantSlots   []int32               `json:"missing_participant_slots,omitempty"`
	MissingRequestedRows      []ReplaySegmentRowKey `json:"missing_requested_rows,omitempty"`
	MissingRequestedRowsTotal int                   `json:"missing_requested_rows_total"`
	CoalescedRows             int                   `json:"coalesced_rows"`
	AmbiguousRows             int                   `json:"ambiguous_rows"`
	AmbiguousParticipants     []int32               `json:"ambiguous_participants,omitempty"`
	MissingFields             []string              `json:"missing_fields,omitempty"`
}

// ReplaySegmentEligibility states whether evidence satisfies scenario use.
type ReplaySegmentEligibility string

const (
	ReplayEligibilityNotDeclared ReplaySegmentEligibility = "not_declared"
	ReplayEligibilityEligible    ReplaySegmentEligibility = "eligible"
	ReplayEligibilityIneligible  ReplaySegmentEligibility = "ineligible"
)

// ReplaySegmentEvidence is versioned, replay-local evidence for one range.
type ReplaySegmentEvidence struct {
	SchemaVersion      int                          `json:"schema_version"`
	Source             ReplaySourceIdentity         `json:"source"`
	Correspondence     ReplayIdentityCorrespondence `json:"correspondence"`
	Range              ReplaySegmentRange           `json:"range"`
	Participants       []ReplayParticipant          `json:"participants"`
	Rows               []ReplaySegmentRow           `json:"rows"`
	Quality            ReplaySegmentQuality         `json:"quality"`
	Eligibility        ReplaySegmentEligibility     `json:"eligibility"`
	EligibilityReasons []string                     `json:"eligibility_reasons,omitempty"`

	// placeholderHeroes records, outside the serialized contract, the proven
	// pre-selection placeholder transition per participant slot: the entity
	// field carried hero_id=0 before exactly one nonzero hero was selected.
	// The Teamfight projection uses it to normalize that transition locally;
	// the serialized evidence keeps the observed zero and the ambiguity.
	placeholderHeroes map[int32]uint32
}

// ExtractReplaySegmentEvidence parses immutable demo bytes and extracts a range.
// ExtractReplaySegmentEvidence refuses binaries without clean VCS identity.
func ExtractReplaySegmentEvidence(demo []byte, request ReplaySegmentRequest) (ReplaySegmentEvidence, error) {
	if _, err := replayHeader(demo); err != nil {
		return ReplaySegmentEvidence{}, err
	}
	revision, clean := s2replay.BuildRevision()
	return extractReplaySegmentEvidenceWithBuild(demo, request, revision, clean)
}

// replayEventParser is the subset of the parser surface needed to consume a
// replay as an event stream.
type replayEventParser interface {
	NextEvent() (s2replay.Event, error)
	SetEventMode(bool)
	ReleasePendingQueues()
}

// consumeReplayEvents drains the parser's event stream into accept and
// releases the parser's pending queues before returning.
func consumeReplayEvents(parser replayEventParser, accept func(s2replay.Event)) error {
	parser.SetEventMode(true)
	defer parser.ReleasePendingQueues()
	for {
		event, err := parser.NextEvent()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		accept(event)
	}
}

func extractReplaySegmentEvidenceWithBuild(demo []byte, request ReplaySegmentRequest, revision string, cleanBuild bool) (ReplaySegmentEvidence, error) {
	if !cleanBuild {
		return ReplaySegmentEvidence{}, errors.New("running parser build has unknown or modified VCS identity")
	}
	if request.StartTick > request.EndTick {
		return ReplaySegmentEvidence{}, errors.New("replay segment start tick exceeds end tick")
	}
	if err := validateReplaySegmentRows(request, len(request.ParticipantSlots)); err != nil {
		return ReplaySegmentEvidence{}, err
	}
	header, err := replayHeader(demo)
	if err != nil {
		return ReplaySegmentEvidence{}, err
	}
	if !cleanBuild {
		return ReplaySegmentEvidence{}, errors.New("running parser build has unknown or modified VCS identity")
	}
	parser, err := s2replay.NewParser(demo)
	if err != nil {
		return ReplaySegmentEvidence{}, err
	}
	acc := newReplaySegmentAccumulator(request)
	var matchID uint64
	if err := consumeReplayEvents(parser, func(event s2replay.Event) {
		if event.PostMatch != nil && event.PostMatch.MatchID != 0 {
			matchID = event.PostMatch.MatchID
		}
		if event.Tick <= request.EndTick {
			acc.accept(event)
		}
	}); err != nil {
		return ReplaySegmentEvidence{}, err
	}

	source := ReplaySourceIdentity{
		SHA256:         sha256Hex(demo),
		MatchID:        matchID,
		Game:           header.GetGame(),
		Map:            header.GetMapName(),
		GameBuild:      header.GetBuildNum(),
		Parser:         "s2replay",
		ParserRevision: s2replay.ParserSourceDigest,
		VCSRevision:    revision,
	}
	if matchID == 0 {
		source.MatchIDMissingReason = "post-match match ID absent or zero"
	}
	return acc.finish(source)
}

// validateReplaySegmentRows refuses requests whose tick range or participant
// list would materialize more rows than the evidence limit allows.
func validateReplaySegmentRows(request ReplaySegmentRequest, participants int) error {
	if request.EndTick < request.StartTick {
		return errors.New("replay segment range overflows")
	}
	seen := map[int32]struct{}{}
	for _, slot := range request.ParticipantSlots {
		if slot < 0 || slot >= MaxReplayParticipants {
			return errors.New("participant slot out of range")
		}
		if _, ok := seen[slot]; ok {
			return errors.New("duplicate participant slot")
		}
		seen[slot] = struct{}{}
	}
	span := uint64(request.EndTick) - uint64(request.StartTick) + 1 + uint64(request.LeadInTicks)
	if span > uint64(^uint32(0)) {
		return errors.New("replay segment range is too large")
	}
	if participants <= 0 {
		participants = MaxReplayParticipants
	}
	count := span * uint64(participants)
	if count > MaxReplaySegmentRows {
		return errors.New("replay segment exceeds materialized row limit")
	}
	return nil
}

// buildReplaySegmentEvidence extracts deterministic evidence from typed events.
func buildReplaySegmentEvidence(events []s2replay.Event, source ReplaySourceIdentity, request ReplaySegmentRequest) (ReplaySegmentEvidence, error) {
	if err := validateReplaySegmentRows(request, len(request.ParticipantSlots)); err != nil {
		return ReplaySegmentEvidence{}, err
	}
	acc := newReplaySegmentAccumulator(request)
	for _, event := range events {
		acc.accept(event)
	}
	return acc.finish(source)
}

// replaySegmentAccumulator folds a stream of typed events into segment
// evidence for one request.
type replaySegmentAccumulator struct {
	request               ReplaySegmentRequest
	leadInStart           uint32
	allSlots              bool
	selected              map[int32]struct{}
	participants          map[int32]ReplayParticipant
	census                map[int32]struct{}
	rows                  []ReplaySegmentRow
	rowIndexes            map[ReplaySegmentRowKey]int
	coalescedRows         int
	ambiguousRows         int
	ambiguousParticipants map[int32]struct{}
	placeholderHeroes     map[int32]uint32
	heroObservations      map[int32][]uint32
	err                   error
}

// newReplaySegmentAccumulator constructs an accumulator for the request.
func newReplaySegmentAccumulator(request ReplaySegmentRequest) *replaySegmentAccumulator {
	leadInStart := uint32(0)
	if request.LeadInTicks <= request.StartTick {
		leadInStart = request.StartTick - request.LeadInTicks
	}
	selected := make(map[int32]struct{}, len(request.ParticipantSlots))
	for _, slot := range request.ParticipantSlots {
		selected[slot] = struct{}{}
	}
	return &replaySegmentAccumulator{request: request, leadInStart: leadInStart, allSlots: len(selected) == 0, selected: selected, participants: make(map[int32]ReplayParticipant), census: make(map[int32]struct{}), rowIndexes: make(map[ReplaySegmentRowKey]int), ambiguousParticipants: make(map[int32]struct{})}
}

// markAmbiguousParticipant records that a slot observed conflicting identity
// values.
func (a *replaySegmentAccumulator) markAmbiguousParticipant(slot int32) {
	a.ambiguousParticipants[slot] = struct{}{}
}

// observeHero records the ordered hero observations per slot, outside the
// serialized contract, so a proven placeholder transition can be distinguished
// from any other hero conflict.
func (a *replaySegmentAccumulator) observeHero(slot int32, hero uint32) {
	if a.heroObservations == nil {
		a.heroObservations = make(map[int32][]uint32)
	}
	a.heroObservations[slot] = append(a.heroObservations[slot], hero)
}

// provenPlaceholderTransition reports whether the slot's hero observation
// history is exactly zero or more placeholder zeros followed only by the
// nonzero selected hero. Any other shape, including a repeated zero after
// selection, a second distinct hero, or a nonzero first observation, is a real
// conflict.
func (a *replaySegmentAccumulator) provenPlaceholderTransition(slot int32, hero uint32) bool {
	if hero == 0 {
		return false
	}
	observations := a.heroObservations[slot]
	i := 0
	for i < len(observations) && observations[i] == 0 {
		i++
	}
	if i == 0 || i == len(observations) {
		return false
	}
	for ; i < len(observations); i++ {
		if observations[i] != hero {
			return false
		}
	}
	return true
}

// accept folds one replay event into the accumulated evidence.
func (a *replaySegmentAccumulator) accept(event s2replay.Event) {
	if event.Type != s2replay.EventEntitySample || event.EntitySample == nil || event.PlayerSlot < 0 || event.Tick > a.request.EndTick {
		return
	}

	// Census every observed slot before dropping samples from unselected
	// slots.
	a.census[event.PlayerSlot] = struct{}{}
	if !a.allSlots {
		if _, ok := a.selected[event.PlayerSlot]; !ok {
			return
		}
	}

	// Track the slot's entity history and identity.
	participant := a.participants[event.PlayerSlot]
	participant.PlayerSlot = event.PlayerSlot
	if !slices.Contains(participant.HistoricalEntityIDs, event.Entity) {
		participant.HistoricalEntityIDs = append(participant.HistoricalEntityIDs, event.Entity)
	}
	if len(participant.HistoricalEntityIDs) == 1 {
		participant.HistoricalEntityID = event.Entity
	}

	// Open a new entity epoch when the entity index or serial changed.
	serial := event.EntitySample.EntitySerial
	if len(participant.Epochs) == 0 || participant.Epochs[len(participant.Epochs)-1].EntityID != event.Entity || participant.Epochs[len(participant.Epochs)-1].Serial != serial {
		participant.Epochs = append(participant.Epochs, ReplayEntityEpoch{EntityID: event.Entity, Serial: serial, FirstSampleTick: event.Tick, LastSampleTick: event.Tick})
	} else {
		participant.Epochs[len(participant.Epochs)-1].LastSampleTick = event.Tick
	}
	if event.EntitySample.HasHeroID {
		if participant.HasHeroID && participant.HeroID != event.EntitySample.HeroID {
			a.markAmbiguousParticipant(event.PlayerSlot)
		}
		hero := event.EntitySample.HeroID
		a.observeHero(event.PlayerSlot, hero)
		if a.provenPlaceholderTransition(event.PlayerSlot, hero) {
			// Proven pre-selection placeholder: hero_id=0 was the slot's only
			// earlier hero value and the selected hero is the only nonzero
			// value observed. Recorded outside the serialized contract for the
			// Teamfight projection; the serialized evidence keeps the conflict.
			if a.placeholderHeroes == nil {
				a.placeholderHeroes = make(map[int32]uint32)
			}
			a.placeholderHeroes[event.PlayerSlot] = hero
		} else {
			// A later hero observation broke the proven placeholder shape, so
			// the earlier substitution is no longer justified.
			delete(a.placeholderHeroes, event.PlayerSlot)
		}
		participant.HeroID, participant.HasHeroID = event.EntitySample.HeroID, true
	}
	if event.EntitySample.HasTeam {
		if participant.HasTeam && participant.Team != event.EntitySample.Team {
			a.markAmbiguousParticipant(event.PlayerSlot)
		}
		participant.Team, participant.HasTeam = event.EntitySample.Team, true
	}
	a.participants[event.PlayerSlot] = participant
	if event.Tick < a.leadInStart {
		return
	}

	// Materialize a row for the sample, coalescing duplicates at one tick.
	row := replayRow(event)
	row.LeadIn = event.Tick < a.request.StartTick
	key := ReplaySegmentRowKey{Tick: row.Tick, PlayerSlot: row.PlayerSlot}
	if _, ok := a.rowIndexes[key]; !ok && len(a.rowIndexes) >= MaxReplaySegmentRows {
		a.err = errors.New("replay segment exceeds materialized row limit")
		return
	}
	if index, ok := a.rowIndexes[key]; ok {
		previous := a.rows[index]
		if !rowsEquivalent(previous, row) {
			a.ambiguousRows++
		}
		a.rows[index] = row
		a.coalescedRows++
		return
	}
	a.rowIndexes[key] = len(a.rows)
	a.rows = append(a.rows, row)
}

// finish assembles the accumulated state into the final evidence record.
func (a *replaySegmentAccumulator) finish(source ReplaySourceIdentity) (ReplaySegmentEvidence, error) {
	if a.err != nil {
		return ReplaySegmentEvidence{}, a.err
	}
	out := ReplaySegmentEvidence{
		SchemaVersion: ReplaySegmentEvidenceSchemaVersion,
		Source:        source,
		Range: ReplaySegmentRange{
			RequestedStartTick:   a.request.StartTick,
			RequestedEndTick:     a.request.EndTick,
			LeadInStartTick:      a.leadInStart,
			RequestedLeadInTicks: a.request.LeadInTicks,
			LeadInTicks:          a.request.StartTick - a.leadInStart,
			ExactStartTick:       a.request.StartTick,
			ExactEndTick:         a.request.EndTick,
		},
		Participants:      []ReplayParticipant{},
		Rows:              a.rows,
		placeholderHeroes: a.placeholderHeroes,
	}

	// Record the identity correspondence, or leave it pending when the caller
	// declared no expectation.
	if a.request.ExpectedIdentity == nil {
		out.Correspondence = ReplayIdentityCorrespondence{Status: ReplayCorrespondencePending, Reason: "no expected replay identity supplied"}
	} else {
		out.Correspondence = compareReplayIdentity(source, *a.request.ExpectedIdentity)
	}

	// Build the participant list: with no explicit slot selection the census
	// is every slot with recorded samples; otherwise flag requested slots
	// absent from the replay.
	participants := a.participants
	if a.allSlots {
		participants = make(map[int32]ReplayParticipant, len(a.census))
		for slot := range a.census {
			if value, ok := a.participants[slot]; ok {
				participants[slot] = value
			}
		}
	} else {
		for slot := range a.selected {
			if _, ok := participants[slot]; !ok {
				participants[slot] = ReplayParticipant{PlayerSlot: slot, HistoricalEntityID: -1, HistoricalEntityIDs: []int32{}, Epochs: []ReplayEntityEpoch{}}
				out.Quality.MissingParticipantSlots = append(out.Quality.MissingParticipantSlots, slot)
			}
		}
		slices.Sort(out.Quality.MissingParticipantSlots)
	}
	for _, participant := range participants {
		out.Participants = append(out.Participants, participant)
	}
	slices.SortFunc(out.Participants, func(a, b ReplayParticipant) int { return cmp.Compare(a.PlayerSlot, b.PlayerSlot) })

	// Materialize a dense row grid over the lead-in and requested range.
	rowCount := (uint64(a.request.EndTick) - uint64(a.leadInStart) + 1) * uint64(len(out.Participants))
	if rowCount > MaxReplaySegmentRows {
		return ReplaySegmentEvidence{}, errors.New("replay segment exceeds materialized row limit")
	}
	observed := make(map[ReplaySegmentRowKey]ReplaySegmentRow, len(out.Rows))
	for _, row := range out.Rows {
		observed[ReplaySegmentRowKey{Tick: row.Tick, PlayerSlot: row.PlayerSlot}] = row
	}
	dense := make([]ReplaySegmentRow, 0, int(uint64(a.request.EndTick)-uint64(a.leadInStart)+1)*len(out.Participants))
	for tick := a.leadInStart; ; tick++ {
		for _, participant := range out.Participants {
			key := ReplaySegmentRowKey{Tick: tick, PlayerSlot: participant.PlayerSlot}
			if row, ok := observed[key]; ok {
				dense = append(dense, row)
				continue
			}
			dense = append(dense, missingReplaySegmentRow(tick, tick < a.request.StartTick, participant.PlayerSlot))
		}
		if tick == a.request.EndTick {
			break
		}
	}

	// Summarize row coverage and field-level quality.
	out.Rows = dense
	out.Quality.Rows, out.Quality.Participants = len(out.Rows), len(out.Participants)
	out.Quality.ObservedRows = len(observed)
	out.Quality.MissingRows = len(out.Rows) - len(observed)
	out.Quality.CoalescedRows = a.coalescedRows
	out.Quality.AmbiguousRows = a.ambiguousRows
	for slot := range a.ambiguousParticipants {
		out.Quality.AmbiguousParticipants = append(out.Quality.AmbiguousParticipants, slot)
	}
	slices.Sort(out.Quality.AmbiguousParticipants)

	// Classify rows as lead-in or requested and count exact field coverage.
	requested := make(map[uint32]map[int32]bool)
	for _, row := range out.Rows {
		if row.LeadIn {
			out.Quality.LeadInRows++
		} else {
			out.Quality.RequestedRows++
			if requested[row.Tick] == nil {
				requested[row.Tick] = map[int32]bool{}
			}
			requested[row.Tick][row.PlayerSlot] = true
		}
		if row.FacingX.Quality == ReplayFieldExact && row.FacingY.Quality == ReplayFieldExact && row.FacingZ.Quality == ReplayFieldExact {
			out.Quality.ExactFacingRows++
		} else {
			out.Quality.MissingFacingRows++
		}
		if row.VelocityX.Quality == ReplayFieldExact && row.VelocityY.Quality == ReplayFieldExact && row.VelocityZ.Quality == ReplayFieldExact {
			out.Quality.ExactVelocityRows++
		} else {
			out.Quality.MissingVelocityRows++
		}
	}
	if out.Quality.MissingFacingRows != 0 {
		out.Quality.MissingFields = append(out.Quality.MissingFields, "m_angEyeAngles")
	}
	if out.Quality.MissingVelocityRows != 0 {
		out.Quality.MissingFields = append(out.Quality.MissingFields, "m_vecVelocity.m_vecX", "m_vecVelocity.m_vecY", "m_vecVelocity.m_vecZ")
	}

	// Count requested rows with no observed sample, keeping a bounded listing.
	for tick := a.request.StartTick; tick <= a.request.EndTick; tick++ {
		for _, participant := range out.Participants {
			if _, ok := observed[ReplaySegmentRowKey{Tick: tick, PlayerSlot: participant.PlayerSlot}]; !ok {
				out.Quality.MissingRequestedRowsTotal++
				if len(out.Quality.MissingRequestedRows) < 64 {
					out.Quality.MissingRequestedRows = append(out.Quality.MissingRequestedRows, ReplaySegmentRowKey{Tick: tick, PlayerSlot: participant.PlayerSlot})
				}
			}
		}
		if tick == ^uint32(0) {
			break
		}
	}

	// Determine whether each requested boundary tick is fully observed.
	startSlots := map[int32]bool{}
	endSlots := map[int32]bool{}
	for _, participant := range out.Participants {
		if _, ok := observed[ReplaySegmentRowKey{Tick: a.request.StartTick, PlayerSlot: participant.PlayerSlot}]; ok {
			startSlots[participant.PlayerSlot] = true
		}
		if _, ok := observed[ReplaySegmentRowKey{Tick: a.request.EndTick, PlayerSlot: participant.PlayerSlot}]; ok {
			endSlots[participant.PlayerSlot] = true
		}
	}
	out.Quality.ExactStartPresent = len(out.Participants) != 0 && len(startSlots) == len(out.Participants)
	out.Quality.ExactEndPresent = len(out.Participants) != 0 && len(endSlots) == len(out.Participants)
	if len(out.Quality.MissingParticipantSlots) != 0 || !out.Quality.ExactStartPresent || !out.Quality.ExactEndPresent {
		out.Quality.BoundaryMissingReason = "requested boundary coverage is incomplete"
	}
	if !out.Quality.ExactStartPresent {
		out.Range.ExactStartTick = 0
	}
	if !out.Quality.ExactEndPresent {
		out.Range.ExactEndTick = 0
	}

	// Grade eligibility against the declared freshness policy.
	if a.request.MaxFreshnessTicks == nil {
		out.Eligibility, out.EligibilityReasons = ReplayEligibilityNotDeclared, []string{"freshness requirement not declared"}
	} else if out.Correspondence.Status != ReplayCorrespondenceMatched {
		out.Eligibility, out.EligibilityReasons = ReplayEligibilityIneligible, []string{"replay identity correspondence is not matched"}
	} else if out.Quality.AmbiguousRows != 0 || len(out.Quality.AmbiguousParticipants) != 0 {
		out.Eligibility, out.EligibilityReasons = ReplayEligibilityIneligible, []string{"ambiguous duplicate source rows"}
	} else if out.Quality.MissingRequestedRowsTotal != 0 || !out.Quality.ExactStartPresent || !out.Quality.ExactEndPresent {
		out.Eligibility, out.EligibilityReasons = ReplayEligibilityIneligible, []string{"requested participant coverage is incomplete"}
	} else {
		out.Eligibility = ReplayEligibilityEligible
		for _, row := range out.Rows {
			if row.LeadIn {
				continue
			}
			for _, field := range []ReplayScalar{row.PositionX, row.PositionY, row.PositionZ, row.FacingX, row.FacingY, row.FacingZ, row.VelocityX, row.VelocityY, row.VelocityZ} {
				if field.Quality != ReplayFieldExact || field.FreshnessTicks > *a.request.MaxFreshnessTicks {
					out.Eligibility, out.EligibilityReasons = ReplayEligibilityIneligible, []string{"requested row has missing or stale exact fields"}
					break
				}
			}
			if out.Eligibility == ReplayEligibilityIneligible {
				break
			}
		}
	}
	return out, nil
}

// compareReplayIdentity grades the observed source identity against the
// consumer's expectation.
func compareReplayIdentity(source ReplaySourceIdentity, expected ReplayIdentityExpectation) ReplayIdentityCorrespondence {
	if expected.SHA256 != "" && !canonicalSHA256(expected.SHA256) {
		return ReplayIdentityCorrespondence{Expected: expected, Status: ReplayCorrespondenceMismatched, Reason: "expected SHA256 must be 64 lowercase hexadecimal characters"}
	}
	if expected.SHA256 == "" || expected.Map == "" || expected.GameBuild == 0 {
		return ReplayIdentityCorrespondence{Expected: expected, Status: ReplayCorrespondencePending, Reason: "expected SHA256, map, and nonzero game build are required"}
	}
	if expected.SHA256 != source.SHA256 {
		return ReplayIdentityCorrespondence{Expected: expected, Status: ReplayCorrespondenceMismatched, Reason: "replay hash mismatch"}
	}
	if expected.MatchID != nil && source.MatchID != *expected.MatchID {
		return ReplayIdentityCorrespondence{Expected: expected, Status: ReplayCorrespondenceMismatched, Reason: "match id mismatch"}
	}
	if expected.Game != "" && expected.Game != source.Game {
		return ReplayIdentityCorrespondence{Expected: expected, Status: ReplayCorrespondenceMismatched, Reason: "game mismatch"}
	}
	if expected.Map != source.Map {
		return ReplayIdentityCorrespondence{Expected: expected, Status: ReplayCorrespondenceMismatched, Reason: "map mismatch"}
	}
	if expected.GameBuild != source.GameBuild {
		return ReplayIdentityCorrespondence{Expected: expected, Status: ReplayCorrespondenceMismatched, Reason: "game build mismatch"}
	}
	return ReplayIdentityCorrespondence{Expected: expected, Status: ReplayCorrespondenceMatched}
}

// canonicalSHA256 reports whether the value is 64 lowercase hexadecimal
// characters.
func canonicalSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

// replayRow converts one entity sample event into a segment row.
func replayRow(event s2replay.Event) ReplaySegmentRow {
	sample := event.EntitySample
	gameTime := event.GameTime
	row := ReplaySegmentRow{Tick: event.Tick, GameTime: &gameTime, PlayerSlot: event.PlayerSlot, EntityID: event.Entity, EntitySerial: sample.EntitySerial}
	row.PositionX = scalar(sample.PositionX, sample.PositionXTick, event.Tick, sample.HasPosition, sample.PositionXSourceField, "position_not_present")
	row.PositionY = scalar(sample.PositionY, sample.PositionYTick, event.Tick, sample.HasPosition, sample.PositionYSourceField, "position_not_present")
	row.PositionZ = scalar(sample.PositionZ, sample.PositionZTick, event.Tick, sample.HasPosition, sample.PositionZSourceField, "position_not_present")
	row.FacingX = scalar(sample.FacingX, sample.FacingXTick, event.Tick, sample.HasFacingX || sample.HasFacing, sample.FacingXSourceField, "m_angEyeAngles_not_present")
	row.FacingY = scalar(sample.FacingY, sample.FacingYTick, event.Tick, sample.HasFacingY || sample.HasFacing, sample.FacingYSourceField, "m_angEyeAngles_not_present")
	row.FacingZ = scalar(sample.FacingZ, sample.FacingZTick, event.Tick, sample.HasFacingZ || sample.HasFacing, sample.FacingZSourceField, "m_angEyeAngles_not_present")
	row.VelocityX = scalar(sample.VelocityX, sample.VelocityXTick, event.Tick, sample.HasVelocityX || sample.HasVelocity, sample.VelocityXSourceField, "m_vecVelocity.m_vecX_not_present")
	row.VelocityY = scalar(sample.VelocityY, sample.VelocityYTick, event.Tick, sample.HasVelocityY || sample.HasVelocity, sample.VelocityYSourceField, "m_vecVelocity.m_vecY_not_present")
	row.VelocityZ = scalar(sample.VelocityZ, sample.VelocityZTick, event.Tick, sample.HasVelocityZ || sample.HasVelocity, sample.VelocityZSourceField, "m_vecVelocity.m_vecZ_not_present")
	return row
}

// missingReplaySegmentRow builds the synthetic row for a participant with no
// sample at a tick.
func missingReplaySegmentRow(tick uint32, leadIn bool, slot int32) ReplaySegmentRow {
	missing := func() ReplayScalar {
		return ReplayScalar{Quality: ReplayFieldMissing, MissingReason: "no_entity_sample_at_tick"}
	}
	return ReplaySegmentRow{Tick: tick, LeadIn: leadIn, PlayerSlot: slot, EntityID: -1, EntitySerial: -1, PositionX: missing(), PositionY: missing(), PositionZ: missing(), FacingX: missing(), FacingY: missing(), FacingZ: missing(), VelocityX: missing(), VelocityY: missing(), VelocityZ: missing()}
}

// rowsEquivalent reports whether two rows carry the same evidence, ignoring
// pointer identity of the game time.
func rowsEquivalent(a, b ReplaySegmentRow) bool {
	if a.GameTime != nil && b.GameTime != nil {
		av, bv := *a.GameTime, *b.GameTime
		a.GameTime, b.GameTime = &av, &bv
	}
	return reflect.DeepEqual(a, b)
}

// scalar builds one scalar record, downgrading absent, non-finite, or
// future-dated values to a typed missing reason.
func scalar(value float32, tick, rowTick uint32, present bool, field, reason string) ReplayScalar {
	if !present || field == "" || tick > rowTick {
		if tick > rowTick {
			reason = "sample_tick_after_row_tick"
		}
		return ReplayScalar{SourceField: field, Quality: ReplayFieldMissing, MissingReason: reason}
	}
	if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return ReplayScalar{SourceField: field, Quality: ReplayFieldMissing, MissingReason: "non_finite_source_value"}
	}
	out := ReplayScalar{Value: value, SourceField: field, SampleTick: tick, Quality: ReplayFieldExact}
	if rowTick >= tick {
		out.FreshnessTicks = rowTick - tick
	}
	return out
}

// sha256Hex returns the hexadecimal SHA-256 digest of the data.
func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// replayHeader parses the DEM_FileHeader message from the demo bytes.
func replayHeader(demo []byte) (*protocol.CDemoFileHeader, error) {
	parser, err := s2replay.NewParser(demo)
	if err != nil {
		return nil, err
	}
	for {
		command, err := parser.Next()
		if err == io.EOF {
			return nil, ErrMissingReplayHeader
		}
		if err != nil {
			return nil, err
		}
		if command.Kind != protocol.EDemoCommands_DEM_FileHeader {
			continue
		}
		header := &protocol.CDemoFileHeader{}
		if err := header.UnmarshalVT(command.Payload); err != nil {
			return nil, err
		}
		return header, nil
	}
}
