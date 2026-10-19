package text

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

type testGame struct {
	tests *testing.M
	done  chan int
	code  int
}

func (g *testGame) Update() error {
	if g.done == nil {
		g.done = make(chan int, 1)
		go func() { g.done <- g.tests.Run() }()
	}
	select {
	case g.code = <-g.done:
		return ebiten.Termination
	default:
		return nil
	}
}
func (*testGame) Draw(*ebiten.Image)         {}
func (*testGame) Layout(int, int) (int, int) { return 1, 1 }
func TestMain(m *testing.M) {
	g := &testGame{tests: m, code: 1}
	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
	os.Exit(g.code)
}

func fixtureFont(t *testing.T, size float64) *Font {
	t.Helper()
	s, err := ParseFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.NewFont(FontOptions{Size: size})
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func fixtureLayout(t *testing.T, spans []Span, options LayoutOptions) *Layout {
	t.Helper()
	c, err := NewContent(spans)
	if err != nil {
		t.Fatal(err)
	}
	l, err := NewLayout(c, options)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func pixels(img *ebiten.Image) []byte {
	b := make([]byte, img.Bounds().Dx()*img.Bounds().Dy()*4)
	img.ReadPixels(b)
	return b
}

func TestFontSnapshotsAndDerivedIdentity(t *testing.T) {
	data := bytes.Clone(goregular.TTF)
	s, err := ParseFont(data)
	if err != nil {
		t.Fatal(err)
	}
	clear(data)
	f, err := s.NewFont(FontOptions{Size: 20})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := f.WithSize(25)
	b, _ := f.WithSize(25)
	if a.Identity() != b.Identity() {
		t.Fatal("equivalent sized fonts differ")
	}
	face := a.CopyEngineFace()
	face.Size = 90
	face.SetFeature(etext.MustParseTag("kern"), 0)
	if a.Size() != 25 || a.CopyEngineFace().Size != 25 {
		t.Fatal("mutable engine copy changed authoritative font")
	}
	if _, err := ParseFont([]byte("invalid")); err == nil {
		t.Fatal("invalid font accepted")
	}
	if _, err := f.WithSize(0); err == nil {
		t.Fatal("zero size accepted")
	}
}

func TestScaledDrawActuallyReusesRasterAndPaintDoesNotChangeLayout(t *testing.T) {
	f := fixtureFont(t, 20)
	r, err := NewRenderer(DefaultRendererOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	target := ebiten.NewImage(160, 50)
	defer target.Deallocate()
	var first Rect
	for i := 0; i < 5; i++ {
		scaled, _ := f.WithSize(25)
		l := fixtureLayout(t, []Span{{Text: "Scaled", Font: scaled, Paint: "ink"}}, LayoutOptions{})
		if i == 0 {
			first = l.Bounds()
		}
		if l.Bounds() != first {
			t.Fatal("unstable scaled metrics")
		}
		if err := r.Draw(target, l, Palette{"ink": color.White}, DrawOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	stats := r.Stats()
	if stats.Misses != 1 || stats.Hits != 4 || stats.Entries != 1 {
		t.Fatalf("scaled raster churn: %+v", stats)
	}
	l := fixtureLayout(t, []Span{{Text: "Scaled", Font: f, Paint: "ink"}}, LayoutOptions{})
	before := l.Bounds()
	for _, c := range []color.Color{color.White, color.RGBA{R: 255, A: 255}} {
		if err := r.Draw(target, l, Palette{"ink": c}, DrawOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if l.Bounds() != before {
		t.Fatal("paint mutated geometry")
	}
}

func TestRenderingMatchesEngineAndUncachedPixels(t *testing.T) {
	f := fixtureFont(t, 22)
	l := fixtureLayout(t, []Span{{Text: "Agj café", Font: f}}, LayoutOptions{})
	r, _ := NewRenderer(DefaultRendererOptions())
	defer r.Close()
	a, b, c := ebiten.NewImage(180, 60), ebiten.NewImage(180, 60), ebiten.NewImage(180, 60)
	defer a.Deallocate()
	defer b.Deallocate()
	defer c.Deallocate()
	o := DrawOptions{}
	o.GeoM.Translate(12, 10)
	if err := r.Draw(a, l, nil, o); err != nil {
		t.Fatal(err)
	}
	o.Uncached = true
	if err := r.Draw(b, l, nil, o); err != nil {
		t.Fatal(err)
	}
	eo := &etext.DrawOptions{}
	eo.GeoM.Translate(12, 10)
	etext.Draw(c, "Agj café", f.CopyEngineFace(), eo)
	if !bytes.Equal(pixels(a), pixels(b)) {
		t.Fatal("fresh/cached pixels differ")
	}
	if !bytes.Equal(pixels(a), pixels(c)) {
		t.Fatal("SDK glyph positioning differs from engine")
	}
}

func TestBitmapSnapshotNonzeroOriginAlphaAndMixedBaseline(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(10, 20, 16, 23))
	for y := 20; y < 23; y++ {
		for x := 10; x < 13; x++ {
			atlas.SetRGBA(x, y, color.RGBA{128, 128, 128, 128})
		}
	}
	desc := map[rune]Glyph{'X': {Region: image.Rect(10, 20, 13, 23), Advance: 4, BearingY: -3}, ' ': {Advance: 2}}
	s, err := NewBitmapSource(atlas, Metrics{Ascent: 3}, desc)
	if err != nil {
		t.Fatal(err)
	}
	clear(atlas.Pix)
	delete(desc, 'X')
	f, _ := s.NewFont(FontOptions{Size: 6})
	l := fixtureLayout(t, []Span{{Text: "X X", Font: f, Paint: "ink", Tag: "object"}}, LayoutOptions{})
	if l.Bounds().Width() != 20 {
		t.Fatalf("advance: %v", l.Bounds())
	}
	r, _ := NewRenderer(DefaultRendererOptions())
	defer r.Close()
	dst := ebiten.NewImage(30, 12)
	defer dst.Deallocate()
	if err := r.Draw(dst, l, Palette{"ink": color.NRGBA{R: 255, A: 128}}, DrawOptions{}); err != nil {
		t.Fatal(err)
	}
	p := pixels(dst)
	if p[0] < 63 || p[0] > 65 || p[1] != 0 || p[3] < 63 || p[3] > 65 {
		t.Fatalf("incorrect snapshot/tint alpha: %v", p[:4])
	}
	hits := l.HitTestLocal(Point{1, 1})
	if len(hits) != 1 || !reflect.DeepEqual(hits[0].Tags, []string{"object"}) {
		t.Fatalf("hit: %+v", hits)
	}
	if _, err := NewLayout(&Content{text: "Q", spans: []contentSpan{{Span: Span{Text: "Q", Font: f}, end: 1}}}, LayoutOptions{}); err == nil {
		t.Fatal("missing bitmap glyph accepted")
	}
}

func TestWrappingSourceRangesAndCaretAffinity(t *testing.T) {
	f := fixtureFont(t, 15)
	source := "one two\r\n\nthree\n"
	l := fixtureLayout(t, []Span{{Text: source, Font: f}}, LayoutOptions{Width: 35})
	if l.Content().Text() != source {
		t.Fatal("source rewritten")
	}
	lines := l.Lines()
	if lines[len(lines)-1].Start != len(source) || lines[len(lines)-1].End != len(source) {
		t.Fatalf("trailing empty line lost: %+v", lines)
	}
	for i, line := range lines[:len(lines)-1] {
		if line.End == line.Next && line.End == lines[i+1].Start {
			a, err := l.CaretAt(line.End, true)
			if err != nil {
				t.Fatal(err)
			}
			b, err := l.CaretAt(line.End, false)
			if err != nil {
				t.Fatal(err)
			}
			if a.Position.Y >= b.Position.Y {
				t.Fatalf("wrap affinity lost: %+v %+v", a, b)
			}
		}
	}
	if len(l.HitTestLocal(Point{999, 999})) != 0 {
		t.Fatal("outside hit activated content")
	}
	nearest, err := l.NearestCaret(Point{999, 999})
	if err != nil || nearest.Inside {
		t.Fatalf("nearest/outside: %+v %v", nearest, err)
	}
	if _, err := l.RangeRects(-1, 2); err == nil {
		t.Fatal("invalid range accepted")
	}
}

func TestPaintBoundariesDoNotSplitShapingAndRangesReportClusters(t *testing.T) {
	f := fixtureFont(t, 24)
	a := fixtureLayout(t, []Span{{Text: "office e\u0301", Font: f}}, LayoutOptions{})
	b := fixtureLayout(t, []Span{{Text: "of", Font: f, Paint: "a", Tag: "one"}, {Text: "fice e\u0301", Font: f, Paint: "b", Tag: "two"}}, LayoutOptions{})
	if a.Bounds() != b.Bounds() || len(a.glyphs) != len(b.glyphs) {
		t.Fatal("paint/tag boundary changed shaping")
	}
	for i := range a.glyphs {
		if a.glyphs[i].glyph.ImageBounds != b.glyphs[i].glyph.ImageBounds {
			t.Fatal("different glyph placement")
		}
	}
	for _, c := range a.clusters {
		if c.end-c.start > 1 {
			coverage, err := a.RangeRects(c.start, c.end)
			if err != nil || len(coverage.Rects) == 0 {
				t.Fatalf("coverage: %+v %v", coverage, err)
			}
		}
	}
}

func TestCacheLimitsCloseAndRetainedGeometry(t *testing.T) {
	f := fixtureFont(t, 12)
	o := DefaultRendererOptions()
	o.MaxEntries = 1
	o.MaxCacheBytes = 10000
	r, _ := NewRenderer(o)
	dst := ebiten.NewImage(200, 80)
	defer dst.Deallocate()
	a := fixtureLayout(t, []Span{{Text: "first", Font: f}}, LayoutOptions{})
	b := fixtureLayout(t, []Span{{Text: "second", Font: f}}, LayoutOptions{})
	for _, l := range []*Layout{a, b, a} {
		if err := r.Draw(dst, l, nil, DrawOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if r.Stats().Entries != 1 || r.Stats().Evictions != 2 {
		t.Fatalf("LRU accounting: %+v", r.Stats())
	}
	r.ClearCaches()
	if r.Stats().Entries != 0 || r.Stats().Bytes != 0 {
		t.Fatal("clear retained pixels")
	}
	r.Close()
	r.Close()
	if err := r.Draw(dst, a, nil, DrawOptions{}); err != ErrClosed {
		t.Fatal(err)
	}
	if _, err := a.CaretAt(0, false); err != nil {
		t.Fatal("closure invalidated CPU geometry")
	}
	dst.Fill(color.White)
	if pixels(dst)[3] != 255 {
		t.Fatal("host target was disposed")
	}
	o.MaxRenderBytes = 4
	r, _ = NewRenderer(o)
	defer r.Close()
	if err := r.Draw(dst, a, nil, DrawOptions{}); err == nil {
		t.Fatal("render limit ignored")
	}
}

func TestContentLogAndLimits(t *testing.T) {
	f := fixtureFont(t, 12)
	spans := []Span{{Text: "é", Font: f}}
	c, err := NewContent(spans)
	if err != nil {
		t.Fatal(err)
	}
	spans[0].Text = "changed"
	if c.Text() != "é" {
		t.Fatal("content alias")
	}
	log, _ := NewLog(2, 4)
	log.Append(c)
	log.Append(c)
	dropped, err := log.Append(c)
	if err != nil || dropped != 1 || len(log.Entries()) != 2 {
		t.Fatalf("log: %d %v", dropped, err)
	}
	entries := log.Entries()
	entries[0] = nil
	if log.Entries()[0] == nil {
		t.Fatal("log view aliases")
	}
	if _, err := NewLayout(c, LayoutOptions{MaxWork: 1}); err == nil {
		t.Fatal("work budget ignored")
	}
	if _, err := NewContent([]Span{{Text: string([]byte{255}), Font: f}}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}
