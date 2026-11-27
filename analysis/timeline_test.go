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
	b := newTimelineBuilder(p, TimelineOptions{})
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
		for name, n := range map[string]int{"alive": len(pl.GetAlive()), "net worth": len(pl.GetNetWorth()), "farm damage": len(pl.GetFarmDamage()), "trooper damage taken": len(pl.GetTrooperDamageTaken()), "kills": len(pl.GetKills()), "last hits": len(pl.GetLastHits())} {
			if n != samples {
				t.Fatalf("player %d has %d %s samples, want %d", pl.GetAccountId(), n, name, samples)
			}
		}
		byAccount[pl.GetAccountId()] = pl
	}

	// Troopers hit the heroes: every hero in a whole match loses health to
	// them in lane.
	for _, pl := range tl.GetPlayers() {
		var taken int32
		for _, v := range pl.GetTrooperDamageTaken() {
			taken += v
		}
		if taken <= 0 {
			t.Fatalf("player %d took no trooper damage", pl.GetAccountId())
		}
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

		// The scoreboard columns end on the record's counts.
		for name, got := range map[string][2]int32{
			"kills": {pl.GetKills()[samples-1], int32(rec.Kills)}, "assists": {pl.GetAssists()[samples-1], int32(rec.Assists)},
			"deaths": {pl.GetDeaths()[samples-1], int32(rec.Deaths)}, "denies": {pl.GetDenies()[samples-1], int32(rec.Denies)},
		} {
			if got[0] != got[1] {
				t.Errorf("player %d ends with %d %s, post-match %d", rec.AccountID, got[0], name, got[1])
			}
		}
	}

	// Each ability cast with a cooldown is ready again after it was used.
	for _, c := range tl.GetAbilityCasts() {
		if end := c.GetCooldownEnd(); end != 0 && end <= c.GetSeconds() {
			t.Errorf("%s at %.1f s is ready at %.1f s, before its use", c.GetClassName(), c.GetSeconds(), end)
		}
	}
	// The match clock reaches the record's duration by the end of the replay.
	clock := tl.GetClock()
	if tl.GetTickInterval() <= 0 || len(clock) == 0 {
		t.Fatalf("tick interval %v and %d clock entries, want both", tl.GetTickInterval(), len(clock))
	}
	last := clock[len(clock)-1]
	if end := last.GetMatchSeconds() + float64(samples-1-int(last.GetSample()))*tl.GetStepSeconds(); end < float64(post.DurationS) {
		t.Errorf("match clock ends at %.1f s, before the record's %d s", end, post.DurationS)
	}
	t.Logf("tick interval %v, clock %v", tl.GetTickInterval(), clock)
	t.Logf("%d players, %d samples, %d hits, %d objects", len(tl.GetPlayers()), samples, len(tl.GetHits()), len(tl.GetObjects()))
}

// TestMatchTimelineSpans reads a whole match with tracks and spans and checks
// that a span never changes what the timeline reads: the result equals the
// plain timeline without its tracks, each span holds every sample and event
// through its end, and no later span changes them. It also checks each track
// against the positions the replay itself records.
func TestMatchTimelineSpans(t *testing.T) {
	path := os.Getenv("S2REPLAY_PINNED_DEMO")
	if path == "" {
		t.Skip("set S2REPLAY_PINNED_DEMO to a whole match to run the span check")
	}
	demo, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parse := func() *s2replay.Parser {
		p, err := s2replay.NewParser(demo)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	// Read the match with tracks, keeping what each span showed.
	const step, span = 0.125, 25.0
	type shown struct {
		hits, casts int
		x           []float32
	}
	var spans []shown
	tl, err := ReadMatchTimeline(parse(), TimelineOptions{TrackStepSeconds: step, SpanSeconds: span, Span: func(tl *MatchTimeline) error {
		end := float64(len(spans)+1) * span
		var at shown
		for _, h := range tl.GetHits() {
			if h.GetSeconds() <= end {
				at.hits++
			}
		}
		for _, c := range tl.GetAbilityCasts() {
			if c.GetSeconds() <= end {
				at.casts++
			}
		}
		for _, pl := range tl.GetPlayers() {
			x := pl.GetTrack().GetX()
			if len(x) <= int(end/step) {
				t.Fatalf("span %d: player %d has %d track samples, want past %d", len(spans), pl.GetAccountId(), len(x), int(end/step))
			}
			at.x = append(at.x, x[int(end/step)])
		}
		spans = append(spans, at)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Later reading changed nothing a span showed, once every hero is listed.
	for i, at := range spans {
		if len(at.x) != len(tl.GetPlayers()) {
			continue
		}
		end := float64(i+1) * span
		hits, casts := 0, 0
		for _, h := range tl.GetHits() {
			if h.GetSeconds() <= end {
				hits++
			}
		}
		for _, c := range tl.GetAbilityCasts() {
			if c.GetSeconds() <= end {
				casts++
			}
		}
		if hits != at.hits || casts != at.casts {
			t.Errorf("span %d showed %d hits and %d casts, the timeline has %d and %d", i, at.hits, at.casts, hits, casts)
		}
		for j, pl := range tl.GetPlayers() {
			if pl.GetTrack().GetX()[int(end/step)] != at.x[j] {
				t.Errorf("span %d: player %d moved after it was shown", i, j)
			}
		}
	}

	// The track columns align, and stay within a tick's movement of the
	// positions the replay records.
	samples := int(tl.GetDurationSeconds()/step) + 1
	byAccount := make(map[uint32]*TimelineTrack)
	for _, pl := range tl.GetPlayers() {
		tr := pl.GetTrack()
		for _, n := range []int{len(tr.GetX()), len(tr.GetY()), len(tr.GetZ()), len(tr.GetYaw()), len(tr.GetPitch())} {
			if n != samples {
				t.Fatalf("player %d has a track column of %d samples, want %d", pl.GetAccountId(), n, samples)
			}
		}
		byAccount[pl.GetAccountId()] = tr
	}
	accounts := make(map[int32]uint32)
	differ, checked, p := 0, 0, parse()
	for {
		ev, err := p.NextEvent()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if c := ev.ControllerSample; c != nil && c.SteamID != 0 {
			accounts[c.Entity] = uint32(c.SteamID - steamID64Base)
		}
		s := ev.EntitySample
		if s == nil || s.ClassName != "CCitadelPlayerPawn" || !s.HasPosition || !s.HasCameraAngles[0] {
			continue
		}
		tr := byAccount[accounts[s.OwnerEntity]]
		k := math.Round(ev.GameTime / step)
		if tr == nil || math.Abs(ev.GameTime-k*step) > 1e-6 || int(k) >= samples {
			continue
		}
		checked++
		i := int(k)
		if math.Hypot(float64(tr.X[i]-s.PositionX), float64(tr.Y[i]-s.PositionY)) > 0.01 || tr.Pitch[i] != s.CameraAngles[0] {
			differ++
		}
	}
	if checked == 0 || differ*200 > checked {
		t.Errorf("%d of %d track samples differ from the replay's own sample at that time", differ, checked)
	}

	// Without its tracks it is the plain timeline.
	plain, err := BuildMatchTimeline(parse())
	if err != nil {
		t.Fatal(err)
	}
	tl.TrackStepSeconds = 0
	for _, pl := range tl.GetPlayers() {
		pl.Track = nil
	}
	if !tl.EqualVT(plain) {
		t.Error("reading tracks and spans changed the timeline")
	}
	t.Logf("%d spans, %d track samples, %d checked, %d differ", len(spans), samples, checked, differ)
}
