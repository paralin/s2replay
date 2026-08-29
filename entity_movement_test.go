package s2replay

import (
	"encoding/json"
	"testing"
)

func TestEntitySampleProjectsMovementState(t *testing.T) {
	movement := &serializer{fields: []*field{{varName: "m_bDucked"}}}
	class := &entityClass{
		id:   1,
		name: "CCitadelPlayerPawn",
		serializer: &serializer{fields: []*field{
			{varName: "m_hGroundEntity"},
			{varName: "m_pMovementServices", serializer: movement, model: fieldModelFixedTable},
		}},
	}
	entity := newEntity(1, 1, class)

	groundPath := fieldPath{last: 0}
	groundPath.path[0] = 0
	entity.state.set(groundPath, uint32(42))
	crouchPath := fieldPath{last: 1}
	crouchPath.path[0] = 1
	crouchPath.path[1] = 0
	entity.state.set(crouchPath, true)

	sample, ok := entity.sample(64, 1)
	if !ok || !sample.HasGrounded || !sample.Grounded || !sample.HasCrouching || !sample.Crouching {
		t.Fatalf("true movement state missing: %+v", sample)
	}

	entity.state.set(groundPath, uint32(invalidEntityHandle))
	entity.state.set(crouchPath, false)
	sample, ok = entity.sample(65, 2)
	if !ok || !sample.HasGrounded || sample.Grounded || !sample.HasCrouching || sample.Crouching {
		t.Fatalf("false movement state missing: %+v", sample)
	}
}

func TestEntitySampleJSONIncludesFalseMovementState(t *testing.T) {
	sample := EntitySample{HasGrounded: true, HasCrouching: true}
	data, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if grounded, ok := fields["grounded"]; !ok || grounded != false {
		t.Fatalf("grounded false state missing from JSON: %s", data)
	}
	if crouching, ok := fields["crouching"]; !ok || crouching != false {
		t.Fatalf("crouching false state missing from JSON: %s", data)
	}
}
