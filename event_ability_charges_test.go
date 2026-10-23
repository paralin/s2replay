package s2replay

import "testing"

func TestAppendAbilityChargeEventSeparatesEntityEpochs(t *testing.T) {
	p := &Parser{clock: &Clock{}, chargeLastSeen: make(map[entityEpoch]int32)}
	first := jumpTestEntity(20, 4, map[string]any{"m_iRemainingCharges": int32(2)})
	second := jumpTestEntity(20, 5, map[string]any{"m_iRemainingCharges": int32(2)})

	p.appendAbilityChargeEvent(10, first)
	p.appendAbilityChargeEvent(11, first)
	p.appendAbilityChargeEvent(12, second)
	if len(p.pendingEvents) != 2 {
		t.Fatalf("want one initial event per epoch, got %d", len(p.pendingEvents))
	}
	if p.pendingEvents[0].AbilityCharges.RemainingCharges != 2 ||
		p.pendingEvents[1].AbilityCharges.RemainingCharges != 2 {
		t.Fatalf("charge events: %+v", p.pendingEvents)
	}
}

func TestAppendAbilityChargeEventValidatesOwnerHandleSerial(t *testing.T) {
	owner := newEntity(7, 3, &entityClass{id: 2, name: "CCitadelPlayerPawn"})
	p := &Parser{
		clock: &Clock{}, entities: map[int32]*Entity{owner.index: owner},
		entityPlayerSlots: map[int32]int32{owner.index: 6},
		chargeLastSeen:    make(map[entityEpoch]int32),
	}
	validHandle := uint32(owner.serial<<14) | uint32(owner.index)
	valid := jumpTestEntity(20, 4, map[string]any{
		"m_iRemainingCharges": int32(2), "m_hOwnerEntity": validHandle,
	})
	p.appendAbilityChargeEvent(10, valid)
	if p.pendingEvents[0].PlayerSlot != 6 {
		t.Fatalf("valid owner slot: got %d", p.pendingEvents[0].PlayerSlot)
	}

	staleHandle := uint32(2<<14) | uint32(owner.index)
	stale := jumpTestEntity(21, 4, map[string]any{
		"m_iRemainingCharges": int32(2), "m_hOwnerEntity": staleHandle,
	})
	p.appendAbilityChargeEvent(11, stale)
	if p.pendingEvents[1].PlayerSlot != -1 {
		t.Fatalf("stale owner attributed to slot %d", p.pendingEvents[1].PlayerSlot)
	}
}

func TestAppendAbilityCastEventEmitsOncePerCooldownStart(t *testing.T) {
	owner := newEntity(7, 3, &entityClass{id: 2, name: "CCitadelPlayerPawn"})
	p := &Parser{
		clock: &Clock{}, entities: map[int32]*Entity{owner.index: owner},
		entityPlayerSlots: map[int32]int32{owner.index: 6},
		castLastSeen:      make(map[entityEpoch]float32),
	}
	handle := uint32(owner.serial<<14) | uint32(owner.index)
	ability := jumpTestEntity(20, 4, map[string]any{
		"m_flCooldownStart": float32(30), "m_hOwnerEntity": handle,
	})

	p.appendAbilityCastEvent(10, ability)
	p.appendAbilityCastEvent(11, ability)
	if len(p.pendingEvents) != 1 {
		t.Fatalf("want one cast for one cooldown start, got %d", len(p.pendingEvents))
	}
	ev := p.pendingEvents[0]
	if ev.AbilityCast.OwnerEntity != owner.index || ev.PlayerSlot != 6 {
		t.Fatalf("cast owner: %+v slot %d", ev.AbilityCast, ev.PlayerSlot)
	}

	// The framed binary form carries the cast and a hero sample's facing.
	pb := ev.ToProto()
	if pb.AbilityCast == nil || pb.AbilityCast.OwnerEntity != owner.index {
		t.Fatalf("cast lost in protobuf: %+v", pb.AbilityCast)
	}
	facing := Event{EntitySample: &EntitySample{FacingX: 5, FacingY: 90, HasFacing: true}}
	sampled := facing.ToProto()
	if !sampled.EntitySample.HasFacing || sampled.EntitySample.FacingY != 90 {
		t.Fatalf("facing lost in protobuf: %+v", sampled.EntitySample)
	}
}
