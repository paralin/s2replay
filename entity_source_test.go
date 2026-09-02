package s2replay

import (
	"errors"
	"io"
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

func TestWorldEntitySnapshotEnumeratesActiveGenerations(t *testing.T) {
	p := &Parser{
		clock: newClock(),
		entities: map[int32]*Entity{
			1: newEntity(1, 7, &entityClass{id: 11, name: "npc_deadlock_tower"}),
			2: func() *Entity {
				e := newEntity(2, 8, &entityClass{id: 12, name: "npc_deadlock_old"})
				e.active = false
				return e
			}(),
			3: newEntity(3, 9, &entityClass{id: 13, name: "CCitadel_Ability_Dash"}),
		},
	}
	// A serial replacement reuses the index but leaves only the new active generation.
	p.entities[2] = newEntity(2, 10, &entityClass{id: 14, name: "npc_deadlock_walker"})
	p.clock.setTick(10)
	samples, err := p.WorldEntitySnapshot(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 3 || samples[0].Entity != 1 || samples[0].EntitySerial != 7 || samples[1].Entity != 2 || samples[1].EntitySerial != 10 || samples[2].Entity != 3 || samples[2].EntitySerial != 9 {
		t.Fatalf("active snapshot: %+v", samples)
	}
	if samples[2].ClassName != "CCitadel_Ability_Dash" {
		t.Fatalf("ability class evidence: %+v", samples[1])
	}
}

func TestWorldEntitySnapshotRefusesPastTick(t *testing.T) {
	p := &Parser{clock: newClock()}
	p.clock.setTick(11)
	if _, err := p.WorldEntitySnapshot(10); err != errWorldSnapshotPastTick {
		t.Fatalf("error = %v, want past-tick error", err)
	}
}

func TestWorldEntitySnapshotRejectsNonFiniteSource(t *testing.T) {
	class := &entityClass{serializer: &serializer{fields: []*field{{varName: "m_iHealth"}}}}
	entity := newEntity(7, 3, class)
	var path fieldPath
	path.path[0] = 0
	path.last = 0
	entity.state.set(path, float32(math.NaN()))
	entity.fieldTicks[path] = 0
	parser := &Parser{clock: newClock(), entities: map[int32]*Entity{7: entity}}
	samples, err := parser.WorldEntitySnapshot(0)
	if samples != nil {
		t.Fatalf("malformed snapshot returned samples: %+v", samples)
	}
	var typed *WorldEntitySampleError
	if !errors.As(err, &typed) || typed.Field != "health" {
		t.Fatalf("error = %v, want typed health error", err)
	}
}

func TestNextReturnsSnapshotLookaheadOnce(t *testing.T) {
	parser := &Parser{clock: newClock(), lookahead: &Command{Tick: 7}}
	command, err := parser.Next()
	if err != nil || command == nil || command.Tick != 7 || parser.lookahead != nil {
		t.Fatalf("lookahead: command=%+v err=%v remaining=%+v", command, err, parser.lookahead)
	}
	if _, err := parser.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("second next error = %v, want EOF", err)
	}
}

func TestWorldEntitySnapshotRejectsUnobservedTick(t *testing.T) {
	parser := &Parser{clock: newClock()}
	_, err := parser.WorldEntitySnapshot(1_000_000_000)
	var typed *WorldSnapshotError
	if !errors.As(err, &typed) || typed.RequestedTick != 1_000_000_000 || typed.FinalTick != 0 {
		t.Fatalf("error = %v, want unobserved typed boundary", err)
	}
}
