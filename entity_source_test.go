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

func TestSanitizeEntitySamplePreservesInvalidSourceEvidence(t *testing.T) {
	sample := EntitySample{
		Health: float32(math.NaN()), HasHealth: true,
		PositionX: float32(math.Inf(1)), HasPosition: true,
	}
	sanitizeEntitySample(&sample)
	if sample.HasHealth || sample.HasPosition {
		t.Fatalf("invalid values must not remain usable: %+v", sample)
	}
	if len(sample.InvalidFields) != 2 || sample.InvalidFields[0] != "health" || sample.InvalidFields[1] != "position_x" {
		t.Fatalf("invalid source evidence: %+v", sample.InvalidFields)
	}
}

func TestWorldEntityModeIsOptIn(t *testing.T) {
	entity := newEntity(7, 3, &entityClass{id: 11, name: "npc_deadlock_tower"})
	parser := &Parser{clock: newClock(), entityPlayerSlots: make(map[int32]int32)}
	parser.appendEntitySample(100, entity)
	if len(parser.pendingEvents) != 0 {
		t.Fatal("generic entity leaked into ordinary event mode")
	}
	parser.SetWorldEntityMode(true)
	parser.appendEntitySample(100, entity)
	if len(parser.pendingEvents) != 1 {
		t.Fatalf("generic entity missing in world mode: %d", len(parser.pendingEvents))
	}
	if got := parser.pendingEvents[0].EntitySample.ClassName; got != "npc_deadlock_tower" {
		t.Fatalf("class evidence: %q", got)
	}
}
