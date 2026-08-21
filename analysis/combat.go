package analysis

import (
	"slices"

	"github.com/paralin/s2replay"
)

// CombatWindowOptions defines caller-owned combat grouping policy.
type CombatWindowOptions struct {
	MaxGap  float64                   `json:"-"`
	Include func(s2replay.Event) bool `json:"-"`
}

// CombatWindow is a deterministic group of caller-selected replay events.
type CombatWindow struct {
	StartTick       uint32   `json:"start_tick"`
	EndTick         uint32   `json:"end_tick"`
	StartTime       float64  `json:"start_time"`
	EndTime         float64  `json:"end_time"`
	Events          int      `json:"events"`
	DamageEvents    int      `json:"damage_events"`
	ModifierEvents  int      `json:"modifier_events"`
	PurchaseEvents  int      `json:"purchase_events"`
	EntitySamples   int      `json:"entity_samples"`
	FirstEventIndex int      `json:"first_event_index"`
	LastEventIndex  int      `json:"last_event_index"`
	PlayerSlots     []int32  `json:"player_slots,omitempty"`
	Entities        []int32  `json:"entities,omitempty"`
	DamageAttackers []int32  `json:"damage_attackers,omitempty"`
	DamageVictims   []int32  `json:"damage_victims,omitempty"`
	ModifierParents []uint32 `json:"modifier_parents,omitempty"`
}

// BuildCombatWindows groups selected events with caller-supplied timing policy.
func BuildCombatWindows(events []s2replay.Event, opts CombatWindowOptions) []CombatWindow {
	include := opts.Include
	if include == nil {
		include = func(s2replay.Event) bool { return true }
	}
	var windows []CombatWindow
	var active combatWindowState
	entityPlayers := make(map[int32]int32)
	var lastTime float64
	activeWindow := false
	for i, ev := range events {
		rememberEntityPlayerSlot(entityPlayers, ev)
		if !include(ev) {
			continue
		}
		if !activeWindow || ev.GameTime-lastTime > opts.MaxGap {
			if activeWindow {
				windows = append(windows, active.finish())
			}
			active = newCombatWindowState(ev, i, entityPlayers)
			activeWindow = true
			lastTime = ev.GameTime
			continue
		}
		active.add(ev, i)
		lastTime = ev.GameTime
	}
	if activeWindow {
		windows = append(windows, active.finish())
	}
	return windows
}

type combatWindowState struct {
	window          CombatWindow
	entityPlayers   map[int32]int32
	playerSlots     map[int32]struct{}
	entities        map[int32]struct{}
	damageAttackers map[int32]struct{}
	damageVictims   map[int32]struct{}
	modifierParents map[uint32]struct{}
}

func newCombatWindowState(ev s2replay.Event, index int, entityPlayers map[int32]int32) combatWindowState {
	state := combatWindowState{
		window: CombatWindow{
			StartTick:       ev.Tick,
			EndTick:         ev.Tick,
			StartTime:       ev.GameTime,
			EndTime:         ev.GameTime,
			FirstEventIndex: index,
			LastEventIndex:  index,
		},
		entityPlayers:   entityPlayers,
		playerSlots:     make(map[int32]struct{}),
		entities:        make(map[int32]struct{}),
		damageAttackers: make(map[int32]struct{}),
		damageVictims:   make(map[int32]struct{}),
		modifierParents: make(map[uint32]struct{}),
	}
	state.add(ev, index)
	return state
}

func (s *combatWindowState) add(ev s2replay.Event, index int) {
	s.window.EndTick = ev.Tick
	s.window.EndTime = ev.GameTime
	s.window.Events++
	s.window.LastEventIndex = index
	if ev.PlayerSlot >= 0 {
		s.playerSlots[ev.PlayerSlot] = struct{}{}
	}
	if ev.Entity >= 0 {
		s.entities[ev.Entity] = struct{}{}
		if slot, ok := s.entityPlayers[ev.Entity]; ok {
			s.playerSlots[slot] = struct{}{}
		}
	}
	switch ev.Type {
	case s2replay.EventDamage:
		s.window.DamageEvents++
		if ev.Damage != nil {
			s.damageAttackers[ev.Damage.Attacker] = struct{}{}
			s.damageVictims[ev.Damage.Victim] = struct{}{}
			s.entities[ev.Damage.Attacker] = struct{}{}
			s.entities[ev.Damage.Victim] = struct{}{}
			if slot, ok := s.entityPlayers[ev.Damage.Victim]; ok {
				s.playerSlots[slot] = struct{}{}
			}
		}
	case s2replay.EventModifier:
		s.window.ModifierEvents++
		if ev.Modifier != nil && ev.Modifier.Parent != 0 {
			s.modifierParents[ev.Modifier.Parent] = struct{}{}
		}
	case s2replay.EventPurchase:
		s.window.PurchaseEvents++
	case s2replay.EventEntitySample:
		s.window.EntitySamples++
	}
}

func rememberEntityPlayerSlot(entityPlayers map[int32]int32, ev s2replay.Event) {
	if ev.Type != s2replay.EventEntitySample || ev.EntitySample == nil || ev.PlayerSlot < 0 {
		return
	}
	entityPlayers[ev.Entity] = ev.PlayerSlot
}

func (s *combatWindowState) finish() CombatWindow {
	out := s.window
	out.PlayerSlots = sortedInt32Keys(s.playerSlots)
	out.Entities = sortedInt32Keys(s.entities)
	out.DamageAttackers = sortedInt32Keys(s.damageAttackers)
	out.DamageVictims = sortedInt32Keys(s.damageVictims)
	out.ModifierParents = sortedUint32Keys(s.modifierParents)
	return out
}

func sortedInt32Keys(values map[int32]struct{}) []int32 {
	if len(values) == 0 {
		return nil
	}
	out := make([]int32, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func sortedUint32Keys(values map[uint32]struct{}) []uint32 {
	if len(values) == 0 {
		return nil
	}
	out := make([]uint32, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
