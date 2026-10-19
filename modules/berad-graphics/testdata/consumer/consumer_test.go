package consumer_test

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"testing"

	graphics "github.com/bradhannah/berad-sdk/modules/berad-graphics"
	backend "github.com/bradhannah/berad-sdk/modules/berad-graphics/backend/ebitengine"
	"github.com/hajimehoshi/ebiten/v2"
)

type host struct {
	tests *testing.M
	done  chan int
	code  int
}

func (h *host) Update() error {
	if h.done == nil {
		h.done = make(chan int, 1)
		go func() { h.done <- h.tests.Run() }()
	}
	select {
	case h.code = <-h.done:
		return ebiten.Termination
	default:
		return nil
	}
}
func (*host) Draw(*ebiten.Image)         {}
func (*host) Layout(int, int) (int, int) { return 1, 1 }
func TestMain(tests *testing.M) {
	h := &host{tests: tests, code: 1}
	if err := ebiten.RunGame(h); err != nil {
		panic(err)
	}
	os.Exit(h.code)
}

func TestIndependentDeviceOwnership(t *testing.T) {
	source := ebiten.NewImage(7, 23)
	defer source.Deallocate()
	source.Fill(color.White)
	target := ebiten.NewImage(32, 32)
	defer target.Deallocate()
	first, err := backend.NewProcessor(backend.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.NewProcessor(backend.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	a, err := first.Derive(source, graphics.RGBGain(.5, graphics.Selection{}))
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.Derive(source, graphics.RGBGain(.5, graphics.Selection{}))
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	for _, result := range []*ebiten.Image{a, b} {
		target.Clear()
		target.DrawImage(result, nil)
		buffer := make([]byte, 32*32*4)
		target.ReadPixels(buffer)
		for y := range 32 {
			for x := range 32 {
				want := []byte{0, 0, 0, 0}
				if x < 7 && y < 23 {
					want = []byte{127, 127, 127, 255}
				}
				if !bytes.Equal(buffer[(y*32+x)*4:][:4], want) {
					t.Fatalf("owner closure affected independent result at %d,%d", x, y)
				}
			}
		}
	}
}

func TestIndependentPixelsAndOwnedNativePadding(t *testing.T) {
	pixels := make([]byte, 7*23*4)
	copy(pixels[(10*7+3)*4:], []byte{90, 20, 0, 128})
	frozen, err := graphics.NewPixels(7, 23, pixels)
	if err != nil {
		t.Fatal(err)
	}
	animated, err := frozen.Apply(graphics.ScrollY(-27))
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, len(pixels))
	copy(want[(6*7+3)*4:], []byte{90, 20, 0, 128})
	if !bytes.Equal(animated.Pixels(), want) || !bytes.Equal(frozen.Pixels(), pixels) {
		t.Fatal("scroll pixels or immutable source")
	}
	source := ebiten.NewImageFromImage(animated.CopyRGBA())
	defer source.Deallocate()
	owned, err := backend.CopyRegion(source, source.Bounds(), graphics.Insets{Left: 2, Top: 1, Right: 3, Bottom: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Deallocate()
	if owned.Bounds() != image.Rect(0, 0, 12, 25) {
		t.Fatal("native padded geometry")
	}
	actual, expected := make([]byte, 12*25*4), make([]byte, 12*25*4)
	copy(expected[(7*12+5)*4:], []byte{90, 20, 0, 128})
	source.Clear()
	owned.ReadPixels(actual)
	if !bytes.Equal(actual, expected) {
		t.Fatal("native copy did not preserve owned pixels")
	}
}
