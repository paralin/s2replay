package s2replay

import "testing"

// TestWorldEntitySnapshotPreservesLifeState pins lifecycle presence and its source tick.
func TestWorldEntitySnapshotPreservesLifeState(t *testing.T) {
	for _, state := range []any{nil, uint32(0), uint32(2)} {
		// Record health and lifecycle at different source ticks.
		class := &entityClass{name: "CNPC_Trooper", serializer: &serializer{fields: []*field{{varName: "m_iHealth"}, {varName: "m_lifeState"}}}}
		entity := newEntity(7, 3, class)
		healthPath := fieldPath{last: 0}
		entity.state.set(healthPath, int32(1))
		entity.fieldTicks[healthPath] = 4
		if state != nil {
			lifePath := fieldPath{last: 0}
			lifePath.path[0] = 1
			entity.state.set(lifePath, state)
			entity.fieldTicks[lifePath] = 9
		}
		parser := &Parser{clock: newClock(), entities: map[int32]*Entity{7: entity}}
		parser.clock.setTick(10)

		// Snapshot the entity without conflating absent life state and alive zero.
		samples, err := parser.WorldEntitySnapshot(10)
		if err != nil {
			t.Fatal(err)
		}
		if len(samples) != 1 {
			t.Fatalf("samples: got %d, want 1", len(samples))
		}
		sample := samples[0]
		if !sample.HasHealth || sample.Health != 1 || sample.HealthTick != 4 {
			t.Fatalf("health evidence changed: %+v", sample)
		}
		if sample.HasLifeState != (state != nil) {
			t.Fatalf("life-state presence for %v: %+v", state, sample)
		}
		if state != nil && (sample.LifeState != state.(uint32) || sample.LifeStateTick != 9) {
			t.Fatalf("life-state evidence for %v: %+v", state, sample)
		}
	}
}
