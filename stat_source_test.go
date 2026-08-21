package s2replay

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/paralin/s2replay/protocol"
)

func TestReplaySourceProofGate(t *testing.T) {
	demoPath := os.Getenv("S2REPLAY_TEST_DEM")
	if demoPath == "" {
		demoPath = filepath.Join(os.Getenv("HOME"), "repos/deadlock-replays/48345595.dem")
	}
	if _, err := os.Stat(demoPath); err != nil {
		t.Skipf("set S2REPLAY_TEST_DEM to a Deadlock .dem to run replay source proof gate: %v", err)
	}

	demo, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}

	var summary sourceProofSummary
	for {
		m, err := p.NextMessage()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		summary.messages++
		switch msg := m.Payload.(type) {
		case *protocol.CCitadelUserMsg_PlayerLifetimeStatInfo:
			summary.playerLifetimeStatMessages++
			summary.playerLifetimeStats += len(msg.GetStats())
			for _, stat := range msg.GetStats() {
				summary.addStatName(stat.GetStatName())
			}
		case *protocol.CCitadelUserMsg_GetDamageStatsResponse:
			summary.damageStatsResponses++
			summary.addDamageStatsAbility(msg.GetAbilityName())
			if damage := msg.GetDamage(); damage != nil {
				summary.damageStatsValues += len(damage.GetValue())
			}
			if healing := msg.GetHealing(); healing != nil {
				summary.damageStatsValues += len(healing.GetValue())
			}
		case *protocol.CCitadelUserMsg_RecentDamageSummary:
			summary.recentDamageSummaries++
			summary.recentDamageRecords += len(msg.GetDamageRecords())
			summary.recentModifierRecords += len(msg.GetModifierRecords())
		}
	}

	summary.collectEntityCandidates(p)
	summary.collectModifierSources(p.pendingModifiers)
	if summary.messages == 0 {
		t.Fatal("source proof scanned zero decoded messages")
	}
	t.Log(summary.String())
}

type sourceProofSummary struct {
	messages                   int
	playerLifetimeStatMessages int
	playerLifetimeStats        int
	statNames                  []string
	damageStatsResponses       int
	damageStatsValues          int
	damageStatsAbilities       []string
	recentDamageSummaries      int
	recentDamageRecords        int
	recentModifierRecords      int
	entityCandidateFields      []string
	entityCandidateValues      []string
	modifierSourceZero         int
	modifierSourceNonZero      int
	modifierSourceValues       []string
}

func (s *sourceProofSummary) addStatName(name string) {
	if name != "" && !slices.Contains(s.statNames, name) {
		s.statNames = append(s.statNames, name)
	}
}

func (s *sourceProofSummary) addDamageStatsAbility(name string) {
	if name != "" && !slices.Contains(s.damageStatsAbilities, name) {
		s.damageStatsAbilities = append(s.damageStatsAbilities, name)
	}
}

func (s *sourceProofSummary) collectEntityCandidates(p *Parser) {
	for _, class := range p.classesByName {
		if class == nil || !isLikelyHeroClass(class.name) || class.serializer == nil {
			continue
		}
		collectCandidateSerializerFields(class.serializer, nil, &s.entityCandidateFields)
	}
	for _, e := range p.entities {
		if e == nil || e.class == nil || !isLikelyHeroClass(e.class.name) {
			continue
		}
		collectCandidateEntityValues(e, e.state, nil, &s.entityCandidateValues)
	}
	slices.Sort(s.entityCandidateFields)
	s.entityCandidateFields = compactStrings(s.entityCandidateFields)
	slices.Sort(s.entityCandidateValues)
	s.entityCandidateValues = compactStrings(s.entityCandidateValues)
}

func (s *sourceProofSummary) collectModifierSources(events []ModifierEvent) {
	for _, ev := range events {
		if ev.AbilitySubclass == 0 {
			s.modifierSourceZero++
			continue
		}
		s.modifierSourceNonZero++
		value := strconv.FormatUint(uint64(ev.AbilitySubclass), 10)
		if !slices.Contains(s.modifierSourceValues, value) {
			s.modifierSourceValues = append(s.modifierSourceValues, value)
		}
	}
	slices.Sort(s.modifierSourceValues)
}

func (s sourceProofSummary) String() string {
	return "source proof: messages=" + strconv.Itoa(s.messages) +
		" player_lifetime_stat_messages=" + strconv.Itoa(s.playerLifetimeStatMessages) +
		" player_lifetime_stats=" + strconv.Itoa(s.playerLifetimeStats) +
		" stat_names=" + limitedStrings(s.statNames, 12) +
		" damage_stats_responses=" + strconv.Itoa(s.damageStatsResponses) +
		" damage_stats_values=" + strconv.Itoa(s.damageStatsValues) +
		" damage_stats_abilities=" + limitedStrings(s.damageStatsAbilities, 12) +
		" recent_damage_summaries=" + strconv.Itoa(s.recentDamageSummaries) +
		" recent_damage_records=" + strconv.Itoa(s.recentDamageRecords) +
		" recent_modifier_records=" + strconv.Itoa(s.recentModifierRecords) +
		" entity_candidate_fields=" + limitedStrings(s.entityCandidateFields, 20) +
		" sampled_entity_candidate_values=" + limitedStrings(s.entityCandidateValues, 20) +
		" modifier_source_nonzero=" + strconv.Itoa(s.modifierSourceNonZero) +
		" modifier_source_zero=" + strconv.Itoa(s.modifierSourceZero) +
		" modifier_source_values=" + limitedStrings(s.modifierSourceValues, 20)
}

func collectCandidateSerializerFields(s *serializer, prefix []string, out *[]string) {
	for _, f := range s.fields {
		parts := appendFieldName(prefix, f)
		name := joinFieldName(parts)
		if isStatCandidateField(name) && !slices.Contains(*out, name) {
			*out = append(*out, name)
		}
		if f.serializer != nil {
			collectCandidateSerializerFields(f.serializer, parts, out)
		}
	}
}

func collectCandidateEntityValues(e *Entity, state *fieldState, indexes []int, out *[]string) {
	keys := make([]int, 0, len(state.values))
	for key := range state.values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		value := state.values[key]
		nextIndexes := append(indexes, key)
		if child, ok := value.(*fieldState); ok {
			collectCandidateEntityValues(e, child, nextIndexes, out)
			continue
		}
		name := entityFieldName(e, nextIndexes)
		if !isStatCandidateField(name) {
			continue
		}
		item := name + "=" + fieldValueString(value)
		if !slices.Contains(*out, item) {
			*out = append(*out, item)
		}
	}
}

func appendFieldName(prefix []string, f *field) []string {
	parts := slices.Clone(prefix)
	if f.sendNode != "" {
		parts = append(parts, strings.Split(f.sendNode, ".")...)
	}
	if f.varName != "" {
		parts = append(parts, f.varName)
	}
	return parts
}

func entityFieldName(e *Entity, indexes []int) string {
	if len(indexes) == 0 || len(indexes) > fieldPathMaxDepth {
		return ""
	}
	fp := fieldPath{last: len(indexes) - 1, path: [fieldPathMaxDepth]int{-1}}
	copy(fp.path[:], indexes)
	return e.class.fieldName(fp)
}

func fieldValueString(v any) string {
	switch x := v.(type) {
	case bool:
		return strconv.FormatBool(x)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case string:
		return x
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	}
	return "unprintable"
}

func isStatCandidateField(name string) bool {
	lower := strings.ToLower(name)
	for _, term := range []string{
		"weapon",
		"spirit",
		"tech",
		"bullet",
		"firerate",
		"fire_rate",
		"move",
		"speed",
		"resist",
		"power",
		"cooldown",
		"lifesteal",
		"bonus",
	} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func compactStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

func limitedStrings(in []string, limit int) string {
	if len(in) == 0 {
		return "[]"
	}
	if len(in) > limit {
		return "[" + strings.Join(in[:limit], ",") + ",...+" + strconv.Itoa(len(in)-limit) + "]"
	}
	return "[" + strings.Join(in, ",") + "]"
}
