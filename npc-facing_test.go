package s2replay

import "testing"

// TestNpcFacingPreservesSceneRotation checks the actual network field used by NPCs.
func TestNpcFacingPreservesSceneRotation(t *testing.T) {
	// Provide the recorded skeleton rotation without a player eye-angle field.
	entity := newEntity(7, 2, &entityClass{name: "CNPC_Boss_Tier2"})
	path := fieldPath{last: 0}
	const name = "CBodyComponent.m_skeletonInstance.m_angRotation"
	entity.paths[name] = path
	entity.state.set(path, []float32{0, 60, 0})
	entity.fieldTicks[path] = 100
	parser := &Parser{clock: newClock(), entities: map[int32]*Entity{7: entity}}
	parser.clock.setTick(110)

	// The owned world snapshot retains orientation and its observation tick.
	rows, err := parser.WorldEntitySnapshot(110)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].HasFacing || rows[0].FacingY != 60 || rows[0].FacingYTick != 100 {
		t.Fatalf("NPC rotation lost: %+v", rows)
	}
}
