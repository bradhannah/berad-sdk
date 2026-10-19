package ebitengine

import (
	"container/list"
	"fmt"

	graphics "github.com/bradhannah/berad-sdk/modules/berad-graphics"
	"github.com/hajimehoshi/ebiten/v2"
)

// ImageCache retains read-only images under caller-supplied collision-free scope
// keys. The host owns key meaning and serializes calls. Eviction never disposes
// retained borrowers; this cache is distinct from Processor's derived operation keys.
type ImageCache struct {
	entries         map[uint64]*imageEntry
	order           list.List
	maxEntries      int
	maxBytes, bytes int64
}

type imageEntry struct {
	key     uint64
	image   *ebiten.Image
	bytes   int64
	element *list.Element
}

// NewImageCache requires positive entry and logical-pixel byte budgets.
func NewImageCache(entries int, bytes int64) (*ImageCache, error) {
	if entries <= 0 || bytes <= 0 {
		return nil, fmt.Errorf("%w: image cache budgets", graphics.ErrInvalid)
	}
	cache := new(ImageCache)
	cache.entries = make(map[uint64]*imageEntry)
	cache.maxEntries, cache.maxBytes = entries, bytes

	return cache, nil
}

// Get borrows a retained image and promotes it in the host-serialized LRU.
func (cache *ImageCache) Get(key uint64) *ebiten.Image {
	entry := cache.entries[key]
	if entry == nil {
		return nil
	}
	cache.order.MoveToFront(entry.element)

	return entry.image
}

// Put admits a valid image or bypasses retention when it exceeds the byte budget.
// The caller must not mutate/deallocate admitted images while borrowers retain them.
func (cache *ImageCache) Put(key uint64, source *ebiten.Image) error {
	if cache == nil || cache.entries == nil {
		return ErrClosed
	}
	bounds, err := Bounds(source)
	if err != nil {
		return err
	}
	const channels = 4
	width, height := int64(bounds.Dx()), int64(bounds.Dy())
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: empty cache image", graphics.ErrInvalid)
	}
	if prior := cache.entries[key]; prior != nil {
		cache.remove(prior)
	}
	if width > cache.maxBytes/channels/height {
		return nil
	}
	payload := width * height * channels
	for len(cache.entries) >= cache.maxEntries || cache.bytes > cache.maxBytes-payload {
		entry, ok := cache.order.Back().Value.(*imageEntry)
		if !ok {
			panic("image cache contains foreign entry")
		}
		cache.remove(entry)
	}
	entry := &imageEntry{key: key, image: source, bytes: payload, element: nil}
	entry.element = cache.order.PushFront(entry)
	cache.entries[key] = entry
	cache.bytes += payload

	return nil
}

func (cache *ImageCache) remove(entry *imageEntry) {
	delete(cache.entries, entry.key)
	cache.order.Remove(entry.element)
	cache.bytes -= entry.bytes
}

// Size reports retained entries without changing LRU order.
func (cache *ImageCache) Size() int { return len(cache.entries) }

// Bytes reports logical cached pixel payload, not backend texture/driver allocation.
func (cache *ImageCache) Bytes() int64 { return cache.bytes }

// Clear drops cache references without invalidating returned borrowed images.
func (cache *ImageCache) Clear() { clear(cache.entries); cache.order.Init(); cache.bytes = 0 }
