package s2replay

import (
	"encoding/json"
	"testing"

	"github.com/paralin/s2replay/protocol"
)

func TestAppendImportantAbilityUsedEventPreservesPresenceAndValidatesHandle(t *testing.T) {
	playerEntity := newEntity(0, 0, &entityClass{id: 1, name: "CCitadelPlayerPawn"})
	p := &Parser{clock: &Clock{}, entities: map[int32]*Entity{0: playerEntity}, entityPlayerSlots: map[int32]int32{0: 5}}
	player, caster, name := uint32(0), uint32(0), ""
	p.appendImportantAbilityUsedEvent(32, &protocol.CCitadelUserMessage_ImportantAbilityUsed{
		Player: &player, Caster: &caster, AbilityName: &name,
	})
	ev := p.pendingEvents[0]
	iau := ev.ImportantAbilityUsed
	if ev.Entity != 0 || ev.PlayerSlot != 5 || iau.Player != 0 || iau.Caster != 0 || iau.AbilityName != "" ||
		!iau.HasPlayer || !iau.HasCaster || !iau.HasAbilityName {
		t.Fatalf("present zero/empty event: %+v", ev)
	}
	pb := ev.ToProto().ImportantAbilityUsed
	if !pb.GetHasPlayer() || !pb.GetHasCaster() || !pb.GetHasAbilityName() {
		t.Fatalf("protobuf presence: %+v", pb)
	}
	encoded, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var jsonEvent struct {
		ImportantAbilityUsed struct {
			AbilityName    string `json:"ability_name"`
			HasAbilityName bool   `json:"has_ability_name"`
		} `json:"important_ability_used"`
	}
	if err := json.Unmarshal(encoded, &jsonEvent); err != nil ||
		jsonEvent.ImportantAbilityUsed.AbilityName != "" || !jsonEvent.ImportantAbilityUsed.HasAbilityName {
		t.Fatalf("JSON present empty: %s err=%v", encoded, err)
	}

	p.pendingEvents = nil
	p.appendImportantAbilityUsedEvent(33, &protocol.CCitadelUserMessage_ImportantAbilityUsed{})
	absent := p.pendingEvents[0]
	if absent.Entity != -1 || absent.PlayerSlot != -1 || absent.ImportantAbilityUsed.HasPlayer ||
		absent.ImportantAbilityUsed.HasCaster || absent.ImportantAbilityUsed.HasAbilityName {
		t.Fatalf("absent fields: %+v", absent)
	}
	absentJSON, err := json.Marshal(absent)
	if err != nil {
		t.Fatal(err)
	}
	jsonEvent = struct {
		ImportantAbilityUsed struct {
			AbilityName    string `json:"ability_name"`
			HasAbilityName bool   `json:"has_ability_name"`
		} `json:"important_ability_used"`
	}{}
	if err := json.Unmarshal(absentJSON, &jsonEvent); err != nil || jsonEvent.ImportantAbilityUsed.HasAbilityName {
		t.Fatalf("JSON absent empty: %s err=%v", absentJSON, err)
	}
	absentPB := absent.ToProto().ImportantAbilityUsed
	if absentPB.GetHasPlayer() || absentPB.GetHasCaster() || absentPB.GetHasAbilityName() {
		t.Fatalf("absent protobuf fields: %+v", absentPB)
	}

	stalePlayer := uint32(1 << 14)
	p.pendingEvents = nil
	p.appendImportantAbilityUsedEvent(34, &protocol.CCitadelUserMessage_ImportantAbilityUsed{Player: &stalePlayer})
	stale := p.pendingEvents[0]
	if stale.Entity != -1 || stale.PlayerSlot != -1 || !stale.ImportantAbilityUsed.HasPlayer ||
		stale.ImportantAbilityUsed.Player != stalePlayer {
		t.Fatalf("stale full handle: %+v", stale)
	}
}

func TestAppendAbilityNotifyEventPreservesPresence(t *testing.T) {
	p := &Parser{clock: &Clock{}, entityPlayerSlots: map[int32]int32{0: 5}}
	victim, attacker := int32(0), int32(0)
	abilityID, impact := uint32(0), uint32(0)
	p.appendAbilityNotifyEvent(48, &protocol.CCitadelUserMessage_AbilityNotify{
		EntindexVictim: &victim, EntindexAttacker: &attacker, AbilityId: &abilityID, StatusImpact: &impact,
	})
	ev := p.pendingEvents[0]
	an := ev.AbilityNotify
	if ev.Entity != 0 || ev.PlayerSlot != 5 || an.EntindexVictim != 0 || an.AbilityID != 0 ||
		!an.HasEntindexVictim || !an.HasEntindexAttacker || !an.HasAbilityID || !an.HasStatusImpact {
		t.Fatalf("present zero event: %+v", ev)
	}
	pb := ev.ToProto().AbilityNotify
	if !pb.GetHasEntindexVictim() || !pb.GetHasEntindexAttacker() || !pb.GetHasAbilityId() || !pb.GetHasStatusImpact() {
		t.Fatalf("protobuf presence: %+v", pb)
	}

	p.pendingEvents = nil
	p.appendAbilityNotifyEvent(49, &protocol.CCitadelUserMessage_AbilityNotify{})
	absent := p.pendingEvents[0]
	an = absent.AbilityNotify
	if absent.Entity != -1 || absent.PlayerSlot != -1 || an.HasEntindexVictim || an.HasEntindexAttacker ||
		an.HasAbilityID || an.HasStatusImpact {
		t.Fatalf("absent fields: %+v", absent)
	}
	pb = absent.ToProto().AbilityNotify
	if pb.GetHasEntindexVictim() || pb.GetHasEntindexAttacker() || pb.GetHasAbilityId() || pb.GetHasStatusImpact() {
		t.Fatalf("absent protobuf fields: %+v", pb)
	}
}
