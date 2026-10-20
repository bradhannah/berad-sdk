package graphics_test

import (
	"bytes"
	"image"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	graphics "github.com/bradhannah/berad-sdk/modules/berad-graphics"
)

type countedRandom struct {
	calls  int
	random *rand.Rand
}

func (random *countedRandom) Intn(n int) int { random.calls++; return random.random.IntN(n) }

func TestDissolveReversalResetAndRandomOwnership(t *testing.T) {
	random := &countedRandom{random: rand.New(rand.NewPCG(1, 2))}
	dissolve, err := graphics.NewDissolve(7, 23, false, random)
	if err != nil {
		t.Fatal(err)
	}
	if random.calls != 160 {
		t.Fatal("unexpected shuffle consumption")
	}
	buffer := make([]byte, 7*23*4)
	dissolve.WritePixels(buffer)
	_, _, err = dissolve.UpdatePixels(.4, buffer)
	if err != nil {
		t.Fatal(err)
	}
	first := bytes.Clone(buffer)
	dissolve.UpdatePixels(1, buffer)
	dissolve.UpdatePixels(.4, buffer)
	if !bytes.Equal(first, buffer) {
		t.Fatal("reverse did not restore coverage")
	}
	dissolve.Reset()
	dissolve.WritePixels(buffer)
	dissolve.UpdatePixels(.4, buffer)
	if !bytes.Equal(first, buffer) || random.calls != 160 {
		t.Fatal("reset reshuffled")
	}
	if _, _, err := dissolve.UpdatePixels(math.NaN(), buffer); err == nil || !bytes.Equal(first, buffer) {
		t.Fatal("NaN mutated dissolve")
	}
	visited := 0
	dissolve.VisitLeading(.1, func(image.Point) { visited++ })
	if visited != 16 {
		t.Fatal("leading cells", visited)
	}
	for index := 0; index < len(first); index += 4 {
		if first[index] != first[index+3] {
			t.Fatal("invalid premultiplied mask")
		}
	}
}

// Choosing the last available entry leaves row-major order unchanged. Expected
// coverage below is therefore independent of the implementation's stored state.
type orderedRandom struct{}

func (orderedRandom) Intn(n int) int { return n - 1 }

func TestDissolveLeadingQueriesPreserveStateAndSuppressSettledOrHiddenHighlights(t *testing.T) {
	for _, hide := range []bool{false, true} {
		pattern, err := graphics.NewDissolve(4, 2, hide, orderedRandom{})
		if err != nil {
			t.Fatal(err)
		}
		buffer := make([]byte, 32)
		pattern.WritePixels(buffer)
		leading := func() []image.Point {
			var points []image.Point
			pattern.VisitLeading(.25, func(point image.Point) { points = append(points, point) })
			return points
		}
		if len(leading()) != 0 {
			t.Fatal("initial state has no leading cells")
		}
		if _, _, err := pattern.UpdatePixels(.5, buffer); err != nil {
			t.Fatal(err)
		}
		before := bytes.Clone(buffer)
		var want []image.Point
		if !hide {
			// Row-major order: four revealed cells, of which the newest two lead.
			want = []image.Point{{X: 2, Y: 0}, {X: 3, Y: 0}}
		}
		for range 2 {
			if got := leading(); !slices.Equal(got, want) {
				t.Fatalf("hide=%v leading=%v, want %v", hide, got, want)
			}
		}
		pattern.WritePixels(buffer)
		if !bytes.Equal(buffer, before) {
			t.Fatal("query mutated ordinary mask state")
		}
		if _, changed, err := pattern.UpdatePixels(.5, buffer); err != nil || changed {
			t.Fatal("query advanced dissolve progress", err)
		}
		if _, _, err := pattern.UpdatePixels(1, buffer); err != nil {
			t.Fatal(err)
		}
		if len(leading()) != 0 {
			t.Fatal("completed silhouettes must have no highlights")
		}
		pattern.Reset()
		pattern.WritePixels(buffer)
		if len(leading()) != 0 {
			t.Fatal("reset retained highlights")
		}
		if _, _, err := pattern.UpdatePixels(.5, buffer); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(leading(), want) {
			t.Fatal("reset changed leading order")
		}
	}
}

func TestDissolveLiteralCoverageCompletionAndReversal(t *testing.T) {
	for _, hide := range []bool{false, true} {
		pattern, err := graphics.NewDissolve(3, 2, hide, orderedRandom{})
		if err != nil {
			t.Fatal(err)
		}
		buffer, full := make([]byte, 24), make([]byte, 24)
		pattern.WritePixels(buffer)
		for _, sample := range []struct {
			progress float64
			count    int
			changed  bool
		}{{-1, 0, false}, {.5, 3, true}, {.6, 3, false}, {1, 6, true}, {2, 6, false}, {.25, 1, true}, {0, 0, true}} {
			complete, changed, err := pattern.UpdatePixels(sample.progress, buffer)
			if err != nil || complete != (sample.count == 6) || changed != sample.changed {
				t.Fatalf("hide=%v progress=%v: complete=%v changed=%v err=%v", hide, sample.progress, complete, changed, err)
			}
			want := make([]byte, 24)
			for cell := range 6 {
				if (cell < sample.count) != hide {
					copy(want[cell*4:], []byte{255, 255, 255, 255})
				}
			}
			pattern.WritePixels(full)
			if !bytes.Equal(buffer, want) || !bytes.Equal(full, want) {
				t.Fatalf("hide=%v progress=%v: incremental=%v full=%v want=%v", hide, sample.progress, buffer, full, want)
			}
		}
	}
}

func TestBayerCoverageAndCycleEndpoints(t *testing.T) {
	for _, tc := range []struct {
		tick, offset int64
		want         int
	}{{-1, 0, 15}, {math.MaxInt64, math.MaxInt64, 14}, {math.MinInt64, 0, 0}} {
		index, err := graphics.CycleIndex(16, tc.tick, tc.offset)
		if err != nil || index != tc.want {
			t.Fatalf("cycle %+v: %d %v", tc, index, err)
		}
	}
	if _, err := graphics.CycleIndex(0, 0, 0); err == nil {
		t.Fatal("empty cycle accepted")
	}
	for _, origin := range []image.Point{{}, {X: -4, Y: -8}} {
		kept := 0
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				if graphics.DitherKeeps(graphics.CoverageFull/2, x+origin.X, y+origin.Y) {
					kept++
				}
			}
		}
		if kept != 8 {
			t.Fatal("Bayer half coverage", kept)
		}
	}
}

func TestPulseLiteralPhasesAndInvalidInputs(t *testing.T) {
	for _, sample := range []struct {
		elapsed int64
		want    float64
	}{{0, 3}, {100, 5}, {200, 3}, {300, 1}, {400, 3}, {-100, 1}} {
		got, err := graphics.Pulse(sample.elapsed, 400, 1, 5)
		if err != nil || math.Abs(got-sample.want) > 1e-12 {
			t.Fatalf("phase %d: %v != %v (%v)", sample.elapsed, got, sample.want, err)
		}
	}
	for _, sample := range []struct {
		period    int64
		low, high float64
	}{{0, 0, 1}, {-1, 0, 1}, {1, 2, 1}, {1, math.NaN(), 1}, {1, 0, math.Inf(1)}, {1, -8193, 1}, {1, 0, 8193}} {
		if _, err := graphics.Pulse(0, sample.period, sample.low, sample.high); err == nil {
			t.Fatal("invalid pulse accepted", sample)
		}
	}
}

func TestDitherPatternPhasesAndStrictThresholds(t *testing.T) {
	bayer := [4][4]uint16{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}
	for coverage := uint16(0); coverage <= 4096; coverage++ {
		for y := range 4 {
			for x := range 4 {
				want := coverage > (2*bayer[y][x]+1)*128
				for _, offset := range [][2]int{{0, 0}, {-20, -16}, {16, 20}, {-256, 256}} {
					if graphics.DitherKeeps(coverage, x+offset[0], y+offset[1]) != want {
						t.Fatalf("phase/wrap/strict-threshold mismatch: coverage=%d coordinate=(%d,%d)",
							coverage, x+offset[0], y+offset[1])
					}
				}
			}
		}
	}
	for _, coordinate := range []int{math.MinInt, math.MaxInt, -1, 0, 1} {
		phase := ((coordinate % 4) + 4) % 4
		if graphics.DitherKeeps(2000, coordinate, coordinate) != graphics.DitherKeeps(2000, phase, phase) {
			t.Fatal("extreme coordinate did not wrap")
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("invalid Q12 coverage accepted")
		}
	}()
	graphics.DitherKeeps(4097, 0, 0)
}
