package analysis

import (
	"cmp"
	"errors"
	"slices"
	"strconv"

	"github.com/paralin/s2replay"
)

// TeamfightEvidenceSchemaVersion identifies the teamfight census contract.
const TeamfightEvidenceSchemaVersion = 1

// TeamfightCensusSize is the required teamfight census participant count.
// The replay-local PlayerSlot domain is observed, never assumed.
const TeamfightCensusSize = 12

// ErrTeamfightExplicitSlots indicates a Teamfight request that names explicit
// participant slots instead of selecting all participants.
var ErrTeamfightExplicitSlots = errors.New("teamfight request is all-participant and refuses explicit participant slots")

// ErrTeamfightAmbiguousEvidence indicates two source rows or two source
// samples for one participant disagree.
var ErrTeamfightAmbiguousEvidence = errors.New("teamfight evidence has ambiguous duplicate source rows")

// ErrTeamfightNonFiniteSource indicates an exact source field carried a
// non-finite value.
var ErrTeamfightNonFiniteSource = errors.New("teamfight evidence has a non-finite source value")

// TeamfightCensusSizeError refuses a replay whose entity-sample census is not
// exactly TeamfightCensusSize participants.
type TeamfightCensusSizeError struct {
	// Observed is the census participant count in the replay.
	Observed int
}

// Error returns the typed refusal with the observed census count.
func (e *TeamfightCensusSizeError) Error() string {
	return "teamfight census requires exactly " + strconv.Itoa(TeamfightCensusSize) + " participants: observed " + strconv.Itoa(e.Observed)
}

// TeamfightParticipantStatus states whether a census participant has rows in
// the requested window.
type TeamfightParticipantStatus string

const (
	// TeamfightParticipantObserved marks a census participant with rows in
	// the requested window.
	TeamfightParticipantObserved TeamfightParticipantStatus = "observed"
	// TeamfightParticipantMissing marks a census participant without rows in
	// the requested window.
	TeamfightParticipantMissing TeamfightParticipantStatus = "missing"
)

// TeamfightParticipant is one replay-local census member projected onto the
// requested window. Hero and team stay the observed numeric replay values;
// the parser knows no runtime roster.
type TeamfightParticipant struct {
	PlayerSlot int32                      `json:"player_slot"`
	Status     TeamfightParticipantStatus `json:"status"`
	Reason     string                     `json:"reason,omitempty"`
	HeroID     uint32                     `json:"hero_id,omitempty"`
	HasHeroID  bool                       `json:"has_hero_id"`
	Team       int32                      `json:"team,omitempty"`
	HasTeam    bool                       `json:"has_team"`
}

// TeamfightBoundaryStatus states how the requested window bounds compare to
// recorded census rows.
type TeamfightBoundaryStatus string

const (
	// TeamfightBoundaryValid marks a window whose bounds carry rows for every
	// census participant.
	TeamfightBoundaryValid TeamfightBoundaryStatus = "valid"
	// TeamfightBoundaryIncomplete marks a window whose bounds carry rows for
	// some but not all census participants.
	TeamfightBoundaryIncomplete TeamfightBoundaryStatus = "incomplete"
	// TeamfightBoundaryInvalidTimestamp marks a bound with no recorded rows.
	// The zero exact-boundary evidence in the wrapped segment stays visible.
	TeamfightBoundaryInvalidTimestamp TeamfightBoundaryStatus = "invalid_timestamp"
)

// TeamfightBoundary records the window-bound verdict with the exact missing
// census slots per bound.
type TeamfightBoundary struct {
	Status            TeamfightBoundaryStatus `json:"status"`
	Reason            string                  `json:"reason,omitempty"`
	MissingStartSlots []int32                 `json:"missing_start_slots,omitempty"`
	MissingEndSlots   []int32                 `json:"missing_end_slots,omitempty"`
}

// TeamfightEvidence is the versioned teamfight census projection of one
// all-participant replay segment. Participants is exactly the observed
// replay-local census sorted ascending by PlayerSlot.
type TeamfightEvidence struct {
	SchemaVersion int                    `json:"schema_version"`
	Participants  []TeamfightParticipant `json:"participants"`
	Boundary      TeamfightBoundary      `json:"boundary"`
	Evidence      ReplaySegmentEvidence  `json:"evidence"`
}

// ExtractTeamfightEvidence extracts the teamfight census projection for one
// replay window. The request must be all-participant; the census and its
// PlayerSlot domain come from the replay, never from the request.
func ExtractTeamfightEvidence(demo []byte, request ReplaySegmentRequest) (TeamfightEvidence, error) {
	revision, clean := s2replay.BuildRevision()
	return ExtractTeamfightEvidenceWithBuild(demo, request, revision, clean)
}

// ExtractTeamfightEvidenceWithBuild extracts the teamfight census projection
// with an explicit parser build identity; it backs tests and fixtures.
func ExtractTeamfightEvidenceWithBuild(demo []byte, request ReplaySegmentRequest, revision string, cleanBuild bool) (TeamfightEvidence, error) {
	if err := validateTeamfightRequest(request); err != nil {
		return TeamfightEvidence{}, err
	}
	segment, err := extractReplaySegmentEvidenceWithBuild(demo, request, revision, cleanBuild)
	if err != nil {
		return TeamfightEvidence{}, err
	}
	return TeamfightEvidenceFromSegment(segment)
}

// TeamfightEvidenceFromSegment projects an all-participant replay segment onto
// the teamfight census contract. The segment must come from a request without
// explicit participant slots so the wrapped census is the replay census.
func TeamfightEvidenceFromSegment(segment ReplaySegmentEvidence) (TeamfightEvidence, error) {
	if err := refuseTeamfightAmbiguity(segment); err != nil {
		return TeamfightEvidence{}, err
	}
	if err := refuseTeamfightNonFiniteRows(segment.Rows); err != nil {
		return TeamfightEvidence{}, err
	}
	if len(segment.Participants) != TeamfightCensusSize {
		return TeamfightEvidence{}, &TeamfightCensusSizeError{Observed: len(segment.Participants)}
	}

	// Classify window coverage and boundary coverage from observed rows.
	windowSlots := map[int32]bool{}
	startSlots := map[int32]bool{}
	endSlots := map[int32]bool{}
	for _, row := range segment.Rows {
		if row.EntityID < 0 {
			continue
		}
		if !row.LeadIn {
			windowSlots[row.PlayerSlot] = true
		}
		if row.Tick == segment.Range.RequestedStartTick {
			startSlots[row.PlayerSlot] = true
		}
		if row.Tick == segment.Range.RequestedEndTick {
			endSlots[row.PlayerSlot] = true
		}
	}

	out := TeamfightEvidence{
		SchemaVersion: TeamfightEvidenceSchemaVersion,
		Participants:  make([]TeamfightParticipant, 0, TeamfightCensusSize),
		Evidence:      segment,
	}
	for _, replay := range segment.Participants {
		participant := TeamfightParticipant{
			PlayerSlot: replay.PlayerSlot,
			HeroID:     replay.HeroID,
			HasHeroID:  replay.HasHeroID,
			Team:       replay.Team,
			HasTeam:    replay.HasTeam,
		}
		if windowSlots[replay.PlayerSlot] {
			participant.Status = TeamfightParticipantObserved
		} else {
			participant.Status = TeamfightParticipantMissing
			participant.Reason = "census participant has no rows in the requested window"
		}
		if !startSlots[replay.PlayerSlot] {
			out.Boundary.MissingStartSlots = append(out.Boundary.MissingStartSlots, replay.PlayerSlot)
		}
		if !endSlots[replay.PlayerSlot] {
			out.Boundary.MissingEndSlots = append(out.Boundary.MissingEndSlots, replay.PlayerSlot)
		}
		out.Participants = append(out.Participants, participant)
	}

	// Segment participants are sorted by PlayerSlot; keep the census contract
	// independent of that upstream order.
	slices.SortFunc(out.Participants, func(a, b TeamfightParticipant) int { return cmp.Compare(a.PlayerSlot, b.PlayerSlot) })
	finalizeTeamfightBoundary(&out.Boundary)
	return out, nil
}

// validateTeamfightRequest refuses requests that try to name the slot domain.
func validateTeamfightRequest(request ReplaySegmentRequest) error {
	if len(request.ParticipantSlots) != 0 {
		return ErrTeamfightExplicitSlots
	}
	return nil
}

// refuseTeamfightAmbiguity refuses conflicting duplicate rows instead of
// picking one silently. Conflicting hero or team samples across one
// participant's entity lifetime stay in the observed evidence: heroes are
// observed mid-pregame before hero selection, and the projection keeps those
// samples explicit.
func refuseTeamfightAmbiguity(segment ReplaySegmentEvidence) error {
	if segment.Quality.AmbiguousRows != 0 {
		return ErrTeamfightAmbiguousEvidence
	}
	return nil
}

// refuseTeamfightNonFiniteRows refuses exact source rows that carried
// non-finite values instead of silently downgrading them to missing fields.
func refuseTeamfightNonFiniteRows(rows []ReplaySegmentRow) error {
	for _, row := range rows {
		for _, scalar := range [...]ReplayScalar{row.PositionX, row.PositionY, row.PositionZ, row.FacingX, row.FacingY, row.FacingZ, row.VelocityX, row.VelocityY, row.VelocityZ} {
			if scalar.Quality == ReplayFieldMissing && scalar.MissingReason == "non_finite_source_value" {
				return ErrTeamfightNonFiniteSource
			}
		}
	}
	return nil
}

// finalizeTeamfightBoundary grades the boundary from the missing census slot
// sets: zero evidence at a bound is invalid_timestamp, partial evidence is
// incomplete, and full coverage is valid.
func finalizeTeamfightBoundary(boundary *TeamfightBoundary) {
	startInvalid := len(boundary.MissingStartSlots) == TeamfightCensusSize
	endInvalid := len(boundary.MissingEndSlots) == TeamfightCensusSize
	switch {
	case startInvalid || endInvalid:
		boundary.Status = TeamfightBoundaryInvalidTimestamp
	case len(boundary.MissingStartSlots) != 0 || len(boundary.MissingEndSlots) != 0:
		boundary.Status = TeamfightBoundaryIncomplete
	default:
		boundary.Status = TeamfightBoundaryValid
		return
	}

	// Name the failing bounds so the refusal reason stays actionable.
	switch {
	case startInvalid && endInvalid:
		boundary.Reason = "start and end ticks have no recorded rows"
	case startInvalid:
		boundary.Reason = "start tick has no recorded rows"
	case endInvalid:
		boundary.Reason = "end tick has no recorded rows"
	case len(boundary.MissingStartSlots) != 0 && len(boundary.MissingEndSlots) != 0:
		boundary.Reason = "start and end ticks do not cover every census participant"
	case len(boundary.MissingStartSlots) != 0:
		boundary.Reason = "start tick does not cover every census participant"
	default:
		boundary.Reason = "end tick does not cover every census participant"
	}
}
