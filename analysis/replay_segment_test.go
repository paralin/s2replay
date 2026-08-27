package analysis

import (
	"bytes"
	"encoding/json"
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
	first := BuildReplaySegmentEvidence(events, source, request)
	second := BuildReplaySegmentEvidence(events, source, request)
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
	if got := len(first.Rows); got != 3 {
		t.Fatalf("lead-in and boundary rows: want 3, got %d", got)
	}
	if first.Rows[0].Tick != 90 || !first.Rows[0].LeadIn || first.Rows[1].Tick != 100 || first.Rows[1].LeadIn || first.Rows[2].Tick != 110 || first.Rows[2].LeadIn {
		t.Fatalf("lead-in and boundary rows: %+v", first.Rows)
	}
	if !first.Quality.ExactStartPresent || !first.Quality.ExactEndPresent || first.Quality.LeadInRows != 1 || first.Quality.RequestedRows != 2 {
		t.Fatalf("boundary quality: %+v", first.Quality)
	}
	if first.Rows[2].VelocityX.SampleTick != 110 || first.Rows[2].VelocityX.FreshnessTicks != 0 {
		t.Fatalf("velocity source freshness: %+v", first.Rows[2].VelocityX)
	}
	if first.Rows[1].PositionX.SampleTick != 90 || first.Rows[1].PositionX.FreshnessTicks != 10 {
		t.Fatalf("position source freshness: %+v", first.Rows[1].PositionX)
	}
	if first.Rows[1].PositionX.SourceField != "CBodyComponent.m_skeletonInstance.m_cellX+CBodyComponent.m_skeletonInstance.m_vecX" {
		t.Fatalf("position source paths: %+v", first.Rows[1].PositionX)
	}
	if first.Range.LeadInTicks != 20 || first.Range.RequestedLeadInTicks != 20 {
		t.Fatalf("lead-in span: %+v", first.Range)
	}
	if first.Quality.ExactVelocityRows != 3 || first.Quality.ExactFacingRows != 3 {
		t.Fatalf("exact quality: %+v", first.Quality)
	}
}

func TestBuildReplaySegmentEvidenceLabelsMissingVelocity(t *testing.T) {
	evidence := BuildReplaySegmentEvidence([]s2replay.Event{replaySample(10, 1, 7, false, 0, 0, 0, 0, 0, 0, 0, 0)}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 10, EndTick: 10})
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
			FacingXTick: tick, FacingYTick: tick, FacingZTick: tick,
			VelocityX: vx, VelocityY: vy, VelocityZ: vz, HasVelocity: exact,
			VelocityXTick: tick, VelocityYTick: tick, VelocityZTick: tick,
		},
	}
}

func TestBuildReplaySegmentEvidenceReportsAbsentBoundary(t *testing.T) {
	evidence := BuildReplaySegmentEvidence([]s2replay.Event{replaySample(101, 1, 7, true, 0, 2, 1, 2, 3, 4, 5, 6)}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110})
	if evidence.Range.ExactStartTick != 0 || evidence.Range.ExactEndTick != 0 {
		t.Fatalf("absent boundaries must not claim exact ticks: %+v", evidence.Range)
	}
	if evidence.Quality.ExactStartPresent || evidence.Quality.ExactEndPresent || evidence.Quality.BoundaryMissingReason == "" {
		t.Fatalf("absent boundary quality: %+v", evidence.Quality)
	}
}

func TestExtractReplaySegmentEvidenceRequiresParserRevision(t *testing.T) {
	demo := append([]byte("PBDEMS2\x00"), make([]byte, 8)...)
	_, err := ExtractReplaySegmentEvidence(demo, ReplaySegmentRequest{EndTick: 1})
	if err == nil {
		t.Fatal("parser revision must be explicit")
	}
	evidence, err := ExtractReplaySegmentEvidence(demo, ReplaySegmentRequest{EndTick: 1, ParserRevision: "test-revision"})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Source.ParserRevision != "test-revision" {
		t.Fatalf("parser revision: %+v", evidence.Source)
	}
}

func TestBuildReplaySegmentEvidenceRequiresEveryRequestedParticipant(t *testing.T) {
	evidence := BuildReplaySegmentEvidence([]s2replay.Event{replaySample(100, 4, 44, true, 1, 2, 1, 2, 3, 4, 5, 6), replaySample(110, 4, 44, true, 1, 2, 1, 2, 3, 4, 5, 6)}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 100, EndTick: 110, ParticipantSlots: []int32{4, 5}})
	if evidence.Quality.ExactStartPresent || evidence.Quality.ExactEndPresent {
		t.Fatalf("missing requested participant must prevent exact boundaries: %+v", evidence.Quality)
	}
	if len(evidence.Quality.MissingParticipantSlots) != 1 || evidence.Quality.MissingParticipantSlots[0] != 5 {
		t.Fatalf("missing requested participants: %+v", evidence.Quality)
	}
	if len(evidence.Participants) != 2 || evidence.Participants[1].HistoricalEntityID != -1 {
		t.Fatalf("requested participants: %+v", evidence.Participants)
	}
}

func TestBuildReplaySegmentEvidenceRecordsFallbackPositionPaths(t *testing.T) {
	event := replaySample(10, 1, 7, true, 1, 2, 1, 2, 3, 4, 5, 6)
	event.EntitySample.PositionXSourceField = "CBodyComponent.m_cellX+CBodyComponent.m_vecX"
	evidence := BuildReplaySegmentEvidence([]s2replay.Event{event}, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 10, EndTick: 10})
	if evidence.Rows[0].PositionX.SourceField != "CBodyComponent.m_cellX+CBodyComponent.m_vecX" {
		t.Fatalf("fallback source path was relabeled: %+v", evidence.Rows[0].PositionX)
	}
}

func TestBuildReplaySegmentEvidenceRecordsTruncatedLeadIn(t *testing.T) {
	evidence := BuildReplaySegmentEvidence(nil, ReplaySourceIdentity{}, ReplaySegmentRequest{StartTick: 10, EndTick: 20, LeadInTicks: 20})
	if evidence.Range.LeadInStartTick != 0 || evidence.Range.LeadInTicks != 10 || evidence.Range.RequestedLeadInTicks != 20 {
		t.Fatalf("truncated lead-in span: %+v", evidence.Range)
	}
}
