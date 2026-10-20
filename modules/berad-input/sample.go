package input

// Sample is an immutable, detached physical observation. It can be inspected
// after later captures, but only the keyboard's current sample can be consumed.
// The zero sample has no held keys or edges and cannot be consumed.
type Sample[K comparable] struct {
	owner      *Keyboard[K]
	generation uint64
	held       map[K]bool
	pressed    map[K]bool
}

// Held reports a physical hold in this sample, irrespective of consumption.
func (s Sample[K]) Held(key K) bool { return s.held[key] }

// Pressed reports a new physical edge, not ordering between simultaneous keys.
func (s Sample[K]) Pressed(key K) bool { return s.pressed[key] }

// Capture observes releases then captures edges without consuming them. The host
// calls it once before advancing input owners. Live Held queries remain live.
func (k *Keyboard[K]) Capture() Sample[K] {
	k.ObserveReleases()
	k.generation++
	k.edges = make(map[K]bool)
	sample := Sample[K]{owner: k, generation: k.generation, held: make(map[K]bool), pressed: make(map[K]bool)}
	if k.Suspended() {
		return sample // Do not reinterpret masking as a physical release.
	}
	for _, key := range k.keys {
		if k.Held(key) {
			sample.held[key] = true
			sample.pressed[key] = !k.previous[key]
			k.edges[key] = sample.pressed[key]
		}
	}
	k.previous = sample.held

	return sample
}

// ConsumeEdge admits one current captured edge without any cadence or global time
// delay. Unlike Ready, it does not service the transition barrier as a side effect.
func (k *Keyboard[K]) ConsumeEdge(sample Sample[K], key K) bool {
	if sample.owner != k || sample.generation != k.generation || !k.edges[key] || !k.Held(key) ||
		k.Suspended() || k.WaitingForRelease() || k.Suppressed(key) || k.Consumed(key) {
		return false
	}
	k.Consume(key)

	return true
}
