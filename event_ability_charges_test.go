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
