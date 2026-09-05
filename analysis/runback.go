package analysis

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/paralin/s2replay"
)

// RunbackFactsSchemaVersion identifies the Runback facts contract.
const RunbackFactsSchemaVersion = 2

// RunbackMissingReason values name why an observed field is absent.
const (
	RunbackMissingNotInSample  = "not_in_sample"
	RunbackMissingNonFinite    = "non_finite_source_value"
	RunbackMissingNoEntity     = "no_entity_sample_at_tick"
	RunbackMissingUnattributed = "player_slot_unattributed"
	RunbackMissingNotRecorded  = "not_recorded_at_tick"

	// RunbackMissingNoRejuvStatus records that no rejuvenator status user
	// message was observed at or before the requested tick.
	RunbackMissingNoRejuvStatus = "rejuv_status_not_observed"
	// RunbackMissingNoServerInfo records that the server tick interval was
	// not decoded from CSVCMsg_ServerInfo before the requested tick.
	RunbackMissingNoServerInfo = "server_info_not_observed"
	// RunbackMissingHeaderField records that an optional demo file header
	// field was absent.
	RunbackMissingHeaderField = "header_field_absent"
	// RunbackMissingOwnerUnattributed records that a transient entity's owner
	// handle does not resolve to an attributed hero pawn at the tick.
	RunbackMissingOwnerUnattributed = "owner_not_attributed"
)

// Runback objective role classes are observed network class names from the
// replay send tables. Each role reads exactly one class; the row carries the
// observed class name so a consumer can verify the mapping. No class is
// inferred from raw memory or from names outside this declared mapping.
const (
	RunbackMidBossClass            = "CNPC_MidBoss"
	RunbackTowerClass              = "CNPC_BaseDefenseSentry"
	RunbackWalkerClass             = "CNPC_BarrackBoss"
	RunbackRejuvenatorEventKind    = "rejuv_status"
	RunbackRejuvenatorStatusAbsent = "no_rejuv_status_observed"
	RunbackRejuvenatorStatusSeen   = "rejuv_status_observed"
)

// RunbackAliveBasis states why an alive verdict holds.
type RunbackAliveBasis string

// RunbackAliveBasis values state why an alive verdict holds.
const (
	RunbackAliveHealthPositive RunbackAliveBasis = "health_positive"
	RunbackAliveActive         RunbackAliveBasis = "active_no_health"
)

// RunbackRequest selects the pinned moment tick.
type RunbackRequest struct {
	Tick              uint32                     `json:"tick"`
	ExpectedIdentity  *ReplayIdentityExpectation `json:"expected_identity,omitempty"`
	MaxFreshnessTicks *uint32                    `json:"max_freshness_ticks,omitempty"`
}

// RunbackFacts is the versioned replay-local world and hero record for one tick.
type RunbackFacts struct {
	SchemaVersion      int                          `json:"schema_version"`
	Source             ReplaySourceIdentity         `json:"source"`
	Correspondence     ReplayIdentityCorrespondence `json:"correspondence"`
	Tick               uint32                       `json:"tick"`
	TickProvenance     RunbackTickProvenance        `json:"tick_provenance"`
	Heroes             []RunbackHero                `json:"heroes"`
	WorldEntities      []RunbackWorldEntity         `json:"world_entities"`
	Objectives         RunbackObjectives            `json:"objectives"`
	Quality            RunbackFactsQuality          `json:"quality"`
	Eligibility        ReplaySegmentEligibility     `json:"eligibility"`
	EligibilityReasons []string                     `json:"eligibility_reasons,omitempty"`
}

// RunbackTickProvenance records the tick interval, header start tick and
// observed network tick with their provenance. Values are populated only from observed source data;
// no default or assumed rate is ever reported as present.
type RunbackTickProvenance struct {
	// TickIntervalSeconds is the seconds-per-tick reported by
	// CSVCMsg_ServerInfo. It is present only when the server message was
	// decoded; the parser's internal placeholder is never reported.
	TickIntervalSeconds RunbackFloat `json:"tick_interval_seconds"`
	// ServerStartTick is the start tick declared in the demo file header.
	ServerStartTick RunbackInt `json:"server_start_tick"`
	// ServerTick is CNETMsg_Tick at SourceTick, not demo tick plus header start.
	ServerTick RunbackUint `json:"server_tick"`
}

// RunbackObjectives is the explicit objective state observed at the tick.
type RunbackObjectives struct {
	// MidBoss is the mid boss world entity observed at the tick.
	MidBoss RunbackObjectiveEntity `json:"mid_boss"`
	// Rejuvenator is the rejuvenator status derived from rejuv_status events.
	Rejuvenator RunbackRejuvenator `json:"rejuvenator"`
	// Towers are the base defense sentry entities observed at the tick.
	Towers []RunbackObjectiveEntity `json:"towers"`
	// Walkers are the barrack boss entities observed at the tick.
	Walkers []RunbackObjectiveEntity `json:"walkers"`
	// Transients are the active unattributed item-class entities observed at the tick.
	Transients []RunbackTransient `json:"transients"`
}

// RunbackObjectiveEntity is one objective entity observed at the tick.
type RunbackObjectiveEntity struct {
	EntityID     int32  `json:"entity_id"`
	EntitySerial int32  `json:"entity_serial"`
	ClassID      int32  `json:"class_id"`
	ClassName    string `json:"class_name"`

	Team     RunbackInt      `json:"team"`
	Position [3]RunbackFloat `json:"position"`

	Health    RunbackFloat `json:"health"`
	MaxHealth RunbackFloat `json:"max_health"`
	Alive     RunbackAlive `json:"alive"`
}

// RunbackRejuvenator is the rejuvenator state observed at the tick. The
// rejuvenator has no directly networked world entity in the current evidence
// set, so its state is derived only from rejuv_status events.
type RunbackRejuvenator struct {
	Status string                   `json:"status"`
	Last   *RunbackRejuvenatorEvent `json:"last,omitempty"`
}

// RunbackRejuvenatorEvent is one rejuv_status event observation.
type RunbackRejuvenatorEvent struct {
	Tick        uint32 `json:"tick"`
	KillingTeam int32  `json:"killing_team"`
	EventType   int32  `json:"event_type"`
	UserTeam    int32  `json:"user_team"`
}

// RunbackFactsQuality summarizes attribution coverage.
type RunbackFactsQuality struct {
	Heroes            int      `json:"heroes"`
	WorldEntities     int      `json:"world_entities"`
	SnapshotEntities  int      `json:"snapshot_entities"`
	UnattributedPawns int      `json:"unattributed_pawns"`
	DuplicateSlots    []int32  `json:"duplicate_slots,omitempty"`
	MissingFields     []string `json:"missing_fields,omitempty"`
}

// RunbackHero is one replay hero slot observed at the tick.
type RunbackHero struct {
	PlayerSlot   int32  `json:"player_slot"`
	EntityID     int32  `json:"entity_id"`
	EntitySerial int32  `json:"entity_serial"`
	ClassID      int32  `json:"class_id"`
	ClassName    string `json:"class_name"`

	HeroID   RunbackUint     `json:"hero_id"`
	Team     RunbackInt      `json:"team"`
	Position [3]RunbackFloat `json:"position"`
	Facing   [3]RunbackFloat `json:"facing"`
	Velocity [3]RunbackFloat `json:"velocity"`

	Health    RunbackFloat `json:"health"`
	MaxHealth RunbackFloat `json:"max_health"`
	Shield    RunbackFloat `json:"shield"`
	MaxShield RunbackFloat `json:"max_shield"`
	Level     RunbackUint  `json:"level"`

	Items     []RunbackItem     `json:"items"`
	Abilities []RunbackAbility  `json:"abilities"`
	Modifiers []RunbackModifier `json:"modifiers"`

	NetWorth RunbackInt    `json:"net_worth"`
	Scores   RunbackScores `json:"scores"`
}

// RunbackWorldEntity is one non-hero world entity observed at the tick.
type RunbackWorldEntity struct {
	EntityID     int32  `json:"entity_id"`
	EntitySerial int32  `json:"entity_serial"`
	ClassID      int32  `json:"class_id"`
	ClassName    string `json:"class_name"`

	Team     RunbackInt      `json:"team"`
	Position [3]RunbackFloat `json:"position"`

	Health    RunbackFloat `json:"health"`
	MaxHealth RunbackFloat `json:"max_health"`
	Shield    RunbackFloat `json:"shield"`
	Alive     RunbackAlive `json:"alive"`
}

// RunbackAlive records the alive verdict and its basis.
type RunbackAlive struct {
	Alive          bool              `json:"alive"`
	Basis          RunbackAliveBasis `json:"basis"`
	SourceTick     uint32            `json:"source_tick"`
	FreshnessTicks uint32            `json:"freshness_ticks"`
}

// RunbackItem is one owned item entity at the tick.
type RunbackItem struct {
	SubclassID   RunbackUint `json:"subclass_id"`
	Slot         RunbackUint `json:"slot"`
	UpgradeInfo  RunbackUint `json:"upgrade_info"`
	EntityID     int32       `json:"entity_id"`
	EntitySerial int32       `json:"entity_serial"`
	ClassName    string      `json:"class_name"`
	SourceTick   uint32      `json:"source_tick"`
	Charges      RunbackInt  `json:"charges"`
	// CooldownStart records the start of the active cooldown interval in server seconds.
	CooldownStart RunbackFloat `json:"cooldown_start"`
	// ChargeRechargeStart records the start of the charge recovery interval in server seconds.
	ChargeRechargeStart RunbackFloat `json:"charge_recharge_start"`
	// ChargeRechargeEnd records the end of the charge recovery interval in server seconds.
	ChargeRechargeEnd RunbackFloat `json:"charge_recharge_end"`
	CooldownEnd       RunbackFloat `json:"cooldown_end"`
}

// RunbackTransient is one active item-class entity at the tick whose owner
// does not resolve to an attributed hero pawn. Owned item entities remain on
// their hero row only.
type RunbackTransient struct {
	EntityID     int32  `json:"entity_id"`
	EntitySerial int32  `json:"entity_serial"`
	ClassName    string `json:"class_name"`

	// OwnerEntity is the observed owner handle; it is present only when the
	// sample carried one.
	OwnerEntity RunbackInt `json:"owner_entity"`
	// Team is the observed team when the sample carried one.
	Team RunbackInt `json:"team"`
	// Position is the observed position when the sample carried one.
	Position [3]RunbackFloat `json:"position"`

	// MissingReason states why the transient is not attributed to a hero row.
	MissingReason string `json:"missing_reason"`
}

// RunbackAbility is one owned ability entity with charge and cooldown state.
type RunbackAbility struct {
	SubclassID   RunbackUint `json:"subclass_id"`
	Slot         RunbackUint `json:"slot"`
	UpgradeInfo  RunbackUint `json:"upgrade_info"`
	EntityID     int32       `json:"entity_id"`
	EntitySerial int32       `json:"entity_serial"`
	ClassName    string      `json:"class_name"`

	Charges RunbackInt `json:"charges"`
	// CooldownStart records the start of the active cooldown interval in server seconds.
	CooldownStart RunbackFloat `json:"cooldown_start"`
	// ChargeRechargeStart records the start of the charge recovery interval in server seconds.
	ChargeRechargeStart RunbackFloat `json:"charge_recharge_start"`
	// ChargeRechargeEnd records the end of the charge recovery interval in server seconds.
	ChargeRechargeEnd RunbackFloat `json:"charge_recharge_end"`
	CooldownEnd       RunbackFloat `json:"cooldown_end"`
}

// RunbackModifier is one modifier active at the tick.
type RunbackModifier struct {
	Subclass   uint32 `json:"subclass"`
	Ability    uint32 `json:"ability"`
	StackCount int32  `json:"stack_count"`
	StartTick  uint32 `json:"start_tick"`
	SourceTick uint32 `json:"source_tick"`
}

// RunbackScores carries the observed scoreboard components at the tick.
type RunbackScores struct {
	Deaths     RunbackInt `json:"deaths"`
	LastHits   RunbackInt `json:"last_hits"`
	Denies     RunbackInt `json:"denies"`
	KillStreak RunbackInt `json:"kill_streak"`
	HeroDamage RunbackInt `json:"hero_damage"`
}

// RunbackFloat is one observed float with provenance or a typed missing reason.
type RunbackFloat struct {
	Value          float32 `json:"value"`
	Present        bool    `json:"present"`
	SourceTick     uint32  `json:"source_tick"`
	FreshnessTicks uint32  `json:"freshness_ticks"`
	MissingReason  string  `json:"missing_reason,omitempty"`
}

// RunbackUint is one observed unsigned integer with provenance.
type RunbackUint struct {
	Value          uint32 `json:"value"`
	Present        bool   `json:"present"`
	SourceTick     uint32 `json:"source_tick"`
	FreshnessTicks uint32 `json:"freshness_ticks"`
	MissingReason  string `json:"missing_reason,omitempty"`
}

// RunbackInt is one observed signed integer with provenance.
type RunbackInt struct {
	Value          int32  `json:"value"`
	Present        bool   `json:"present"`
	SourceTick     uint32 `json:"source_tick"`
	FreshnessTicks uint32 `json:"freshness_ticks"`
	MissingReason  string `json:"missing_reason,omitempty"`
}

// RunbackErrorKind identifies a refused Runback facts input.
type RunbackErrorKind string

const (
	RunbackErrorInvalidTick    RunbackErrorKind = "invalid_requested_tick"
	RunbackErrorTickUnobserved RunbackErrorKind = "tick_not_observed"
	RunbackErrorDuplicateSlot  RunbackErrorKind = "duplicate_player_slot"
	RunbackErrorSlotOutOfRange RunbackErrorKind = "player_slot_out_of_range"
	RunbackErrorNonFinite      RunbackErrorKind = "non_finite_data"
	RunbackErrorInvalidEntity  RunbackErrorKind = "invalid_entity"
)

// RunbackError reports why Runback facts were refused.
type RunbackError struct {
	Kind         RunbackErrorKind
	Tick         uint32
	EntityID     int32
	EntitySerial int32
	PlayerSlot   int32
	Field        string
}

// Error returns the typed refusal message with the offending values.
func (e *RunbackError) Error() string {
	return fmt.Sprintf("runback facts: %s tick=%d entity=%d serial=%d slot=%d field=%s", e.Kind, e.Tick, e.EntityID, e.EntitySerial, e.PlayerSlot, e.Field)
}

// ExtractRunbackFacts parses immutable demo bytes and extracts one tick.
// ExtractRunbackFacts refuses binaries without clean VCS identity.
func ExtractRunbackFacts(demo []byte, request RunbackRequest) (RunbackFacts, error) {
	if request.Tick == s2replay.PreGameTick || request.Tick == 0 {
		return RunbackFacts{}, &RunbackError{Kind: RunbackErrorInvalidTick, Tick: request.Tick, Field: "tick"}
	}
	revision, clean := s2replay.BuildRevision()
	if !clean {
		return RunbackFacts{}, errors.New("running parser build has unknown or modified VCS identity")
	}
	return extractRunbackFactsWithBuild(demo, request, revision, clean)
}

// extractRunbackFactsWithBuild extracts Runback facts with an explicit parser
// build identity; it backs tests and fixtures.
func extractRunbackFactsWithBuild(demo []byte, request RunbackRequest, revision string, cleanBuild bool) (RunbackFacts, error) {
	if !cleanBuild {
		return RunbackFacts{}, errors.New("running parser build has unknown or modified VCS identity")
	}
	if request.Tick == s2replay.PreGameTick || request.Tick == 0 {
		return RunbackFacts{}, &RunbackError{Kind: RunbackErrorInvalidTick, Tick: request.Tick, Field: "tick"}
	}
	header, err := replayHeader(demo)
	if err != nil {
		return RunbackFacts{}, err
	}
	// One parser collects the event stream for modifier lifecycles; one
	// advances to the tick for the owned active-world snapshot.
	modifierParser, err := s2replay.NewParser(demo)
	if err != nil {
		return RunbackFacts{}, err
	}
	var events []s2replay.Event
	if err := consumeReplayEvents(modifierParser, func(event s2replay.Event) {
		if event.Tick <= request.Tick {
			events = append(events, event)
		} else if event.Tick != s2replay.PreGameTick {
			// Later commands cannot contribute state at the selected moment.
			// Stop consumes queued events before ending the parser stream.
			modifierParser.Stop()
		}
	}); err != nil {
		return RunbackFacts{}, err
	}
	timelines := Build(events)

	snapshotParser, err := s2replay.NewParser(demo)
	if err != nil {
		return RunbackFacts{}, err
	}
	samples, err := snapshotParser.WorldEntitySnapshot(request.Tick)
	if err != nil {
		var unavailable *s2replay.WorldSnapshotError
		if errors.As(err, &unavailable) {
			return RunbackFacts{}, &RunbackError{Kind: RunbackErrorTickUnobserved, Tick: unavailable.RequestedTick, Field: "tick_not_observed"}
		}
		var sampleErr *s2replay.WorldEntitySampleError
		if errors.As(err, &sampleErr) {
			return RunbackFacts{}, &RunbackError{Kind: RunbackErrorNonFinite, Tick: request.Tick, EntityID: sampleErr.EntityID, EntitySerial: sampleErr.EntitySerial, Field: sampleErr.Field}
		}
		return RunbackFacts{}, err
	}
	clock := snapshotParser.Clock()
	provenance := RunbackTickProvenance{}
	serverTick, sourceTick, known := clock.ServerTick()
	provenance.ServerTick = runbackUint(serverTick, sourceTick, known, request.Tick, "no_network_tick")
	if clock.TickIntervalKnown() {
		provenance.TickIntervalSeconds = runbackFloat(float32(clock.TickInterval()), 0, true, request.Tick, RunbackMissingNotInSample)
		provenance.TickIntervalSeconds.SourceTick = request.Tick
		provenance.TickIntervalSeconds.FreshnessTicks = 0
	} else {
		provenance.TickIntervalSeconds = RunbackFloat{MissingReason: RunbackMissingNoServerInfo}
	}
	if startTick := header.GetServerStartTick(); startTick != 0 {
		provenance.ServerStartTick = RunbackInt{Value: startTick, Present: true, SourceTick: 0, FreshnessTicks: request.Tick}
	} else {
		provenance.ServerStartTick = RunbackInt{MissingReason: RunbackMissingHeaderField}
	}
	// The file header can name the bootstrap map "start". The snapshot
	// parser owns the server world at the requested tick. Do not invent a
	// world identity when that message is absent.
	game, mapName := snapshotParser.ServerWorld()
	return buildRunbackFacts(samples, timelines, ReplaySourceIdentity{
		SHA256:         sha256Hex(demo),
		Game:           game,
		Map:            mapName,
		GameBuild:      header.GetBuildNum(),
		Parser:         "s2replay",
		ParserRevision: s2replay.ParserSourceDigest,
		VCSRevision:    revision,
	}, request, provenance, events)
}

// buildRunbackFacts assembles deterministic facts from the snapshot and timelines.
func buildRunbackFacts(samples []s2replay.EntitySample, timelines Result, source ReplaySourceIdentity, request RunbackRequest, provenance RunbackTickProvenance, events []s2replay.Event) (RunbackFacts, error) {
	tick := request.Tick
	out := RunbackFacts{
		SchemaVersion:  RunbackFactsSchemaVersion,
		Source:         source,
		Tick:           tick,
		TickProvenance: normalizeRunbackTickProvenance(provenance),
		Heroes:         []RunbackHero{},
		WorldEntities:  []RunbackWorldEntity{},
	}
	if request.ExpectedIdentity == nil {
		out.Correspondence = ReplayIdentityCorrespondence{Status: ReplayCorrespondencePending, Reason: "no expected replay identity supplied"}
	} else {
		out.Correspondence = compareReplayIdentity(source, *request.ExpectedIdentity)
	}

	byEntity := make(map[int32]*s2replay.EntitySample, len(samples))
	for i := range samples {
		sample := &samples[i]
		if sample.Entity < 0 || sample.EntitySerial < 0 {
			return RunbackFacts{}, &RunbackError{Kind: RunbackErrorInvalidEntity, Tick: tick, EntityID: sample.Entity, EntitySerial: sample.EntitySerial}
		}
		if prior, ok := byEntity[sample.Entity]; ok {
			return RunbackFacts{}, &RunbackError{Kind: RunbackErrorInvalidEntity, Tick: tick, EntityID: sample.Entity, EntitySerial: sample.EntitySerial, Field: fmt.Sprintf("duplicate_entity_sample_serial_%d_vs_%d", prior.EntitySerial, sample.EntitySerial)}
		}
		byEntity[sample.Entity] = sample
	}

	// Hero slots come from pawn entities attributed to lobby slots by the
	// parser. Slots are recorded as observed; they are never assumed 0..11.
	// A pawn whose own attribution was lost falls back to its controller's
	// observed lobby slot.
	controllerByPawn := make(map[int32]*s2replay.EntitySample)
	for i := range samples {
		sample := &samples[i]
		if !isRunbackControllerClass(sample.ClassName) || !sample.HasPawnEntity {
			continue
		}
		if pawn := byEntity[sample.PawnEntity]; pawn != nil && pawn.EntitySerial == sample.PawnEntitySerial && pawn.ClassName == "CCitadelPlayerPawn" {
			controllerByPawn[sample.PawnEntity] = sample
		}
	}
	pawnSlots := make(map[int32]int32)
	slotPawns := make(map[int32]int32)
	var unattributedPawns int
	for i := range samples {
		sample := &samples[i]
		if sample.ClassName != "CCitadelPlayerPawn" {
			continue
		}
		if sample.PlayerSlot < 0 {
			if controller := controllerByPawn[sample.Entity]; controller != nil && controller.PlayerSlot >= 0 {
				pawnSlots[sample.Entity] = controller.PlayerSlot
				continue
			}
			unattributedPawns++
			continue
		}
		if sample.PlayerSlot >= MaxReplayParticipants {
			return RunbackFacts{}, &RunbackError{Kind: RunbackErrorSlotOutOfRange, Tick: tick, EntityID: sample.Entity, EntitySerial: sample.EntitySerial, PlayerSlot: sample.PlayerSlot, Field: "player_slot"}
		}
		if prior, ok := slotPawns[sample.PlayerSlot]; ok {
			return RunbackFacts{}, &RunbackError{Kind: RunbackErrorDuplicateSlot, Tick: tick, EntityID: sample.Entity, EntitySerial: sample.EntitySerial, PlayerSlot: sample.PlayerSlot, Field: fmt.Sprintf("pawn_%d", prior)}
		}
		pawnSlots[sample.Entity] = sample.PlayerSlot
		slotPawns[sample.PlayerSlot] = sample.Entity
	}

	for i := range samples {
		sample := &samples[i]
		if sample.ClassName != "CCitadelPlayerPawn" {
			continue
		}
		slot, ok := pawnSlots[sample.Entity]
		if !ok {
			continue
		}
		hero := RunbackHero{
			PlayerSlot: slot, EntityID: sample.Entity, EntitySerial: sample.EntitySerial,
			ClassID: sample.ClassID, ClassName: sample.ClassName,
			HeroID:    runbackUint(sample.HeroID, sample.HeroIDTick, sample.HasHeroID, tick, RunbackMissingNotInSample),
			Team:      runbackInt(sample.Team, sample.TeamTick, sample.HasTeam, tick, RunbackMissingNotInSample),
			Position:  runbackPosition(sample, tick),
			Facing:    runbackVector3(sample.FacingX, sample.FacingY, sample.FacingZ, sample.FacingXTick, sample.FacingYTick, sample.FacingZTick, sample.HasFacingX || sample.HasFacing, sample.HasFacingY || sample.HasFacing, sample.HasFacingZ || sample.HasFacing, tick, "m_angEyeAngles_not_present"),
			Velocity:  runbackVector3(sample.VelocityX, sample.VelocityY, sample.VelocityZ, sample.VelocityXTick, sample.VelocityYTick, sample.VelocityZTick, sample.HasVelocityX || sample.HasVelocity, sample.HasVelocityY || sample.HasVelocity, sample.HasVelocityZ || sample.HasVelocity, tick, "m_vecVelocity_not_present"),
			Health:    runbackFloat(sample.Health, sample.HealthTick, sample.HasHealth, tick, RunbackMissingNotInSample),
			MaxHealth: runbackFloat(sample.MaxHealth, sample.MaxHealthTick, sample.HasHealth, tick, RunbackMissingNotInSample),
			Shield:    runbackFloat(sample.Shield, sample.ShieldTick, sample.HasShield, tick, RunbackMissingNotInSample),
			MaxShield: runbackFloat(sample.MaxShield, sample.MaxShieldTick, sample.HasShield, tick, RunbackMissingNotInSample),
			Level:     runbackUint(sample.Level, sample.LevelTick, sample.HasLevel, tick, RunbackMissingNotInSample),
		}
		if controller := controllerByPawn[sample.Entity]; controller != nil {
			hero.NetWorth = runbackInt(controller.NetWorth, controller.NetWorthTick, controller.HasNetWorth, tick, RunbackMissingNotInSample)
			hero.Scores = RunbackScores{
				Deaths:     runbackInt(controller.Deaths, controller.DeathsTick, controller.HasDeaths, tick, RunbackMissingNotInSample),
				LastHits:   runbackInt(controller.LastHits, controller.LastHitsTick, controller.HasLastHits, tick, RunbackMissingNotInSample),
				Denies:     runbackInt(controller.Denies, controller.DeniesTick, controller.HasDenies, tick, RunbackMissingNotInSample),
				KillStreak: runbackInt(controller.KillStreak, controller.KillStreakTick, controller.HasKillStreak, tick, RunbackMissingNotInSample),
				HeroDamage: runbackInt(controller.HeroDamage, controller.HeroDamageTick, controller.HasHeroDamage, tick, RunbackMissingNotInSample),
			}
		} else {
			hero.NetWorth = missingRunbackInt(tick, RunbackMissingNoEntity)
			hero.Scores = RunbackScores{
				Deaths: missingRunbackInt(tick, RunbackMissingNoEntity), LastHits: missingRunbackInt(tick, RunbackMissingNoEntity),
				Denies: missingRunbackInt(tick, RunbackMissingNoEntity), KillStreak: missingRunbackInt(tick, RunbackMissingNoEntity),
				HeroDamage: missingRunbackInt(tick, RunbackMissingNoEntity),
			}
		}
		hero.Items = runbackItems(samples, sample, tick)
		hero.Abilities = runbackAbilities(samples, sample, tick)
		hero.Modifiers = runbackModifiers(timelines, slot, tick)
		out.Heroes = append(out.Heroes, hero)
	}
	slices.SortFunc(out.Heroes, func(a, b RunbackHero) int { return int(a.PlayerSlot) - int(b.PlayerSlot) })

	for i := range samples {
		sample := &samples[i]
		if sample.ClassName == "CCitadelPlayerPawn" || isRunbackControllerClass(sample.ClassName) ||
			isRunbackAbilityClass(sample.ClassName) || isRunbackItemClass(sample.ClassName) {
			continue
		}
		out.WorldEntities = append(out.WorldEntities, RunbackWorldEntity{
			EntityID: sample.Entity, EntitySerial: sample.EntitySerial, ClassID: sample.ClassID, ClassName: sample.ClassName,
			Team:      runbackInt(sample.Team, sample.TeamTick, sample.HasTeam, tick, RunbackMissingNotInSample),
			Position:  runbackPosition(sample, tick),
			Health:    runbackFloat(sample.Health, sample.HealthTick, sample.HasHealth, tick, RunbackMissingNotInSample),
			MaxHealth: runbackFloat(sample.MaxHealth, sample.MaxHealthTick, sample.HasHealth, tick, RunbackMissingNotInSample),
			Shield:    runbackFloat(sample.Shield, sample.ShieldTick, sample.HasShield, tick, RunbackMissingNotInSample),
			Alive:     runbackAlive(sample, tick),
		})
	}
	slices.SortFunc(out.WorldEntities, func(a, b RunbackWorldEntity) int {
		if a.EntityID != b.EntityID {
			return int(a.EntityID - b.EntityID)
		}
		return int(a.EntitySerial - b.EntitySerial)
	})

	out.Objectives = runbackObjectives(samples, byEntity, events, pawnSlots, tick)
	out.Quality = RunbackFactsQuality{
		Heroes: len(out.Heroes), WorldEntities: len(out.WorldEntities), SnapshotEntities: len(samples),
		UnattributedPawns: unattributedPawns,
	}
	out.Quality.MissingFields = runbackMissingFields(out)
	out.Eligibility, out.EligibilityReasons = runbackEligibility(out, request)
	return out, nil
}

// isRunbackControllerClass reports whether the class name is a player
// controller.
func isRunbackControllerClass(name string) bool {
	return strings.Contains(name, "CitadelPlayerController")
}
func isRunbackAbilityClass(name string) bool { return strings.Contains(name, "Ability") }
func isRunbackItemClass(name string) bool {
	return strings.Contains(name, "CCitadel_Item_") || strings.Contains(name, "CCitadelItem")
}

// runbackPosition converts the sample's position components.
func runbackPosition(sample *s2replay.EntitySample, tick uint32) [3]RunbackFloat {
	return [3]RunbackFloat{
		runbackFloat(sample.PositionX, sample.PositionXTick, sample.HasPosition, tick, RunbackMissingNotInSample),
		runbackFloat(sample.PositionY, sample.PositionYTick, sample.HasPosition, tick, RunbackMissingNotInSample),
		runbackFloat(sample.PositionZ, sample.PositionZTick, sample.HasPosition, tick, RunbackMissingNotInSample),
	}
}

// runbackVector3 converts per-axis vector components with independent
// presence and source ticks.
func runbackVector3(x, y, z float32, xTick, yTick, zTick uint32, hasX, hasY, hasZ bool, tick uint32, reason string) [3]RunbackFloat {
	return [3]RunbackFloat{
		runbackFloat(x, xTick, hasX, tick, reason),
		runbackFloat(y, yTick, hasY, tick, reason),
		runbackFloat(z, zTick, hasZ, tick, reason),
	}
}

// runbackFloat builds a float record, downgrading absent or non-finite
// values to a typed missing reason.
func runbackFloat(value float32, sourceTick uint32, present bool, requestedTick uint32, reason string) RunbackFloat {
	if !present {
		return RunbackFloat{MissingReason: reason}
	}
	if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return RunbackFloat{MissingReason: RunbackMissingNonFinite}
	}
	out := RunbackFloat{Value: value, Present: true, SourceTick: sourceTick}
	if requestedTick >= sourceTick {
		out.FreshnessTicks = requestedTick - sourceTick
	}
	return out
}

// runbackUint builds an unsigned integer record with provenance.
func runbackUint(value uint32, sourceTick uint32, present bool, requestedTick uint32, reason string) RunbackUint {
	if !present {
		return RunbackUint{MissingReason: reason}
	}
	out := RunbackUint{Value: value, Present: true, SourceTick: sourceTick}
	if requestedTick >= sourceTick {
		out.FreshnessTicks = requestedTick - sourceTick
	}
	return out
}

// runbackInt builds a signed integer record with provenance.
func runbackInt(value int32, sourceTick uint32, present bool, requestedTick uint32, reason string) RunbackInt {
	if !present {
		return RunbackInt{MissingReason: reason}
	}
	out := RunbackInt{Value: value, Present: true, SourceTick: sourceTick}
	if requestedTick >= sourceTick {
		out.FreshnessTicks = requestedTick - sourceTick
	}
	return out
}

// missingRunbackInt builds a typed-missing integer record.
func missingRunbackInt(requestedTick uint32, reason string) RunbackInt {
	return RunbackInt{MissingReason: reason}
}

// runbackAlive derives the alive verdict from health or activity state.
func runbackAlive(sample *s2replay.EntitySample, tick uint32) RunbackAlive {
	if sample.HasHealth {
		alive := sample.Health > 0
		out := RunbackAlive{Alive: alive, Basis: RunbackAliveHealthPositive, SourceTick: sample.HealthTick}
		if tick >= sample.HealthTick {
			out.FreshnessTicks = tick - sample.HealthTick
		}
		return out
	}
	return RunbackAlive{Alive: true, Basis: RunbackAliveActive, SourceTick: tick}
}

// runbackItems collects the item entities owned by one pawn.
func runbackItems(samples []s2replay.EntitySample, pawn *s2replay.EntitySample, tick uint32) []RunbackItem {
	items := []RunbackItem{}
	for i := range samples {
		sample := &samples[i]
		if !isRunbackItemClass(sample.ClassName) || !sample.HasOwnerEntity || sample.OwnerEntity != pawn.Entity || sample.OwnerEntitySerial != pawn.EntitySerial {
			continue
		}
		items = append(items, RunbackItem{
			Charges:             runbackInt(sample.RemainingCharges, sample.RemainingChargesTick, sample.HasRemainingCharges, tick, RunbackMissingNotInSample),
			CooldownStart:       runbackFloat(sample.CooldownStart, sample.CooldownStartTick, sample.HasCooldownStart, tick, RunbackMissingNotInSample),
			ChargeRechargeStart: runbackFloat(sample.ChargeRechargeStart, sample.ChargeRechargeStartTick, sample.HasChargeRechargeStart, tick, RunbackMissingNotInSample),
			ChargeRechargeEnd:   runbackFloat(sample.ChargeRechargeEnd, sample.ChargeRechargeEndTick, sample.HasChargeRechargeEnd, tick, RunbackMissingNotInSample),
			CooldownEnd:         runbackFloat(sample.CooldownEnd, sample.CooldownEndTick, sample.HasCooldownEnd, tick, RunbackMissingNotInSample),
			EntityID:            sample.Entity, EntitySerial: sample.EntitySerial, ClassName: sample.ClassName, SourceTick: sample.Tick,
			SubclassID:  runbackUint(sample.SubclassID, sample.SubclassIDTick, sample.HasSubclassID, tick, "m_nSubclassID_not_present"),
			Slot:        runbackUint(sample.AbilitySlot, sample.AbilitySlotTick, sample.HasAbilitySlot, tick, "m_eAbilitySlot_not_present"),
			UpgradeInfo: runbackUint(sample.UpgradeInfo, sample.UpgradeInfoTick, sample.HasUpgradeInfo, tick, "m_nUpgradeInfo_not_present"),
		})
	}
	slices.SortFunc(items, func(a, b RunbackItem) int { return int(a.EntityID - b.EntityID) })
	return items
}

// runbackAbilities collects the ability entities owned by one pawn.
func runbackAbilities(samples []s2replay.EntitySample, pawn *s2replay.EntitySample, tick uint32) []RunbackAbility {
	abilities := []RunbackAbility{}
	for i := range samples {
		sample := &samples[i]
		if !isRunbackAbilityClass(sample.ClassName) || !sample.HasOwnerEntity || sample.OwnerEntity != pawn.Entity || sample.OwnerEntitySerial != pawn.EntitySerial {
			continue
		}
		abilities = append(abilities, RunbackAbility{
			SubclassID:  runbackUint(sample.SubclassID, sample.SubclassIDTick, sample.HasSubclassID, tick, "m_nSubclassID_not_present"),
			Slot:        runbackUint(sample.AbilitySlot, sample.AbilitySlotTick, sample.HasAbilitySlot, tick, "m_eAbilitySlot_not_present"),
			UpgradeInfo: runbackUint(sample.UpgradeInfo, sample.UpgradeInfoTick, sample.HasUpgradeInfo, tick, "m_nUpgradeInfo_not_present"),
			EntityID:    sample.Entity, EntitySerial: sample.EntitySerial, ClassName: sample.ClassName,
			Charges:             runbackInt(sample.RemainingCharges, sample.RemainingChargesTick, sample.HasRemainingCharges, tick, RunbackMissingNotInSample),
			CooldownStart:       runbackFloat(sample.CooldownStart, sample.CooldownStartTick, sample.HasCooldownStart, tick, RunbackMissingNotInSample),
			ChargeRechargeStart: runbackFloat(sample.ChargeRechargeStart, sample.ChargeRechargeStartTick, sample.HasChargeRechargeStart, tick, RunbackMissingNotInSample),
			ChargeRechargeEnd:   runbackFloat(sample.ChargeRechargeEnd, sample.ChargeRechargeEndTick, sample.HasChargeRechargeEnd, tick, RunbackMissingNotInSample),
			CooldownEnd:         runbackFloat(sample.CooldownEnd, sample.CooldownEndTick, sample.HasCooldownEnd, tick, RunbackMissingNotInSample),
		})
	}
	slices.SortFunc(abilities, func(a, b RunbackAbility) int { return int(a.EntityID - b.EntityID) })
	return abilities
}

// runbackModifiers collects the modifier intervals open for a slot at the
// tick.
func runbackModifiers(timelines Result, slot int32, tick uint32) []RunbackModifier {
	modifiers := []RunbackModifier{}
	for _, interval := range timelines.Modifiers.Modifiers {
		if interval.PlayerSlot != slot {
			continue
		}
		if interval.StartTick > tick {
			continue
		}
		if !interval.Open && interval.EndTick <= tick {
			continue
		}
		modifiers = append(modifiers, RunbackModifier{
			Subclass: interval.ModifierSubclass, Ability: interval.Ability,
			StackCount: interval.StackCount, StartTick: interval.StartTick, SourceTick: tick,
		})
	}
	slices.SortFunc(modifiers, func(a, b RunbackModifier) int {
		if a.StartTick != b.StartTick {
			return int(a.StartTick - b.StartTick)
		}
		return int(a.Subclass - b.Subclass)
	})
	return modifiers
}

// runbackMissingFields lists the vector components missing from every hero.
func runbackMissingFields(facts RunbackFacts) []string {
	var missing []string
	missing = append(missing, runbackFloatMissing("position", facts.Heroes, func(hero RunbackHero) [3]RunbackFloat { return hero.Position })...)
	missing = append(missing, runbackFloatMissing("facing", facts.Heroes, func(hero RunbackHero) [3]RunbackFloat { return hero.Facing })...)
	missing = append(missing, runbackFloatMissing("velocity", facts.Heroes, func(hero RunbackHero) [3]RunbackFloat { return hero.Velocity })...)
	slices.Sort(missing)
	missing = slices.Compact(missing)
	return missing
}

// runbackFloatMissing lists the absent components of one hero vector field.
func runbackFloatMissing(name string, heroes []RunbackHero, pick func(RunbackHero) [3]RunbackFloat) []string {
	var missing []string
	for _, hero := range heroes {
		for i, field := range pick(hero) {
			if !field.Present {
				missing = append(missing, fmt.Sprintf("%s.%c", name, 'x'+rune(i)))
			}
		}
	}
	return missing
}

// runbackEligibility grades the facts against the declared freshness policy.
func runbackEligibility(facts RunbackFacts, request RunbackRequest) (ReplaySegmentEligibility, []string) {
	if request.MaxFreshnessTicks == nil {
		return ReplayEligibilityNotDeclared, []string{"freshness requirement not declared"}
	}
	if facts.Correspondence.Status != ReplayCorrespondenceMatched {
		return ReplayEligibilityIneligible, []string{"replay identity correspondence is not matched"}
	}
	if len(facts.Heroes) == 0 {
		return ReplayEligibilityIneligible, []string{"no attributed hero slots"}
	}
	for _, hero := range facts.Heroes {
		for _, field := range [9]RunbackFloat{hero.Position[0], hero.Position[1], hero.Position[2], hero.Facing[0], hero.Facing[1], hero.Facing[2], hero.Velocity[0], hero.Velocity[1], hero.Velocity[2]} {
			if !field.Present || field.FreshnessTicks > *request.MaxFreshnessTicks {
				return ReplayEligibilityIneligible, []string{"hero row has missing or stale exact fields"}
			}
		}
	}
	return ReplayEligibilityEligible, nil
}

// missingRunbackPosition builds a fully typed-missing position vector.
func missingRunbackPosition() [3]RunbackFloat {
	return [3]RunbackFloat{
		{MissingReason: RunbackMissingNotInSample},
		{MissingReason: RunbackMissingNotInSample},
		{MissingReason: RunbackMissingNotInSample},
	}
}

// normalizeRunbackTickProvenance fills typed missing reasons for absent
// provenance fields so downstream consumers never see silently empty values.
func normalizeRunbackTickProvenance(p RunbackTickProvenance) RunbackTickProvenance {
	if !p.ServerTick.Present && p.ServerTick.MissingReason == "" {
		p.ServerTick.MissingReason = "no_network_tick"
	}
	if p.TickIntervalSeconds.MissingReason == "" && !p.TickIntervalSeconds.Present {
		p.TickIntervalSeconds.MissingReason = RunbackMissingNoServerInfo
	}
	if p.ServerStartTick.MissingReason == "" && !p.ServerStartTick.Present {
		p.ServerStartTick.MissingReason = RunbackMissingHeaderField
	}
	return p
}

// runbackObjectives assembles the explicit objective facts for the tick from
// the world snapshot and the objective event stream.
func runbackObjectives(samples []s2replay.EntitySample, byEntity map[int32]*s2replay.EntitySample, events []s2replay.Event, pawnSlots map[int32]int32, tick uint32) RunbackObjectives {
	out := RunbackObjectives{
		Towers:  []RunbackObjectiveEntity{},
		Walkers: []RunbackObjectiveEntity{},
	}

	// Rejuvenator state comes exclusively from rejuv_status user messages.
	// No networked rejuvenator entity class is known, so none is claimed.
	out.Rejuvenator = RunbackRejuvenator{Status: RunbackRejuvenatorStatusAbsent, Last: nil}
	var rejuvEvents []s2replay.Event
	for i := range events {
		event := &events[i]
		if event.Tick > tick {
			continue
		}
		if event.Type != s2replay.EventObjective || event.Objective == nil {
			continue
		}
		if event.Objective.Kind == RunbackRejuvenatorEventKind {
			rejuvEvents = append(rejuvEvents, *event)
		}
	}
	if len(rejuvEvents) > 0 {
		last := rejuvEvents[len(rejuvEvents)-1]
		// packet.go maps the RejuvStatus user message into ObjectiveEvent as
		// ObjectiveTeam=killing_team, ObjectiveID=event_type, EntityType=user_team.
		out.Rejuvenator = RunbackRejuvenator{
			Status: RunbackRejuvenatorStatusSeen,
			Last: &RunbackRejuvenatorEvent{
				Tick:        last.Tick,
				KillingTeam: last.Objective.ObjectiveTeam,
				EventType:   last.Objective.ObjectiveID,
				UserTeam:    last.Objective.EntityType,
			},
		}
	}

	// Objective entities come from the world snapshot by exact class match.
	for i := range samples {
		sample := &samples[i]
		if sample.ClassName != RunbackMidBossClass && sample.ClassName != RunbackTowerClass && sample.ClassName != RunbackWalkerClass {
			continue
		}
		row := runbackObjectiveEntity(sample, tick)
		switch sample.ClassName {
		case RunbackMidBossClass:
			out.MidBoss = row
		case RunbackTowerClass:
			out.Towers = append(out.Towers, row)
		case RunbackWalkerClass:
			out.Walkers = append(out.Walkers, row)
		}
	}
	slices.SortFunc(out.Towers, func(a, b RunbackObjectiveEntity) int { return int(a.EntityID - b.EntityID) })
	slices.SortFunc(out.Walkers, func(a, b RunbackObjectiveEntity) int { return int(a.EntityID - b.EntityID) })

	// An absent role entity is typed missing, never an inferred zero.
	if out.MidBoss.ClassName != RunbackMidBossClass {
		out.MidBoss = RunbackObjectiveEntity{
			Team:      RunbackInt{MissingReason: RunbackMissingNotInSample},
			Position:  missingRunbackPosition(),
			Health:    RunbackFloat{MissingReason: RunbackMissingNotInSample},
			MaxHealth: RunbackFloat{MissingReason: RunbackMissingNotInSample},
		}
	}

	// Active transients: item-class entities whose owner does not resolve to
	// an attributed pawn at the tick.
	out.Transients = runbackTransients(samples, byEntity, pawnSlots, tick)
	return out
}

// runbackObjectiveEntity converts one objective entity sample into a row.
func runbackObjectiveEntity(sample *s2replay.EntitySample, tick uint32) RunbackObjectiveEntity {
	return RunbackObjectiveEntity{
		EntityID: sample.Entity, EntitySerial: sample.EntitySerial, ClassID: sample.ClassID, ClassName: sample.ClassName,
		Team:      runbackInt(sample.Team, sample.TeamTick, sample.HasTeam, tick, RunbackMissingNotInSample),
		Position:  runbackPosition(sample, tick),
		Health:    runbackFloat(sample.Health, sample.HealthTick, sample.HasHealth, tick, RunbackMissingNotInSample),
		MaxHealth: runbackFloat(sample.MaxHealth, sample.MaxHealthTick, sample.HasHealth, tick, RunbackMissingNotInSample),
		Alive:     runbackAlive(sample, tick),
	}
}

// runbackTransients collects item-class entities whose owner handle is absent
// or does not resolve to an attributed pawn at the tick.
func runbackTransients(samples []s2replay.EntitySample, byEntity map[int32]*s2replay.EntitySample, pawnSlots map[int32]int32, tick uint32) []RunbackTransient {
	transients := []RunbackTransient{}
	for i := range samples {
		sample := &samples[i]
		if !isRunbackItemClass(sample.ClassName) {
			continue
		}
		if sample.HasOwnerEntity {
			owner := byEntity[sample.OwnerEntity]
			if owner != nil && owner.EntitySerial == sample.OwnerEntitySerial && owner.ClassName == "CCitadelPlayerPawn" {
				if _, ok := pawnSlots[sample.OwnerEntity]; ok {
					// Attributed to a hero: recorded on the hero row instead.
					continue
				}
			}
		}
		row := RunbackTransient{
			EntityID: sample.Entity, EntitySerial: sample.EntitySerial, ClassName: sample.ClassName,
			OwnerEntity:   runbackInt(sample.OwnerEntity, sample.OwnerEntityTick, sample.HasOwnerEntity, tick, RunbackMissingNotRecorded),
			Team:          runbackInt(sample.Team, sample.TeamTick, sample.HasTeam, tick, RunbackMissingNotInSample),
			Position:      runbackPosition(sample, tick),
			MissingReason: RunbackMissingOwnerUnattributed,
		}
		transients = append(transients, row)
	}
	slices.SortFunc(transients, func(a, b RunbackTransient) int { return int(a.EntityID - b.EntityID) })
	return transients
}
