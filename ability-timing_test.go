package s2replay

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
)

// TestAbilityTimerSnapshotPreservesZeroAndSourceTicks checks the entity sampling boundary.
func TestAbilityTimerSnapshotPreservesZeroAndSourceTicks(t *testing.T) {
	// Sample independently updated timers without an end for the charge interval.
	class := &entityClass{serializer: &serializer{fields: []*field{
		{varName: "m_flCooldownStart"}, {varName: "m_flCooldownEnd"}, {varName: "m_flChargeRechargeStart"},
	}}}
	entity := newEntity(7, 3, class)
	for index, value := range []float32{0, 14.5, 9.25} {
		path := fieldPath{last: 0}
		path.path[0] = index
		entity.state.set(path, value)
		entity.fieldTicks[path] = uint32(90 + index)
	}
	parser := &Parser{clock: newClock(), entities: map[int32]*Entity{7: entity}}
	parser.clock.setTick(100)
	samples, err := parser.WorldEntitySnapshot(100)
	if err != nil {
		t.Fatal(err)
	}

	// Preserve explicit zero and independent source times, without fabricating the missing endpoint.
	sample := samples[0]
	if !sample.HasCooldownStart {
		t.Fatal("observed zero cooldown start became missing")
	}
	if sample.CooldownStart != 0 {
		t.Fatalf("cooldown start: %v", sample.CooldownStart)
	}
	if sample.CooldownStartTick != 90 {
		t.Fatalf("cooldown start tick: %d", sample.CooldownStartTick)
	}
	if sample.CooldownEndTick != 91 {
		t.Fatalf("cooldown end tick: %d", sample.CooldownEndTick)
	}
	if sample.ChargeRechargeStart != 9.25 {
		t.Fatalf("charge start: %v", sample.ChargeRechargeStart)
	}
	if sample.ChargeRechargeStartTick != 92 {
		t.Fatalf("charge start tick: %d", sample.ChargeRechargeStartTick)
	}
	if sample.HasChargeRechargeEnd {
		t.Fatal("missing charge end fabricated")
	}
}

// TestAbilityTimerEventRejectsNonFiniteValues checks the public event encoding boundary.
func TestAbilityTimerEventRejectsNonFiniteValues(t *testing.T) {
	// Exercise all four timer fields, including an invalid value marked absent.
	sample := EntitySample{
		CooldownStart: float32(math.NaN()), HasCooldownStart: true,
		CooldownEnd: float32(math.Inf(1)), HasCooldownEnd: true,
		ChargeRechargeStart: float32(math.Inf(-1)), HasChargeRechargeStart: true,
		ChargeRechargeEnd: float32(math.NaN()),
	}
	event := Event{EntitySample: &sample}
	sanitizeEvent(&event)
	if _, err := json.Marshal(event); err != nil {
		t.Fatal(err)
	}

	// Invalid endpoints stay unavailable with their reason recorded independently.
	if sample.HasCooldownStart || sample.HasCooldownEnd || sample.HasChargeRechargeStart || sample.HasChargeRechargeEnd {
		t.Fatal("invalid timer remained present")
	}
	for _, name := range []string{"cooldown_start", "cooldown_end", "charge_recharge_start", "charge_recharge_end"} {
		if !slices.Contains(sample.InvalidFields, name) {
			t.Fatalf("missing invalid field %s", name)
		}
	}
}
