package s2replay

import (
	"errors"
	"math"
)

// MatchClock reads elapsed match seconds from the recorded rules and network
// clock. Missing fields remain an error; demo duration is not match time.
func MatchClock(rules *Entity, clock *Clock) (float64, error) {
	// Require the rules and simulation interval from the same parser snapshot.
	if rules == nil || rules.ClassName() != "CCitadelGameRulesProxy" || clock == nil || !clock.TickIntervalKnown() {
		return 0, errors.New("match clock requires game rules and an observed tick interval")
	}
	serverTick, _, known := clock.ServerTick()
	if !known {
		return 0, errors.New("match clock requires an observed network tick")
	}

	// Pauses consume server ticks without advancing match time.
	start, hasStart := rules.Float32("m_pGameRules.m_flGameStartTime")
	pausedTicks, hasPausedTicks := rules.UInt32("m_pGameRules.m_nTotalPausedTicks")
	paused, hasPaused := rules.Get("m_pGameRules.m_bGamePaused").(bool)
	if !hasStart || !hasPausedTicks || !hasPaused || math.IsNaN(float64(start)) || math.IsInf(float64(start), 0) {
		return 0, errors.New("match clock rules are absent or nonfinite")
	}
	if paused {
		pauseStart, present := rules.UInt32("m_pGameRules.m_nPauseStartTick")
		if !present {
			return 0, errors.New("paused match clock requires its pause start tick")
		}
		serverTick = min(serverTick, pauseStart)
	}

	// Subtract only recorded paused ticks and the actual match start time.
	if pausedTicks > serverTick {
		return 0, errors.New("paused tick count exceeds the network clock")
	}
	return float64(serverTick-pausedTicks)*clock.TickInterval() - float64(start), nil
}
