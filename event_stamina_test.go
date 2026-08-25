package s2replay

import (
	"testing"

	"github.com/paralin/s2replay/protocol"
)

func TestAppendStaminaConsumedEventProjectsFields(t *testing.T) {
	p := &Parser{
		clock:             &Clock{},
		entityPlayerSlots: map[int32]int32{0x1234: 3},
	}
	msg := &protocol.CCitadelUserMsg_StaminaConsumed{}
	ent := int32(0x1234)
	before, after, max := float32(4.0), float32(2.5), float32(6.0)
	msg.EntindexTarget = &ent
	msg.StaminaBefore = &before
	msg.StaminaAfter = &after
	drained := false
	msg.Drained = &drained
	msg.StaminaMax = &max

	p.appendStaminaConsumedEvent(64, msg)
	if len(p.pendingEvents) != 1 {
		t.Fatalf("want 1 event, got %d", len(p.pendingEvents))
	}
	ev := p.pendingEvents[0]
	if ev.Type != EventStaminaConsumed {
		t.Fatalf("type: want %s, got %s", EventStaminaConsumed, ev.Type)
	}
	sc := ev.StaminaConsumed
	if sc == nil {
		t.Fatal("nil stamina payload")
	}
	if sc.EntindexTarget != 0x1234 || ev.Entity != 0x1234 || ev.PlayerSlot != 3 {
		t.Fatalf("attribution: target %d entity %d slot %d", sc.EntindexTarget, ev.Entity, ev.PlayerSlot)
	}
	if sc.StaminaBefore != 4.0 || sc.StaminaAfter != 2.5 || sc.StaminaMax != 6.0 || sc.Drained {
		t.Fatalf("stamina fields: %+v", sc)
	}

	// Unattributed messages keep the sentinel instead of masking to junk.
	unset := &protocol.CCitadelUserMsg_StaminaConsumed{}
	p.pendingEvents = nil
	p.appendStaminaConsumedEvent(96, unset)
	if len(p.pendingEvents) != 1 {
		t.Fatalf("want 1 event, got %d", len(p.pendingEvents))
	}
	if p.pendingEvents[0].Entity != -1 || p.pendingEvents[0].PlayerSlot != -1 {
		t.Fatalf("unattributed: entity %d slot %d", p.pendingEvents[0].Entity, p.pendingEvents[0].PlayerSlot)
	}
}
