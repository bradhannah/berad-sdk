package text_test

import (
	"bytes"
	"image"
	"image/color"
	"reflect"
	"testing"

	text "github.com/bradhannah/berad-sdk/modules/berad-text"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

func bitmapFont(t *testing.T, metrics text.Metrics, width, height int, glyph text.Glyph, characters string) *text.Font {
	t.Helper()
	atlas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			atlas.Set(x, y, color.White)
		}
	}
	glyph.Region = atlas.Bounds()
	glyphs := make(map[rune]text.Glyph)
	for _, character := range characters {
		glyphs[character] = glyph
	}
	source, err := text.NewBitmapSource(atlas, metrics, glyphs)
	if err != nil {
		t.Fatal(err)
	}
	font, err := source.NewFont(text.FontOptions{Size: metrics.Ascent + metrics.Descent})
	if err != nil {
		t.Fatal(err)
	}
	return font
}

func layoutFor(t *testing.T, spans []text.Span, options text.LayoutOptions) *text.Layout {
	t.Helper()
	content, err := text.NewContent(spans)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := text.NewLayout(content, options)
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func requirePixels(t *testing.T, target *ebiten.Image, expected *image.RGBA) {
	t.Helper()
	actual := make([]byte, len(expected.Pix))
	target.ReadPixels(actual)
	if !bytes.Equal(actual, expected.Pix) {
		for y := range expected.Bounds().Dy() {
			for x := range expected.Bounds().Dx() {
				offset := (y*expected.Bounds().Dx() + x) * 4
				if !bytes.Equal(actual[offset:offset+4], expected.Pix[offset:offset+4]) {
					t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, actual[offset:offset+4], expected.Pix[offset:offset+4])
				}
			}
		}
	}
}

func fillPixels(target *image.RGBA, rect image.Rectangle, ink color.Color) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			target.Set(x, y, ink)
		}
	}
}

func TestMixedBitmapBaselineAndTopAlignmentHaveAuthoredGeometryAndPixels(t *testing.T) {
	small := bitmapFont(t, text.Metrics{Ascent: 2, Descent: 1, LineGap: 1}, 2, 2,
		text.Glyph{Advance: 3, BearingX: -1, BearingY: -2}, "S")
	tall := bitmapFont(t, text.Metrics{Ascent: 4, Descent: 2, LineGap: 2}, 2, 3,
		text.Glyph{Advance: 4, BearingX: 1, BearingY: -4}, "T")
	for _, top := range []bool{false, true} {
		name := "baseline"
		if top {
			name = "top"
		}
		t.Run(name, func(t *testing.T) {
			layout := layoutFor(t, []text.Span{{Text: "S", Font: small, Paint: "small"},
				{Text: "T", Font: tall, Paint: "tall"}, {Text: "\nS", Font: small, Paint: "small"}}, text.LayoutOptions{TopAlign: top})
			wantLines := []text.Line{
				{Start: 0, End: 2, Next: 3, Bounds: text.Rect{Max: text.Point{X: 7, Y: 6}}, Baseline: 4},
				{Start: 3, End: 4, Next: 4, Bounds: text.Rect{Min: text.Point{Y: 8}, Max: text.Point{X: 3, Y: 11}}, Baseline: 10},
			}
			if !reflect.DeepEqual(layout.Lines(), wantLines) || layout.Bounds() != (text.Rect{Max: text.Point{X: 7, Y: 11}}) {
				t.Fatalf("lines=%+v bounds=%+v", layout.Lines(), layout.Bounds())
			}
			coverage, err := layout.RangeRects(1, 4)
			wantCoverage := text.Coverage{RequestedStart: 1, RequestedEnd: 4, Start: 1, End: 4,
				Rects: []text.Rect{{Min: text.Point{X: 3}, Max: text.Point{X: 7, Y: 6}},
					{Min: text.Point{Y: 8}, Max: text.Point{X: 3, Y: 11}}}}
			if err != nil || !reflect.DeepEqual(coverage, wantCoverage) {
				t.Fatalf("coverage=%+v, error=%v; want %+v", coverage, err, wantCoverage)
			}
			caret, err := layout.CaretAt(1, false)
			if err != nil || caret.Position != (text.Point{X: 3}) || caret.Height != 6 || caret.Offset != 1 || caret.Snapped {
				t.Fatalf("caret=%+v, error=%v", caret, err)
			}
			renderer, err := text.NewRenderer(text.DefaultRendererOptions())
			if err != nil {
				t.Fatal(err)
			}
			defer renderer.Close()
			target := ebiten.NewImage(12, 14)
			defer target.Deallocate()
			var options text.DrawOptions
			options.GeoM.Translate(2, 1)
			red, green := color.RGBA{R: 255, A: 255}, color.RGBA{G: 255, A: 255}
			if err := renderer.Draw(target, layout, text.Palette{"small": red, "tall": green}, options); err != nil {
				t.Fatal(err)
			}
			expected := image.NewRGBA(image.Rect(0, 0, 12, 14))
			firstY := 3 // Four-pixel baseline minus the small glyph's two-pixel ascent, plus translation.
			if top {
				firstY = 1
			}
			fillPixels(expected, image.Rect(1, firstY, 3, firstY+2), red)
			fillPixels(expected, image.Rect(6, 1, 8, 4), green)
			fillPixels(expected, image.Rect(1, 9, 3, 11), red)
			requirePixels(t, target, expected)
		})
	}
}

func TestWrappedAndHardBreakCaretSitesHaveExactSourceOffsets(t *testing.T) {
	font := bitmapFont(t, text.Metrics{Ascent: 2, Descent: 1, LineGap: 1}, 2, 2,
		text.Glyph{Advance: 3, BearingY: -2}, "Sé")
	layout := layoutFor(t, []text.Span{{Text: "SSS", Font: font}}, text.LayoutOptions{Width: 6})
	for _, test := range []struct {
		offset   int
		upstream bool
		position text.Point
	}{{0, false, text.Point{}}, {2, true, text.Point{X: 6}}, {2, false, text.Point{Y: 4}}, {3, false, text.Point{X: 3, Y: 4}}} {
		caret, err := layout.CaretAt(test.offset, test.upstream)
		if err != nil || caret.Position != test.position || caret.Requested != test.offset || caret.Offset != test.offset || caret.Snapped || caret.Height != 3 {
			t.Fatalf("caret(%d,%v)=%+v, error=%v", test.offset, test.upstream, caret, err)
		}
	}
	layout = layoutFor(t, []text.Span{{Text: "S\r\nS\n", Font: font}}, text.LayoutOptions{})
	wantLines := []text.Line{
		{Start: 0, End: 1, Next: 3, Bounds: text.Rect{Max: text.Point{X: 3, Y: 3}}, Baseline: 2},
		{Start: 3, End: 4, Next: 5, Bounds: text.Rect{Min: text.Point{Y: 4}, Max: text.Point{X: 3, Y: 7}}, Baseline: 6},
		{Start: 5, End: 5, Next: 5, Bounds: text.Rect{Min: text.Point{Y: 8}, Max: text.Point{Y: 11}}, Baseline: 10},
	}
	if !reflect.DeepEqual(layout.Lines(), wantLines) {
		t.Fatalf("hard break lines=%+v, want %+v", layout.Lines(), wantLines)
	}
	for _, limits := range [][2]int{{1, 3}, {3, 3}} {
		coverage, err := layout.RangeRects(limits[0], limits[1])
		if err != nil || len(coverage.Rects) != 0 || coverage.Snapped {
			t.Fatalf("hard break/empty range=%+v, error=%v", coverage, err)
		}
	}
	lines := layout.Lines()
	lines[0].Start = 99
	if layout.Lines()[0].Start != 0 {
		t.Fatal("line result aliases retained geometry")
	}
	empty := layoutFor(t, nil, text.LayoutOptions{DefaultFont: font})
	if caret, err := empty.CaretAt(0, false); err != nil || caret.Position != (text.Point{}) || caret.Height != 3 {
		t.Fatalf("empty layout caret=%+v error=%v", caret, err)
	}
	unicode := layoutFor(t, []text.Span{{Text: "éS", Font: font}}, text.LayoutOptions{})
	if _, err := unicode.CaretAt(1, false); err == nil {
		t.Fatal("caret inside a UTF-8 encoding accepted")
	}
	if _, err := unicode.RangeRects(1, 2); err == nil {
		t.Fatal("range inside a UTF-8 encoding accepted")
	}
}

func TestCombiningClusterReportsRequestedAndRepresentedOffsets(t *testing.T) {
	source, err := text.ParseFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	font, err := source.NewFont(text.FontOptions{Size: 24})
	if err != nil {
		t.Fatal(err)
	}
	layout := layoutFor(t, []text.Span{{Text: "e\u0301X", Font: font}}, text.LayoutOptions{})
	coverage, err := layout.RangeRects(1, 3) // Only the combining acute, a valid UTF-8 range.
	if err != nil || coverage.RequestedStart != 1 || coverage.RequestedEnd != 3 || coverage.Start != 0 || coverage.End != 3 || !coverage.Snapped || len(coverage.Rects) != 1 {
		t.Fatalf("combining coverage=%+v error=%v", coverage, err)
	}
	caret, err := layout.CaretAt(1, false)
	if err != nil || caret.Requested != 1 || caret.Offset != 0 || !caret.Snapped || caret.Position != (text.Point{}) {
		t.Fatalf("combining caret=%+v error=%v", caret, err)
	}
}

func TestMeasurementRetainsZeroSpacingCompatibility(t *testing.T) {
	bitmap := bitmapFont(t, text.Metrics{Ascent: 2, Descent: 1, LineGap: 4}, 2, 2,
		text.Glyph{Advance: 3, BearingY: -2}, "S")
	width, height, err := bitmap.Measure("SS\nS")
	if err != nil || width != 6 || height != 3 {
		t.Fatalf("bitmap compatibility measurement=%v,%v error=%v; want 6,3", width, height, err)
	}
	if width, height, err := bitmap.Measure(""); err != nil || width != 0 || height != 0 {
		t.Fatalf("empty measurement=%v,%v error=%v", width, height, err)
	}
	source, err := text.ParseFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	font, err := source.NewFont(text.FontOptions{Size: 24})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "Agj café", "Agj\ncafé"} {
		width, height, err := font.Measure(value)
		wantWidth, wantHeight := etext.Measure(value, font.CopyEngineFace(), 0)
		if err != nil || width != wantWidth || height != wantHeight {
			t.Fatalf("outline compatibility measurement(%q)=%v,%v error=%v; want %v,%v", value, width, height, err, wantWidth, wantHeight)
		}
	}
}
