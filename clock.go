package s2replay

// DefaultTickInterval is the seconds-per-tick used until the server's
// CSVCMsg_ServerInfo tick_interval is decoded from the packet stream. Source 2
// servers record at 1/64 s by default; SetInterval overwrites this with the
// exact value once message dispatch is in place.
const DefaultTickInterval = 1.0 / 64.0

// Clock converts demo ticks to game-time seconds. The interval starts at
// DefaultTickInterval and is replaced by the exact server value when known;
// callers read GameTime and never depend on the placeholder interval.
type Clock struct {
	tick     uint32
	interval float64
}

func newClock() *Clock { return &Clock{interval: DefaultTickInterval} }

// Tick returns the most recent real demo tick.
func (c *Clock) Tick() uint32 { return c.tick }

// TickInterval returns the current seconds-per-tick.
func (c *Clock) TickInterval() float64 { return c.interval }

// GameTime returns the current tick expressed in seconds.
func (c *Clock) GameTime() float64 { return float64(c.tick) * c.interval }

// setTick advances the clock to tick t.
func (c *Clock) setTick(t uint32) { c.tick = t }

// SetInterval replaces the seconds-per-tick with the exact server value.
func (c *Clock) SetInterval(seconds float64) {
	if seconds > 0 {
		c.interval = seconds
	}
}
