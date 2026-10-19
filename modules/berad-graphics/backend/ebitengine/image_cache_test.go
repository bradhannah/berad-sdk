package ebitengine_test

import (
	"image/color"
	"testing"

	graphics "github.com/bradhannah/berad-sdk/modules/berad-graphics"
	backend "github.com/bradhannah/berad-sdk/modules/berad-graphics/backend/ebitengine"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestHostKeyCacheReplacementBypassAndBorrowLifetime(t *testing.T) {
	cache, err := backend.NewImageCache(2, 32)
	if err != nil {
		t.Fatal(err)
	}
	first, second, large := ebiten.NewImage(2, 2), ebiten.NewImage(2, 2), ebiten.NewImage(4, 4)
	defer first.Deallocate()
	defer second.Deallocate()
	defer large.Deallocate()
	first.Fill(color.White)
	if err := cache.Put(1, first); err != nil {
		t.Fatal(err)
	}
	if err := cache.Put(2, second); err != nil {
		t.Fatal(err)
	}
	if cache.Size() != 2 || cache.Bytes() != 32 || cache.Get(1) != first {
		t.Fatal("cache accounting")
	}
	if err := cache.Put(1, large); err != nil {
		t.Fatal(err)
	}
	if cache.Get(1) != nil || cache.Get(2) != second || cache.Bytes() != 16 {
		t.Fatal("oversized replacement left stale entry or evicted unrelated image")
	}
	if err := cache.Put(2, nil); err == nil || cache.Get(2) != second {
		t.Fatal("invalid replacement silently admitted")
	}
	cache.Clear()
	if pixels(first)[0] != 255 {
		t.Fatal("cache clear invalidated borrowed image")
	}
	processor, err := backend.NewProcessor(backend.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer processor.Close()
	padding := graphics.Insets{Left: 1, Top: 1, Right: 1, Bottom: 1}
	a, err := processor.PadAndDerive(first, padding, graphics.Outline(color.RGBA{A: 255}, 0))
	if err != nil {
		t.Fatal(err)
	}
	b, err := processor.PadAndDerive(first, padding, graphics.Outline(color.RGBA{A: 255}, 0))
	if err != nil || a != b {
		t.Fatal("padding identity not cached", err)
	}
	if a.Bounds().Dx() != 4 || pixels(a)[3] != 255 {
		t.Fatal("outline did not use padded source")
	}
}
