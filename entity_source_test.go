package s2replay

import (
	"math"
	"testing"
)

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

func TestNextEntitySampleSanitizesMotion(t *testing.T) {
	parser := &Parser{pendingSamples: []EntitySample{{FacingX: float32(math.NaN()), HasFacing: true, HasFacingX: true, VelocityY: float32(math.NaN()), HasVelocity: true, HasVelocityY: true}}}
	// NaN construction through arithmetic is not constant-folded by the compiler.
	sample, err := parser.NextEntitySample()
	if err != nil {
		t.Fatal(err)
	}
	if sample.HasFacing || sample.HasFacingX || sample.HasVelocity || sample.HasVelocityY {
		t.Fatalf("non-finite motion must be missing: %+v", sample)
	}
}

func TestParserEventModeDoesNotRetainSampleQueue(t *testing.T) {
	p := &Parser{}
	p.SetEventMode(true)
	if !p.eventOnly {
		t.Fatal("event mode not enabled")
	}
	p.pendingSamples = append(p.pendingSamples, EntitySample{})
	p.ReleasePendingQueues()
	if len(p.pendingSamples) != 0 {
		t.Fatal("event-mode pending samples retained")
	}
	p.SetEventMode(false)
	if p.eventOnly {
		t.Fatal("sample mode not restored")
	}
}
