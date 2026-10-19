package graphics_test

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"testing"

	graphics "github.com/bradhannah/berad-sdk/modules/berad-graphics"
)

func imageOf(t *testing.T, width, height int, pixels []byte) *graphics.Image {
	t.Helper()
	result, err := graphics.NewPixels(width, height, pixels)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func apply(t *testing.T, source *graphics.Image, operations ...graphics.Operation) *graphics.Image {
	t.Helper()
	result, err := source.Apply(operations...)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPixelsSnapshotAndDetachedCopies(t *testing.T) {
	pixels := []byte{80, 40, 20, 128, 90, 60, 30, 0}
	source := imageOf(t, 2, 1, pixels)
	clear(pixels)
	copy := source.CopyRGBA()
	if copy.Bounds() != image.Rect(0, 0, 2, 1) || copy.Stride != 8 ||
		copy.RGBAAt(0, 0) != (color.RGBA{R: 80, G: 40, B: 20, A: 128}) {
		t.Fatal("snapshot/origin lost")
	}
	clear(copy.Pix)
	clear(source.Pixels())
	if !bytes.Equal(source.Pixels(), []byte{80, 40, 20, 128, 90, 60, 30, 0}) || source.Bytes() != 8 {
		t.Fatal("returned storage aliases source or payload accounting changed")
	}
}

func TestChannelOperationsPreserveDeclaredAppearanceAndOrdering(t *testing.T) {
	source := imageOf(t, 1, 1, []byte{80, 40, 20, 128})
	independent := apply(t, source, graphics.AlphaChannelScale(.5, graphics.Selection{})).Pixels()
	if !bytes.Equal(independent, []byte{80, 40, 20, 64}) {
		t.Fatalf("alpha-only scaling must preserve RGB: %v", independent)
	}
	if !bytes.Equal(apply(t, source, graphics.RGBGain(0, graphics.Selection{})).Pixels(), []byte{0, 0, 0, 128}) {
		t.Fatal("zero gain is not black")
	}
	first := apply(t, source, graphics.RGBGain(4, graphics.Selection{}), graphics.RGBGain(.5, graphics.Selection{}))
	second := apply(t, source, graphics.RGBGain(.5, graphics.Selection{}), graphics.RGBGain(4, graphics.Selection{}))
	if !bytes.Equal(first.Pixels(), []byte{127, 80, 40, 128}) ||
		!bytes.Equal(second.Pixels(), []byte{160, 80, 40, 128}) {
		t.Fatal("ordered saturation changed", first.Pixels(), second.Pixels())
	}
	if !bytes.Equal(source.Pixels(), []byte{80, 40, 20, 128}) {
		t.Fatal("source mutated")
	}
}

func TestRGBGainIncludesAlphaZeroEmission(t *testing.T) {
	source := imageOf(t, 2, 1, []byte{80, 40, 20, 0, 80, 40, 20, 128})
	for _, sample := range []struct {
		gain float64
		want []byte
	}{
		{0, []byte{0, 0, 0, 0, 0, 0, 0, 128}},
		{2, []byte{160, 80, 40, 0, 160, 80, 40, 128}},
		{4, []byte{255, 160, 80, 0, 255, 160, 80, 128}},
	} {
		got := apply(t, source, graphics.RGBGain(sample.gain, graphics.Selection{})).Pixels()
		if !bytes.Equal(got, sample.want) {
			t.Fatalf("gain %v: %v != %v", sample.gain, got, sample.want)
		}
	}
	got := apply(t, source, graphics.AlphaChannelScale(0, graphics.Selection{}),
		graphics.RGBGain(0, graphics.Selection{})).Pixels()
	if !bytes.Equal(got, make([]byte, 8)) {
		t.Fatalf("zero gain after alpha removal retained emission: %v", got)
	}
	if !bytes.Equal(source.Pixels(), []byte{80, 40, 20, 0, 80, 40, 20, 128}) {
		t.Fatal("gain mutated source")
	}
}

func TestOutlineAndBodyFreeContourHaveIndependentPixelOracles(t *testing.T) {
	pixels := make([]byte, 7*5*4)
	center := (2*7 + 3) * 4
	copy(pixels[center:], []byte{40, 0, 0, 80})
	source := imageOf(t, 7, 5, pixels)
	face, backing := color.RGBA{G: 255, A: 255}, color.RGBA{A: 255}
	outline := apply(t, source, graphics.Outline(face, 0)).CopyRGBA()
	contour := apply(t, source, graphics.Contour(face, backing)).CopyRGBA()
	for y := 0; y < 5; y++ {
		for x := 0; x < 7; x++ {
			distance := max(abs(x-3), abs(y-2))
			expected := color.RGBA{}
			if distance == 1 {
				expected = face
			}
			if distance == 2 {
				expected = backing
			}
			if contour.RGBAAt(x, y) != expected {
				t.Fatalf("contour %d,%d: %v != %v", x, y, contour.RGBAAt(x, y), expected)
			}
		}
	}
	if outline.RGBAAt(3, 2) != (color.RGBA{R: 40, A: 80}) {
		t.Fatal("outline removed body")
	}
	if apply(t, source, graphics.CheckerOutline(face, backing, 0)).CopyRGBA().RGBAAt(2, 1) != backing {
		t.Fatal("checker phase")
	}
	checker := apply(t, source, graphics.CheckerOutline(face, backing, 1)).CopyRGBA()
	if checker.RGBAAt(2, 1) != face || checker.RGBAAt(3, 2) != (color.RGBA{R: 40, A: 80}) {
		t.Fatal("alternate checker/body preservation")
	}
	if source.Silhouette(0).CopyRGBA().RGBAAt(3, 2) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("silhouette depends on RGB")
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func TestProceduralKernelsUseDimensionsSelectionsAndStablePhases(t *testing.T) {
	pixels := make([]byte, 7*23*4)
	for y := 0; y < 23; y++ {
		for x := 0; x < 7; x++ {
			copy(pixels[(y*7+x)*4:], []byte{byte(x), byte(y), 0, 255})
		}
	}
	source := imageOf(t, 7, 23, pixels)
	for _, offset := range []int{-1000, -1, 0, 1, 23, 1000} {
		result := apply(t, source, graphics.ScrollY(offset)).CopyRGBA()
		for y := 0; y < 23; y++ {
			row := (y - offset) % 23
			if row < 0 {
				row += 23
			}
			if result.RGBAAt(3, y).G != byte(row) {
				t.Fatalf("scroll %d row %d", offset, y)
			}
		}
	}
	selected := make([]byte, 7*23)
	selected[4*7+3] = 1
	mask, err := graphics.NewMask(7, 23, selected)
	if err != nil {
		t.Fatal(err)
	}
	clear(selected)
	selection := graphics.Select(image.Rect(2, 3, 6, 7), mask)
	wave := apply(t, source, graphics.Wave(1, math.Pi/2, 0, selection)).CopyRGBA()
	if wave.RGBAAt(3, 4).R != 4 || wave.RGBAAt(2, 4).R != 2 {
		t.Fatal("selection was mutated or applied to source samples")
	}
	rise := apply(t, source, graphics.Rise(0, selection)).CopyRGBA()
	// (3*31 + 4*17) % 100 = 61; jitter 27 selects one column right, no rise.
	if rise.RGBAAt(3, 4) != (color.RGBA{4, 4, 0, 255}) {
		t.Fatal("rise oracle", rise.RGBAAt(3, 4))
	}
	if !bytes.Equal(apply(t, source, graphics.Rise(0, selection)).Pixels(), apply(t, source, graphics.Rise(100, selection)).Pixels()) {
		t.Fatal("phase unstable")
	}
	replaced := apply(t, source, graphics.Replace(source.Silhouette(0), selection)).CopyRGBA()
	if replaced.RGBAAt(3, 4).R != 255 || replaced.RGBAAt(2, 4).R != 2 {
		t.Fatal("replacement selection")
	}
}

func TestInvalidInputsRejectBeforeAllocationAndDoNotReturnSuccessFallback(t *testing.T) {
	if _, err := graphics.NewPixels(math.MaxInt, 2, nil); err == nil {
		t.Fatal("overflow dimensions accepted")
	}
	source := imageOf(t, 1, 1, []byte{0, 0, 0, 255})
	for _, operation := range []graphics.Operation{{}, graphics.RGBGain(math.NaN(), graphics.Selection{}), graphics.AlphaChannelScale(2, graphics.Selection{}), graphics.Replace(nil, graphics.Selection{})} {
		if result, err := source.Apply(operation); err == nil || result != nil {
			t.Fatal("invalid request returned success-shaped fallback")
		}
	}
}
