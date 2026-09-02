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

// ErrTeamfightAmbiguousEvidence indicates duplicate source rows for one
// tick and participant disagree. Identity conflicts are returned as evidence.
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

// TeamfightIdentityStatus states whether participant hero/team identity is
// usable by a caller. The wrapped evidence remains authoritative for the
// exact conflicting participant slots.
type TeamfightIdentityStatus string

const (
	// TeamfightIdentityResolved means every observed participant identity is
	// consistent and complete.
	TeamfightIdentityResolved TeamfightIdentityStatus = "resolved"
	// TeamfightIdentityIncomplete means identity fields are absent, but no
	// conflicting nonzero values were observed.
	TeamfightIdentityIncomplete TeamfightIdentityStatus = "incomplete"
	// TeamfightIdentityAmbiguous means a participant has conflicting hero/team
	// values and the evidence is launch-ineligible.
	TeamfightIdentityAmbiguous TeamfightIdentityStatus = "ambiguous"
)

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
	SchemaVersion  int                     `json:"schema_version"`
	IdentityStatus TeamfightIdentityStatus `json:"identity_status"`
	IdentityReason string                  `json:"identity_reason,omitempty"`
	Participants   []TeamfightParticipant  `json:"participants"`
	Boundary       TeamfightBoundary       `json:"boundary"`
	// HeroPlaceholderSubstitutions records the census slots whose observed
	// pre-selection hero placeholder (hero_id=0) was replaced by the selected
	// nonzero hero in this projection. The wrapped segment evidence keeps the
	// observed zero and its identity conflict.
	HeroPlaceholderSubstitutions []int32               `json:"hero_placeholder_substitutions,omitempty"`
	Evidence                     ReplaySegmentEvidence `json:"evidence"`
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
//
// The replay entity sample carries hero_id=0 as the pre-selection placeholder.
// A 0 -> one nonzero transition is normalized here, in the Teamfight
// projection, with the substitution recorded on the top-level evidence. The
// wrapped ReplaySegmentEvidence keeps the established contract: the placeholder
// stays a participant identity conflict, and Quality.AmbiguousParticipants
// keeps naming those slots.
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
	segment = normalizeTeamfightHeroPlaceholder(segment)
	slots := make([]int32, 0, len(segment.placeholderHeroes))
	for slot := range segment.placeholderHeroes {
		slots = append(slots, slot)
	}
	slices.Sort(slots)
	identityStatus, identityReason := teamfightIdentityStatus(segment)
	if identityStatus == TeamfightIdentityAmbiguous {
		// Identity conflicts are a valid evidence result, not a reason to drop
		// the source rows. They must nevertheless be launch-ineligible even
		// when the caller did not declare a freshness policy.
		segment.Eligibility = ReplayEligibilityIneligible
		segment.EligibilityReasons = append(segment.EligibilityReasons, "ambiguous participant identity")
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
		SchemaVersion:                TeamfightEvidenceSchemaVersion,
		IdentityStatus:               identityStatus,
		IdentityReason:               identityReason,
		Participants:                 make([]TeamfightParticipant, 0, TeamfightCensusSize),
		HeroPlaceholderSubstitutions: slots,
		Evidence:                     segment,
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

// refuseTeamfightAmbiguity refuses conflicting duplicate source rows instead
// of picking one silently. Hero/team identity conflicts are deliberately not
// refused here: TeamfightEvidenceFromSegment exposes them at the top level
// while retaining Quality.AmbiguousParticipants in the wrapped evidence.
func refuseTeamfightAmbiguity(segment ReplaySegmentEvidence) error {
	if segment.Quality.AmbiguousRows != 0 {
		return ErrTeamfightAmbiguousEvidence
	}
	return nil
}

// normalizeTeamfightHeroPlaceholder resolves the pre-selection hero
// placeholder for projection only. The replay contract uses hero_id=0 as the
// placeholder before selection; a transition from 0 to exactly one nonzero
// hero is not a real identity conflict. Each such participant keeps the
// selected hero, the substitution is recorded in HeroPlaceholderSubstitutions,
// and the wrapped segment keeps its observed zero. Any other hero conflict,
// including 0 -> nonzero -> different nonzero, stays ambiguous.
func normalizeTeamfightHeroPlaceholder(segment ReplaySegmentEvidence) ReplaySegmentEvidence {
	if len(segment.placeholderHeroes) == 0 {
		return segment
	}
	for i := range segment.Participants {
		participant := &segment.Participants[i]
		hero, ok := segment.placeholderHeroes[participant.PlayerSlot]
		// The recorded substitution is only the proven final identity when the
		// last observed hero is still the selected one; a later real conflict
		// must keep its ambiguity.
		if ok && participant.HasHeroID && participant.HeroID == hero {
			participant.HeroID, participant.HasHeroID = hero, true
		} else {
			delete(segment.placeholderHeroes, participant.PlayerSlot)
		}
	}
	remaining := make([]int32, 0, len(segment.Quality.AmbiguousParticipants))
	for _, slot := range segment.Quality.AmbiguousParticipants {
		if _, ok := segment.placeholderHeroes[slot]; !ok {
			remaining = append(remaining, slot)
		}
	}
	segment.Quality.AmbiguousParticipants = remaining
	return segment
}

func teamfightIdentityStatus(segment ReplaySegmentEvidence) (TeamfightIdentityStatus, string) {
	if len(segment.Quality.AmbiguousParticipants) != 0 {
		return TeamfightIdentityAmbiguous, "hero or team identity conflicts for participant slots " + formatTeamfightSlots(segment.Quality.AmbiguousParticipants)
	}
	missing := make([]int32, 0)
	for _, participant := range segment.Participants {
		if !participant.HasHeroID || participant.HeroID == 0 || !participant.HasTeam {
			missing = append(missing, participant.PlayerSlot)
		}
	}
	if len(missing) != 0 {
		return TeamfightIdentityIncomplete, "hero or team identity is missing for participant slots " + formatTeamfightSlots(missing)
	}
	return TeamfightIdentityResolved, ""
}

func formatTeamfightSlots(slots []int32) string {
	text := "["
	for i, slot := range slots {
		if i != 0 {
			text += ","
		}
		text += strconv.FormatInt(int64(slot), 10)
	}
	return text + "]"
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
