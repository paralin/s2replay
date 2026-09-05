package analysis

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/paralin/s2replay"
)

// teamfightEvents builds pawn sample events for the ticks and slots.
func teamfightEvents(ticks []uint32, slots []int32) []s2replay.Event {
	events := make([]s2replay.Event, 0, len(ticks)*len(slots))
	for _, tick := range ticks {
		for _, slot := range slots {
			events = append(events, replaySample(tick, slot, 40+slot, true, uint32(100+slot), 2+slot%2, 1, 2, 3, 4, 5, 6))
		}
	}
	return events
}

func TestTeamfightEvidenceDerivesReplayLocalCensus(t *testing.T) {
	// The real replay domain in the pinned fixture is 1..12; this fixture uses
	// an arbitrary domain to pin that no 0..11 or 1..12 range is hardcoded.
	slots := []int32{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	events := teamfightEvents([]uint32{100, 110}, slots)
	out, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Participants) != 12 {
		t.Fatalf("census size: want 12, got %d", len(out.Participants))
	}
	want := []int32{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	got := make([]int32, 0, len(out.Participants))
	for _, participant := range out.Participants {
		got = append(got, participant.PlayerSlot)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("census order: want %v, got %v", want, got)
		}
	}
	if out.Participants[0].HeroID != 102 || !out.Participants[0].HasHeroID || out.Participants[0].Team != 2 || !out.Participants[0].HasTeam {
		t.Fatalf("observed hero and team: %+v", out.Participants[0])
	}
	if out.Boundary.Status != TeamfightBoundaryValid {
		t.Fatalf("full boundary coverage: %+v", out.Boundary)
	}
	for _, participant := range out.Participants {
		if participant.Status != TeamfightParticipantObserved || participant.Reason != "" {
			t.Fatalf("observed participant: %+v", participant)
		}
	}
}

func TestTeamfightEvidenceUnstableFirstSeenOrderSortedAscending(t *testing.T) {
	// Feed the same census in descending order; the output must be sorted.
	slots := []int32{13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2}
	events := teamfightEvents([]uint32{100, 110}, slots)
	out, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Participants) != 12 || out.Participants[0].PlayerSlot != 2 || out.Participants[11].PlayerSlot != 13 {
		t.Fatalf("census must be sorted ascending: %+v", out.Participants)
	}
}

func TestTeamfightEvidenceRefusesCensusOfEleven(t *testing.T) {
	events := teamfightEvents([]uint32{100, 110}, []int32{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12})
	_, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
	var census *TeamfightCensusSizeError
	if !errors.As(err, &census) || census.Observed != 11 {
		t.Fatalf("eleven-participant census must refuse with observed count: %v", err)
	}
}

func TestTeamfightEvidenceRefusesCensusOfThirteen(t *testing.T) {
	events := teamfightEvents([]uint32{100, 110}, []int32{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14})
	_, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
	var census *TeamfightCensusSizeError
	if !errors.As(err, &census) || census.Observed != 13 {
		t.Fatalf("thirteen-participant census must refuse with observed count: %v", err)
	}
}

func TestTeamfightEvidencePreservesMissingWindowRows(t *testing.T) {
	// Eleven census participants appear in the window; one census member is
	// sampled only in the lead-in. The census derives from all entity samples,
	// so slot 13 stays a census participant with an explicit missing row.
	events := teamfightEvents([]uint32{100, 110}, []int32{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12})
	events = append(events, replaySample(95, 13, 53, true, 113, 3, 1, 2, 3, 4, 5, 6))
	out, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110, LeadInTicks: 10}))
	if err != nil {
		t.Fatal(err)
	}
	missing := 0
	for _, participant := range out.Participants {
		if participant.PlayerSlot == 13 {
			if participant.Status != TeamfightParticipantMissing || participant.Reason == "" {
				t.Fatalf("missing window participant must be explicit: %+v", participant)
			}
			missing++
		} else if participant.Status != TeamfightParticipantObserved {
			t.Fatalf("window participant: %+v", participant)
		}
	}
	if missing != 1 {
		t.Fatalf("exactly one missing window participant: %d", missing)
	}
	if out.Boundary.Status != TeamfightBoundaryIncomplete {
		t.Fatalf("partial boundary coverage must be incomplete: %+v", out.Boundary)
	}
}

func TestTeamfightEvidenceBoundaryInvalidTimestamp(t *testing.T) {
	// No rows at the requested bounds: zero boundary evidence.
	out, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(teamfightEvents([]uint32{105}, []int32{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}), ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Boundary.Status != TeamfightBoundaryInvalidTimestamp || out.Boundary.Reason == "" {
		t.Fatalf("zero boundary evidence: %+v", out.Boundary)
	}
	if out.Evidence.Range.ExactStartTick != 0 || out.Evidence.Range.ExactEndTick != 0 {
		t.Fatalf("invalid timestamp boundary evidence must stay zero: %+v", out.Evidence.Range)
	}
}

func TestTeamfightEvidenceRefusesExplicitParticipantSlots(t *testing.T) {
	if _, err := ExtractTeamfightEvidenceWithBuild(nil, ReplaySegmentRequest{ParticipantSlots: []int32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}}, "", true); !errors.Is(err, ErrTeamfightExplicitSlots) {
		t.Fatalf("explicit slot list must refuse: %v", err)
	}
}

func TestTeamfightEvidenceIsByteDeterministic(t *testing.T) {
	events := teamfightEvents([]uint32{100, 110}, []int32{13, 5, 2, 9, 6, 3, 12, 7, 4, 11, 8, 10})
	request := ReplaySegmentRequest{StartTick: 100, EndTick: 110}
	first, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, request))
	if err != nil {
		t.Fatal(err)
	}
	second, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, request))
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("teamfight evidence is not byte-identical")
	}
}

func TestTeamfightEvidenceNormalizesUnselectedHeroPlaceholder(t *testing.T) {
	// The replay entity sample contract uses hero_id=0 as the pre-selection
	// placeholder; one later nonzero value is the selected hero identity.
	slots := []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	events := teamfightEvents([]uint32{100, 110}, slots)
	events[0].EntitySample.HeroID = 0
	out, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
	if err != nil {
		t.Fatal(err)
	}
	if out.IdentityStatus != TeamfightIdentityResolved || out.IdentityReason != "" {
		t.Fatalf("placeholder transition should resolve identity: %+v", out)
	}
	if out.Participants[0].HeroID != 101 || !out.Participants[0].HasHeroID || len(out.Evidence.Quality.AmbiguousParticipants) != 0 {
		t.Fatalf("placeholder transition identity: participant=%+v quality=%+v", out.Participants[0], out.Evidence.Quality)
	}
}

func TestTeamfightEvidencePlaceholderOnlyAfterFirstZero(t *testing.T) {
	// Normalization applies only when hero_id=0 was the slot's first observed
	// hero value followed by exactly one nonzero selected hero. A->0->A and
	// any nonzero-vs-nonzero conflict stay ambiguous with no substitution.
	slots := []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	t.Run("zero placeholder with repeated selected hero resolves", func(t *testing.T) {
		// 0 -> A -> A is still the proven placeholder transition; a repeated
		// identical hero is not a conflict.
		events := teamfightEvents([]uint32{100, 105, 110}, slots)
		events[0].EntitySample.HeroID = 0
		out, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
		if err != nil {
			t.Fatal(err)
		}
		if out.IdentityStatus != TeamfightIdentityResolved || out.IdentityReason != "" {
			t.Fatalf("0->A->A must resolve: %+v", out)
		}
		if !slices.Equal(out.HeroPlaceholderSubstitutions, []int32{1}) {
			t.Fatalf("0->A->A must record the substitution: %+v", out.HeroPlaceholderSubstitutions)
		}
	})
	t.Run("nonzero then zero then same nonzero stays ambiguous", func(t *testing.T) {
		// A -> 0 -> A is a real identity conflict: the zero is not a
		// pre-selection placeholder when a nonzero hero was observed first.
		events := teamfightEvents([]uint32{100, 105, 110}, slots)
		events[len(slots)].EntitySample.HeroID = 0
		out, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
		if err != nil {
			t.Fatal(err)
		}
		if out.IdentityStatus != TeamfightIdentityAmbiguous || out.IdentityReason == "" {
			t.Fatalf("A->0->A must stay ambiguous: %+v", out)
		}
		if !slices.Equal(out.HeroPlaceholderSubstitutions, []int32{}) {
			t.Fatalf("A->0->A must not record a substitution: %+v", out.HeroPlaceholderSubstitutions)
		}
		if !slices.Equal(out.Evidence.Quality.AmbiguousParticipants, []int32{1}) {
			t.Fatalf("A->0->A wrapped ambiguity must stay visible: %+v", out.Evidence.Quality.AmbiguousParticipants)
		}
	})
	t.Run("two distinct nonzero heroes stay ambiguous", func(t *testing.T) {
		events := teamfightEvents([]uint32{100, 105, 110}, slots)
		events[len(slots)].EntitySample.HeroID = 999
		out, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
		if err != nil {
			t.Fatal(err)
		}
		if out.IdentityStatus != TeamfightIdentityAmbiguous || out.IdentityReason == "" {
			t.Fatalf("nonzero conflict must stay ambiguous: %+v", out)
		}
		if !slices.Equal(out.HeroPlaceholderSubstitutions, []int32{}) {
			t.Fatalf("nonzero conflict must not record a substitution: %+v", out.HeroPlaceholderSubstitutions)
		}
	})
}

func TestTeamfightEvidenceKeepsRealHeroConflictAmbiguous(t *testing.T) {
	// A zero placeholder followed by two different nonzero heroes is a real
	// identity conflict; the placeholder normalization must not absorb it.
	slots := []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	events := teamfightEvents([]uint32{100, 105, 110}, slots)
	events[0].EntitySample.HeroID = 0
	// events[len(slots)] is tick 105 slot 1: the first selected hero.
	events[2*len(slots)].EntitySample.HeroID = 999
	out, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
	if err != nil {
		t.Fatal(err)
	}
	if out.IdentityStatus != TeamfightIdentityAmbiguous || out.IdentityReason == "" {
		t.Fatalf("real hero conflict must stay ambiguous: %+v", out)
	}
	if !slices.Equal(out.HeroPlaceholderSubstitutions, []int32{}) {
		t.Fatalf("no substitution may be recorded for a real conflict: %+v", out.HeroPlaceholderSubstitutions)
	}
	if !slices.Equal(out.Evidence.Quality.AmbiguousParticipants, []int32{1}) {
		t.Fatalf("wrapped ambiguity must stay visible: %+v", out.Evidence.Quality.AmbiguousParticipants)
	}
}

func TestTeamfightEvidenceExposesNonzeroIdentityConflict(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*s2replay.Event)
	}{
		{name: "hero", apply: func(event *s2replay.Event) { event.EntitySample.HeroID = 999 }},
		{name: "team", apply: func(event *s2replay.Event) { event.EntitySample.Team = 99 }},
	} {
		t.Run(change.name, func(t *testing.T) {
			slots := []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
			events := teamfightEvents([]uint32{100, 110}, slots)
			change.apply(&events[len(slots)])
			out, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110}))
			if err != nil {
				t.Fatal(err)
			}
			if out.IdentityStatus != TeamfightIdentityAmbiguous || out.IdentityReason == "" {
				t.Fatalf("identity conflict must be explicit: %+v", out)
			}
			if !slices.Equal(out.Evidence.Quality.AmbiguousParticipants, []int32{1}) {
				t.Fatalf("wrapped ambiguous participants hidden or changed: %+v", out.Evidence.Quality)
			}
			if out.Evidence.Eligibility != ReplayEligibilityIneligible {
				t.Fatalf("identity conflict must be launch-ineligible: %+v", out.Evidence)
			}
		})
	}
}

func TestTeamfightEvidenceRefusesAmbiguousAndNonFiniteSource(t *testing.T) {
	conflict := replaySample(100, 1, 44, true, 1, 2, 1, 2, 3, 4, 5, 6)
	other := replaySample(100, 1, 44, true, 1, 2, 9, 8, 7, 6, 5, 4)
	_, err := TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence([]s2replay.Event{conflict, other}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 100}))
	if !errors.Is(err, ErrTeamfightAmbiguousEvidence) {
		t.Fatalf("conflicting duplicate row must refuse: %v", err)
	}

	nonFinite := replaySample(100, 1, 44, true, 1, 2, 1, 2, 3, 4, 5, 6)
	nonFinite.EntitySample.VelocityX = float32(math.NaN())
	_, err = TeamfightEvidenceFromSegment(mustBuildReplaySegmentEvidence([]s2replay.Event{nonFinite}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 100}))
	if !errors.Is(err, ErrTeamfightNonFiniteSource) {
		t.Fatalf("non-finite source value must refuse: %v", err)
	}
}

func TestExtractTeamfightEvidenceRequiresFileHeader(t *testing.T) {
	demo := append([]byte("PBDEMS2\x00"), make([]byte, 8)...)
	_, err := ExtractTeamfightEvidenceWithBuild(demo, ReplaySegmentRequest{}, "fixture", true)
	if err != ErrMissingReplayHeader {
		t.Fatalf("missing header error: want %v, got %v", ErrMissingReplayHeader, err)
	}
}

func TestTeamfightEvidenceFromSegmentDoesNotMutateCallerPlaceholderHeroes(t *testing.T) {
	// The caller owns the segment value, including the unexported placeholder
	// map; the projection must not mutate it through the value copy.
	slots := []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	events := teamfightEvents([]uint32{100, 110}, slots)
	events[0].EntitySample.HeroID = 0
	segment := mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110})
	if len(segment.placeholderHeroes) != 1 || segment.placeholderHeroes[1] != 101 {
		t.Fatalf("fixture must carry one placeholder transition: %+v", segment.placeholderHeroes)
	}
	ambiguous := make([]int32, len(segment.Quality.AmbiguousParticipants))
	copy(ambiguous, segment.Quality.AmbiguousParticipants)
	if _, err := TeamfightEvidenceFromSegment(segment); err != nil {
		t.Fatal(err)
	}
	if len(segment.placeholderHeroes) != 1 || segment.placeholderHeroes[1] != 101 {
		t.Fatalf("caller placeholderHeroes map must stay unchanged: %+v", segment.placeholderHeroes)
	}
	if !slices.Equal(segment.Quality.AmbiguousParticipants, ambiguous) {
		t.Fatalf("caller ambiguous participants must stay unchanged: %+v", segment.Quality.AmbiguousParticipants)
	}
	if segment.Participants[0].HeroID != 101 || !segment.Participants[0].HasHeroID {
		t.Fatalf("caller participant hero must stay the last observed hero: %+v", segment.Participants[0])
	}
}

func TestOptInPinnedTeamfightCensusFixture(t *testing.T) {
	path := os.Getenv("S2REPLAY_PINNED_DEMO")
	if path == "" {
		t.Skip("set S2REPLAY_PINNED_DEMO to run the 63280 fixture")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// The pinned window derives the replay-local census. Delta compression
	// records only changed entities, so one pawn row exists at the bound and
	// the remaining census participants are explicitly missing rows.
	q := ReplaySegmentRequest{StartTick: 63280, EndTick: 63280}
	a, err := ExtractTeamfightEvidenceWithBuild(b, q, "fixture", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Participants) != 12 {
		t.Fatalf("pinned census: want 12, got %d", len(a.Participants))
	}
	slots := make([]int32, 0, len(a.Participants))
	for _, participant := range a.Participants {
		slots = append(slots, participant.PlayerSlot)
		if participant.PlayerSlot < 1 || participant.PlayerSlot > 12 {
			t.Fatalf("pinned census slot outside the observed replay domain: %+v", participant)
		}
		if !participant.HasHeroID || participant.HeroID == 0 || !participant.HasTeam {
			t.Fatalf("pinned census participant identity: %+v", participant)
		}
	}
	if !slices.IsSorted(slots) {
		t.Fatalf("pinned census must be sorted ascending: %v", slots)
	}
	if !slices.Contains(slots, 12) || slices.Contains(slots, 0) {
		t.Fatalf("pinned census must keep the real 1..12 domain: %v", slots)
	}
	if a.Participants[0].PlayerSlot != 1 || a.Participants[0].Status != TeamfightParticipantObserved {
		t.Fatalf("pinned slot one row: %+v", a.Participants[0])
	}
	// Slots 3 and 9 are the only census members whose hero entity field
	// carried the pre-selection zero placeholder before one selected hero.
	// The projection normalizes them locally and records the substitution;
	// the wrapped segment keeps its observed conflict.
	if !slices.Equal(a.HeroPlaceholderSubstitutions, []int32{3, 9}) {
		t.Fatalf("pinned hero placeholder substitutions: %+v", a.HeroPlaceholderSubstitutions)
	}
	if a.IdentityStatus != TeamfightIdentityResolved || a.IdentityReason != "" {
		t.Fatalf("pinned identity after placeholder normalization: %+v %+v", a.IdentityStatus, a.IdentityReason)
	}
	for _, participant := range a.Participants[1:] {
		if participant.Status != TeamfightParticipantMissing || participant.Reason == "" {
			t.Fatalf("pinned missing window participants must be explicit: %+v", participant)
		}
	}
	if a.Boundary.Status != TeamfightBoundaryIncomplete {
		t.Fatalf("pinned boundary must be incomplete: %+v", a.Boundary)
	}
	wantMissing := []int32{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	if !slices.Equal(a.Boundary.MissingStartSlots, wantMissing) || !slices.Equal(a.Boundary.MissingEndSlots, wantMissing) {
		t.Fatalf("pinned boundary missing slots: %+v", a.Boundary)
	}

	// The same window extracts byte-identical evidence.
	a2, err := ExtractTeamfightEvidenceWithBuild(b, q, "fixture", true)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(a2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("pinned teamfight evidence is not byte-identical")
	}

	// A bound outside the recorded evidence stays invalid_timestamp with the
	// zero exact-boundary evidence visible in the wrapped segment.
	outside := ReplaySegmentRequest{StartTick: 63281, EndTick: 63281}
	c, err := ExtractTeamfightEvidenceWithBuild(b, outside, "fixture", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Participants) != 12 {
		t.Fatalf("pinned outside census: want 12, got %d", len(c.Participants))
	}
	if c.Boundary.Status != TeamfightBoundaryInvalidTimestamp || c.Boundary.Reason == "" {
		t.Fatalf("pinned outside boundary: %+v", c.Boundary)
	}
	if c.Evidence.Range.ExactStartTick != 0 || c.Evidence.Range.ExactEndTick != 0 {
		t.Fatalf("pinned outside boundary evidence must stay zero: %+v", c.Evidence.Range)
	}
}
