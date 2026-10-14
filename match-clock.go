package s2replay

import (
	"errors"
	"io"
	"math"
)

// ErrMatchClockNotReached reports a replay that ends before the requested
// match time.
var ErrMatchClockNotReached = errors.New("the replay ends before that match time")

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

// TickAtMatchClock advances the parser to the first tick whose match clock
// reaches seconds and returns it. The parser is consumed: a snapshot of that
// tick needs a fresh parser. A replay that ends first returns
// ErrMatchClockNotReached.
func (p *Parser) TickAtMatchClock(seconds float64) (uint32, error) {
	// Walk commands as a snapshot does, retaining no samples or events.
	p.worldSnapshotMode = true
	defer func() { p.worldSnapshotMode = false }()
	var rules *Entity
	for {
		command, err := p.Next()
		if err == io.EOF {
			return 0, ErrMatchClockNotReached
		}
		if err != nil {
			return 0, err
		}
		if err := p.queueCommandMessages(command); err != nil {
			return 0, err
		}
		for _, message := range p.pending {
			if message.err != nil {
				return 0, message.err
			}
		}
		p.pending, p.pendingSamples, p.pendingEvents, p.pendingModifiers = nil, nil, nil, nil
		if command.Tick == PreGameTick {
			continue
		}

		// Read the clock from the game rules once they exist.
		if rules == nil || !rules.active {
			rules = p.gameRules()
		}
		if rules == nil {
			continue
		}
		if clock, err := MatchClock(rules, p.clock); err == nil && clock >= seconds {
			return command.Tick, nil
		}
	}
}

// gameRules returns the active game rules entity, or nil before it exists.
func (p *Parser) gameRules() *Entity {
	for _, entity := range p.entities {
		if entity != nil && entity.active && entity.class != nil && entity.class.name == "CCitadelGameRulesProxy" {
			return entity
		}
	}
	return nil
}
