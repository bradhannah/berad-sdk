package ebitengine_test

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	backend "github.com/bradhannah/berad-sdk/modules/berad-graphics/backend/ebitengine"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestSurfaceResizeBudgetFailureAndExplicitUpload(t *testing.T) {
	var surface backend.Surface
	defer surface.Close()
	buffer, err := surface.Resize(2, 3)
	if err != nil {
		t.Fatal(err)
	}
	buffer[3] = 255
	first := surface.Image()
	for _, size := range []image.Point{{0, 1}, {1, 0}, {-1, 1}, {8193, 1}} {
		if _, err := surface.Resize(size.X, size.Y); err == nil || surface.Image() != first || buffer[3] != 255 {
			t.Fatal("invalid resize changed live resources")
		}
	}
	if err := surface.Upload(); err != nil {
		t.Fatal(err)
	}
	if testing.AllocsPerRun(100, func() {
		_, err := surface.Resize(2, 3)
		if err != nil {
			panic(err)
		}
	}) != 0 {
		t.Fatal("same-size resize allocated")
	}
	if buffer[3] != 0 || pixels(first)[3] != 255 {
		t.Fatal("clearing CPU storage implicitly uploaded")
	}
	if err := surface.Upload(); err != nil {
		t.Fatal(err)
	}
	if pixels(first)[3] != 0 {
		t.Fatal("upload did not replace content")
	}
	second, err := surface.Resize(3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if surface.Image() == first || &second[0] == &buffer[0] || !bytes.Equal(second, make([]byte, 24)) {
		t.Fatal("resize failed to replace storage")
	}
	surface.Close()
	surface.Close()
	if surface.Image() != nil {
		t.Fatal("close retained GPU view")
	}
}

func TestSurfaceFractionalSamplingAndClippingIndependentOfAtlas(t *testing.T) {
	background := color.RGBA{R: 80, G: 120, B: 200, A: 255}
	for _, size := range []image.Point{{304, 208}, {368, 272}} {
		rejection, opaque := make([]byte, size.X*size.Y*4), make([]byte, size.X*size.Y*4)
		for y := range size.Y {
			for x := range size.X {
				index := (y*size.X + x) * 4
				if (x+2*y)%5 < 2 {
					rejection[index+3] = 255
				} else {
					copy(opaque[index:index+3], []byte{background.R, background.G, background.B})
				}
				opaque[index+3] = 255
			}
		}
		reference := ebiten.NewImageWithOptions(image.Rectangle{Max: size}, &ebiten.NewImageOptions{Unmanaged: true})
		t.Cleanup(reference.Deallocate)
		reference.WritePixels(opaque)
		var surfaces [2]backend.Surface
		for index := range surfaces {
			surface := &surfaces[index]
			t.Cleanup(surface.Close)
			buffer, err := surface.Resize(size.X, size.Y)
			if err != nil {
				t.Fatal(err)
			}
			copy(buffer, rejection)
			if err := surface.Upload(); err != nil {
				t.Fatal(err)
			}
		}
		for index := range surfaces {
			for _, placement := range []struct{ sx, sy, tx, ty float64 }{{1, 1, 19.25, -13.5}, {1, 1, -9.5, 8.25}, {1.25, 1.75, 0, 0}, {2.5025, 701.0 / 208.0, 7.25, -3.5}} {
				target := ebiten.NewImageWithOptions(image.Rect(0, 0, 400, 208), &ebiten.NewImageOptions{Unmanaged: true})
				target.Fill(background)
				expected := ebiten.NewImageWithOptions(image.Rect(0, 0, 400, 208), &ebiten.NewImageOptions{Unmanaged: true})
				expected.Fill(background)
				clip := image.Rect(4, 6, 396, 202)
				var options ebiten.DrawImageOptions
				options.GeoM.Scale(placement.sx, placement.sy)
				options.GeoM.Translate(placement.tx, placement.ty)
				if err := surfaces[index].Draw(target.SubImage(clip).(*ebiten.Image), options); err != nil {
					t.Fatal(err)
				}
				expected.SubImage(clip).(*ebiten.Image).DrawImage(reference, &options)
				actual, want := pixels(target), pixels(expected)
				target.Deallocate()
				expected.Deallocate()
				if !bytes.Equal(actual, want) {
					t.Fatalf("fractional rejection differs from opaque oracle: %v %+v", size, placement)
				}
			}
		}
	}
}
