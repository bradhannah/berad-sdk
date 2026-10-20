package ebitengine_test

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

type harness struct {
	tests *testing.M
	done  chan int
	code  int
}

func (h *harness) Update() error {
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
func (*harness) Draw(*ebiten.Image)         {}
func (*harness) Layout(int, int) (int, int) { return 1, 1 }
func TestMain(tests *testing.M) {
	h := &harness{tests: tests, code: 1}
	if err := ebiten.RunGame(h); err != nil {
		panic(err)
	}
	os.Exit(h.code)
}

func pixels(source *ebiten.Image) []byte {
	result := make([]byte, source.Bounds().Dx()*source.Bounds().Dy()*4)
	source.ReadPixels(result)
	return result
}

func TestCacheExactParametersInvalidationBorrowLifetimeAndLimits(t *testing.T) {
	source := ebiten.NewImage(1, 1)
	defer source.Deallocate()
	source.Fill(color.White)
	limits := backend.DefaultLimits()
	limits.MaxEntries = 2
	processor, err := backend.NewProcessor(limits)
	if err != nil {
		t.Fatal(err)
	}
	a, err := processor.Derive(source, graphics.RGBGain(.50195, graphics.Selection{}))
	if err != nil {
		t.Fatal(err)
	}
	b, err := processor.Derive(source, graphics.RGBGain(.50199, graphics.Selection{}))
	if err != nil {
		t.Fatal(err)
	}
	if pixels(a)[0] != 127 || pixels(b)[0] != 128 {
		t.Fatal("parameter quantization changed pixels", pixels(a), pixels(b))
	}
	again, err := processor.Derive(source, graphics.RGBGain(.50195, graphics.Selection{}))
	if err != nil || again != a {
		t.Fatal("cache miss", err)
	}
	if _, err := processor.Derive(source, graphics.Operation{}); err == nil {
		t.Fatal("invalid operation accepted")
	}
	if _, err := processor.Derive(source, graphics.RGBGain(.25, graphics.Selection{})); err != nil {
		t.Fatal(err)
	}
	if processor.Stats().Derived > 2 {
		t.Fatal("unbounded cache")
	}
	source.Fill(color.Black)
	processor.Forget(source)
	updated, err := processor.Derive(source)
	if err != nil {
		t.Fatal(err)
	}
	if pixels(updated)[0] != 0 || pixels(a)[0] != 127 {
		t.Fatal("explicit replacement mutated old borrower")
	}
	processor.Close()
	processor.Close()
	if pixels(b)[0] != 128 {
		t.Fatal("close invalidated borrower")
	}
	if _, err := processor.Derive(source); err != backend.ErrClosed {
		t.Fatal("post-close", err)
	}
}

func TestAdditiveAlphaPulsePreservesRGBAndSurfaceTargets(t *testing.T) {
	source := ebiten.NewImage(3, 5)
	defer source.Deallocate()
	source.Fill(color.White)
	a, b := ebiten.NewImage(3, 5), ebiten.NewImage(3, 5)
	defer a.Deallocate()
	defer b.Deallocate()
	a.Fill(color.Black)
	b.Fill(color.Black)
	options := ebiten.DrawImageOptions{Blend: ebiten.BlendLighter}
	if err := backend.DrawAdditiveChannels(a, source, 40, 70, 90, .2, options); err != nil {
		t.Fatal(err)
	}
	if err := backend.DrawAdditiveChannels(b, source, 40, 70, 90, .8, options); err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte{40, 70, 90, 255}, 15)
	if !bytes.Equal(pixels(a), want) || !bytes.Equal(pixels(b), want) {
		t.Fatal("additive RGB must match supplied channels at both alpha factors", pixels(a), pixels(b))
	}
	var surface backend.Surface
	buffer, err := surface.Resize(3, 5)
	if err != nil {
		t.Fatal(err)
	}
	buffer[3] = 255
	if err := surface.Upload(); err != nil {
		t.Fatal(err)
	}
	if err := surface.Draw(a, ebiten.DrawImageOptions{}); err != nil {
		t.Fatal(err)
	}
	again, err := surface.Resize(3, 5)
	if err != nil || &again[0] != &buffer[0] || again[3] != 0 {
		t.Fatal("surface reuse/reset", err)
	}
	surface.Close()
	surface.Close()
	a.Fill(color.White)
	if pixels(a)[3] != 255 {
		t.Fatal("target disposed")
	}
	if _, err := surface.Resize(1, 1); err != backend.ErrClosed {
		t.Fatal("surface revived", err)
	}
}

func TestRegionCopyNonzeroOriginPaddingAndOwnership(t *testing.T) {
	atlas := ebiten.NewImage(9, 8)
	defer atlas.Deallocate()
	payload := make([]byte, 9*8*4)
	for y := range 8 {
		for x := range 9 {
			copy(payload[(y*9+x)*4:], []byte{byte(x * 10), byte(y * 10), 20, 128})
		}
	}
	atlas.WritePixels(payload)
	source := atlas.SubImage(image.Rect(2, 1, 8, 7)).(*ebiten.Image)
	owned, err := backend.CopyRegion(source, image.Rect(3, 2, 6, 4), graphics.Insets{Left: 2, Top: 1, Right: 1, Bottom: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Deallocate()
	want := make([]byte, 6*6*4)
	for y := range 2 {
		for x := range 3 {
			copy(want[((y+1)*6+x+2)*4:], []byte{byte((x + 3) * 10), byte((y + 2) * 10), 20, 128})
		}
	}
	if owned.Bounds() != image.Rect(0, 0, 6, 6) || !bytes.Equal(pixels(owned), want) {
		t.Fatal("wrong source region, zero origin, channels or asymmetric padding", pixels(owned))
	}
	atlas.Clear()
	if !bytes.Equal(pixels(owned), want) {
		t.Fatal("owned copy aliases atlas")
	}
	owned.Clear()
	if !bytes.Equal(pixels(atlas), make([]byte, len(payload))) {
		t.Fatal("owned copy mutated atlas")
	}
	processor, err := backend.NewProcessor(backend.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer processor.Close()
	if _, err := processor.Derive(owned); err != nil {
		t.Fatal(err)
	}
	owned.Dispose() // Permanent retirement; Deallocate alone is a reusable engine resource reset.
	if _, err := processor.Derive(owned); err == nil {
		t.Fatal("disposed source revived cached snapshot")
	}
}

func TestAdditiveChannelsTransparentTargetAndPartialSource(t *testing.T) {
	source, target := ebiten.NewImage(1, 1), ebiten.NewImage(3, 1)
	defer source.Deallocate()
	defer target.Deallocate()
	source.Fill(color.White)
	for _, sample := range []struct {
		alpha float32
		want  byte
	}{{.2, 51}, {.8, 204}} {
		target.Clear()
		options := ebiten.DrawImageOptions{Blend: ebiten.BlendLighter}
		options.GeoM.Translate(1, 0)
		options.ColorScale.Scale(0, 0, 0, 0) // Explicit channel drawing replaces this scale.
		if err := backend.DrawAdditiveChannels(target, source, 40, 70, 90, sample.alpha, options); err != nil {
			t.Fatal(err)
		}
		want := []byte{0, 0, 0, 0, 40, 70, 90, sample.want, 0, 0, 0, 0}
		if !bytes.Equal(pixels(target), want) {
			t.Fatalf("alpha %v: %v != %v", sample.alpha, pixels(target), want)
		}
		if options.ColorScale.R() != 0 || options.GeoM.Element(0, 2) != 1 {
			t.Fatal("caller options mutated")
		}
	}
	source.WritePixels([]byte{128, 64, 32, 128})
	target.Clear()
	if err := backend.DrawAdditiveChannels(target, source, 255, 255, 255, .5, ebiten.DrawImageOptions{Blend: ebiten.BlendLighter}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pixels(target), []byte{128, 64, 32, 64, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("partial source channels were treated as ordinary opacity", pixels(target))
	}
}
