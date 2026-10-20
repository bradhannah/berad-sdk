package input

// ReleaseBarrier waits for every key held at Capture to be observed released.
// Its zero value is ready. Masked input must not be passed to Observe as a release.
type ReleaseBarrier[K comparable] struct {
	keys []K
}

// Capture replaces the barrier with the currently held subset of keys.
func (b *ReleaseBarrier[K]) Capture(keys []K, held func(K) bool) {
	b.keys = nil
	for _, key := range keys {
		if held(key) {
			b.keys = append(b.keys, key)
		}
	}
}

// Observe clears the entire barrier when all captured keys are released together.
// A key released and repressed before the others release still holds the barrier.
func (b *ReleaseBarrier[K]) Observe(held func(K) bool) bool {
	for _, key := range b.keys {
		if held(key) {
			return false
		}
	}
	b.Reset()

	return true
}

// Waiting reports retained state without sampling or advancing it.
func (b *ReleaseBarrier[K]) Waiting() bool { return len(b.keys) != 0 }

// Reset discards this barrier, not any keyboard's consumption or suppression.
func (b *ReleaseBarrier[K]) Reset() { b.keys = nil }
