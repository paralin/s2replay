package s2replay

import (
	"math"
	"testing"
)

// TestStaminaLatchSnapshotPreservesIndependentSourceTicks checks the recorded send-node path.
func TestStaminaLatchSnapshotPreservesIndependentSourceTicks(t *testing.T) {
	// Use the replay's flattened resource fields, including an observed zero value.
	class := &entityClass{serializer: &serializer{fields: []*field{
		{sendNode: "m_CCitadelAbilityComponent.m_ResourceStamina", varName: "m_flLatchTime"},
		{sendNode: "m_CCitadelAbilityComponent.m_ResourceStamina", varName: "m_flLatchValue"},
	}}}
	entity := newEntity(7, 3, class)
	for i, value := range []float32{90, 0} {
		path := fieldPath{last: 0}
		path.path[0] = i
		entity.state.set(path, value)
		entity.fieldTicks[path] = uint32(80 + i)
	}
	parser := &Parser{clock: newClock(), entities: map[int32]*Entity{7: entity}}
	parser.clock.setTick(100)
	samples, err := parser.WorldEntitySnapshot(100)
	if err != nil {
		t.Fatal(err)
	}
	sample := samples[0]
	if !sample.HasStaminaLatchTime || sample.StaminaLatchTime != 90 || sample.StaminaLatchTimeTick != 80 {
		t.Fatalf("time: %+v", sample)
	}
	if !sample.HasStaminaLatchValue || sample.StaminaLatchValue != 0 || sample.StaminaLatchValueTick != 81 {
		t.Fatalf("value: %+v", sample)
	}

	// Invalid network evidence must not survive the event serialization boundary.
	sample.StaminaLatchValue = float32(math.NaN())
	sanitizeEntitySample(&sample)
	if sample.HasStaminaLatchValue || sample.StaminaLatchValue != 0 {
		t.Fatal("invalid stamina latch remained available")
	}
}
