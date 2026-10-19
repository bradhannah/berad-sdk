package text

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"image"
	"image/draw"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/text/language"
)

const maxSourceBytes = 32 << 20

// Source is immutable parsed font data or a detached bitmap atlas. Sources are
// Go-owned values: retain them while needed; there is no hot-reload/global registry.
// Engine-owned glyph caches follow the lifetime of the parsed TTF source.
type Source struct {
	ttf     *etext.GoTextFaceSource
	bitmap  *image.RGBA
	metrics Metrics
	glyphs  map[rune]Glyph
	key     [32]byte
}

// Metrics are baseline distances in source pixels. Ascent and descent are
// nonnegative; the baseline lies Ascent pixels below the line's top.
type Metrics struct{ Ascent, Descent, LineGap float64 }

// Glyph describes a bitmap region in the supplied image's coordinate system.
// BearingY is the top edge relative to the baseline (usually negative Ascent).
// Empty regions with a positive advance represent spaces.
type Glyph struct {
	Region                      image.Rectangle
	Advance, BearingX, BearingY float64
	// Colored preserves source RGB and applies only paint alpha (e.g. inline icons).
	Colored bool
}

// Feature and Variation select OpenType settings without exposing mutable faces.
type Feature struct {
	Tag   string
	Value uint32
}
type Variation struct {
	Tag   string
	Value float32
}

// FontOptions describe an immutable sized instance. Size is in pixels. The zero
// Language uses the engine default; text layout is horizontal. Feature/variation
// slices are copied and canonicalized; duplicate tags are rejected.
type FontOptions struct {
	Size       float64
	Language   language.Tag
	Features   []Feature
	Variations []Variation
}

// Font is an immutable sized font. Equivalent source/settings have identical
// rendering identity, even when independently constructed (including scaled text).
type Font struct {
	source  *Source
	options FontOptions
	face    *etext.GoTextFace
	metrics Metrics
	key     [32]byte
}

// ParseFont copies and parses supplied TTF/OTF bytes. No file is opened or closed.
func ParseFont(data []byte) (*Source, error) {
	if len(data) == 0 || len(data) > maxSourceBytes {
		return nil, errors.New("text: font data must contain 1..32 MiB bytes")
	}
	owned := bytes.Clone(data)
	parsed, err := etext.NewGoTextFaceSource(bytes.NewReader(owned))
	if err != nil {
		return nil, fmt.Errorf("text: parse font: %w", err)
	}
	return &Source{ttf: parsed, key: sha256.Sum256(owned)}, nil
}

// NewBitmapSource snapshots an atlas and glyph descriptors. It accepts arbitrary
// regions and nonzero image origins. Missing glyphs return errors during layout;
// consumers explicitly supply their desired replacement glyph in content.
func NewBitmapSource(atlas image.Image, metrics Metrics, glyphs map[rune]Glyph) (*Source, error) {
	if atlas == nil || !validMetrics(metrics) || len(glyphs) == 0 || len(glyphs) > 65536 {
		return nil, errors.New("text: invalid bitmap source or metrics")
	}
	bounds := atlas.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || width > 8192 || height > 8192 || int64(width)*int64(height)*4 > maxSourceBytes {
		return nil, errors.New("text: bitmap source exceeds dimension/byte limit")
	}
	s := &Source{metrics: metrics, glyphs: make(map[rune]Glyph, len(glyphs))}
	keys := make([]rune, 0, len(glyphs))
	for r, g := range glyphs {
		if !utf8.ValidRune(r) || !finite(g.Advance) || g.Advance < 0 || !finite(g.BearingX) || !finite(g.BearingY) ||
			(!g.Region.Empty() && !g.Region.In(bounds)) {
			return nil, fmt.Errorf("text: invalid bitmap glyph %U", r)
		}
		if !g.Region.Empty() {
			g.Region = g.Region.Sub(bounds.Min)
		}
		s.glyphs[r] = g
		keys = append(keys, r)
	}
	s.bitmap = image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(s.bitmap, s.bitmap.Bounds(), atlas, bounds.Min, draw.Src)
	h := sha256.New()
	h.Write(s.bitmap.Pix)
	writeNumbers(h, float64(width), float64(height), metrics.Ascent, metrics.Descent, metrics.LineGap)
	slices.Sort(keys)
	for _, r := range keys {
		g := s.glyphs[r]
		writeNumbers(h, float64(r), float64(g.Region.Min.X), float64(g.Region.Min.Y),
			float64(g.Region.Max.X), float64(g.Region.Max.Y), g.Advance, g.BearingX, g.BearingY)
		if g.Colored {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
	}
	copy(s.key[:], h.Sum(nil))
	return s, nil
}

// NewFont creates an immutable instance; no renderer or graphics device is needed
// for the SDK's source/metric operations beyond Ebitengine's platform initialization.
func (s *Source) NewFont(options FontOptions) (*Font, error) {
	if s == nil || (s.ttf == nil && s.bitmap == nil) || !finite(options.Size) || options.Size <= 0 || options.Size > 4096 {
		return nil, errors.New("text: font requires a source and finite size in (0,4096]")
	}
	options.Features = slices.Clone(options.Features)
	options.Variations = slices.Clone(options.Variations)
	slices.SortFunc(options.Features, func(a, b Feature) int { return bytes.Compare([]byte(a.Tag), []byte(b.Tag)) })
	slices.SortFunc(options.Variations, func(a, b Variation) int { return bytes.Compare([]byte(a.Tag), []byte(b.Tag)) })
	f := &Font{source: s, options: options}
	h := sha256.New()
	h.Write(s.key[:])
	writeNumbers(h, options.Size)
	h.Write([]byte(options.Language.String()))
	if s.ttf != nil {
		f.face = &etext.GoTextFace{Source: s.ttf, Size: options.Size, Language: options.Language}
	}
	for i, v := range options.Features {
		tag, err := etext.ParseTag(v.Tag)
		if err != nil || (i > 0 && options.Features[i-1].Tag == v.Tag) || f.face == nil {
			return nil, errors.New("text: invalid/duplicate feature or non-TTF font")
		}
		f.face.SetFeature(tag, v.Value)
		h.Write([]byte("F" + v.Tag))
		writeNumbers(h, float64(v.Value))
	}
	for i, v := range options.Variations {
		tag, err := etext.ParseTag(v.Tag)
		if err != nil || !finite(float64(v.Value)) || (i > 0 && options.Variations[i-1].Tag == v.Tag) || f.face == nil {
			return nil, errors.New("text: invalid/duplicate variation or non-TTF font")
		}
		f.face.SetVariation(tag, v.Value)
		h.Write([]byte("V" + v.Tag))
		writeNumbers(h, float64(v.Value))
	}
	copy(f.key[:], h.Sum(nil))
	if f.face != nil {
		m := f.face.Metrics()
		f.metrics = Metrics{m.HAscent, m.HDescent, m.HLineGap}
	} else {
		scale := options.Size / (s.metrics.Ascent + s.metrics.Descent)
		f.metrics = Metrics{s.metrics.Ascent * scale, s.metrics.Descent * scale, s.metrics.LineGap * scale}
	}
	return f, nil
}

// WithSize returns another immutable configuration sharing the source.
func (f *Font) WithSize(size float64) (*Font, error) {
	if f == nil {
		return nil, errors.New("text: nil font")
	}
	options := f.options
	options.Size = size
	return f.source.NewFont(options)
}
func (f *Font) Size() float64    { return f.options.Size }
func (f *Font) Metrics() Metrics { return f.metrics }

// Identity is a content/settings identity, not a process address or mutable face.
func (f *Font) Identity() [32]byte { return f.key }

// Measure returns horizontal advances and natural height without constructing a
// retained layout. Empty text measures zero. Wrapping/caret queries use NewLayout.
func (f *Font) Measure(value string, lineSpacing float64) (float64, float64, error) {
	if f == nil || !utf8.ValidString(value) || len(value) > 1<<20 || !finite(lineSpacing) || lineSpacing < 0 {
		return 0, 0, errors.New("text: invalid measurement input")
	}
	if value == "" {
		return 0, 0, nil
	}
	if f.face != nil {
		w, h := etext.Measure(value, f.face, lineSpacing)
		if !finite(w) || !finite(h) {
			return 0, 0, errors.New("text: nonfinite measurement")
		}
		return w, h, nil
	}
	width, count := 0.0, 0
	for _, line := range strings.Split(value, "\n") {
		advance := 0.0
		for _, ch := range strings.TrimSuffix(line, "\r") {
			glyph, ok := f.source.glyphs[ch]
			if !ok {
				return 0, 0, fmt.Errorf("text: bitmap font has no glyph %U", ch)
			}
			advance += glyph.Advance * f.options.Size / (f.source.metrics.Ascent + f.source.metrics.Descent)
		}
		width = max(width, advance)
		count++
	}
	if !finite(width) {
		return 0, 0, errors.New("text: nonfinite bitmap advance")
	}
	return width, f.metrics.Ascent + f.metrics.Descent + float64(count-1)*lineSpacing, nil
}

// CopyEngineFace supplies an independently configurable engine face for host-only
// integration with existing Ebitengine APIs. Changes to its fields/features do not
// change this Font. Its shared Source must remain read-only (no UnsafeInternal).
// Bitmap fonts return nil. This is not the SDK's rendering identity.
func (f *Font) CopyEngineFace() *etext.GoTextFace {
	if f == nil || f.face == nil {
		return nil
	}
	face := &etext.GoTextFace{Source: f.source.ttf, Size: f.options.Size, Language: f.options.Language}
	for _, v := range f.options.Features {
		face.SetFeature(etext.MustParseTag(v.Tag), v.Value)
	}
	for _, v := range f.options.Variations {
		face.SetVariation(etext.MustParseTag(v.Tag), v.Value)
	}
	return face
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func validMetrics(m Metrics) bool {
	return finite(m.Ascent+m.Descent+m.LineGap) && finite(m.Ascent) && finite(m.Descent) && finite(m.LineGap) && m.Ascent >= 0 && m.Descent >= 0 && m.Ascent+m.Descent > 0 && m.LineGap >= 0
}
func writeNumbers(h hash.Hash, values ...float64) {
	var b [8]byte
	for _, v := range values {
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
		h.Write(b[:])
	}
}
