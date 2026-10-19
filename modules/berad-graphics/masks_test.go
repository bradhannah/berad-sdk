package graphics_test

import (
	"bytes"
	"image"
	"math"
	"math/rand/v2"
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
	_, err = dissolve.Update(.4)
	if err != nil {
		t.Fatal(err)
	}
	first := dissolve.Pixels()
	dissolve.Update(1)
	dissolve.Update(.4)
	if !bytes.Equal(first, dissolve.Pixels()) {
		t.Fatal("reverse did not restore coverage")
	}
	dissolve.Reset()
	dissolve.Update(.4)
	if !bytes.Equal(first, dissolve.Pixels()) || random.calls != 160 {
		t.Fatal("reset reshuffled")
	}
	if _, err := dissolve.Update(math.NaN()); err == nil || !bytes.Equal(first, dissolve.Pixels()) {
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

func TestBayerCoverageAndAnimationEndpoints(t *testing.T) {
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
	source := imageOf(t, 1, 1, []byte{10, 20, 30, 255})
	first, _ := graphics.NewFrame(source, image.Pt(4, 7), image.Pt(1, 2), image.Pt(2, 6))
	second, _ := first.Pad(graphics.Insets{Left: 1})
	frames := []graphics.Frame{first, second}
	animation, err := graphics.NewAnimation(frames, 100)
	if err != nil {
		t.Fatal(err)
	}
	frames[0] = second
	if animation.At(0).Image() != source || animation.At(99).Image() != source || animation.At(100).Image() != second.Image() || animation.At(-1).Image() != second.Image() || animation.At(200).Image() != source {
		t.Fatal("frame endpoints or snapshot")
	}
	if _, err := graphics.NewAnimation(nil, 100); err == nil {
		t.Fatal("empty frames")
	}
	if _, err := graphics.NewAnimation(frames, 0); err == nil {
		t.Fatal("zero interval")
	}
}
