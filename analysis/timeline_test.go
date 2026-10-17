package analysis

import (
	"errors"
	"io"
	"math"
	"os"
	"testing"

	"github.com/paralin/s2replay"
)

// TestMatchTimelineMatchesPostMatch builds the timeline of a whole match and
// checks it against the replay's own post-match record: the winner, each
// player's final souls, and each death.
func TestMatchTimelineMatchesPostMatch(t *testing.T) {
	// Build the timeline, keeping the post-match record.
	path := os.Getenv("S2REPLAY_PINNED_DEMO")
	if path == "" {
		t.Skip("set S2REPLAY_PINNED_DEMO to a whole match to run the timeline check")
	}
	demo, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s2replay.NewParser(demo)
	if err != nil {
		t.Fatal(err)
	}
	b := newTimelineBuilder(p)
	var post *s2replay.PostMatchEvent
	for {
		ev, err := p.NextEvent()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev.PostMatch != nil {
			post = ev.PostMatch
		}
		b.accept(ev)
	}
	tl := b.finish()
	if post == nil {
		t.Fatal("replay has no post-match record")
		return
	}

	// The columns align, and the winner is numbered as the players' teams.
	samples := len(tl.GetPlayers()[0].GetX())
	if want := int(tl.GetDurationSeconds()/tl.GetStepSeconds()) + 1; samples != want {
		t.Fatalf("%d samples over %.1f s, want %d", samples, tl.GetDurationSeconds(), want)
	}
	byAccount := make(map[uint32]*TimelinePlayer)
	for _, pl := range tl.GetPlayers() {
		for name, n := range map[string]int{"alive": len(pl.GetAlive()), "net worth": len(pl.GetNetWorth()), "farm damage": len(pl.GetFarmDamage())} {
			if n != samples {
				t.Fatalf("player %d has %d %s samples, want %d", pl.GetAccountId(), n, name, samples)
			}
		}
		byAccount[pl.GetAccountId()] = pl
	}
	if tl.GetWinningTeam() != post.WinningTeam+entityTeamOffset {
		t.Fatalf("winning team %d, post-match %d", tl.GetWinningTeam(), post.WinningTeam)
	}

	// Each player's final souls are within 2 percent of the record, and their
	// deaths match the record's count. A dead span under minDeadSeconds is a
	// death the game reversed, which the record does not count.
	const minDeadSeconds = 5
	end := int(float64(post.DurationS) / tl.GetStepSeconds())
	for _, rec := range post.Players {
		pl := byAccount[rec.AccountID]
		if pl == nil {
			t.Fatalf("player %d missing", rec.AccountID)
		}
		final := float64(pl.GetNetWorth()[samples-1])
		if math.Abs(final-float64(rec.NetWorth)) > 0.02*float64(rec.NetWorth) {
			t.Errorf("player %d ends with %.0f souls, post-match %d", rec.AccountID, final, rec.NetWorth)
		}
		deaths := 0
		alive := pl.GetAlive()
		for i := 1; i < min(end, samples); i++ {
			if !alive[i-1] || alive[i] {
				continue
			}
			j := i
			for j < samples && !alive[j] {
				j++
			}
			if float64(j-i)*tl.GetStepSeconds() >= minDeadSeconds {
				deaths++
			}
		}
		if deaths != int(rec.Deaths) {
			t.Errorf("player %d died %d times, post-match %d", rec.AccountID, deaths, rec.Deaths)
		}
	}
	t.Logf("%d players, %d samples, %d hits, %d objects", len(tl.GetPlayers()), samples, len(tl.GetHits()), len(tl.GetObjects()))
}
