package consumer_test

import (
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
	a, err := first.Derive(source, graphics.TintRGB(100, 200, 50))
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.Derive(source, graphics.TintRGB(100, 200, 50))
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	target.DrawImage(a, nil)
	target.DrawImage(b, nil)
	buffer := make([]byte, 32*32*4)
	target.ReadPixels(buffer)
	if buffer[0] != 100 || buffer[1] != 200 || buffer[3] != 255 {
		t.Fatal("owner closure affected borrower or another owner")
	}
}

func TestIndependentProceduralSignage(t *testing.T) {
	source := image.NewRGBA(image.Rect(10, 20, 17, 43))
	source.SetRGBA(13, 30, color.RGBA{R: 90, G: 20, A: 128})
	frozen, err := graphics.NewImage(source)
	if err != nil {
		t.Fatal(err)
	}
	framed, err := frozen.Pad(graphics.Insets{Left: 2, Top: 1, Right: 3, Bottom: 1})
	if err != nil {
		t.Fatal(err)
	}
	animated, err := framed.Apply(graphics.Rise(3, graphics.Selection{}), graphics.Wave(1.1, 2.4, .5, graphics.Selection{}), graphics.ScrollY(-27), graphics.Outline(color.RGBA{G: 180, A: 255}, 0))
	if err != nil {
		t.Fatal(err)
	}
	if animated.Bounds() != image.Rect(0, 0, 12, 25) || source.RGBAAt(13, 30).R != 90 {
		t.Fatal("host source/geometry changed")
	}
	frame, err := graphics.NewFrame(animated, image.Pt(19, 31), image.Pt(3, 4), image.Pt(9, 30))
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := graphics.NewAnimation([]graphics.Frame{frame}, 90)
	if err != nil {
		t.Fatal(err)
	}
	if sequence.At(-1000).Anchor() != image.Pt(9, 30) {
		t.Fatal("host anchor lost")
	}
}
