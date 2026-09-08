package s2replay

import "testing"

// TestNpcFacingPreservesSceneRotation checks the actual network field used by NPCs.
func TestNpcFacingPreservesSceneRotation(t *testing.T) {
	// Provide the recorded skeleton rotation without a player eye-angle field.
	class := &entityClass{name: "CNPC_Boss_Tier2", serializer: &serializer{fields: []*field{
		{sendNode: "CBodyComponent.m_skeletonInstance", varName: "m_angRotation"},
	}}}
	entity := newEntity(7, 2, class)
	path := fieldPath{last: 0}
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
