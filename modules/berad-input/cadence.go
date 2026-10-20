package input

// Cadence is an explicitly serviced millisecond acceptance throttle. Hosts decide
// which candidates bypass its interval (for example a changed physical key).
// It is distinct from Repeat's per-owner countdown. The zero value has zero times.
type Cadence struct {
	lastAt    int64
	blockedTo int64
}

// Accept records acceptance before returning. Equality at the interval is not
// enough: elapsed must be strictly greater. Negative intervals retain their
// mathematical meaning. immediate bypasses only the interval, not BlockUntil.
func (c *Cadence) Accept(now, interval int64, immediate bool) bool {
	if !c.Ready(now) {
		return false
	}
	if now-c.lastAt > interval || immediate {
		c.lastAt = now

		return true
	}

	return false
}

// Ready reports the time gate without changing cadence.
func (c *Cadence) Ready(now int64) bool { return now >= c.blockedTo }

// Elapsed reports milliseconds since the last accepted or explicitly postponed time.
func (c *Cadence) Elapsed(now int64) int64 { return now - c.lastAt }

// PostponeUntil changes the acceptance timestamp, not the all-key time gate.
func (c *Cadence) PostponeUntil(at int64) { c.lastAt = at }

// BlockUntil replaces the all-key deadline; equality at the deadline is ready.
func (c *Cadence) BlockUntil(at int64) { c.blockedTo = at }

// ResetClock clears timestamps. Physical release state belongs to Keyboard.
func (c *Cadence) ResetClock() {
	c.lastAt, c.blockedTo = 0, 0
}
