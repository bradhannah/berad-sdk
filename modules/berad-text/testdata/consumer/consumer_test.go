package consumer_test

import (
	"image"
	"image/color"
	"os"
	"testing"

	text "github.com/bradhannah/berad-sdk/modules/berad-text"
	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font/gofont/goregular"
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
func TestMain(m *testing.M) {
	h := &host{tests: m, code: 1}
	if err := ebiten.RunGame(h); err != nil {
		panic(err)
	}
	os.Exit(h.code)
}

func TestUnrelatedHostUsesPublicAPI(t *testing.T) {
	source, err := text.ParseFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	font, err := source.NewFont(text.FontOptions{Size: 18})
	if err != nil {
		t.Fatal(err)
	}
	atlas := image.NewRGBA(image.Rect(0, 0, 3, 5))
	for y := 0; y < 5; y++ {
		for x := 0; x < 3; x++ {
			atlas.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	icons, err := text.NewBitmapSource(atlas, text.Metrics{Ascent: 5}, map[rune]text.Glyph{'\ufffc': {Region: atlas.Bounds(), Advance: 4, BearingY: -5}})
	if err != nil {
		t.Fatal(err)
	}
	iconFont, err := icons.NewFont(text.FontOptions{Size: 15})
	if err != nil {
		t.Fatal(err)
	}
	content, err := text.NewContent([]text.Span{{Text: "Reactor ", Font: font, Paint: "label"}, {Text: "\ufffc", Font: iconFont, Paint: "warning"}, {Text: " online\n", Font: font, Paint: "label"}})
	if err != nil {
		t.Fatal(err)
	}
	layout, err := text.NewLayout(content, text.LayoutOptions{Width: 100})
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := text.NewRenderer(text.DefaultRendererOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	target := ebiten.NewImage(160, 100)
	defer target.Deallocate()
	palette := text.Palette{"label": color.White, "warning": color.RGBA{R: 255, A: 255}}
	for i := 0; i < 2; i++ {
		if err := renderer.Draw(target, layout, palette, text.DrawOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if renderer.Stats().Hits != 1 {
		t.Fatal("consumer did not reuse raster")
	}
	if _, err := layout.RangeRects(0, len(content.Text())); err != nil {
		t.Fatal(err)
	}
	palette["label"] = color.RGBA{G: 255, A: 255}
	if err := renderer.Draw(target, layout, palette, text.DrawOptions{}); err != nil {
		t.Fatal(err)
	}
	renderer.Close()
	if _, err := layout.CaretAt(0, false); err != nil {
		t.Fatal("CPU query after renderer closure", err)
	}
}
