package s2replay

import "testing"

// TestSampleEntityReadsNonHero checks that an objective, which never gets a
// sample event, can be sampled on demand at the current tick.
func TestSampleEntityReadsNonHero(t *testing.T) {
	class := &entityClass{name: "CNPC_Boss_Tier2", serializer: &serializer{fields: []*field{
		{varName: "m_iHealth"},
		{varName: "m_iTeamNum"},
	}}}
	entity := newEntity(9, 1, class)
	health := fieldPath{path: [fieldPathMaxDepth]int{0}}
	team := fieldPath{path: [fieldPathMaxDepth]int{1}}
	entity.state.set(health, int32(4200))
	entity.state.set(team, int32(3))
	entity.fieldTicks[health], entity.fieldTicks[team] = 50, 50
	parser := &Parser{clock: newClock(), entities: map[int32]*Entity{9: entity}}
	parser.clock.setTick(60)

	s, ok := parser.SampleEntity(parser.FindEntity(9))
	if !ok || s.Tick != 60 || s.ClassName != "CNPC_Boss_Tier2" || !s.HasHealth || s.Health != 4200 || s.Team != 3 {
		t.Fatalf("objective sample = %+v, %v", s, ok)
	}
	if _, ok := parser.SampleEntity(parser.FindEntity(10)); ok {
		t.Fatal("absent entity sampled")
	}
	entity.active = false
	if _, ok := parser.SampleEntity(entity); ok {
		t.Fatal("inactive entity sampled")
	}
}
