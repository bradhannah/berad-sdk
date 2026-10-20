package rng

import (
	"encoding/binary"
	"math/bits"

	//nolint:staticcheck // x/exp/rand preserves the supported PCG stream and draw consumption.
	"golang.org/x/exp/rand"
)

const (
	pcgMultiplier     = 47026247687942121848144207491837523525
	pcgIncrement      = 117397592171526113268558934119004209487
	pcgMultiplierHigh = uint64(pcgMultiplier >> 64)
	pcgMultiplierLow  = uint64(pcgMultiplier & (1<<64 - 1))
	pcgIncrementHigh  = uint64(pcgIncrement >> 64)
	pcgIncrementLow   = uint64(pcgIncrement & (1<<64 - 1))
	pcgStateBytes     = 16
)

// Source owns one reproducible stream and its underlying draw count.
// Use New, Restore, or Reset before drawing; the zero value is not initialized.
// Do not copy a Source: share its pointer and serialize all access in the host.
type Source struct {
	random *rand.Rand
	source *countingSource
	seed   uint64
}

type countingSource struct {
	source rand.Source
	calls  uint64
}

type uint128 struct {
	high uint64
	low  uint64
}

// New constructs an independent stream at the beginning of seed's sequence.
// Every uint64 seed, including zero, is meaningful; there is no implicit seed.
func New(seed uint64) *Source {
	source := &countingSource{source: rand.NewSource(seed), calls: 0}

	return &Source{random: rand.New(source), source: source, seed: seed}
}

// Reset restarts seed's sequence and clears Calls while preserving this pointer
// for all borrowers. It also initializes a zero-valued Source.
func (source *Source) Reset(seed uint64) {
	*source = *New(seed)
}

// Restore constructs an independent stream after calls underlying source draws.
// It takes O(log(calls+1)) work and constant space, even for math.MaxUint64 calls.
// The host owns serialization of the seed and count; calls is not an Intn or
// Shuffle invocation count, because rejection sampling can consume extra draws.
func Restore(seed, calls uint64) *Source {
	state := advancePCG(uint128{high: seed, low: seed}, calls)
	encoded := make([]byte, pcgStateBytes)
	binary.BigEndian.PutUint64(encoded[:8], state.high)
	binary.BigEndian.PutUint64(encoded[8:], state.low)

	pcg := &rand.PCGSource{}
	if err := pcg.UnmarshalBinary(encoded); err != nil {
		panic(err)
	}

	counted := &countingSource{source: pcg, calls: calls}

	return &Source{random: rand.New(counted), source: counted, seed: seed}
}

func advancePCG(state uint128, calls uint64) uint128 {
	accMultiplier := uint128{high: 0, low: 1}
	accIncrement := uint128{high: 0, low: 0}
	currentMultiplier := uint128{high: pcgMultiplierHigh, low: pcgMultiplierLow}
	currentIncrement := uint128{high: pcgIncrementHigh, low: pcgIncrementLow}

	for calls > 0 {
		if calls&1 != 0 {
			accMultiplier = multiply128(accMultiplier, currentMultiplier)
			accIncrement = add128(multiply128(accIncrement, currentMultiplier), currentIncrement)
		}

		currentIncrement = multiply128(add128(currentMultiplier, uint128{high: 0, low: 1}), currentIncrement)
		currentMultiplier = multiply128(currentMultiplier, currentMultiplier)
		calls >>= 1
	}

	return add128(multiply128(accMultiplier, state), accIncrement)
}

func add128(left, right uint128) uint128 {
	low, carry := bits.Add64(left.low, right.low, 0)
	high, _ := bits.Add64(left.high, right.high, carry)

	return uint128{high: high, low: low}
}

func multiply128(left, right uint128) uint128 {
	productHigh, productLow := bits.Mul64(left.low, right.low)
	productHigh += left.low*right.high + left.high*right.low

	return uint128{high: productHigh, low: productLow}
}

func (source *countingSource) Uint64() uint64 {
	source.calls++

	return source.source.Uint64()
}

func (source *countingSource) Seed(seed uint64) {
	source.source.Seed(seed)
	source.calls = 0
}

// Seed reports the initialization seed without advancing the stream.
func (source *Source) Seed() uint64 { return source.seed }

// Calls reports underlying 64-bit source draws without advancing the stream.
// The uint64 counter wraps on overflow; exact seed/count persistence requires
// capturing before that wrap, since the PCG period exceeds the counter's range.
func (source *Source) Calls() uint64 { return source.source.calls }

// Intn returns an unbiased integer in [0, n), preserving x/exp/rand ordering.
// It panics without consuming a draw when n <= 0. Intn(1) still consumes a draw.
func (source *Source) Intn(n int) int { return source.random.Intn(n) }

// Shuffle permutes n elements through the caller's synchronous swap callback,
// preserving x/exp/rand's descending Fisher-Yates swap and draw order.
// Negative n panics before drawing; sizes 0 and 1 make no swaps or draws. For n > 1,
// swap must be non-nil and must not reenter this Source. A callback panic is
// propagated; already consumed draws and completed swaps are not rolled back.
func (source *Source) Shuffle(n int, swap func(i, j int)) { source.random.Shuffle(n, swap) }
