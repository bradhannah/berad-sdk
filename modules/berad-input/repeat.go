package input

import "time"

// RepeatTiming supplies focused-update countdown durations. Tolerance permits
// explicit host tick-rounding policy; it is not an implicit SDK epsilon.
type RepeatTiming struct {
	Initial   time.Duration
	Interval  time.Duration
	Tolerance time.Duration
}

// Repeat owns one input owner's held-key countdown. Its zero value is inactive.
// Reset on owner close, focus change, or input-mode change. Do not share between
// independently focused owners. Physical consumption still belongs to Keyboard.
type Repeat[K comparable] struct {
	key       K
	active    bool
	remaining time.Duration
}

// Result distinguishes owned-but-throttled input from a triggered action.
// Key is meaningful only when Triggered is true. Results contain no borrowed state.
type Result[K comparable] struct {
	Key       K
	Owned     bool
	Triggered bool
}

// Poll accepts a fresh hold immediately, then advances by one supplied focused
// update duration. fresh also rearms a key released while this owner was dormant.
// Conflicts reset cadence but retain ownership. At most one action is emitted per
// call; residual countdown is retained. Call once per focused update, not per reader.
func (r *Repeat[K]) Poll(selection Selection[K], fresh bool, step time.Duration, timing RepeatTiming) Result[K] {
	if step < 0 || timing.Initial < 0 || timing.Interval <= 0 ||
		timing.Tolerance < 0 || timing.Tolerance >= timing.Interval {
		panic("input.Repeat.Poll: invalid repeat timing")
	}
	var result Result[K]
	result.Owned = selection.Held
	if !selection.Held || selection.Conflicting {
		r.Reset()

		return result
	}
	if !r.active || r.key != selection.Key || fresh {
		r.key, r.active, r.remaining = selection.Key, true, timing.Initial
	} else {
		r.remaining -= step
		if r.remaining > timing.Tolerance {
			return result
		}
		r.remaining += timing.Interval
	}
	result.Key, result.Triggered = selection.Key, true

	return result
}

// Reset discards the previous owner's hold without altering shared key ownership.
func (r *Repeat[K]) Reset() {
	var reset Repeat[K]
	*r = reset
}
