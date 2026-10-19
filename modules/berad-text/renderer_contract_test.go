package text_test

import (
	"image"
	"image/color"
	"testing"

	text "github.com/bradhannah/berad-sdk/modules/berad-text"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestRasterLRUPromotionUnderEntryAndBytePressure(t *testing.T) {
	font := bitmapFont(t, text.Metrics{Ascent: 2}, 2, 2, text.Glyph{Advance: 3, BearingY: -2}, "ABC")
	for _, bytePressure := range []bool{false, true} {
		name := "entries"
		options := text.DefaultRendererOptions()
		options.MaxEntries = 2
		if bytePressure {
			name = "bytes"
			options.MaxEntries = 10
			options.MaxCacheBytes = 34 // Two 2x2 RGBA rasters plus one source byte each.
		}
		t.Run(name, func(t *testing.T) {
			renderer, err := text.NewRenderer(options)
			if err != nil {
				t.Fatal(err)
			}
			defer renderer.Close()
			target := ebiten.NewImage(5, 4)
			defer target.Deallocate()
			expected := image.NewRGBA(image.Rect(0, 0, 5, 4))
			fillPixels(expected, image.Rect(1, 1, 3, 3), color.White)
			var draw text.DrawOptions
			draw.GeoM.Translate(1, 1)
			for index, value := range []string{"A", "B", "A", "C", "A", "B"} {
				target.Clear()
				layout := layoutFor(t, []text.Span{{Text: value, Font: font}}, text.LayoutOptions{})
				if err := renderer.Draw(target, layout, nil, draw); err != nil {
					t.Fatal(err)
				}
				requirePixels(t, target, expected)
				// Accessing A promotes it before C arrives: A must still hit, B must miss.
				if index == 4 && (renderer.Stats().Hits != 2 || renderer.Stats().Misses != 3) {
					t.Fatalf("recent A was not retained: %+v", renderer.Stats())
				}
			}
			stats := renderer.Stats()
			if stats.Entries != 2 || stats.Bytes != 34 || stats.PeakBytes != 34 || stats.Hits != 2 || stats.Misses != 4 || stats.Evictions != 2 || stats.Bypasses != 0 {
				t.Fatalf("LRU accounting=%+v", stats)
			}
			renderer.ClearCaches()
			if after := renderer.Stats(); after.Bytes != 0 || after.Entries != 0 || after.PeakBytes != 34 || after.Hits != stats.Hits || after.Misses != stats.Misses {
				t.Fatalf("clear lost lifetime accounting: %+v", after)
			}
		})
	}
}

func TestOversizedRasterBypassesWithoutEvictingUsefulLabel(t *testing.T) {
	font := bitmapFont(t, text.Metrics{Ascent: 2}, 2, 2, text.Glyph{Advance: 3, BearingY: -2}, "A")
	options := text.DefaultRendererOptions()
	options.MaxCacheBytes = 34
	renderer, err := text.NewRenderer(options)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	target := ebiten.NewImage(9, 3)
	defer target.Deallocate()
	for _, value := range []string{"A", "AAA", "A"} {
		target.Clear()
		layout := layoutFor(t, []text.Span{{Text: value, Font: font}}, text.LayoutOptions{})
		if err := renderer.Draw(target, layout, nil, text.DrawOptions{}); err != nil {
			t.Fatal(err)
		}
		expected := image.NewRGBA(image.Rect(0, 0, 9, 3))
		for index := range len(value) {
			fillPixels(expected, image.Rect(index*3, 0, index*3+2, 2), color.White)
		}
		requirePixels(t, target, expected)
	}
	stats := renderer.Stats()
	if stats.Entries != 1 || stats.Bytes != 17 || stats.Hits != 1 || stats.Misses != 2 || stats.Evictions != 0 || stats.Bypasses != 1 {
		t.Fatalf("bypass disturbed retained label: %+v", stats)
	}
}

func TestPaletteRepaintAndTransparentPaintHaveIndependentPixels(t *testing.T) {
	font := bitmapFont(t, text.Metrics{Ascent: 2}, 2, 2, text.Glyph{Advance: 3, BearingY: -2}, "A")
	layout := layoutFor(t, []text.Span{{Text: "A", Font: font, Paint: "ink"}}, text.LayoutOptions{})
	renderer, err := text.NewRenderer(text.DefaultRendererOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	target := ebiten.NewImage(4, 3)
	defer target.Deallocate()
	palette := text.Palette{"ink": color.White}
	for _, ink := range []color.Color{color.White, color.RGBA{R: 255, A: 255}, color.Transparent, color.White} {
		palette["ink"] = ink
		target.Clear()
		if err := renderer.Draw(target, layout, palette, text.DrawOptions{}); err != nil {
			t.Fatal(err)
		}
		expected := image.NewRGBA(image.Rect(0, 0, 4, 3))
		fillPixels(expected, image.Rect(0, 0, 2, 2), ink)
		requirePixels(t, target, expected)
	}
	if stats := renderer.Stats(); stats.Hits != 1 || stats.Misses != 3 {
		t.Fatalf("numeric paint identity=%+v", stats)
	}
	for _, palette := range []text.Palette{nil, {"ink": nil}} {
		if err := renderer.Draw(target, layout, palette, text.DrawOptions{}); err == nil {
			t.Fatal("unresolved paint accepted")
		}
	}
}

func TestBitmapPixelsAlwaysReceivePaintTintAndAlpha(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	atlas.SetRGBA(0, 0, color.RGBA{R: 128, G: 64, B: 32, A: 128})
	atlas.SetRGBA(1, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	atlas.SetRGBA(1, 1, color.RGBA{R: 64, G: 128, B: 64, A: 128})
	source, err := text.NewBitmapSource(atlas, text.Metrics{Ascent: 2},
		map[rune]text.Glyph{'X': {Region: atlas.Bounds(), Advance: 3, BearingY: -2}})
	if err != nil {
		t.Fatal(err)
	}
	font, err := source.NewFont(text.FontOptions{Size: 2})
	if err != nil {
		t.Fatal(err)
	}
	layout := layoutFor(t, []text.Span{{Text: "X", Font: font, Paint: "ink"}}, text.LayoutOptions{})
	renderer, err := text.NewRenderer(text.DefaultRendererOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	target := ebiten.NewImage(3, 3)
	defer target.Deallocate()
	palette := text.Palette{"ink": color.NRGBA{G: 255, A: 128}}
	// Authored premultiplied pixel expectations: source RGB is tinted, and source
	// alpha is multiplied by paint alpha. No bitmap RGB-preservation mode exists.
	expected := image.NewRGBA(image.Rect(0, 0, 3, 3))
	expected.SetRGBA(0, 0, color.RGBA{G: 32, A: 64})
	expected.SetRGBA(1, 0, color.RGBA{G: 128, A: 128})
	expected.SetRGBA(1, 1, color.RGBA{G: 64, A: 64})
	for range 2 {
		target.Clear()
		if err := renderer.Draw(target, layout, palette, text.DrawOptions{}); err != nil {
			t.Fatal(err)
		}
		requirePixels(t, target, expected)
	}
}
