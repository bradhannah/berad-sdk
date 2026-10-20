package input

import "slices"

// Keyboard shares physical-key ownership across the host's ordered input owners.
// Construct with NewKeyboard; do not copy a constructed Keyboard.
type Keyboard[K comparable] struct {
	keys        []K
	held        func(K) bool
	known       map[K]bool
	consumed    map[K]bool
	suppressed  map[K]bool
	previous    map[K]bool
	edges       map[K]bool
	barrier     ReleaseBarrier[K]
	suspensions int
	generation  uint64
}

// NewKeyboard copies a finite key universe and borrows a synchronous held source.
// Include modifiers and unbound keys needed by transitions. Nil sources, duplicate
// keys, and non-reflexive keys are programming errors and panic.
func NewKeyboard[K comparable](keys []K, held func(K) bool) *Keyboard[K] {
	if held == nil {
		panic("input.NewKeyboard: held source is required")
	}
	keyboard := new(Keyboard[K])
	keyboard.keys, keyboard.held = slices.Clone(keys), held
	keyboard.known = make(map[K]bool, len(keys))
	keyboard.consumed = make(map[K]bool)
	keyboard.suppressed = make(map[K]bool)
	for _, key := range keys {
		//nolint:gocritic // Non-reflexive keys (for example NaN) cannot identify retained map entries.
		if key != key || keyboard.known[key] {
			panic("input.NewKeyboard: keys must be distinct and reflexive")
		}
		keyboard.known[key] = true
	}

	return keyboard
}

// Held reads the supplied source. Suspension masks it; consumption does not.
func (k *Keyboard[K]) Held(key K) bool {
	return !k.Suspended() && k.known[key] && k.held(key)
}

// Suspended reports whether any unclosed suspension owns this keyboard.
func (k *Keyboard[K]) Suspended() bool { return k.suspensions != 0 }

// Consumed reports ordinary one-shot ownership without polling physical state.
func (k *Keyboard[K]) Consumed(key K) bool { return k.consumed[key] }

// Suppressed reports stronger protection that repeat owners must also honor.
func (k *Keyboard[K]) Suppressed(key K) bool { return k.suppressed[key] }

// Consume records an already accepted key, even when supplied by a host command.
// It does not poll, apply timing, or clear suppression. Unknown keys are ignored.
func (k *Keyboard[K]) Consume(key K) {
	if k.known[key] {
		k.consumed[key] = true
		delete(k.edges, key)
	}
}

// SuppressUntilRelease protects a currently held key from all subsequent owners.
func (k *Keyboard[K]) SuppressUntilRelease(key K) {
	if k.Held(key) {
		k.suppressed[key] = true
	}
}

// ObserveReleases services ownership once per host update, including reader-only
// updates. Repeated calls are harmless. Suspended masking cannot manufacture release.
func (k *Keyboard[K]) ObserveReleases() {
	if k.Suspended() {
		return
	}
	for key := range k.suppressed {
		if !k.Held(key) {
			delete(k.suppressed, key)
		}
	}
	k.barrier.Observe(k.Held)
	for key := range k.consumed {
		if !k.Held(key) {
			delete(k.consumed, key)
		}
	}
	for key := range k.edges {
		if !k.Held(key) {
			delete(k.edges, key)
		}
	}
}

// PrepareTransition captures current holds, including before the first Update.
// Existing captured press samples become stale; no physical edge is synthesized.
func (k *Keyboard[K]) PrepareTransition() {
	k.generation++
	k.barrier.Capture(k.keys, k.Held)
}

// WaitingForRelease is a non-servicing transition status query.
func (k *Keyboard[K]) WaitingForRelease() bool { return k.barrier.Waiting() }

// ResetTransition clears only the transition barrier, never stronger suppression.
func (k *Keyboard[K]) ResetTransition() { k.barrier.Reset() }

// Ready observes the transition barrier. It does not require the candidate to be
// physically held: callers may already have selected it. Repeat owners set once
// false to bypass ordinary consumption, but never suspension or suppression.
func (k *Keyboard[K]) Ready(key K, once bool) bool {
	return k.known[key] && !k.Suspended() && !k.Suppressed(key) &&
		k.barrier.Observe(k.Held) && (!once || !k.Consumed(key))
}

// Suspend masks polling until the returned idempotent closure is called. On the
// last restore, every currently held key is suppressed through physical release.
// Samples captured before suspension cannot be replayed after it.
func (k *Keyboard[K]) Suspend() func() {
	k.suspensions++
	k.generation++
	restored := false

	return func() {
		if restored {
			return
		}
		restored = true
		k.suspensions--
		if !k.Suspended() {
			for _, key := range k.keys {
				k.SuppressUntilRelease(key)
			}
		}
	}
}
