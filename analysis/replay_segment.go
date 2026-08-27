package analysis

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"slices"

	"github.com/paralin/s2replay"
	"github.com/paralin/s2replay/protocol"
)

// ReplaySegmentEvidenceSchemaVersion identifies the replay segment contract.
const ReplaySegmentEvidenceSchemaVersion = 1

// ReplayFieldQuality describes whether a scalar came directly from the replay.
type ReplayFieldQuality string

const (
	// ReplayFieldExact marks a value decoded from a networked replay field.
	ReplayFieldExact ReplayFieldQuality = "exact"
	// ReplayFieldMissing marks a field absent from the source sample.
	ReplayFieldMissing ReplayFieldQuality = "missing"
	// ReplayFieldDerived marks a computed value that cannot satisfy exact evidence.
	ReplayFieldDerived ReplayFieldQuality = "derived"
)

// ReplaySegmentRequest selects an inclusive tick range and optional lead-in.
type ReplaySegmentRequest struct {
	StartTick        uint32  `json:"start_tick"`
	EndTick          uint32  `json:"end_tick"`
	LeadInTicks      uint32  `json:"lead_in_ticks"`
	ParticipantSlots []int32 `json:"participant_slots,omitempty"`
	ParserRevision   string  `json:"parser_revision"`
}

// ReplaySourceIdentity is immutable identity for the bytes and parser build.
type ReplaySourceIdentity struct {
	SHA256            string `json:"sha256"`
	MatchID           uint64 `json:"match_id"`
	Game              string `json:"game"`
	Map               string `json:"map"`
	GameBuild         int32  `json:"game_build"`
	Parser            string `json:"parser"`
	ParserRevision    string `json:"parser_revision"`
	MapCorrespondence string `json:"map_correspondence"`
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
	PlayerSlot         int32  `json:"player_slot"`
	HeroID             uint32 `json:"hero_id,omitempty"`
	Team               int32  `json:"team,omitempty"`
	HasHeroID          bool   `json:"has_hero_id"`
	HasTeam            bool   `json:"has_team"`
	HistoricalEntityID int32  `json:"historical_entity_id"`
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
	Tick       uint32  `json:"tick"`
	GameTime   float64 `json:"game_time"`
	LeadIn     bool    `json:"lead_in"`
	PlayerSlot int32   `json:"player_slot"`
	EntityID   int32   `json:"entity_id"`

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

// ReplaySegmentQuality summarizes source coverage without deriving motion.
type ReplaySegmentQuality struct {
	Rows                    int      `json:"rows"`
	LeadInRows              int      `json:"lead_in_rows"`
	RequestedRows           int      `json:"requested_rows"`
	Participants            int      `json:"participants"`
	ExactFacingRows         int      `json:"exact_facing_rows"`
	ExactVelocityRows       int      `json:"exact_velocity_rows"`
	MissingFacingRows       int      `json:"missing_facing_rows"`
	MissingVelocityRows     int      `json:"missing_velocity_rows"`
	ExactStartPresent       bool     `json:"exact_start_present"`
	ExactEndPresent         bool     `json:"exact_end_present"`
	BoundaryMissingReason   string   `json:"boundary_missing_reason,omitempty"`
	MissingParticipantSlots []int32  `json:"missing_participant_slots,omitempty"`
	MissingFields           []string `json:"missing_fields,omitempty"`
}

// ReplaySegmentEvidence is versioned, replay-local evidence for one range.
type ReplaySegmentEvidence struct {
	SchemaVersion int                  `json:"schema_version"`
	Source        ReplaySourceIdentity `json:"source"`
	Range         ReplaySegmentRange   `json:"range"`
	Participants  []ReplayParticipant  `json:"participants"`
	Rows          []ReplaySegmentRow   `json:"rows"`
	Quality       ReplaySegmentQuality `json:"quality"`
}

// ExtractReplaySegmentEvidence parses immutable demo bytes and extracts a range.
func ExtractReplaySegmentEvidence(demo []byte, request ReplaySegmentRequest) (ReplaySegmentEvidence, error) {
	if request.StartTick > request.EndTick {
		return ReplaySegmentEvidence{}, errors.New("replay segment start tick exceeds end tick")
	}
	if request.ParserRevision == "" {
		return ReplaySegmentEvidence{}, errors.New("replay segment parser revision is required")
	}
	header, err := replayHeader(demo)
	if err != nil {
		return ReplaySegmentEvidence{}, err
	}
	parser, err := s2replay.NewParser(demo)
	if err != nil {
		return ReplaySegmentEvidence{}, err
	}
	events, err := parser.CollectEvents(0)
	if err != nil {
		return ReplaySegmentEvidence{}, err
	}
	source := ReplaySourceIdentity{
		SHA256:            sha256Hex(demo),
		Game:              header.GetGame(),
		Map:               header.GetMapName(),
		GameBuild:         header.GetBuildNum(),
		Parser:            "s2replay",
		ParserRevision:    request.ParserRevision,
		MapCorrespondence: "observed replay header map; installed map correspondence pending",
	}
	for _, event := range events {
		if event.PostMatch != nil && event.PostMatch.MatchID != 0 {
			source.MatchID = event.PostMatch.MatchID
		}
	}
	return BuildReplaySegmentEvidence(events, source, request), nil
}

// BuildReplaySegmentEvidence extracts deterministic evidence from typed events.
func BuildReplaySegmentEvidence(events []s2replay.Event, source ReplaySourceIdentity, request ReplaySegmentRequest) ReplaySegmentEvidence {
	leadInStart := uint32(0)
	if request.LeadInTicks <= request.StartTick {
		leadInStart = request.StartTick - request.LeadInTicks
	}
	out := ReplaySegmentEvidence{
		SchemaVersion: ReplaySegmentEvidenceSchemaVersion,
		Source:        source,
		Range: ReplaySegmentRange{
			RequestedStartTick:   request.StartTick,
			RequestedEndTick:     request.EndTick,
			LeadInStartTick:      leadInStart,
			RequestedLeadInTicks: request.LeadInTicks,
			LeadInTicks:          request.StartTick - leadInStart,
			ExactStartTick:       request.StartTick,
			ExactEndTick:         request.EndTick,
		},
		Participants: []ReplayParticipant{},
		Rows:         []ReplaySegmentRow{},
	}
	selected := make(map[int32]struct{}, len(request.ParticipantSlots))
	for _, slot := range request.ParticipantSlots {
		selected[slot] = struct{}{}
	}
	allSlots := len(selected) == 0
	participants := make(map[int32]ReplayParticipant)
	seenEntities := make(map[int32]bool)
	for _, event := range events {
		if event.Type != s2replay.EventEntitySample || event.EntitySample == nil || event.PlayerSlot < 0 {
			continue
		}
		if event.Tick < leadInStart || event.Tick > request.EndTick || (!allSlots && !containsSlot(selected, event.PlayerSlot)) {
			continue
		}
		participant := participants[event.PlayerSlot]
		participant.PlayerSlot = event.PlayerSlot
		if !seenEntities[event.PlayerSlot] {
			participant.HistoricalEntityID = event.Entity
			seenEntities[event.PlayerSlot] = true
		}
		if event.EntitySample.HasHeroID {
			participant.HeroID, participant.HasHeroID = event.EntitySample.HeroID, true
		}
		if event.EntitySample.HasTeam {
			participant.Team, participant.HasTeam = event.EntitySample.Team, true
		}
		participants[event.PlayerSlot] = participant
		row := replayRow(event)
		row.LeadIn = event.Tick < request.StartTick
		out.Rows = append(out.Rows, row)
	}
	if !allSlots {
		for slot := range selected {
			if _, ok := participants[slot]; ok {
				continue
			}
			participants[slot] = ReplayParticipant{PlayerSlot: slot, HistoricalEntityID: -1}
			out.Quality.MissingParticipantSlots = append(out.Quality.MissingParticipantSlots, slot)
		}
		slices.Sort(out.Quality.MissingParticipantSlots)
	}
	out.Participants = make([]ReplayParticipant, 0, len(participants))
	for _, participant := range participants {
		out.Participants = append(out.Participants, participant)
	}
	slices.SortFunc(out.Participants, func(a, b ReplayParticipant) int { return cmp.Compare(a.PlayerSlot, b.PlayerSlot) })
	out.Quality.Rows = len(out.Rows)
	out.Quality.Participants = len(out.Participants)
	startSlots := make(map[int32]bool)
	endSlots := make(map[int32]bool)
	for _, row := range out.Rows {
		if row.LeadIn {
			out.Quality.LeadInRows++
		} else {
			out.Quality.RequestedRows++
			if row.Tick == request.StartTick {
				startSlots[row.PlayerSlot] = true
			}
			if row.Tick == request.EndTick {
				endSlots[row.PlayerSlot] = true
			}
		}
	}
	if len(out.Participants) != 0 && len(startSlots) == len(out.Participants) {
		out.Quality.ExactStartPresent = true
	} else {
		out.Quality.BoundaryMissingReason = "requested start has no row for every selected participant"
	}
	if len(out.Participants) != 0 && len(endSlots) == len(out.Participants) {
		out.Quality.ExactEndPresent = true
	} else if out.Quality.BoundaryMissingReason == "" {
		out.Quality.BoundaryMissingReason = "requested end has no row for every selected participant"
	}
	if out.Quality.ExactStartPresent {
		out.Range.ExactStartTick = request.StartTick
	} else {
		out.Range.ExactStartTick = 0
	}
	if out.Quality.ExactEndPresent {
		out.Range.ExactEndTick = request.EndTick
	} else {
		out.Range.ExactEndTick = 0
	}
	for _, row := range out.Rows {
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
	return out
}

func replayRow(event s2replay.Event) ReplaySegmentRow {
	sample := event.EntitySample
	row := ReplaySegmentRow{Tick: event.Tick, GameTime: event.GameTime, PlayerSlot: event.PlayerSlot, EntityID: event.Entity}
	positionXField := sample.PositionXSourceField
	if positionXField == "" {
		positionXField = "CBodyComponent.m_skeletonInstance.m_cellX+CBodyComponent.m_skeletonInstance.m_vecX"
	}
	positionYField := sample.PositionYSourceField
	if positionYField == "" {
		positionYField = "CBodyComponent.m_skeletonInstance.m_cellY+CBodyComponent.m_skeletonInstance.m_vecY"
	}
	positionZField := sample.PositionZSourceField
	if positionZField == "" {
		positionZField = "CBodyComponent.m_skeletonInstance.m_cellZ+CBodyComponent.m_skeletonInstance.m_vecZ"
	}
	row.PositionX = scalar(sample.PositionX, sample.PositionXTick, event.Tick, sample.HasPosition, positionXField, "position_not_present")
	row.PositionY = scalar(sample.PositionY, sample.PositionYTick, event.Tick, sample.HasPosition, positionYField, "position_not_present")
	row.PositionZ = scalar(sample.PositionZ, sample.PositionZTick, event.Tick, sample.HasPosition, positionZField, "position_not_present")
	row.FacingX = scalar(sample.FacingX, sample.FacingXTick, event.Tick, sample.HasFacingX || sample.HasFacing, "CCitadelPlayerPawn.m_angEyeAngles", "m_angEyeAngles_not_present")
	row.FacingY = scalar(sample.FacingY, sample.FacingYTick, event.Tick, sample.HasFacingY || sample.HasFacing, "CCitadelPlayerPawn.m_angEyeAngles", "m_angEyeAngles_not_present")
	row.FacingZ = scalar(sample.FacingZ, sample.FacingZTick, event.Tick, sample.HasFacingZ || sample.HasFacing, "CCitadelPlayerPawn.m_angEyeAngles", "m_angEyeAngles_not_present")
	row.VelocityX = scalar(sample.VelocityX, sample.VelocityXTick, event.Tick, sample.HasVelocityX || sample.HasVelocity, "CCitadelPlayerPawn.m_vecVelocity.m_vecX", "m_vecVelocity.m_vecX_not_present")
	row.VelocityY = scalar(sample.VelocityY, sample.VelocityYTick, event.Tick, sample.HasVelocityY || sample.HasVelocity, "CCitadelPlayerPawn.m_vecVelocity.m_vecY", "m_vecVelocity.m_vecY_not_present")
	row.VelocityZ = scalar(sample.VelocityZ, sample.VelocityZTick, event.Tick, sample.HasVelocityZ || sample.HasVelocity, "CCitadelPlayerPawn.m_vecVelocity.m_vecZ", "m_vecVelocity.m_vecZ_not_present")
	return row
}

func scalar(value float32, tick, rowTick uint32, present bool, field, reason string) ReplayScalar {
	if !present {
		return ReplayScalar{SourceField: field, Quality: ReplayFieldMissing, MissingReason: reason}
	}
	out := ReplayScalar{Value: value, SourceField: field, SampleTick: tick, Quality: ReplayFieldExact}
	if rowTick >= tick {
		out.FreshnessTicks = rowTick - tick
	}
	return out
}

func containsSlot(slots map[int32]struct{}, slot int32) bool { _, ok := slots[slot]; return ok }

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func replayHeader(demo []byte) (*protocol.CDemoFileHeader, error) {
	parser, err := s2replay.NewParser(demo)
	if err != nil {
		return nil, err
	}
	for {
		command, err := parser.Next()
		if err == io.EOF {
			return &protocol.CDemoFileHeader{}, nil
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
