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

var (
	errFontData      = errors.New("text: font data must contain 1..32 MiB bytes")
	errBitmapSource  = errors.New("text: invalid bitmap source or metrics")
	errBitmapLimit   = errors.New("text: bitmap source exceeds dimension/byte limit")
	errBitmapGlyph   = errors.New("text: invalid bitmap glyph")
	errMissingGlyph  = errors.New("text: bitmap font has no glyph")
	errFontSize      = errors.New("text: font requires a source and finite size in (0,4096]")
	errFontMetrics   = errors.New("text: scaled font metrics must be finite and nonnegative")
	errNilFont       = errors.New("text: nil font")
	errMeasurement   = errors.New("text: invalid measurement input")
	errMeasureExtent = errors.New("text: nonfinite measurement")
	errBitmapAdvance = errors.New("text: nonfinite bitmap advance")
)

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
// Empty regions with a positive advance represent spaces. Bitmap pixels always
// receive the supplied paint tint and alpha.
type Glyph struct {
	Region                      image.Rectangle
	Advance, BearingX, BearingY float64
}

// FontOptions describe an immutable sized instance. Size is in pixels. The zero
// Language uses the engine default; text layout is horizontal.
type FontOptions struct {
	Size     float64
	Language language.Tag
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
		return nil, errFontData
	}
	owned := bytes.Clone(data)
	parsed, err := etext.NewGoTextFaceSource(bytes.NewReader(owned))
	if err != nil {
		return nil, fmt.Errorf("text: parse font: %w", err)
	}

	source := new(Source)
	source.ttf, source.key = parsed, sha256.Sum256(owned)

	return source, nil
}

// NewBitmapSource snapshots an atlas and glyph descriptors. It accepts arbitrary
// regions and nonzero image origins. Missing glyphs return errors during layout;
// consumers explicitly supply their desired replacement glyph in content.
func NewBitmapSource(atlas image.Image, metrics Metrics, glyphs map[rune]Glyph) (*Source, error) {
	if atlas == nil || !validMetrics(metrics) || len(glyphs) == 0 || len(glyphs) > 65536 {
		return nil, errBitmapSource
	}
	bounds := atlas.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || width > 8192 || height > 8192 || int64(width)*int64(height)*4 > maxSourceBytes {
		return nil, errBitmapLimit
	}
	source := new(Source)
	source.metrics, source.glyphs = metrics, make(map[rune]Glyph, len(glyphs))
	keys := make([]rune, 0, len(glyphs))
	for character, glyph := range glyphs {
		if !validGlyph(character, glyph, bounds) {
			return nil, fmt.Errorf("%w %U", errBitmapGlyph, character)
		}
		if !glyph.Region.Empty() {
			glyph.Region = glyph.Region.Sub(bounds.Min)
		}
		source.glyphs[character] = glyph
		keys = append(keys, character)
	}
	source.bitmap = image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(source.bitmap, source.bitmap.Bounds(), atlas, bounds.Min, draw.Src)
	digest := sha256.New()
	digest.Write(source.bitmap.Pix)
	writeNumbers(digest, float64(width), float64(height), metrics.Ascent, metrics.Descent, metrics.LineGap)
	slices.Sort(keys)
	for _, character := range keys {
		glyph := source.glyphs[character]
		writeNumbers(digest, float64(character), float64(glyph.Region.Min.X), float64(glyph.Region.Min.Y),
			float64(glyph.Region.Max.X), float64(glyph.Region.Max.Y), glyph.Advance, glyph.BearingX, glyph.BearingY)
	}
	copy(source.key[:], digest.Sum(nil))

	return source, nil
}

func validGlyph(character rune, glyph Glyph, bounds image.Rectangle) bool {
	return utf8.ValidRune(character) && finite(glyph.Advance) && glyph.Advance >= 0 &&
		finite(glyph.BearingX) && finite(glyph.BearingY) && (glyph.Region.Empty() || glyph.Region.In(bounds))
}

// NewFont creates an immutable instance; no renderer or graphics device is needed
// for the SDK's source/metric operations beyond Ebitengine's platform initialization.
func (s *Source) NewFont(options FontOptions) (*Font, error) {
	if s == nil || (s.ttf == nil && s.bitmap == nil) || !finite(options.Size) || options.Size <= 0 || options.Size > 4096 {
		return nil, errFontSize
	}
	font := new(Font)
	font.source, font.options = s, options
	digest := sha256.New()
	digest.Write(s.key[:])
	writeNumbers(digest, options.Size)
	digest.Write([]byte(options.Language.String()))
	if s.ttf != nil {
		font.face = new(etext.GoTextFace)
		font.face.Source, font.face.Size, font.face.Language = s.ttf, options.Size, options.Language
	}
	copy(font.key[:], digest.Sum(nil))
	if font.face != nil {
		metrics := font.face.Metrics()
		font.metrics = Metrics{metrics.HAscent, metrics.HDescent, metrics.HLineGap}
	} else {
		scale := options.Size / (s.metrics.Ascent + s.metrics.Descent)
		if !finite(scale) || scale <= 0 {
			return nil, errFontMetrics
		}
		font.metrics = Metrics{s.metrics.Ascent * scale, s.metrics.Descent * scale, s.metrics.LineGap * scale}
	}
	if !validMetrics(font.metrics) {
		return nil, errFontMetrics
	}

	return font, nil
}

// WithSize returns another immutable configuration sharing the source.
func (f *Font) WithSize(size float64) (*Font, error) {
	if f == nil {
		return nil, errNilFont
	}
	options := f.options
	options.Size = size

	return f.source.NewFont(options)
}

// Size returns the configured pixel size.
func (f *Font) Size() float64 { return f.options.Size }

// Metrics returns detached baseline metrics in configured pixels.
func (f *Font) Metrics() Metrics { return f.metrics }

// Identity is a content/settings identity, not a process address or mutable face.
func (f *Font) Identity() [32]byte { return f.key }

// Measure returns horizontal advances and height using the engine's zero-spacing
// measurement convention. Empty text measures zero. Use NewLayout for naturally
// spaced multiline geometry, wrapping and caret queries.
func (f *Font) Measure(value string) (float64, float64, error) {
	if f == nil || !utf8.ValidString(value) || len(value) > 1<<20 {
		return 0, 0, errMeasurement
	}
	if value == "" {
		return 0, 0, nil
	}
	if f.face != nil {
		w, h := etext.Measure(value, f.face, 0)
		if !finite(w) || !finite(h) {
			return 0, 0, errMeasureExtent
		}

		return w, h, nil
	}
	width := 0.0
	for _, line := range strings.Split(value, "\n") {
		advance := 0.0
		for _, ch := range strings.TrimSuffix(line, "\r") {
			glyph, ok := f.source.glyphs[ch]
			if !ok {
				return 0, 0, fmt.Errorf("%w %U", errMissingGlyph, ch)
			}
			advance += glyph.Advance * f.options.Size / (f.source.metrics.Ascent + f.source.metrics.Descent)
		}
		width = max(width, advance)
	}
	if !finite(width) {
		return 0, 0, errBitmapAdvance
	}

	return width, f.metrics.Ascent + f.metrics.Descent, nil
}

// CopyEngineFace supplies an independently configurable engine face for host-only
// integration with existing Ebitengine APIs. Changes to its fields/features do not
// change this Font. Its shared Source must remain read-only (no UnsafeInternal).
// Bitmap fonts return nil. This is not the SDK's rendering identity.
func (f *Font) CopyEngineFace() *etext.GoTextFace {
	if f == nil || f.face == nil {
		return nil
	}
	face := new(etext.GoTextFace)
	face.Source, face.Size, face.Language = f.source.ttf, f.options.Size, f.options.Language

	return face
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func validMetrics(m Metrics) bool {
	return finite(m.Ascent+m.Descent+m.LineGap) && finite(m.Ascent) && finite(m.Descent) && finite(m.LineGap) &&
		m.Ascent >= 0 && m.Descent >= 0 && m.Ascent+m.Descent > 0 && m.LineGap >= 0
}
func writeNumbers(h hash.Hash, values ...float64) {
	var b [8]byte
	for _, v := range values {
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
		h.Write(b[:])
	}
}
