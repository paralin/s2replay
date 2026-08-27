package s2replay

import "testing"

func TestFirstFloat32AtReportsFallbackFieldPathAndTick(t *testing.T) {
	class := &entityClass{serializer: &serializer{fields: []*field{{varName: "m_cellX"}}}}
	entity := newEntity(1, 1, class)
	path := fieldPath{last: 0}
	path.path[0] = 0
	entity.state.set(path, uint32(12))
	entity.fieldTicks[path] = 90
	value, tick, name, ok := firstFloat32At(entity, "CBodyComponent.m_skeletonInstance.m_cellX", "m_cellX")
	if !ok || value != 12 || tick != 90 || name != "m_cellX" {
		t.Fatalf("fallback field: value=%v tick=%d name=%q ok=%t", value, tick, name, ok)
	}
}
