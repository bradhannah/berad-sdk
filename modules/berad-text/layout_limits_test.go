package text

import (
	"image"
	"math"
	"testing"
)

func TestLayoutRejectsAccumulatedHeightOverflow(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	source, err := NewBitmapSource(atlas, Metrics{Ascent: 1, LineGap: math.MaxFloat64 / 2},
		map[rune]Glyph{'X': {Region: atlas.Bounds(), Advance: 1, BearingY: -1}})
	if err != nil {
		t.Fatal(err)
	}
	font, err := source.NewFont(FontOptions{Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	content, err := NewContent([]Span{{Text: "X\nX\nX\nX\nX", Font: font}})
	if err != nil {
		t.Fatal(err)
	}
	if layout, err := NewLayout(content, LayoutOptions{}); err == nil || layout != nil {
		t.Fatalf("overflowing natural line spacing returned layout=%v, error=%v", layout, err)
	}
}

func TestBitmapDefinitionsRejectNegativeAndNonfiniteValues(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	glyph := Glyph{Region: atlas.Bounds(), Advance: 1, BearingY: -1}
	for _, metrics := range []Metrics{
		{Ascent: -1, Descent: 2}, {Ascent: 2, Descent: -1}, {Ascent: 1, LineGap: -1}, {},
		{Ascent: math.NaN()}, {Ascent: math.Inf(1)}, {Ascent: math.MaxFloat64, Descent: math.MaxFloat64},
	} {
		if source, err := NewBitmapSource(atlas, metrics, map[rune]Glyph{'X': glyph}); err == nil || source != nil {
			t.Fatalf("invalid metrics %+v accepted: source=%v error=%v", metrics, source, err)
		}
	}
	for _, invalid := range []Glyph{
		{Advance: -1}, {Advance: math.Inf(1)}, {Advance: math.NaN()},
		{Advance: 1, BearingX: math.Inf(-1)}, {Advance: 1, BearingY: math.NaN()},
	} {
		if source, err := NewBitmapSource(atlas, Metrics{Ascent: 1}, map[rune]Glyph{'X': invalid}); err == nil || source != nil {
			t.Fatalf("invalid glyph %+v accepted: source=%v error=%v", invalid, source, err)
		}
	}
}

func TestFontRejectsScaledMetricOverflow(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	for _, metrics := range []Metrics{{Ascent: math.SmallestNonzeroFloat64}, {Ascent: 1, LineGap: math.MaxFloat64 / 2}} {
		source, err := NewBitmapSource(atlas, metrics, map[rune]Glyph{'X': {Region: atlas.Bounds(), Advance: 1}})
		if err != nil {
			t.Fatal(err)
		}
		if font, err := source.NewFont(FontOptions{Size: 4}); err == nil || font != nil {
			t.Fatalf("overflowing scaled metrics accepted: font=%v error=%v", font, err)
		}
	}
}

func TestLayoutRejectsExcessiveInkAndAdvance(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	for _, glyph := range []Glyph{
		{Region: atlas.Bounds(), Advance: 1, BearingX: math.MaxFloat64},
		{Region: atlas.Bounds(), Advance: 1, BearingY: -math.MaxFloat64},
		{Region: atlas.Bounds(), Advance: math.MaxFloat64, BearingY: -1},
	} {
		source, err := NewBitmapSource(atlas, Metrics{Ascent: 1}, map[rune]Glyph{'X': glyph})
		if err != nil {
			t.Fatal(err)
		}
		font, err := source.NewFont(FontOptions{Size: 1})
		if err != nil {
			t.Fatal(err)
		}
		content, err := NewContent([]Span{{Text: "XX", Font: font}})
		if err != nil {
			t.Fatal(err)
		}
		if layout, err := NewLayout(content, LayoutOptions{}); err == nil || layout != nil {
			t.Fatalf("excessive glyph %+v returned layout=%v error=%v", glyph, layout, err)
		}
	}
}

func TestLayoutOptionsRejectInvalidExtents(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	source, err := NewBitmapSource(atlas, Metrics{Ascent: 1}, map[rune]Glyph{'X': {Region: atlas.Bounds(), Advance: 1, BearingY: -1}})
	if err != nil {
		t.Fatal(err)
	}
	font, err := source.NewFont(FontOptions{Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	content, err := NewContent([]Span{{Text: "X", Font: font}})
	if err != nil {
		t.Fatal(err)
	}
	for _, options := range []LayoutOptions{{Width: -1}, {Width: math.Inf(1)}, {Width: math.NaN()}, {Width: math.MaxFloat64}, {MaxWork: -1}} {
		if layout, err := NewLayout(content, options); err == nil || layout != nil {
			t.Fatalf("invalid options %+v accepted: layout=%v error=%v", options, layout, err)
		}
	}
	if width, height, err := font.Measure(string([]byte{255})); err == nil || width != 0 || height != 0 {
		t.Fatalf("invalid UTF-8 measurement returned %v,%v error=%v", width, height, err)
	}
}

func TestBitmapMeasurementRejectsAccumulatedAdvanceOverflow(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	source, err := NewBitmapSource(atlas, Metrics{Ascent: 1}, map[rune]Glyph{'X': {Region: atlas.Bounds(), Advance: math.MaxFloat64}})
	if err != nil {
		t.Fatal(err)
	}
	font, err := source.NewFont(FontOptions{Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	if width, height, err := font.Measure("XX"); err == nil || width != 0 || height != 0 {
		t.Fatalf("overflowing measurement returned %v,%v error=%v", width, height, err)
	}
}
