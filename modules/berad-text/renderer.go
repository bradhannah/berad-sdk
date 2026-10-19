package text

import (
	"container/list"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Palette resolves inert paint names. The empty name defaults to white when not
// supplied; other missing names are errors. Explicit zero alpha is transparent.
// The host serializes palette mutation with Draw.
type Palette map[string]color.Color

// RendererOptions separately bound retained raster entries/bytes and a single
// render allocation. Accounted bytes are pixels plus retained content bytes,
// not total heap/GPU memory or the engine's source-owned glyph caches.
type RendererOptions struct {
	MaxEntries     int
	MaxCacheBytes  int64
	MaxRenderBytes int64
	MaxDimension   int
}

func DefaultRendererOptions() RendererOptions {
	return RendererOptions{MaxEntries: 512, MaxCacheBytes: 16 << 20, MaxRenderBytes: 64 << 20, MaxDimension: 8192}
}

// DrawOptions exposes deliberate Ebitengine placement/filtering. Clip is in
// destination coordinates; it is intersected with the target's existing bounds.
// Uncached is useful for one-shot content and fresh/cached acceptance comparisons.
type DrawOptions struct {
	GeoM     ebiten.GeoM
	Filter   ebiten.Filter
	Clip     *image.Rectangle
	Uncached bool
}

type CacheStats struct {
	Entries                           int
	Bytes, PeakBytes                  int64
	Hits, Misses, Evictions, Bypasses uint64
}
type rasterEntry struct {
	key     [32]byte
	image   *ebiten.Image
	origin  image.Point
	bytes   int64
	layout  *Layout
	element *list.Element
}

// Renderer owns only its private raster cache. Serialize all of its calls with
// the host's render lane; it starts no goroutines and never closes a host target.
// Fonts/layouts are independently retained immutable values, not renderer handles.
type Renderer struct {
	options RendererOptions
	cache   map[[32]byte]*rasterEntry
	lru     list.List
	stats   CacheStats
	closed  bool
}

var ErrClosed = errors.New("text: renderer is closed")

func NewRenderer(options RendererOptions) (*Renderer, error) {
	if options.MaxEntries <= 0 || options.MaxCacheBytes <= 0 || options.MaxRenderBytes <= 0 || options.MaxDimension <= 0 || options.MaxDimension > 16384 {
		return nil, errors.New("text: invalid renderer limits")
	}
	return &Renderer{options: options, cache: make(map[[32]byte]*rasterEntry)}, nil
}

// Draw renders a retained layout. Selection/background decorations can be drawn
// by the host using RangeRects. A paint boundary inside a shaped cluster uses
// the leading source span's foreground; geometry/coverage reports the cluster.
func (r *Renderer) Draw(target *ebiten.Image, layout *Layout, palette Palette, options DrawOptions) error {
	if r == nil || r.closed {
		return ErrClosed
	}
	if target == nil || layout == nil {
		return errors.New("text: Draw requires target and layout")
	}
	a, b, c, d, tx, ty := options.GeoM.Element(0, 0), options.GeoM.Element(0, 1), options.GeoM.Element(1, 0), options.GeoM.Element(1, 1), options.GeoM.Element(0, 2), options.GeoM.Element(1, 2)
	if !finite(a) || !finite(b) || !finite(c) || !finite(d) || !finite(tx) || !finite(ty) {
		return errors.New("text: nonfinite transform")
	}
	key, err := paintKey(layout, palette)
	if err != nil {
		return err
	}
	entry := r.cache[key]
	if entry != nil && !options.Uncached {
		r.stats.Hits++
		r.lru.MoveToFront(entry.element)
	} else {
		r.stats.Misses++
		entry, err = r.rasterize(layout, palette, key)
		if err != nil {
			return err
		}
		if options.Uncached || entry.bytes > r.options.MaxCacheBytes {
			r.stats.Bypasses++
			defer entry.image.Deallocate()
		} else {
			for len(r.cache) >= r.options.MaxEntries || r.stats.Bytes > r.options.MaxCacheBytes-entry.bytes {
				r.evict()
			}
			entry.element = r.lru.PushFront(entry)
			r.cache[key] = entry
			r.stats.Bytes += entry.bytes
			r.stats.Entries = len(r.cache)
			r.stats.PeakBytes = max(r.stats.PeakBytes, r.stats.Bytes)
		}
	}
	if options.Clip != nil {
		clip := target.Bounds().Intersect(*options.Clip)
		if clip.Empty() {
			return nil
		}
		target = target.SubImage(clip).(*ebiten.Image)
	}
	op := &ebiten.DrawImageOptions{Filter: options.Filter}
	op.GeoM.Translate(float64(entry.origin.X), float64(entry.origin.Y))
	op.GeoM.Concat(options.GeoM)
	target.DrawImage(entry.image, op)
	return nil
}

func resolvePaint(palette Palette, name string) (color.RGBA64, error) {
	c, ok := palette[name]
	if !ok && name == "" {
		return color.RGBA64{R: 65535, G: 65535, B: 65535, A: 65535}, nil
	}
	if !ok || c == nil {
		return color.RGBA64{}, fmt.Errorf("text: missing paint %q", name)
	}
	r, g, b, a := c.RGBA()
	return color.RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)}, nil
}

func paintKey(layout *Layout, palette Palette) ([32]byte, error) {
	h := sha256.New()
	h.Write(layout.key[:])
	for _, s := range layout.content.spans {
		c, err := resolvePaint(palette, s.Paint)
		if err != nil {
			return [32]byte{}, err
		}
		writeNumbers(h, float64(c.R), float64(c.G), float64(c.B), float64(c.A))
	}
	var key [32]byte
	copy(key[:], h.Sum(nil))
	return key, nil
}

func (r *Renderer) rasterize(layout *Layout, palette Palette, key [32]byte) (*rasterEntry, error) {
	ink := layout.ink
	if !finite(ink.Min.X) || !finite(ink.Min.Y) || !finite(ink.Max.X) || !finite(ink.Max.Y) {
		return nil, errors.New("text: nonfinite ink bounds")
	}
	minX, minY, maxX, maxY := math.Floor(ink.Min.X), math.Floor(ink.Min.Y), math.Ceil(ink.Max.X), math.Ceil(ink.Max.Y)
	w, h := max(1, maxX-minX), max(1, maxY-minY)
	if w > float64(r.options.MaxDimension) || h > float64(r.options.MaxDimension) || w*h*4 > float64(r.options.MaxRenderBytes) || math.Abs(minX) > 1<<30 || math.Abs(minY) > 1<<30 {
		return nil, errors.New("text: raster exceeds render limits")
	}
	origin := image.Pt(int(minX), int(minY))
	img := ebiten.NewImageWithOptions(image.Rect(0, 0, int(w), int(h)), &ebiten.NewImageOptions{Unmanaged: true})
	atlases := make(map[*Source]*ebiten.Image)
	defer func() {
		for _, atlas := range atlases {
			atlas.Deallocate()
		}
	}()
	for _, g := range layout.glyphs {
		paint, err := resolvePaint(palette, g.paint)
		if err != nil {
			img.Deallocate()
			return nil, err
		}
		op := &ebiten.DrawImageOptions{}
		var source *ebiten.Image
		colored := false
		if g.isBitmap {
			if g.bitmap.Region.Empty() {
				continue
			}
			atlas := atlases[g.font.source]
			if atlas == nil {
				atlas = ebiten.NewImageFromImage(g.font.source.bitmap)
				atlases[g.font.source] = atlas
			}
			source = atlas.SubImage(g.bitmap.Region).(*ebiten.Image)
			op.GeoM.Scale(g.scale, g.scale)
			op.GeoM.Translate(g.x-minX, g.y-minY)
			op.Filter = ebiten.FilterNearest
			colored = g.bitmap.Colored
		} else {
			source = g.glyph.Image()
			if source == nil {
				continue
			}
			op.GeoM.Translate(g.x+float64(g.glyph.ImageBounds.Min.X)-minX, g.y+float64(g.glyph.ImageBounds.Min.Y)-minY)
			colored = g.glyph.Colored()
		}
		if colored {
			op.ColorScale.ScaleAlpha(float32(paint.A) / 65535)
		} else {
			op.ColorScale.ScaleWithColor(paint)
		}
		img.DrawImage(source, op)
	}
	return &rasterEntry{key: key, image: img, origin: origin, bytes: int64(w)*int64(h)*4 + int64(len(layout.content.text)), layout: layout}, nil
}

func (r *Renderer) evict() {
	e := r.lru.Back()
	if e == nil {
		return
	}
	entry := e.Value.(*rasterEntry)
	r.lru.Remove(e)
	delete(r.cache, entry.key)
	r.stats.Bytes -= entry.bytes
	r.stats.Evictions++
	r.stats.Entries = len(r.cache)
	entry.image.Deallocate()
}
func (r *Renderer) Stats() CacheStats {
	if r == nil {
		return CacheStats{}
	}
	return r.stats
}

// ClearCaches drops owned raster resources, but keeps lifetime counters and peak.
func (r *Renderer) ClearCaches() {
	if r == nil {
		return
	}
	for len(r.cache) > 0 {
		r.evict()
	}
}
func (r *Renderer) Close() {
	if r == nil || r.closed {
		return
	}
	r.ClearCaches()
	r.closed = true
}
