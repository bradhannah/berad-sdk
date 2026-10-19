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

func TestSnapshotRegionStrideAndFramePadding(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(10, 20, 60, 50))
	atlas.SetRGBA(14, 23, color.RGBA{R: 80, A: 128})
	region := atlas.SubImage(image.Rect(14, 23, 32, 32))
	source, err := graphics.NewImage(region)
	if err != nil {
		t.Fatal(err)
	}
	atlas.SetRGBA(14, 23, color.RGBA{})
	copy := source.CopyRGBA()
	if copy.Bounds() != image.Rect(0, 0, 18, 9) || copy.RGBAAt(0, 0) != (color.RGBA{R: 80, A: 128}) {
		t.Fatal("snapshot/origin lost")
	}
	copy.SetRGBA(0, 0, color.RGBA{})
	if source.Pixels()[0] != 80 {
		t.Fatal("returned image aliases source")
	}
	frame, err := graphics.NewFrame(source, image.Pt(41, 23), image.Pt(3, 5), image.Pt(20, 22))
	if err != nil {
		t.Fatal(err)
	}
	padded, err := frame.Pad(graphics.Insets{Left: 2, Top: 3, Right: 1, Bottom: 4})
	if err != nil {
		t.Fatal(err)
	}
	if padded.Offset() != image.Pt(1, 2) || padded.Anchor() != frame.Anchor() || padded.Image().Bounds() != image.Rect(0, 0, 21, 16) {
		t.Fatal("padding changed logical placement")
	}
	if padded.Image().CopyRGBA().RGBAAt(2, 3).R != 80 {
		t.Fatal("padded content moved incorrectly")
	}
	cropped, err := padded.Image().Crop(image.Rect(2, 3, 20, 12))
	if err != nil || !bytes.Equal(cropped.Pixels(), source.Pixels()) {
		t.Fatal("crop does not restore original", err)
	}
}

func TestChannelOperationsPreserveDeclaredAppearanceAndOrdering(t *testing.T) {
	source := imageOf(t, 1, 1, []byte{80, 40, 20, 128})
	ordinary := apply(t, source, graphics.Opacity(.5)).Pixels()
	independent := apply(t, source, graphics.AlphaChannelScale(.5, graphics.Selection{})).Pixels()
	if !bytes.Equal(ordinary, []byte{40, 20, 10, 64}) || !bytes.Equal(independent, []byte{80, 40, 20, 64}) {
		t.Fatalf("opacity and alpha-only must differ: %v %v", ordinary, independent)
	}
	if !bytes.Equal(apply(t, source, graphics.RGBGain(0, graphics.Selection{})).Pixels(), []byte{0, 0, 0, 128}) {
		t.Fatal("zero gain is not black")
	}
	first := apply(t, source, graphics.RGBGain(4, graphics.Selection{}), graphics.TintRGB(128, 255, 255))
	second := apply(t, source, graphics.TintRGB(128, 255, 255), graphics.RGBGain(4, graphics.Selection{}))
	if bytes.Equal(first.Pixels(), second.Pixels()) {
		t.Fatal("ordered clipping/composition lost")
	}
	if !bytes.Equal(source.Pixels(), []byte{80, 40, 20, 128}) {
		t.Fatal("source mutated")
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
	for _, operation := range []graphics.Operation{{}, graphics.RGBGain(math.NaN(), graphics.Selection{}), graphics.Opacity(2), graphics.Replace(nil, graphics.Selection{})} {
		if result, err := source.Apply(operation); err == nil || result != nil {
			t.Fatal("invalid request returned success-shaped fallback")
		}
	}
	if _, err := source.Pad(graphics.Insets{Left: math.MaxInt}); err == nil {
		t.Fatal("padding overflow accepted")
	}
}
