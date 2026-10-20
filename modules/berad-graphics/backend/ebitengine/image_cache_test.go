package ebitengine_test

import (
	"image/color"
	"testing"

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
}

func TestHostKeyCacheLRUPromotionAndEvictedBorrow(t *testing.T) {
	cache, err := backend.NewImageCache(2, 32)
	if err != nil {
		t.Fatal(err)
	}
	images := []*ebiten.Image{ebiten.NewImage(2, 2), ebiten.NewImage(2, 2), ebiten.NewImage(2, 2)}
	for _, source := range images {
		t.Cleanup(source.Deallocate)
		source.Fill(color.White)
	}
	for index := range 2 {
		if err := cache.Put(uint64(index), images[index]); err != nil {
			t.Fatal(err)
		}
	}
	cache.Get(0)
	if err := cache.Put(2, images[2]); err != nil {
		t.Fatal(err)
	}
	if cache.Get(0) != images[0] || cache.Get(1) != nil || cache.Get(2) != images[2] || cache.Bytes() != 32 {
		t.Fatal("LRU promotion/eviction")
	}
	if pixels(images[1])[0] != 255 {
		t.Fatal("eviction invalidated a borrow")
	}
}
