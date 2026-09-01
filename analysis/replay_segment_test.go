package analysis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/paralin/s2replay"
)

func TestBuildReplaySegmentEvidenceIsDeterministicAndKeepsBoundaries(t *testing.T) {
	events := []s2replay.Event{
		replaySample(90, 4, 44, true, 7, 2, 0, 0, 0, 0, 0, 0),
		replaySample(100, 4, 44, true, 7, 2, 12, 24, 0, 100, 101, 102),
		replaySample(110, 4, 44, true, 7, 2, 13, 25, 1, 110, 111, 112),
		replaySample(120, 4, 44, true, 7, 2, 14, 26, 2, 120, 121, 122),
	}
	events[1].EntitySample.PositionXTick = 90
	request := ReplaySegmentRequest{StartTick: 100, EndTick: 110, LeadInTicks: 20, ParticipantSlots: []int32{4}}
	source := ReplaySourceIdentity{SHA256: "pinned", MatchID: 123, Map: "dl_midtown", GameBuild: 42, Parser: "test"}
	first := mustBuildReplaySegmentEvidence(events, source, request)
	second := mustBuildReplaySegmentEvidence(events, source, request)
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("replay segment evidence is not byte-identical")
	}
	if got := len(first.Rows); got != 31 {
		t.Fatalf("dense lead-in and boundary rows: want 31, got %d", got)
	}
	if first.Rows[0].Tick != 80 || !first.Rows[0].LeadIn || first.Rows[10].Tick != 90 || !first.Rows[10].LeadIn || first.Rows[20].Tick != 100 || first.Rows[20].LeadIn || first.Rows[30].Tick != 110 || first.Rows[30].LeadIn {
		t.Fatalf("dense lead-in and boundary rows: %+v", first.Rows)
	}
	if !first.Quality.ExactStartPresent || !first.Quality.ExactEndPresent || first.Quality.LeadInRows != 20 || first.Quality.RequestedRows != 11 {
		t.Fatalf("boundary quality: %+v", first.Quality)
	}
	if first.Rows[30].VelocityX.SampleTick != 110 || first.Rows[30].VelocityX.FreshnessTicks != 0 {
		t.Fatalf("velocity source freshness: %+v", first.Rows[30].VelocityX)
	}
	if first.Rows[0].PositionX.Quality != ReplayFieldMissing || first.Rows[0].PositionX.MissingReason != "no_entity_sample_at_tick" || first.Rows[0].VelocityZ.Quality != ReplayFieldMissing {
		t.Fatalf("synthetic missing row quality: %+v", first.Rows[0])
	}
	if first.Rows[20].PositionX.SampleTick != 90 || first.Rows[20].PositionX.FreshnessTicks != 10 {
		t.Fatalf("position source freshness: %+v", first.Rows[20].PositionX)
	}
	if first.Rows[20].PositionX.SourceField != "CBodyComponent.m_skeletonInstance.m_cellX+CBodyComponent.m_skeletonInstance.m_vecX" {
		t.Fatalf("position source paths: %+v", first.Rows[20].PositionX)
	}
	if first.Range.LeadInTicks != 20 || first.Range.RequestedLeadInTicks != 20 {
		t.Fatalf("lead-in span: %+v", first.Range)
	}
	if first.Quality.ExactVelocityRows != 3 || first.Quality.ExactFacingRows != 3 {
		t.Fatalf("exact quality: %+v", first.Quality)
	}
}

func TestBuildReplaySegmentEvidenceLabelsMissingVelocity(t *testing.T) {
	evidence := mustBuildReplaySegmentEvidence([]s2replay.Event{replaySample(10, 1, 7, false, 0, 0, 0, 0, 0, 0, 0, 0)}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 10, EndTick: 10})
	if evidence.Rows[0].VelocityX.Quality != ReplayFieldMissing {
		t.Fatalf("velocity quality: want missing, got %s", evidence.Rows[0].VelocityX.Quality)
	}
	if evidence.Rows[0].VelocityX.MissingReason == "" {
		t.Fatal("missing velocity needs a reason")
	}
}

func replaySample(tick uint32, slot, entity int32, exact bool, hero uint32, team int32, fx, fy, fz, vx, vy, vz float32) s2replay.Event {
	return s2replay.Event{
		Type: s2replay.EventEntitySample, Tick: tick, GameTime: float64(tick) / 64,
		Entity: entity, PlayerSlot: slot,
		EntitySample: &s2replay.EntitySample{
			Entity: entity, ClassName: "CCitadelPlayerPawn", HeroID: hero, Team: team,
			HasHeroID: true, HasTeam: true, HasPosition: true,
			PositionX: fx, PositionY: fy, PositionZ: fz,
			FacingX: fx, FacingY: fy, FacingZ: fz, HasFacing: exact,
			FacingXSourceField: "m_angEyeAngles", FacingYSourceField: "m_angEyeAngles", FacingZSourceField: "m_angEyeAngles",
			VelocityXSourceField: "m_vecVelocity.m_vecX", VelocityYSourceField: "m_vecVelocity.m_vecY", VelocityZSourceField: "m_vecVelocity.m_vecZ",
			PositionXSourceField: "CBodyComponent.m_skeletonInstance.m_cellX+CBodyComponent.m_skeletonInstance.m_vecX",
			PositionYSourceField: "CBodyComponent.m_skeletonInstance.m_cellY+CBodyComponent.m_skeletonInstance.m_vecY",
			PositionZSourceField: "CBodyComponent.m_skeletonInstance.m_cellZ+CBodyComponent.m_skeletonInstance.m_vecZ",
			FacingXTick:          tick, FacingYTick: tick, FacingZTick: tick,
			VelocityX: vx, VelocityY: vy, VelocityZ: vz, HasVelocity: exact,
			VelocityXTick: tick, VelocityYTick: tick, VelocityZTick: tick,
		},
	}
}

func TestBuildReplaySegmentEvidenceReportsAbsentBoundary(t *testing.T) {
	evidence := mustBuildReplaySegmentEvidence([]s2replay.Event{replaySample(101, 1, 7, true, 0, 2, 1, 2, 3, 4, 5, 6)}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110})
	if evidence.Range.ExactStartTick != 0 || evidence.Range.ExactEndTick != 0 {
		t.Fatalf("absent boundaries must not claim exact ticks: %+v", evidence.Range)
	}
	if evidence.Quality.ExactStartPresent || evidence.Quality.ExactEndPresent || evidence.Quality.BoundaryMissingReason == "" {
		t.Fatalf("absent boundary quality: %+v", evidence.Quality)
	}
}

func TestBuildReplaySegmentEvidenceRejectsStaleExactFieldsForEligibility(t *testing.T) {
	event := replaySample(100, 1, 7, true, 1, 2, 1, 2, 3, 4, 5, 6)
	event.EntitySample.FacingXTick = 0
	event.EntitySample.FacingYTick = 0
	event.EntitySample.FacingZTick = 0
	event.EntitySample.VelocityXTick = 0
	event.EntitySample.VelocityYTick = 0
	event.EntitySample.VelocityZTick = 0
	evidence := mustBuildReplaySegmentEvidence([]s2replay.Event{event}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 100, MaxFreshnessTicks: uint32Ptr(10)})
	if evidence.Eligibility != ReplayEligibilityIneligible || len(evidence.EligibilityReasons) == 0 {
		t.Fatalf("stale exact fields must be ineligible: %+v", evidence)
	}
	if evidence.Rows[0].FacingX.SampleTick != 0 || evidence.Rows[0].FacingX.FreshnessTicks != 100 {
		t.Fatalf("source freshness was not retained: %+v", evidence.Rows[0].FacingX)
	}
}

func TestBuildReplaySegmentEvidenceReportsIdentityCorrespondence(t *testing.T) {
	source := ReplaySourceIdentity{SHA256: "0000000000000000000000000000000000000000000000000000000000000000", Map: "start", GameBuild: 10854}
	evidence := mustBuildReplaySegmentEvidence(nil, source, ReplaySegmentRequest{ExpectedIdentity: &ReplayIdentityExpectation{SHA256: source.SHA256, Map: "dl_midtown", GameBuild: 6684}})
	if evidence.Correspondence.Status != ReplayCorrespondenceMismatched {
		t.Fatalf("identity mismatch: %+v", evidence.Correspondence)
	}
}

func TestExtractReplaySegmentEvidenceRequiresFileHeader(t *testing.T) {
	demo := append([]byte("PBDEMS2\x00"), make([]byte, 8)...)
	_, err := ExtractReplaySegmentEvidence(demo, ReplaySegmentRequest{})
	if err != ErrMissingReplayHeader {
		t.Fatalf("missing header error: want %v, got %v", ErrMissingReplayHeader, err)
	}
}

func TestBuildReplaySegmentEvidenceRetainsEntityReplacements(t *testing.T) {
	events := []s2replay.Event{replaySample(10, 1, 44, true, 1, 2, 1, 2, 3, 4, 5, 6), replaySample(20, 1, 45, true, 1, 2, 1, 2, 3, 4, 5, 6)}
	evidence := mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 10, EndTick: 20})
	got := evidence.Participants[0].HistoricalEntityIDs
	if len(got) != 2 || got[0] != 44 || got[1] != 45 {
		t.Fatalf("entity replacements: %+v", evidence.Participants[0])
	}
}

func uint32Ptr(value uint32) *uint32 { return &value }

func TestBuildReplaySegmentEvidenceRequiresEveryRequestedParticipant(t *testing.T) {
	evidence := mustBuildReplaySegmentEvidence([]s2replay.Event{replaySample(100, 4, 44, true, 1, 2, 1, 2, 3, 4, 5, 6), replaySample(110, 4, 44, true, 1, 2, 1, 2, 3, 4, 5, 6)}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110, ParticipantSlots: []int32{4, 5}})
	if evidence.Quality.ExactStartPresent || evidence.Quality.ExactEndPresent {
		t.Fatalf("missing requested participant must prevent exact boundaries: %+v", evidence.Quality)
	}
	if len(evidence.Quality.MissingParticipantSlots) != 1 || evidence.Quality.MissingParticipantSlots[0] != 5 {
		t.Fatalf("missing requested participants: %+v", evidence.Quality)
	}
}

func TestBuildReplaySegmentEvidenceReportsFallbackPositionPath(t *testing.T) {
	event := replaySample(10, 1, 7, true, 1, 2, 1, 2, 3, 4, 5, 6)
	event.EntitySample.PositionXSourceField = "CBodyComponent.m_cellX+CBodyComponent.m_vecX"
	evidence := mustBuildReplaySegmentEvidence([]s2replay.Event{event}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 10, EndTick: 10})
	if evidence.Rows[0].PositionX.SourceField != event.EntitySample.PositionXSourceField {
		t.Fatalf("fallback source path was relabeled: %+v", evidence.Rows[0].PositionX)
	}
}

func TestBuildReplaySegmentEvidenceReportsTruncatedLeadIn(t *testing.T) {
	evidence := mustBuildReplaySegmentEvidence(nil, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 10, EndTick: 20, LeadInTicks: 20})
	if evidence.Range.LeadInStartTick != 0 || evidence.Range.LeadInTicks != 10 || evidence.Range.RequestedLeadInTicks != 20 {
		t.Fatalf("truncated lead-in span: %+v", evidence.Range)
	}
}

func TestBuildReplaySegmentEvidenceRequiresDenseRequestedRows(t *testing.T) {
	events := []s2replay.Event{replaySample(100, 4, 44, true, 1, 2, 1, 2, 3, 4, 5, 6), replaySample(100, 5, 45, true, 2, 3, 1, 2, 3, 4, 5, 6), replaySample(110, 4, 44, true, 1, 2, 1, 2, 3, 4, 5, 6)}
	evidence := mustBuildReplaySegmentEvidence(events, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110, MaxFreshnessTicks: uint32Ptr(0)})
	missing := false
	for _, row := range evidence.Quality.MissingRequestedRows {
		if row.Tick == 110 && row.PlayerSlot == 5 {
			missing = true
			break
		}
	}
	if !missing || evidence.Quality.MissingRequestedRowsTotal == 0 {
		t.Fatalf("missing interior row: %+v", evidence.Quality)
	}
	if evidence.Eligibility != ReplayEligibilityIneligible {
		t.Fatalf("dense missing row must be ineligible: %+v", evidence)
	}
}

func TestBuildReplaySegmentEvidenceDistinguishesNilAndZeroFreshness(t *testing.T) {
	event := replaySample(100, 1, 7, true, 1, 2, 1, 2, 3, 4, 5, 6)
	none := mustBuildReplaySegmentEvidence([]s2replay.Event{event}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 100})
	if none.Eligibility != ReplayEligibilityNotDeclared {
		t.Fatalf("nil freshness declaration: %s", none.Eligibility)
	}
	strict := mustBuildReplaySegmentEvidence([]s2replay.Event{event}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 100, MaxFreshnessTicks: uint32Ptr(0), ExpectedIdentity: &ReplayIdentityExpectation{SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}})
	if strict.Eligibility != ReplayEligibilityIneligible {
		t.Fatalf("strict freshness declaration: %s", strict.Eligibility)
	}
}

func TestBuildReplaySegmentEvidenceRejectsNonCanonicalExpectedSHA(t *testing.T) {
	evidence := mustBuildReplaySegmentEvidence(nil, ReplaySourceIdentity{}, ReplaySegmentRequest{ExpectedIdentity: &ReplayIdentityExpectation{SHA256: "ABC"}})
	if evidence.Correspondence.Status != ReplayCorrespondenceMismatched {
		t.Fatalf("non-canonical expected SHA: %+v", evidence.Correspondence)
	}
}

func TestBuildReplaySegmentEvidenceAllowsNullableMatchIDWithReason(t *testing.T) {
	source := ReplaySourceIdentity{SHA256: "0000000000000000000000000000000000000000000000000000000000000000", Map: "start", GameBuild: 10854, MatchIDMissingReason: "upload source has no match id"}
	expected := &ReplayIdentityExpectation{SHA256: source.SHA256, Map: source.Map, GameBuild: source.GameBuild}
	evidence := mustBuildReplaySegmentEvidence(nil, source, ReplaySegmentRequest{ExpectedIdentity: expected})
	if evidence.Source.MatchID != 0 || evidence.Source.MatchIDMissingReason == "" {
		t.Fatalf("nullable match identity: %+v", evidence.Source)
	}
	if evidence.Correspondence.Status != ReplayCorrespondenceMatched {
		t.Fatalf("matchless upload correspondence: %+v", evidence.Correspondence)
	}
}

func TestBuildReplaySegmentEvidenceDoesNotMatchMatchIDOnly(t *testing.T) {
	matchID := uint64(101514223)
	evidence := mustBuildReplaySegmentEvidence(nil, ReplaySourceIdentity{MatchID: matchID}, ReplaySegmentRequest{ExpectedIdentity: &ReplayIdentityExpectation{MatchID: &matchID}})
	if evidence.Correspondence.Status != ReplayCorrespondencePending || evidence.Eligibility != ReplayEligibilityNotDeclared {
		t.Fatalf("match ID alone cannot establish correspondence: %+v", evidence)
	}
}

func TestBuildReplaySegmentEvidenceCoalescesRowsByTickAndSlot(t *testing.T) {
	first := replaySample(100, 1, 44, true, 1, 2, 1, 2, 3, 4, 5, 6)
	second := replaySample(100, 1, 44, true, 1, 2, 9, 8, 7, 6, 5, 4)
	evidence := mustBuildReplaySegmentEvidence([]s2replay.Event{first, second}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 100})
	if len(evidence.Rows) != 1 || evidence.Quality.CoalescedRows != 1 || evidence.Quality.AmbiguousRows != 1 || evidence.Rows[0].PositionX.Value != 9 {
		t.Fatalf("conflicting coalesced rows: %+v", evidence)
	}
}

func TestBuildReplaySegmentEvidenceEpochsUseEntitySerial(t *testing.T) {
	first := replaySample(100, 1, 44, true, 1, 2, 1, 2, 3, 4, 5, 6)
	first.EntitySample.EntitySerial = 3
	second := replaySample(101, 1, 44, true, 1, 2, 9, 8, 7, 6, 5, 4)
	second.EntitySample.EntitySerial = 4
	evidence := mustBuildReplaySegmentEvidence([]s2replay.Event{first, second}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 101})
	if len(evidence.Participants[0].Epochs) != 2 || evidence.Participants[0].Epochs[0].Serial != 3 || evidence.Participants[0].Epochs[1].Serial != 4 {
		t.Fatalf("serial epochs: %+v", evidence.Participants[0])
	}
}

func TestBuildReplaySegmentEvidenceRejectsAmbiguousCoalescedRows(t *testing.T) {
	first := replaySample(100, 1, 44, true, 1, 2, 1, 2, 3, 4, 5, 6)
	second := replaySample(100, 1, 44, false, 1, 2, 9, 8, 7, 6, 5, 4)
	evidence := mustBuildReplaySegmentEvidence([]s2replay.Event{first, second}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 100, MaxFreshnessTicks: uint32Ptr(0), ExpectedIdentity: &ReplayIdentityExpectation{SHA256: "0000000000000000000000000000000000000000000000000000000000000000", Map: "m", GameBuild: 1}})
	if evidence.Quality.AmbiguousRows != 1 || evidence.Eligibility != ReplayEligibilityIneligible {
		t.Fatalf("ambiguous coalesced row: %+v", evidence)
	}
}

func TestValidateReplaySegmentRowsRejectsOverflow(t *testing.T) {
	if err := validateReplaySegmentRows(ReplaySegmentRequest{StartTick: 0, EndTick: ^uint32(0)}, 2); err == nil {
		t.Fatal("overflowing dense range must fail")
	}
	if err := validateReplaySegmentRows(ReplaySegmentRequest{StartTick: 0, EndTick: MaxReplaySegmentRows}, 2); err == nil {
		t.Fatal("overlarge dense range must fail")
	}
}

func mustBuildReplaySegmentEvidence(events []s2replay.Event, source ReplaySourceIdentity, request ReplaySegmentRequest) ReplaySegmentEvidence {
	e, err := buildReplaySegmentEvidence(events, source, request)
	if err != nil {
		panic(err)
	}
	return e
}

func TestExtractReplaySegmentEvidenceBuildIdentityPolicy(t *testing.T) {
	if _, err := extractReplaySegmentEvidenceWithBuild(nil, ReplaySegmentRequest{}, "", false); err == nil {
		t.Fatal("modified build must be refused")
	}
	if _, err := extractReplaySegmentEvidenceWithBuild(nil, ReplaySegmentRequest{}, "clean", true); err == nil {
		t.Fatal("clean build should proceed to input validation")
	}
}

type failingReplayParser struct{ released, mode int }

func (p *failingReplayParser) SetEventMode(bool)     { p.mode++ }
func (p *failingReplayParser) ReleasePendingQueues() { p.released++ }
func (p *failingReplayParser) NextEvent() (s2replay.Event, error) {
	return s2replay.Event{}, context.Canceled
}

func TestConsumeReplayEventsReleasesQueuesOnError(t *testing.T) {
	parser := &failingReplayParser{}
	accepted := 0
	err := consumeReplayEvents(parser, func(s2replay.Event) { accepted++ })
	if !errors.Is(err, context.Canceled) || accepted != 0 || parser.released != 1 || parser.mode != 1 {
		t.Fatalf("cleanup: err=%v accepted=%d released=%d mode=%d", err, accepted, parser.released, parser.mode)
	}
}

func TestOptInPinnedVelocityFixture(t *testing.T) {
	path := os.Getenv("S2REPLAY_PINNED_DEMO")
	if path == "" {
		t.Skip("set S2REPLAY_PINNED_DEMO to run the 63280 fixture")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	q := ReplaySegmentRequest{StartTick: 63280, EndTick: 63280}
	a, err := extractReplaySegmentEvidenceWithBuild(b, q, "fixture", true)
	if err != nil {
		t.Fatal(err)
	}
	j, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := extractReplaySegmentEvidenceWithBuild(b, q, "fixture", true)
	if err != nil {
		t.Fatal(err)
	}
	j2, _ := json.Marshal(b2)
	if !bytes.Equal(j, j2) {
		t.Fatal("fixture output is not deterministic")
	}
	const pinnedSHA = "b612e43f4055d4dde728c7eedbdd7ec38c3478ef90f33b870bfb29310b79194f"
	if got := sha256.Sum256(b); hex.EncodeToString(got[:]) != pinnedSHA {
		t.Fatalf("fixture source SHA256: got %x want %s", got, pinnedSHA)
	}
	if a.Source.SHA256 != pinnedSHA || a.Source.MatchID != 101514223 || a.Source.Map != "start" || a.Source.GameBuild != 10854 {
		t.Fatalf("fixture identity: %+v", a.Source)
	}
	if len(a.Rows) != 12 || a.Quality.ExactVelocityRows == 0 {
		t.Fatalf("fixture quality: %+v", a.Quality)
	}
	var slotOne *ReplaySegmentRow
	for i := range a.Rows {
		if a.Rows[i].PlayerSlot == 1 && a.Rows[i].EntityID == 92 {
			slotOne = &a.Rows[i]
			break
		}
	}
	if slotOne == nil || slotOne.Tick != 63280 || slotOne.VelocityX.Value != float32(-52.98462) || slotOne.VelocityY.Value != float32(-339.14185) || slotOne.VelocityZ.Value != 0 || slotOne.VelocityX.SampleTick != 63280 || slotOne.VelocityY.SampleTick != 63240 || slotOne.VelocityZ.SampleTick != 62285 || slotOne.VelocityX.SourceField != "m_vecVelocity.m_vecX" || slotOne.VelocityY.SourceField != "m_vecVelocity.m_vecY" || slotOne.VelocityZ.SourceField != "m_vecVelocity.m_vecZ" {
		t.Fatalf("fixture exact velocity row: %+v", slotOne)
	}
	sum := sha256.Sum256(j)
	if len(j) != 20876 || hex.EncodeToString(sum[:]) != "9b889f3842020f3c722ed4f38af23f4d5252308d36229f4637d9e3e57f661486" {
		t.Fatalf("fixture bytes=%d SHA256=%x", len(j), sum)
	}
}
