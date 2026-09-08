package s2replay

import (
	"testing"

	"github.com/paralin/s2replay/protocol"
)

func TestStaminaConsumedProtoRoundTrip(t *testing.T) {
	event := &Event{
		SchemaVersion: EventSchemaVersion,
		Type:          EventStaminaConsumed,
		Tick:          640,
		GameTime:      10.25,
		Entity:        123,
		PlayerSlot:    4,
		StaminaConsumed: &StaminaConsumedEvent{
			Tick: 640, GameTime: 10.25, EntindexTarget: 123,
			StaminaBefore: 3.5, StaminaAfter: 2.5, Drained: true, StaminaMax: 4,
		},
	}

	decoded := protoRoundTrip(t, event)
	got := decoded.GetStaminaConsumed()
	if got == nil {
		t.Fatal("stamina_consumed payload was lost")
	}
	if decoded.GetType() != string(EventStaminaConsumed) || decoded.GetEntity() != 123 || decoded.GetPlayerSlot() != 4 {
		t.Fatalf("event attribution changed: %+v", decoded)
	}
	if got.GetTick() != 640 || got.GetGameTime() != 10.25 || got.GetEntindexTarget() != 123 || got.GetStaminaBefore() != 3.5 || got.GetStaminaAfter() != 2.5 || !got.GetDrained() || got.GetStaminaMax() != 4 {
		t.Fatalf("stamina_consumed payload changed: %+v", got)
	}
}

func TestAbilityChargesProtoRoundTrip(t *testing.T) {
	event := &Event{
		SchemaVersion: EventSchemaVersion,
		Type:          EventAbilityCharges,
		Tick:          704,
		GameTime:      11,
		Entity:        456,
		PlayerSlot:    7,
		AbilityCharges: &AbilityChargesEvent{
			Tick: 704, GameTime: 11, ClassName: "citadel_ability_dash", RemainingCharges: 2,
		},
	}

	decoded := protoRoundTrip(t, event)
	got := decoded.GetAbilityCharges()
	if got == nil {
		t.Fatal("ability_charges payload was lost")
	}
	if decoded.GetType() != string(EventAbilityCharges) || decoded.GetEntity() != 456 || decoded.GetPlayerSlot() != 7 {
		t.Fatalf("event attribution changed: %+v", decoded)
	}
	if got.GetTick() != 704 || got.GetGameTime() != 11 || got.GetClassName() != "citadel_ability_dash" || got.GetRemainingCharges() != 2 {
		t.Fatalf("ability_charges payload changed: %+v", got)
	}
}

func protoRoundTrip(t *testing.T, event *Event) *protocol.ReplayEvent {
	t.Helper()

	encoded, err := event.ToProto().MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	decoded := &protocol.ReplayEvent{}
	if err := decoded.UnmarshalVT(encoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}
