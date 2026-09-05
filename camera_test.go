package s2replay

import (
	"math"
	"slices"
	"testing"
)

// TestCameraSnapshotKeepsViewSeparateFromAim preserves both recorded orientations.
func TestCameraSnapshotKeepsViewSeparateFromAim(t *testing.T) {
	class := &entityClass{serializer: &serializer{fields: []*field{
		{varName: "m_angClientCamera"}, {varName: "m_angEyeAngles"},
	}}}
	entity := newEntity(7, 3, class)
	for i, value := range [][]float32{{4.21875, 270, 0}, {3.8671875, 268.2422, 0}} {
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
	if sample.CameraAngles != [3]float32{4.21875, 270, 0} || sample.CameraAnglesTicks != [3]uint32{80, 80, 80} || sample.HasCameraAngles != [3]bool{true, true, true} {
		t.Fatalf("camera provenance: %+v", sample)
	}
	if sample.FacingY != 268.2422 || sample.FacingYTick != 81 {
		t.Fatal("camera replaced aim evidence")
	}
	sample.CameraAngles[1] = float32(math.NaN())
	sanitizeEntitySample(&sample)
	if sample.HasCameraAngles[1] || sample.CameraAngles[1] != 0 || !sample.HasCameraAngles[0] || !slices.Contains(sample.InvalidFields, "camera_yaw") {
		t.Fatal("invalid camera component was not isolated")
	}
}
