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

func TestWorldEntitySnapshotDoesNotAllocateForDormantEntities(t *testing.T) {
	entities := make(map[int32]*Entity, 102)
	entities[1] = newEntity(1, 7, &entityClass{id: 11, name: "npc_active"})
	for index := int32(2); index < 102; index++ {
		entity := newEntity(index, index, &entityClass{id: 12, name: "npc_dormant"})
		entity.active = false
		entities[index] = entity
	}
	parser := &Parser{clock: newClock(), entities: entities}
	parser.clock.setTick(10)
	samples, err := parser.WorldEntitySnapshot(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || cap(samples) != 1 {
		t.Fatalf("snapshot allocation: len=%d cap=%d", len(samples), cap(samples))
	}
}

func TestWorldSnapshotKeepsPlayerSlotAttribution(t *testing.T) {
	ownerClass := &entityClass{serializer: &serializer{fields: []*field{{varName: "m_iPlayerSlot"}}}}
	owner := newEntity(1, 7, ownerClass)
	var ownerPath fieldPath
	ownerPath.path[0] = 0
	ownerPath.last = 0
	owner.state.set(ownerPath, int32(4))

	abilityClass := &entityClass{serializer: &serializer{fields: []*field{{varName: "m_iRemainingCharges"}, {varName: "m_hOwnerEntity"}}}}
	ability := newEntity(2, 8, abilityClass)
	var chargesPath, ownerPathInAbility fieldPath
	chargesPath.path[0], chargesPath.last = 0, 0
	ownerPathInAbility.path[0], ownerPathInAbility.last = 1, 0
	ability.state.set(chargesPath, int32(2))
	ability.state.set(ownerPathInAbility, int32(1))

	parser := &Parser{clock: newClock(), entityPlayerSlots: make(map[int32]int32), chargeLastSeen: make(map[int32]int32), worldSnapshotMode: true}
	parser.appendEntitySample(10, owner)
	if parser.entityPlayerSlots[1] != 4 || len(parser.pendingEvents) != 0 {
		t.Fatalf("snapshot attribution update: slots=%v events=%d", parser.entityPlayerSlots, len(parser.pendingEvents))
	}
	parser.worldSnapshotMode = false
	parser.appendAbilityChargeEvent(10, ability)
	if len(parser.pendingEvents) != 1 || parser.pendingEvents[0].PlayerSlot != 4 {
		t.Fatalf("ability attribution after snapshot: %+v", parser.pendingEvents)
	}
}

func TestWorldEntitySnapshotPreservesUnsignedSubclassIdentity(t *testing.T) {
	class := &entityClass{name: "CCitadel_Item_Empty", serializer: &serializer{fields: []*field{{varName: "m_nSubclassID"}}}}
	entity := newEntity(7, 3, class)
	path := fieldPath{last: 0}
	entity.state.set(path, uint32(0xf1234567))
	entity.fieldTicks[path] = 9
	parser := &Parser{clock: newClock(), entities: map[int32]*Entity{7: entity}}
	parser.clock.setTick(10)
	samples, err := parser.WorldEntitySnapshot(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || !samples[0].HasSubclassID || samples[0].SubclassID != 0xf1234567 || samples[0].SubclassIDTick != 9 {
		t.Fatalf("subclass identity lost: %+v", samples)
	}
}

func TestWorldEntitySnapshotPreservesEquipmentSlotAndUpgradeInfo(t *testing.T) {
	class := &entityClass{name: "CCitadel_Item_Empty", serializer: &serializer{fields: []*field{{varName: "m_eAbilitySlot"}, {varName: "m_nUpgradeInfo"}}}}
	entity := newEntity(7, 3, class)
	for i, value := range []uint32{12, 0xf1234567} {
		path := fieldPath{last: 0}
		path.path[0] = i
		entity.state.set(path, value)
		entity.fieldTicks[path] = 9
	}
	parser := &Parser{clock: newClock(), entities: map[int32]*Entity{7: entity}}
	parser.clock.setTick(10)
	samples, err := parser.WorldEntitySnapshot(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples: %+v", samples)
	}
	sample := samples[0]
	if !sample.HasAbilitySlot || sample.AbilitySlot != 12 || sample.AbilitySlotTick != 9 || !sample.HasUpgradeInfo || sample.UpgradeInfo != 0xf1234567 || sample.UpgradeInfoTick != 9 {
		t.Fatalf("equipment state lost: %+v", sample)
	}
}
