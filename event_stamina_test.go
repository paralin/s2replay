package s2replay

import (
	"encoding/json"
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
	gameTime := float32(0)
	msg.Drained = &drained
	msg.StaminaMax = &max
	msg.Gametime = &gameTime

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
	if !sc.HasEntindexTarget || !sc.HasStaminaBefore || !sc.HasStaminaAfter || !sc.HasDrained ||
		!sc.HasStaminaMax || !sc.HasMessageGameTime || sc.MessageGameTime != 0 {
		t.Fatalf("present fields: %+v", sc)
	}
	encoded, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var jsonEvent struct {
		Stamina struct {
			Drained            bool    `json:"drained"`
			HasDrained         bool    `json:"has_drained"`
			MessageGameTime    float32 `json:"message_game_time"`
			HasMessageGameTime bool    `json:"has_message_game_time"`
		} `json:"stamina_consumed"`
	}
	if err := json.Unmarshal(encoded, &jsonEvent); err != nil {
		t.Fatal(err)
	}
	if jsonEvent.Stamina.Drained || !jsonEvent.Stamina.HasDrained ||
		jsonEvent.Stamina.MessageGameTime != 0 || !jsonEvent.Stamina.HasMessageGameTime {
		t.Fatalf("JSON presence did not preserve present zero: %s", encoded)
	}

	pb := ev.ToProto().StaminaConsumed
	if !pb.GetHasDrained() || !pb.GetHasMessageGameTime() {
		t.Fatalf("protobuf presence: %+v", pb)
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
	absentEvent := p.pendingEvents[0]
	absent := absentEvent.StaminaConsumed
	if absent.HasEntindexTarget || absent.HasStaminaBefore || absent.HasStaminaAfter || absent.HasDrained ||
		absent.HasStaminaMax || absent.HasMessageGameTime {
		t.Fatalf("absent fields reported present: %+v", absent)
	}
	absentPB := absentEvent.ToProto().StaminaConsumed
	if absentPB.GetHasDrained() || absentPB.GetHasMessageGameTime() {
		t.Fatalf("protobuf absence: %+v", absentPB)
	}
}
