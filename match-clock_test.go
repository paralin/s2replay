package s2replay

import "testing"

// TestMatchClockUsesRulesAndPauseTicks distinguishes HUD time from demo duration.
func TestMatchClockUsesRulesAndPauseTicks(t *testing.T) {
	// Model recorded rules with an actual start, a prior pause, and a live pause.
	rules := newEntity(1, 1, &entityClass{name: "CCitadelGameRulesProxy"})
	fields := []string{"m_flGameStartTime", "m_nTotalPausedTicks", "m_bGamePaused", "m_nPauseStartTick"}
	values := []any{float32(58.140625), uint32(64), false, uint32(63735)}
	for i, name := range fields {
		path := fieldPath{last: 0}
		path.path[0] = i
		rules.paths["m_pGameRules."+name] = path
		rules.state.set(path, values[i])
	}
	clock := newClock()
	clock.SetInterval(1.0 / 64)
	clock.setTick(62000)
	clock.observeServerTick(63799, 62000)

	// A prior pause reduces elapsed time; a current pause freezes it at its start.
	for _, paused := range []bool{false, true} {
		rules.state.set(rules.paths["m_pGameRules.m_bGamePaused"], paused)
		got, err := MatchClock(rules, clock)
		if err != nil {
			t.Fatal(err)
		}
		want := 937.71875
		if paused {
			want--
		}
		if got != want {
			t.Fatalf("paused=%t: match time %v, want %v", paused, got, want)
		}
	}

	// The parser's default interval and absent rules cannot invent a match clock.
	if _, err := MatchClock(rules, newClock()); err == nil {
		t.Fatal("unobserved network clock accepted")
	}
	if _, err := MatchClock(nil, clock); err == nil {
		t.Fatal("absent game rules accepted")
	}
}
