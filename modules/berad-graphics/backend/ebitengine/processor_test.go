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
	processor.Derive(source, graphics.TintRGB(255, 0, 0))
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
	if !bytes.Equal(pixels(a), pixels(b)) {
		t.Fatal("independent additive alpha unexpectedly modulated RGB")
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

func TestRegionCopyNonzeroOriginAndFrameAnchor(t *testing.T) {
	atlas := ebiten.NewImage(40, 30)
	defer atlas.Deallocate()
	atlas.Fill(color.RGBA{R: 80, A: 128})
	copy, err := backend.CopyRegion(atlas, image.Rect(7, 9, 25, 18), graphics.Insets{Left: 2, Top: 3, Right: 1, Bottom: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Deallocate()
	if copy.Bounds() != image.Rect(0, 0, 21, 16) || pixels(copy)[3] != 0 || pixels(copy)[(3*21+2)*4] != 80 {
		t.Fatal("origin/padding")
	}
	processor, _ := backend.NewProcessor(backend.DefaultLimits())
	defer processor.Close()
	snapshot, err := processor.Snapshot(copy)
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := graphics.NewFrame(snapshot, image.Pt(30, 30), image.Pt(5, 6), image.Pt(2, 3))
	target := ebiten.NewImage(40, 40)
	defer target.Deallocate()
	if err := processor.DrawFrame(target, frame, ebiten.DrawImageOptions{}); err != nil {
		t.Fatal(err)
	}
	if pixels(target)[((3+3)*40+3+2)*4] != 80 {
		t.Fatal("logical anchor placement")
	}
	copy.Dispose() // Permanent retirement; Deallocate alone is a reusable engine resource reset.
	if _, err := processor.Snapshot(copy); err == nil {
		t.Fatal("disposed source revived cached snapshot")
	}
}
