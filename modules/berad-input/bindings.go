package input

import "slices"

// Bindings is a registered, copied priority order of physical keys. It has no
// command callbacks, modifier policy, or retained consumption. Its zero value
// selects nothing. Duplicate entries are preserved, not silently reordered.
type Bindings[K comparable] struct {
	keys []K
}

// NewBindings registers keys in caller order. Later caller mutation is isolated.
func NewBindings[K comparable](keys ...K) Bindings[K] {
	return Bindings[K]{keys: slices.Clone(keys)}
}

// FirstHeld selects the first held key allowed by filter (nil allows every key).
// It never consumes. In particular, it does not skip consumed keys unless the
// caller's filter explicitly requests that different policy.
//
//nolint:ireturn // Return the caller's concrete key identity, not an SDK-owned interface.
func (b Bindings[K]) FirstHeld(held func(K) bool, filter func(K) bool) (K, bool) {
	for _, key := range b.keys {
		if held(key) && (filter == nil || filter(key)) {
			return key, true
		}
	}
	var zero K

	return zero, false
}

// Selection distinguishes no input from an ambiguous simultaneous hold.
type Selection[K comparable] struct {
	Key         K
	Held        bool
	Conflicting bool
}

// SelectSingle inspects all registered keys until a second hold is found.
func (b Bindings[K]) SelectSingle(held func(K) bool) Selection[K] {
	var selection Selection[K]
	for _, key := range b.keys {
		if !held(key) {
			continue
		}
		if selection.Held {
			selection.Conflicting = true

			return selection
		}
		selection.Key, selection.Held = key, true
	}

	return selection
}
