package s2replay

import (
	"encoding/json"
	"testing"
)

func jumpTestEntity(index, serial int32, values map[string]any) *Entity {
	e := newEntity(index, serial, &entityClass{id: 1, name: "CCitadel_Ability_Jump"})
	i := 0
	for name, value := range values {
		fp := fieldPath{last: 0}
		fp.path[0] = i
		e.paths[name] = fp
		e.state.set(fp, value)
		i++
	}
	return e
}

func TestAppendJumpStateEventTracksChangesAndPresence(t *testing.T) {
	owner := newEntity(7, 3, &entityClass{id: 2, name: "CCitadelPlayerPawn"})
	ownerHandle := uint32(owner.serial<<14) | uint32(owner.index)
	jump := jumpTestEntity(20, 4, map[string]any{
		"m_hOwnerEntity":          ownerHandle,
		"m_bJumped":               false,
		"m_nDesiredAirJumpCount":  int32(0),
		"m_nConsecutiveWallJumps": int32(0),
		"m_bCanDashJump":          false,
	})
	p := &Parser{
		clock: &Clock{}, entities: map[int32]*Entity{owner.index: owner},
		entityPlayerSlots: map[int32]int32{owner.index: 6},
		jumpLastSeen:      make(map[entityEpoch]jumpState),
	}

	p.appendJumpStateEvent(10, jump)
	if len(p.pendingEvents) != 1 {
		t.Fatalf("initial events: %d", len(p.pendingEvents))
	}
	initial := p.pendingEvents[0]
	js := initial.JumpState
	if !js.InitialObservation || initial.PlayerSlot != 6 || !js.HasJumped || js.Jumped ||
		!js.HasDesiredAirJumpCount || js.DesiredAirJumpCount != 0 || !js.HasCanDashJump || js.CanDashJump {
		t.Fatalf("initial state: %+v event=%+v", js, initial)
	}
	if js.HasExecutedAirJumpCount || js.HasConsecutiveAirJumps || js.HasInSlideJump {
		t.Fatalf("missing fields reported present: %+v", js)
	}
	if pb := initial.ToProto().JumpState; !pb.GetInitialObservation() || !pb.GetHasJumped() || pb.GetJumped() {
		t.Fatalf("protobuf initial state: %+v", pb)
	}
	encoded, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	var jsonEvent struct {
		JumpState struct {
			Jumped    bool `json:"jumped"`
			HasJumped bool `json:"has_jumped"`
		} `json:"jump_state"`
	}
	if err := json.Unmarshal(encoded, &jsonEvent); err != nil {
		t.Fatal(err)
	}
	if jsonEvent.JumpState.Jumped || !jsonEvent.JumpState.HasJumped {
		t.Fatalf("JSON lost present false: %s", encoded)
	}

	p.appendJumpStateEvent(11, jump)
	if len(p.pendingEvents) != 1 {
		t.Fatalf("unchanged sample emitted: %d", len(p.pendingEvents))
	}

	fp := jump.paths["m_nConsecutiveWallJumps"]
	jump.state.set(fp, int32(1))
	p.appendJumpStateEvent(12, jump)
	changed := p.pendingEvents[1].JumpState
	if changed.InitialObservation || changed.ConsecutiveWallJumps != 1 || !changed.ChangedConsecutiveWallJumps {
		t.Fatalf("0 -> 1 wall count: %+v", changed)
	}

	jump.state.set(fp, int32(0))
	p.appendJumpStateEvent(13, jump)
	reset := p.pendingEvents[2].JumpState
	if reset.ConsecutiveWallJumps != 0 || !reset.ChangedConsecutiveWallJumps {
		t.Fatalf("wall count reset: %+v", reset)
	}
}

func TestAppendJumpStateEventSeparatesEpochsAndChecksOwnerSerial(t *testing.T) {
	owner := newEntity(7, 3, &entityClass{id: 2, name: "CCitadelPlayerPawn"})
	staleOwnerHandle := uint32(2<<14 | 7)
	values := map[string]any{"m_hOwnerEntity": staleOwnerHandle, "m_bInSlideJump": false}
	p := &Parser{
		clock: &Clock{}, entities: map[int32]*Entity{7: owner}, entityPlayerSlots: map[int32]int32{7: 6},
		jumpLastSeen: make(map[entityEpoch]jumpState),
	}
	first := jumpTestEntity(20, 4, values)
	p.appendJumpStateEvent(20, first)
	if p.pendingEvents[0].PlayerSlot != -1 {
		t.Fatalf("stale owner serial attributed to slot %d", p.pendingEvents[0].PlayerSlot)
	}
	second := jumpTestEntity(20, 5, values)
	p.appendJumpStateEvent(21, second)
	if len(p.pendingEvents) != 2 || !p.pendingEvents[1].JumpState.InitialObservation {
		t.Fatalf("new epoch did not emit initial observation: %+v", p.pendingEvents)
	}

	missing := jumpTestEntity(21, 1, nil)
	p.appendJumpStateEvent(22, missing)
	if len(p.pendingEvents) != 2 {
		t.Fatalf("fieldless jump entity emitted: %d", len(p.pendingEvents))
	}
}
