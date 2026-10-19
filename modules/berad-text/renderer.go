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

const (
	defaultCacheEntries = 512
	defaultCacheBytes   = 16 << 20
	defaultRenderBytes  = 64 << 20
	defaultDimension    = 8192
	transformColumns    = 3
)

// ErrClosed reports drawing after terminal renderer closure.
var ErrClosed = errors.New("text: renderer is closed")

var (
	errRendererLimits = errors.New("text: invalid renderer limits")
	errDrawInput      = errors.New("text: Draw requires target and layout")
	errTransform      = errors.New("text: nonfinite transform")
	errPaint          = errors.New("text: missing paint")
	errInkBounds      = errors.New("text: nonfinite ink bounds")
	errRenderLimit    = errors.New("text: raster exceeds render limits")
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

// DefaultRendererOptions supplies explicit cache and single-allocation limits.
func DefaultRendererOptions() RendererOptions {
	return RendererOptions{MaxEntries: defaultCacheEntries, MaxCacheBytes: defaultCacheBytes,
		MaxRenderBytes: defaultRenderBytes, MaxDimension: defaultDimension}
}

// DrawOptions exposes deliberate Ebitengine placement/filtering. Clip is in
// destination coordinates; it is intersected with the target's existing bounds.
// Uncached is useful for one-shot content and fresh/cached acceptance comparisons.
type DrawOptions struct {
	GeoM   ebiten.GeoM
	Filter ebiten.Filter
	Clip   *image.Rectangle
	// LocalClip clips layout pixels before placement without changing geometry.
	LocalClip *image.Rectangle
	Uncached  bool
}

// CacheStats is detached lifetime accounting, not total heap or GPU memory.
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

// NewRenderer constructs a host-serialized renderer with positive resource limits.
func NewRenderer(options RendererOptions) (*Renderer, error) {
	if options.MaxEntries <= 0 || options.MaxCacheBytes <= 0 || options.MaxRenderBytes <= 0 ||
		options.MaxDimension <= 0 || options.MaxDimension > 16384 {
		return nil, errRendererLimits
	}
	renderer := new(Renderer)
	renderer.options, renderer.cache = options, make(map[[32]byte]*rasterEntry)

	return renderer, nil
}

// Draw renders a retained layout. Selection/background decorations can be drawn
// by the host using RangeRects. A paint boundary inside a shaped cluster uses
// the leading source span's foreground; geometry/coverage reports the cluster.
func (r *Renderer) Draw(target *ebiten.Image, layout *Layout, palette Palette, options DrawOptions) error {
	if r == nil || r.closed {
		return ErrClosed
	}
	if target == nil || layout == nil {
		return errDrawInput
	}
	if !validTransform(options.GeoM) {
		return errTransform
	}
	entry, temporary, err := r.preparedRaster(layout, palette, options.Uncached)
	if err != nil {
		return err
	}
	if temporary {
		defer entry.image.Deallocate()
	}
	if options.Clip != nil {
		clip := target.Bounds().Intersect(*options.Clip)
		if clip.Empty() {
			return nil
		}
		target = subImage(target, clip)
	}
	source, origin := entry.image, entry.origin
	if options.LocalClip != nil {
		clip := options.LocalClip.Sub(origin).Intersect(source.Bounds())
		if clip.Empty() {
			return nil
		}
		source = subImage(source, clip)
		origin = origin.Add(clip.Min)
	}
	op := &ebiten.DrawImageOptions{Filter: options.Filter}
	op.GeoM.Translate(float64(origin.X), float64(origin.Y))
	op.GeoM.Concat(options.GeoM)
	target.DrawImage(source, op)

	return nil
}

func validTransform(transform ebiten.GeoM) bool {
	for row := range 2 {
		for column := range transformColumns {
			if !finite(transform.Element(row, column)) {
				return false
			}
		}
	}

	return true
}

func subImage(source *ebiten.Image, region image.Rectangle) *ebiten.Image {
	result, ok := source.SubImage(region).(*ebiten.Image)
	if !ok {
		panic("Ebitengine SubImage returned a non-Ebitengine image")
	}

	return result
}

func (r *Renderer) preparedRaster(layout *Layout, palette Palette, uncached bool) (*rasterEntry, bool, error) {
	key, err := paintKey(layout, palette)
	if err != nil {
		return nil, false, err
	}
	if entry := r.cache[key]; entry != nil && !uncached {
		r.stats.Hits++
		r.lru.MoveToFront(entry.element)

		return entry, false, nil
	}
	r.stats.Misses++
	entry, err := r.rasterize(layout, palette, key)
	if err != nil {
		return nil, false, err
	}
	if uncached || entry.bytes > r.options.MaxCacheBytes {
		r.stats.Bypasses++

		return entry, true, nil
	}
	for len(r.cache) >= r.options.MaxEntries || r.stats.Bytes > r.options.MaxCacheBytes-entry.bytes {
		r.evict()
	}
	entry.element = r.lru.PushFront(entry)
	r.cache[key] = entry
	r.stats.Bytes += entry.bytes
	r.stats.Entries = len(r.cache)
	r.stats.PeakBytes = max(r.stats.PeakBytes, r.stats.Bytes)

	return entry, false, nil
}

func resolvePaint(palette Palette, name string) (color.RGBA64, error) {
	paint, exists := palette[name]
	if !exists && name == "" {
		return color.RGBA64{R: math.MaxUint16, G: math.MaxUint16, B: math.MaxUint16, A: math.MaxUint16}, nil
	}
	if !exists || paint == nil {
		return color.RGBA64{}, fmt.Errorf("%w %q", errPaint, name)
	}
	converted, ok := color.RGBA64Model.Convert(paint).(color.RGBA64)
	if !ok {
		panic("RGBA64 model returned a different colour type")
	}

	return converted, nil
}

func paintKey(layout *Layout, palette Palette) ([32]byte, error) {
	digest := sha256.New()
	digest.Write(layout.key[:])
	for _, s := range layout.content.spans {
		c, err := resolvePaint(palette, s.Paint)
		if err != nil {
			return [32]byte{}, err
		}
		writeNumbers(digest, float64(c.R), float64(c.G), float64(c.B), float64(c.A))
	}
	var key [32]byte
	copy(key[:], digest.Sum(nil))

	return key, nil
}

func (r *Renderer) rasterize(layout *Layout, palette Palette, key [32]byte) (*rasterEntry, error) {
	ink := layout.ink
	if !finite(ink.Min.X) || !finite(ink.Min.Y) || !finite(ink.Max.X) || !finite(ink.Max.Y) {
		return nil, errInkBounds
	}
	minX, minY, maxX, maxY := math.Floor(ink.Min.X), math.Floor(ink.Min.Y), math.Ceil(ink.Max.X), math.Ceil(ink.Max.Y)
	width, height := max(1, maxX-minX), max(1, maxY-minY)
	if width > float64(r.options.MaxDimension) || height > float64(r.options.MaxDimension) ||
		width*height*4 > float64(r.options.MaxRenderBytes) || math.Abs(minX) > 1<<30 || math.Abs(minY) > 1<<30 {
		return nil, errRenderLimit
	}
	origin := image.Pt(int(minX), int(minY))
	img := ebiten.NewImageWithOptions(image.Rect(0, 0, int(width), int(height)), &ebiten.NewImageOptions{Unmanaged: true})
	atlases := make(map[*Source]*ebiten.Image)
	defer func() {
		for _, atlas := range atlases {
			atlas.Deallocate()
		}
	}()
	for _, glyph := range layout.glyphs {
		paint, err := resolvePaint(palette, glyph.paint)
		if err != nil {
			img.Deallocate()

			return nil, err
		}
		drawGlyph(img, glyph, atlases, Point{minX, minY}, paint)
	}

	return &rasterEntry{key: key, image: img, origin: origin,
		bytes: int64(width)*int64(height)*4 + int64(len(layout.content.text)), layout: layout, element: nil}, nil
}

func drawGlyph(
	target *ebiten.Image, glyph placedGlyph, atlases map[*Source]*ebiten.Image, origin Point, paint color.RGBA64,
) {
	options := new(ebiten.DrawImageOptions)
	var source *ebiten.Image
	var colored bool
	if glyph.isBitmap {
		if glyph.bitmap.Region.Empty() {
			return
		}
		atlas := atlases[glyph.font.source]
		if atlas == nil {
			atlas = ebiten.NewImageFromImage(glyph.font.source.bitmap)
			atlases[glyph.font.source] = atlas
		}
		source = subImage(atlas, glyph.bitmap.Region)
		options.GeoM.Scale(glyph.scale, glyph.scale)
		options.GeoM.Translate(glyph.x-origin.X, glyph.y-origin.Y)
		options.Filter = ebiten.FilterNearest
		colored = glyph.bitmap.Colored
	} else {
		source = glyph.glyph.Image()
		if source == nil {
			return
		}
		options.GeoM.Translate(glyph.x+float64(glyph.glyph.ImageBounds.Min.X)-origin.X,
			glyph.y+float64(glyph.glyph.ImageBounds.Min.Y)-origin.Y)
		colored = glyph.glyph.Colored()
	}
	if colored {
		options.ColorScale.ScaleAlpha(float32(paint.A) / math.MaxUint16)
	} else {
		options.ColorScale.ScaleWithColor(paint)
	}
	target.DrawImage(source, options)
}

func (r *Renderer) evict() {
	element := r.lru.Back()
	if element == nil {
		return
	}
	entry, ok := element.Value.(*rasterEntry)
	if !ok {
		panic("text raster LRU contains a foreign entry")
	}
	r.lru.Remove(element)
	delete(r.cache, entry.key)
	r.stats.Bytes -= entry.bytes
	r.stats.Evictions++
	r.stats.Entries = len(r.cache)
	entry.image.Deallocate()
}

// Stats returns detached observational lifetime counters.
func (r *Renderer) Stats() CacheStats {
	if r == nil {
		var empty CacheStats

		return empty
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

// Close releases private rasters and permanently disables drawing; repeated calls are harmless.
func (r *Renderer) Close() {
	if r == nil || r.closed {
		return
	}
	r.ClearCaches()
	r.closed = true
}
