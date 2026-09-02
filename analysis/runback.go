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
const RunbackFactsSchemaVersion = 1

// RunbackMissingReason values name why an observed field is absent.
const (
	RunbackMissingNotInSample  = "not_in_sample"
	RunbackMissingNonFinite    = "non_finite_source_value"
	RunbackMissingNoEntity     = "no_entity_sample_at_tick"
	RunbackMissingUnattributed = "player_slot_unattributed"
	RunbackMissingNotRecorded  = "not_recorded_at_tick"
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
	Heroes             []RunbackHero                `json:"heroes"`
	WorldEntities      []RunbackWorldEntity         `json:"world_entities"`
	Quality            RunbackFactsQuality          `json:"quality"`
	Eligibility        ReplaySegmentEligibility     `json:"eligibility"`
	EligibilityReasons []string                     `json:"eligibility_reasons,omitempty"`
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
	EntityID     int32  `json:"entity_id"`
	EntitySerial int32  `json:"entity_serial"`
	ClassName    string `json:"class_name"`
	SourceTick   uint32 `json:"source_tick"`
}

// RunbackAbility is one owned ability entity with charge and cooldown state.
type RunbackAbility struct {
	EntityID     int32  `json:"entity_id"`
	EntitySerial int32  `json:"entity_serial"`
	ClassName    string `json:"class_name"`

	Charges     RunbackInt   `json:"charges"`
	CooldownEnd RunbackFloat `json:"cooldown_end"`
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
	return buildRunbackFacts(samples, timelines, ReplaySourceIdentity{
		SHA256:         sha256Hex(demo),
		Game:           header.GetGame(),
		Map:            header.GetMapName(),
		GameBuild:      header.GetBuildNum(),
		Parser:         "s2replay",
		ParserRevision: s2replay.ParserSourceDigest,
		VCSRevision:    revision,
	}, request)
}

// buildRunbackFacts assembles deterministic facts from the snapshot and timelines.
func buildRunbackFacts(samples []s2replay.EntitySample, timelines Result, source ReplaySourceIdentity, request RunbackRequest) (RunbackFacts, error) {
	tick := request.Tick
	out := RunbackFacts{
		SchemaVersion: RunbackFactsSchemaVersion,
		Source:        source,
		Tick:          tick,
		Heroes:        []RunbackHero{},
		WorldEntities: []RunbackWorldEntity{},
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
		if pawn := byEntity[sample.PawnEntity]; pawn != nil && pawn.ClassName == "CCitadelPlayerPawn" {
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
		hero.Items = runbackItems(samples, sample.Entity, tick)
		hero.Abilities = runbackAbilities(samples, sample.Entity, tick)
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

	out.Quality = RunbackFactsQuality{
		Heroes: len(out.Heroes), WorldEntities: len(out.WorldEntities), SnapshotEntities: len(samples),
		UnattributedPawns: unattributedPawns,
	}
	out.Quality.MissingFields = runbackMissingFields(out)
	out.Eligibility, out.EligibilityReasons = runbackEligibility(out, request)
	return out, nil
}

func isRunbackControllerClass(name string) bool {
	return strings.Contains(name, "CitadelPlayerController")
}
func isRunbackAbilityClass(name string) bool { return strings.Contains(name, "Ability") }
func isRunbackItemClass(name string) bool {
	return strings.Contains(name, "CCitadel_Item_") || strings.Contains(name, "CCitadelItem")
}

func runbackPosition(sample *s2replay.EntitySample, tick uint32) [3]RunbackFloat {
	return [3]RunbackFloat{
		runbackFloat(sample.PositionX, sample.PositionXTick, sample.HasPosition, tick, RunbackMissingNotInSample),
		runbackFloat(sample.PositionY, sample.PositionYTick, sample.HasPosition, tick, RunbackMissingNotInSample),
		runbackFloat(sample.PositionZ, sample.PositionZTick, sample.HasPosition, tick, RunbackMissingNotInSample),
	}
}

func runbackVector3(x, y, z float32, xTick, yTick, zTick uint32, hasX, hasY, hasZ bool, tick uint32, reason string) [3]RunbackFloat {
	return [3]RunbackFloat{
		runbackFloat(x, xTick, hasX, tick, reason),
		runbackFloat(y, yTick, hasY, tick, reason),
		runbackFloat(z, zTick, hasZ, tick, reason),
	}
}

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

func missingRunbackInt(requestedTick uint32, reason string) RunbackInt {
	return RunbackInt{MissingReason: reason}
}

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

func runbackItems(samples []s2replay.EntitySample, pawnEntity int32, tick uint32) []RunbackItem {
	items := []RunbackItem{}
	for i := range samples {
		sample := &samples[i]
		if !isRunbackItemClass(sample.ClassName) || !sample.HasOwnerEntity || sample.OwnerEntity != pawnEntity {
			continue
		}
		items = append(items, RunbackItem{EntityID: sample.Entity, EntitySerial: sample.EntitySerial, ClassName: sample.ClassName, SourceTick: sample.Tick})
	}
	slices.SortFunc(items, func(a, b RunbackItem) int { return int(a.EntityID - b.EntityID) })
	return items
}

func runbackAbilities(samples []s2replay.EntitySample, pawnEntity int32, tick uint32) []RunbackAbility {
	abilities := []RunbackAbility{}
	for i := range samples {
		sample := &samples[i]
		if !isRunbackAbilityClass(sample.ClassName) || !sample.HasOwnerEntity || sample.OwnerEntity != pawnEntity {
			continue
		}
		abilities = append(abilities, RunbackAbility{
			EntityID: sample.Entity, EntitySerial: sample.EntitySerial, ClassName: sample.ClassName,
			Charges:     runbackInt(sample.RemainingCharges, sample.RemainingChargesTick, sample.HasRemainingCharges, tick, RunbackMissingNotInSample),
			CooldownEnd: runbackFloat(sample.CooldownEnd, sample.CooldownEndTick, sample.HasCooldownEnd, tick, RunbackMissingNotInSample),
		})
	}
	slices.SortFunc(abilities, func(a, b RunbackAbility) int { return int(a.EntityID - b.EntityID) })
	return abilities
}

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

func runbackMissingFields(facts RunbackFacts) []string {
	var missing []string
	missing = append(missing, runbackFloatMissing("position", facts.Heroes, func(hero RunbackHero) [3]RunbackFloat { return hero.Position })...)
	missing = append(missing, runbackFloatMissing("facing", facts.Heroes, func(hero RunbackHero) [3]RunbackFloat { return hero.Facing })...)
	missing = append(missing, runbackFloatMissing("velocity", facts.Heroes, func(hero RunbackHero) [3]RunbackFloat { return hero.Velocity })...)
	slices.Sort(missing)
	missing = slices.Compact(missing)
	return missing
}

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
