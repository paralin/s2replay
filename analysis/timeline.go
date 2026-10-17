package analysis

import (
	"errors"
	"io"
	"math"

	"github.com/paralin/s2replay"
)

// timelineStep is the game time between timeline samples, in seconds.
const timelineStep = 0.5

// objectScanSamples is how many samples pass between scans of the entity table
// for map objects that appeared since the last scan.
const objectScanSamples = 20

// clockDriftSeconds is how far the match clock may drift from the samples
// before the timeline records a new clock entry.
const clockDriftSeconds = 0.25

// objectPathUnits is how far an object moves before its path records it.
const objectPathUnits = 100

// maxEntities is the size of the entity table the object scan walks.
const maxEntities = 8192

// entityTeamOffset converts the post-match record's team numbers, which start
// at 0, to the entities' team numbers, which start at 2.
const entityTeamOffset = 2

// steamID64Base converts a 64-bit Steam ID to its account ID.
const steamID64Base = 76561197960265728

// timelineObjectKinds maps the entity classes of tracked map objects to their
// kind.
var timelineObjectKinds = map[string]TimelineObjectKind{
	"CNPC_TrooperBoss":              TimelineObjectKind_TIMELINE_OBJECT_KIND_GUARDIAN,
	"CNPC_Boss_Tier2":               TimelineObjectKind_TIMELINE_OBJECT_KIND_WALKER,
	"CNPC_BarrackBoss":              TimelineObjectKind_TIMELINE_OBJECT_KIND_BASE_GUARDIAN,
	"CCitadel_Destroyable_Building": TimelineObjectKind_TIMELINE_OBJECT_KIND_SHRINE,
	"CNPC_Boss_Tier3":               TimelineObjectKind_TIMELINE_OBJECT_KIND_PATRON,
	"CNPC_MidBoss":                  TimelineObjectKind_TIMELINE_OBJECT_KIND_MID_BOSS,
	"CNPC_Neutral_SinnersSacrifice": TimelineObjectKind_TIMELINE_OBJECT_KIND_SINNERS_SACRIFICE,
	"CCitadelItemPickupIdol":        TimelineObjectKind_TIMELINE_OBJECT_KIND_URN,
	"CCitadelIdolReturnTrigger":     TimelineObjectKind_TIMELINE_OBJECT_KIND_URN_RETURN,
}

// timelineObjectiveKinds maps the parser's objective event kinds to the
// timeline's; other kinds, such as boss damage, which object health already
// records, are left out.
var timelineObjectiveKinds = map[string]TimelineObjectiveKind{
	"boss_killed":      TimelineObjectiveKind_TIMELINE_OBJECTIVE_KIND_BOSS_KILLED,
	"mid_boss_spawned": TimelineObjectiveKind_TIMELINE_OBJECTIVE_KIND_MID_BOSS_SPAWNED,
	"rejuv_status":     TimelineObjectiveKind_TIMELINE_OBJECTIVE_KIND_REJUVENATOR,
}

// damageTarget is the column a hero's damage to a non-hero counts toward.
type damageTarget int

const (
	// targetNone is damage the timeline does not count.
	targetNone damageTarget = iota
	// targetFarm is damage to troopers and neutral camps.
	targetFarm
	// targetObjective is damage to Guardians, Walkers, Base Guardians, Shrines
	// and the Patron.
	targetObjective
	// targetMidBoss is damage to the Mid-Boss.
	targetMidBoss
)

// damageTargetOf returns the column for a damage event's victim class. The
// values are the game's own target types, measured against entity classes: 4
// troopers, 5 Guardians, 6 neutral camps (inferred), 8 the Mid-Boss, 28
// Shrines, 29 Walkers, 30 Base Guardians and 31 the Patron. Other classes,
// such as 25 for soul orbs, are not counted.
func damageTargetOf(victimClass uint32) damageTarget {
	switch victimClass {
	case 4, 6:
		return targetFarm
	case 5, 28, 29, 30, 31:
		return targetObjective
	case 8:
		return targetMidBoss
	}
	return targetNone
}

// BuildMatchTimeline reads a replay from the parser's current position to its
// end and reduces it to a timeline.
func BuildMatchTimeline(p *s2replay.Parser) (*MatchTimeline, error) {
	b := newTimelineBuilder(p)
	for {
		ev, err := p.NextEvent()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		b.accept(ev)
	}
	return b.finish(), nil
}

// newTimelineBuilder starts an empty timeline of the parser's replay.
func newTimelineBuilder(p *s2replay.Parser) *timelineBuilder {
	return &timelineBuilder{
		parser:  p,
		out:     &MatchTimeline{StepSeconds: timelineStep},
		byCtrl:  make(map[int32]*timelinePlayer),
		byPawn:  make(map[int32]*timelinePlayer),
		objects: make(map[int32]*timelineObject),
		rules:   -1,
	}
}

// timelineBuilder accumulates a timeline from a replay's events.
type timelineBuilder struct {
	// parser is the replay, read for map object samples.
	parser *s2replay.Parser
	// out is the timeline under construction; its player list is filled at
	// the end.
	out *MatchTimeline
	// players lists every controller seen, in order of first appearance.
	players []*timelinePlayer
	// byCtrl and byPawn find a player by controller and hero entity.
	byCtrl, byPawn map[int32]*timelinePlayer
	// objects holds the tracked map objects still alive, by entity.
	objects map[int32]*timelineObject
	// allObjects lists every tracked map object, in order of first sighting.
	allObjects []*timelineObject
	// samples is the number of samples taken.
	samples int
	// rules is the game rules entity, or -1 until a scan finds it.
	rules int32
	// hits, casts and purchases hold events by player until the player list
	// is final.
	hits      []timelineHit
	casts     []timelineCast
	purchases []timelinePurchase
}

// timelinePlayer is a controller and its hero's latest state.
type timelinePlayer struct {
	// row is the player's output record.
	row *TimelinePlayer
	// controller is the controller entity.
	controller int32
	// netWorth is the controller's latest net worth.
	netWorth int32
	// damagedMaxHealth is the hero's maximum health from damage events, which
	// includes items where the pawn's own field does not.
	damagedMaxHealth float32
	// frame is the hero's latest state; seen is false until the hero appears.
	frame timelineFrame
	seen  bool
	// index is the player's index in the output, assigned at the end.
	index uint32
}

// timelineFrame is one hero's state.
type timelineFrame struct {
	x, y, z, health, maxHealth, yaw, speed float32
	alive                                  bool
}

// timelineHit, timelineCast and timelinePurchase are events waiting for their
// players' output indexes.
type timelineHit struct {
	attacker, victim *timelinePlayer
	hit              *TimelineHit
}

type timelineCast struct {
	player *timelinePlayer
	cast   *TimelineCast
}

type timelinePurchase struct {
	player   *timelinePlayer
	purchase *TimelinePurchase
}

// timelineObject is a tracked map object and the entity serial that
// identifies its incarnation.
type timelineObject struct {
	row    *TimelineObject
	entity int32
	serial int32
}

// accept folds one event into the timeline, then samples up to its time.
func (b *timelineBuilder) accept(ev s2replay.Event) {
	t := ev.GameTime
	b.out.DurationSeconds = max(b.out.DurationSeconds, t)
	switch ev.Type {
	case s2replay.EventControllerSample:
		b.acceptController(ev)
	case s2replay.EventEntitySample:
		b.acceptPawn(ev.EntitySample)
	case s2replay.EventDamage:
		b.acceptDamage(t, ev.Damage)
	case s2replay.EventImportantAbilityUsed:
		// Attribute the cast to the hero whose handle it carries.
		u := ev.ImportantAbilityUsed
		if pl := b.byPawn[int32(u.Caster&0x3fff)]; pl != nil {
			b.casts = append(b.casts, timelineCast{pl, &TimelineCast{Seconds: t, Ability: u.AbilityName}})
		}
	case s2replay.EventPurchase:
		// Attribute the purchase to the player in its slot.
		pu := ev.Purchase
		for _, pl := range b.players {
			if pu.PlayerSlot >= 0 && pl.row.GetSlot() == pu.PlayerSlot {
				b.purchases = append(b.purchases, timelinePurchase{pl, &TimelinePurchase{Seconds: t, ItemId: pu.AbilityID, Sell: pu.Sell}})
			}
		}
	case s2replay.EventObjective:
		o := ev.Objective
		if kind, ok := timelineObjectiveKinds[o.Kind]; ok {
			b.out.Objectives = append(b.out.Objectives, &TimelineObjective{Seconds: t, Kind: kind, Team: o.ObjectiveTeam, Detail: o.ObjectiveID})
		}
	case s2replay.EventPostMatch:
		// The post-match record numbers teams from 0, entities from 2.
		if ev.PostMatch != nil {
			b.out.WinningTeam = ev.PostMatch.WinningTeam + entityTeamOffset
		}
	}
	for float64(b.samples)*timelineStep <= t {
		b.sample()
	}
}

// player returns the player for a controller, adding one on first sight.
func (b *timelineBuilder) player(controller int32) *timelinePlayer {
	pl := b.byCtrl[controller]
	if pl == nil {
		pl = &timelinePlayer{row: &TimelinePlayer{Slot: -1}, controller: controller}
		b.byCtrl[controller] = pl
		b.players = append(b.players, pl)
	}
	return pl
}

// acceptController records a controller's identity and net worth.
func (b *timelineBuilder) acceptController(ev s2replay.Event) {
	c := ev.ControllerSample
	pl := b.player(c.Entity)
	if c.SteamID != 0 {
		pl.row.AccountId = uint32(c.SteamID - steamID64Base)
	}
	if c.PlayerName != "" {
		pl.row.Name = c.PlayerName
	}
	if ev.PlayerSlot >= 0 {
		pl.row.Slot = ev.PlayerSlot
	}
	pl.netWorth = c.NetWorth
}

// acceptPawn records a hero's latest state.
func (b *timelineBuilder) acceptPawn(s *s2replay.EntitySample) {
	// Find the hero's player.
	if s.ClassName != "CCitadelPlayerPawn" || !s.HasOwnerEntity {
		return
	}
	pl := b.player(s.OwnerEntity)
	b.byPawn[s.Entity] = pl
	if s.HasHeroID && s.HeroID != 0 {
		pl.row.HeroId = s.HeroID
	}
	if s.HasTeam && s.Team != 0 {
		pl.row.Team = s.Team
	}

	// Update the fields the sample carries.
	f := &pl.frame
	pl.seen = true
	if s.HasPosition {
		f.x, f.y, f.z = s.PositionX, s.PositionY, s.PositionZ
	}
	if s.HasHealth {
		f.health, f.maxHealth = s.Health, max(s.MaxHealth, pl.damagedMaxHealth, s.Health)
	}
	f.alive = !s.HasLifeState || s.LifeState == 0
	if s.HasCameraAngles[1] {
		f.yaw = s.CameraAngles[1]
	}
	if s.HasVelocity {
		f.speed = float32(math.Hypot(float64(s.VelocityX), float64(s.VelocityY)))
	}
}

// acceptDamage records hero to hero damage between enemies and a hero's
// damage to farm and objectives.
func (b *timelineBuilder) acceptDamage(t float64, d *s2replay.DamageEvent) {
	attacker, victim := b.byPawn[d.Attacker], b.byPawn[d.Victim]
	if victim != nil && d.VictimHealthMax > 0 {
		victim.damagedMaxHealth = float32(d.VictimHealthMax)
	}
	switch {
	case attacker == nil:
	case victim != nil:
		if attacker.row.GetTeam() != victim.row.GetTeam() {
			b.hits = append(b.hits, timelineHit{attacker, victim, &TimelineHit{
				Seconds: t, Amount: d.HealthLost + int32(d.DamageAbsorbed), AbilityId: d.AbilityID,
			}})
		}
	default:
		if column := attacker.damage(damageTargetOf(d.VictimClass)); column != nil {
			k := int(t / timelineStep)
			for len(*column) <= k {
				*column = append(*column, 0)
			}
			(*column)[k] += d.Damage
		}
	}
}

// damage returns the player's damage column for a target, or nil.
func (pl *timelinePlayer) damage(target damageTarget) *[]int32 {
	switch target {
	case targetFarm:
		return &pl.row.FarmDamage
	case targetObjective:
		return &pl.row.ObjectiveDamage
	case targetMidBoss:
		return &pl.row.MidBossDamage
	}
	return nil
}

// sample takes the next sample of every hero and map object.
func (b *timelineBuilder) sample() {
	k := b.samples
	b.samples++
	b.sampleObjects(k)
	for _, pl := range b.players {
		// A hero not yet seen is backfilled when it appears.
		if !pl.seen {
			continue
		}
		for len(pl.row.X) < b.samples {
			pl.appendFrame()
		}
	}
}

// appendFrame appends the hero's latest state to its columns.
func (pl *timelinePlayer) appendFrame() {
	r, f := pl.row, &pl.frame
	r.X = append(r.X, f.x)
	r.Y = append(r.Y, f.y)
	r.Z = append(r.Z, f.z)
	r.Health = append(r.Health, f.health)
	r.MaxHealth = append(r.MaxHealth, f.maxHealth)
	r.Alive = append(r.Alive, f.alive)
	r.Yaw = append(r.Yaw, f.yaw)
	r.NetWorth = append(r.NetWorth, pl.netWorth)
	r.Speed = append(r.Speed, f.speed)
}

// sampleClock records the match clock at sample k when it has drifted from
// the last entry, as it does at the match start and through a pause.
func (b *timelineBuilder) sampleClock(k int) {
	if b.rules < 0 {
		return
	}
	match, err := s2replay.MatchClock(b.parser.FindEntity(b.rules), b.parser.Clock())
	if err != nil {
		return
	}
	clock := b.out.Clock
	if n := len(clock); n != 0 {
		last := clock[n-1]
		predicted := last.GetMatchSeconds() + float64(k-int(last.GetSample()))*b.out.GetStepSeconds()
		if math.Abs(predicted-match) <= clockDriftSeconds {
			return
		}
	}
	b.out.Clock = append(clock, &TimelineClock{Sample: uint32(k), MatchSeconds: match})
}

// sampleObjects records each tracked object's changes at sample k, and every
// objectScanSamples looks for objects that appeared.
func (b *timelineBuilder) sampleObjects(k int) {
	// Look for new objects and the game rules, and read the match clock.
	if k%objectScanSamples == 0 {
		for e := range int32(maxEntities) {
			if b.objects[e] != nil {
				continue
			}
			en := b.parser.FindEntity(e)
			if en == nil {
				continue
			}
			if b.rules < 0 && en.ClassName() == "CCitadelGameRulesProxy" {
				b.rules = e
			}
			kind, ok := timelineObjectKinds[en.ClassName()]
			if !ok {
				continue
			}
			s, ok := b.parser.SampleEntity(en)
			if !ok || (s.HasLifeState && s.LifeState != 0) {
				continue
			}
			o := &timelineObject{entity: e, serial: s.EntitySerial, row: &TimelineObject{
				Kind: kind, Team: s.Team, X: s.PositionX, Y: s.PositionY, Z: s.PositionZ, MaxHealth: s.MaxHealth, FirstSample: uint32(k),
			}}
			b.objects[e] = o
			b.allObjects = append(b.allObjects, o)
		}
		b.sampleClock(k)
	}

	// Record each live object's health and moves, and retire the gone.
	for e, o := range b.objects {
		r := o.row
		s, ok := b.parser.SampleEntity(b.parser.FindEntity(e))
		if !ok || s.EntitySerial != o.serial || timelineObjectKinds[s.ClassName] != r.Kind || (s.HasLifeState && s.LifeState != 0) {
			r.GoneSample = uint32(k)
			delete(b.objects, e)
			continue
		}
		if s.HasHealth && (len(r.Health) == 0 || r.Health[len(r.Health)-1] != s.Health) {
			r.HealthSamples = append(r.HealthSamples, uint32(k))
			r.Health = append(r.Health, s.Health)
		}
		if s.HasPosition {
			x, y := r.X, r.Y
			if n := len(r.PathX); n > 0 {
				x, y = r.PathX[n-1], r.PathY[n-1]
			}
			if dx, dy := s.PositionX-x, s.PositionY-y; dx*dx+dy*dy > objectPathUnits*objectPathUnits {
				r.PathSamples = append(r.PathSamples, uint32(k))
				r.PathX = append(r.PathX, s.PositionX)
				r.PathY = append(r.PathY, s.PositionY)
			}
		}
	}
}

// finish keeps the players who controlled a hero, aligns their columns to the
// sample count, and resolves events to player indexes.
func (b *timelineBuilder) finish() *MatchTimeline {
	// Keep the hero players and align their columns.
	out := b.out
	out.TickInterval = b.parser.Clock().TickInterval()
	kept := func(pl *timelinePlayer) bool {
		return pl.row.GetHeroId() != 0 && pl.seen
	}
	for _, pl := range b.players {
		if !kept(pl) {
			continue
		}
		pl.index = uint32(len(out.Players))
		out.Players = append(out.Players, pl.row)
		for len(pl.row.X) < b.samples {
			pl.appendFrame()
		}
		for _, target := range []damageTarget{targetFarm, targetObjective, targetMidBoss} {
			column := pl.damage(target)
			*column = append(*column, make([]int32, max(0, b.samples-len(*column)))...)[:b.samples]
		}
	}

	// Resolve the events of kept players.
	for _, h := range b.hits {
		if kept(h.attacker) && kept(h.victim) {
			h.hit.Attacker, h.hit.Victim = h.attacker.index, h.victim.index
			out.Hits = append(out.Hits, h.hit)
		}
	}
	for _, c := range b.casts {
		if kept(c.player) {
			c.cast.Player = c.player.index
			out.Casts = append(out.Casts, c.cast)
		}
	}
	for _, pu := range b.purchases {
		if kept(pu.player) {
			pu.purchase.Player = pu.player.index
			out.Purchases = append(out.Purchases, pu.purchase)
		}
	}
	for _, o := range b.allObjects {
		out.Objects = append(out.Objects, o.row)
	}
	return out
}
