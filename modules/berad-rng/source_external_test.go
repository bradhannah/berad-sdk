package rng_test

import (
	"math"
	"slices"
	"strconv"
	"testing"

	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"
	"golang.org/x/exp/rand"
)

type countedDependency struct {
	rand.Source
	calls uint64
}

func (source *countedDependency) Uint64() uint64 {
	source.calls++
	return source.Source.Uint64()
}

func TestMixedIntnShuffleRestoreMatchesDependency(t *testing.T) {
	for _, seed := range []uint64{0, 1, 8675309, math.MaxUint64} {
		oracle := &countedDependency{Source: rand.NewSource(seed)}
		want := rand.New(oracle)
		got := rng.New(seed)
		// This bound causes frequent rejection on 64-bit platforms; the same
		// test remains valid with the smaller int range on 32-bit platforms.
		largeBound := math.MaxInt/2 + 2
		rejected := false
		for round := range 20 {
			for _, bound := range []int{1, 8, 1000, largeBound, 3, largeBound} {
				before := oracle.calls
				if actual, expected := got.Intn(bound), want.Intn(bound); actual != expected {
					t.Fatalf("seed %d round %d Intn(%d) = %d, want %d", seed, round, bound, actual, expected)
				}
				rejected = rejected || oracle.calls-before > 1
				if got.Calls() != oracle.calls {
					t.Fatalf("draw count = %d, want %d", got.Calls(), oracle.calls)
				}
				got = rng.Restore(got.Seed(), got.Calls())
			}
			var actualSwaps, expectedSwaps [][2]int
			got.Shuffle(11, func(i, j int) { actualSwaps = append(actualSwaps, [2]int{i, j}) })
			want.Shuffle(11, func(i, j int) { expectedSwaps = append(expectedSwaps, [2]int{i, j}) })
			if !slices.Equal(actualSwaps, expectedSwaps) || got.Calls() != oracle.calls {
				t.Fatalf("shuffle order/count changed: %v/%d, want %v/%d", actualSwaps, got.Calls(), expectedSwaps, oracle.calls)
			}
			got = rng.Restore(got.Seed(), got.Calls())
		}
		if strconv.IntSize == 64 && !rejected {
			t.Fatal("fixture did not exercise rejection draws")
		}
	}
}

func TestKnownPublicSequenceIncludesRejectionAndShuffle(t *testing.T) {
	if strconv.IntSize != 64 {
		// Fixed full-width values below require a 64-bit int; the mixed test
		// above still covers the public API on narrower platforms.
		return
	}
	// Recorded directly from x/exp/rand at a4bb9ffd2546. In particular, the
	// fifth Intn rejects one raw value and consumes two source draws.
	largeBound := uint64(1<<62 + 1)
	fixtures := []struct {
		bound uint64
		want  uint64
		calls uint64
	}{
		{1, 0, 1}, {8, 1, 2}, {1000, 72, 3},
		{largeBound, 867649948573917593, 4},
		{largeBound, 4246677790793934366, 6},
		{3, 2, 7}, {largeBound, 406519965772129914, 8},
	}
	source := rng.New(1)
	for _, fixture := range fixtures {
		if actual := source.Intn(int(fixture.bound)); uint64(actual) != fixture.want || source.Calls() != fixture.calls {
			t.Fatalf("Intn(%d) = %d at %d draws, want %d at %d", fixture.bound, actual, source.Calls(), fixture.want, fixture.calls)
		}
		source = rng.Restore(source.Seed(), source.Calls())
	}
	values := []int{0, 1, 2, 3, 4, 5, 6, 7}
	var swaps [][2]int
	source.Shuffle(len(values), func(i, j int) {
		swaps = append(swaps, [2]int{i, j})
		values[i], values[j] = values[j], values[i]
	})
	wantSwaps := [][2]int{{7, 7}, {6, 3}, {5, 3}, {4, 2}, {3, 0}, {2, 2}, {1, 1}}
	if !slices.Equal(swaps, wantSwaps) || !slices.Equal(values, []int{5, 1, 4, 0, 2, 6, 3, 7}) || source.Calls() != 15 {
		t.Fatalf("shuffle = %v, swaps %v, calls %d", values, swaps, source.Calls())
	}
	if next := rng.Restore(source.Seed(), source.Calls()).Intn(1000); next != 744 {
		t.Fatalf("restored next = %d, want 744", next)
	}
}

func TestResetPreservesBorrowersAndIndependentSources(t *testing.T) {
	source := rng.New(1)
	borrowed := source
	source.Intn(1000)
	source.Reset(8675309)
	independent := rng.New(8675309)
	oracle := rand.New(rand.NewSource(8675309))
	if borrowed != source || source.Seed() != 8675309 || source.Calls() != 0 {
		t.Fatal("Reset lost pointer identity or initial metadata")
	}
	for range 20 {
		expected := oracle.Intn(1000)
		if actual := borrowed.Intn(1000); actual != expected {
			t.Fatalf("borrowed source = %d, want %d", actual, expected)
		}
		if actual := independent.Intn(1000); actual != expected {
			t.Fatalf("independent source = %d, want %d", actual, expected)
		}
	}
	var zero rng.Source
	zero.Reset(0)
	if zero.Seed() != 0 || zero.Calls() != 0 || zero.Intn(1000) != rand.New(rand.NewSource(0)).Intn(1000) {
		t.Fatal("Reset did not initialize zero Source with meaningful seed zero")
	}
}

func TestInvalidBoundsAndShuffleCallbackContracts(t *testing.T) {
	source := rng.New(1)
	for _, bound := range []int{-1, 0, math.MinInt} {
		assertPanic(t, "invalid argument to Intn", func() { source.Intn(bound) })
	}
	assertPanic(t, "invalid argument to Shuffle", func() { source.Shuffle(-1, nil) })
	for _, length := range []int{0, 1} {
		source.Shuffle(length, nil)
		source.Shuffle(length, func(_, _ int) { t.Fatal("empty/single shuffle called swap") })
	}
	if source.Calls() != 0 {
		t.Fatalf("invalid bounds or empty shuffles consumed %d draws", source.Calls())
	}
	if source.Intn(1) != 0 || source.Calls() != 1 {
		t.Fatal("Intn(1) must consume one draw and return zero")
	}
	assertPanic(t, "host swap failed", func() {
		source.Shuffle(2, func(i, j int) {
			if i != 1 || j != 1 {
				t.Fatalf("swap(%d,%d), want (1,1)", i, j)
			}
			panic("host swap failed")
		})
	})
	if source.Calls() != 2 || source.Intn(1000) != 72 {
		t.Fatal("callback panic rolled back or added source draws")
	}
}

func assertPanic(t *testing.T, want string, action func()) {
	t.Helper()
	defer func() {
		if actual := recover(); actual != want {
			t.Errorf("panic = %v, want %q", actual, want)
		}
	}()
	action()
}
