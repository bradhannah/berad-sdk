package text

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/rivo/uniseg"
)

// Point and Rect use layout-local pixels, independent of camera/window coordinates.
type Point struct{ X, Y float64 }
type Rect struct{ Min, Max Point }

func (r Rect) Width() float64  { return r.Max.X - r.Min.X }
func (r Rect) Height() float64 { return r.Max.Y - r.Min.Y }
func (r Rect) Contains(p Point) bool {
	return p.X >= r.Min.X && p.Y >= r.Min.Y && p.X < r.Max.X && p.Y < r.Max.Y
}

type Align uint8

const (
	AlignStart Align = iota
	AlignCenter
	AlignEnd
)

// LayoutOptions use zero Width for no soft wrapping and zero LineSpacing for
// natural line spacing. Explicit positive spacing may overlap tall lines.
// TopAlign aligns mixed font tops instead of baselines for compatibility layouts.
// MaxWork bounds total text bytes submitted to measurement/shaping (default 8 MiB).
type LayoutOptions struct {
	Width, LineSpacing float64
	Align              Align
	TopAlign           bool
	DefaultFont        *Font
	MaxWork            int
}

type shapedRun struct {
	font       *Font
	start, end int
	width      float64
	glyphs     []etext.LazyGlyph
}
type placedGlyph struct {
	font        *Font
	glyph       etext.LazyGlyph
	bitmap      Glyph
	isBitmap    bool
	x, y, scale float64
	start, end  int
	paint       string
}
type cluster struct {
	start, end int
	rect       Rect
}

// Line reports source ranges and geometric extents; Next includes a hard break,
// while a soft break has Next == End. Returned line data is detached.
type Line struct {
	Start, End, Next int
	Bounds           Rect
	Baseline         float64
}

// Layout is an immutable presentation/geometry snapshot. Queries are CPU-only
// and remain valid after a Renderer closes. Fonts/source glyph data remain retained
// while layouts are retained; this memory is distinct from renderer cache bytes.
type Layout struct {
	content     *Content
	lines       []Line
	clusters    []cluster
	glyphs      []placedGlyph
	bounds, ink Rect
	key         [32]byte
}

// NewLayout shapes compatible adjacent font runs across paint/tag boundaries.
// Colours do not change geometry. Horizontal engine shaping is used; queries
// expose actual cluster coverage, not invented interior-ligature caret accuracy.
func NewLayout(content *Content, options LayoutOptions) (*Layout, error) {
	if content == nil || !finite(options.Width) || options.Width < 0 || !finite(options.LineSpacing) || options.LineSpacing < 0 || options.Align > AlignEnd || options.MaxWork < 0 {
		return nil, errors.New("text: invalid content/layout options")
	}
	font := options.DefaultFont
	if font == nil && len(content.spans) > 0 {
		font = content.spans[0].Font
	}
	if font == nil {
		return nil, errors.New("text: empty layout requires a default font")
	}
	budget := options.MaxWork
	if budget == 0 {
		budget = 8 << 20
	}
	l := &Layout{content: content}
	start := 0
	y := 0.0
	for {
		end, next := paragraph(content.text, start)
		for {
			lineEnd := end
			if options.Width > 0 && start < end {
				var err error
				lineEnd, err = wrapEnd(content, start, end, options.Width, &budget)
				if err != nil {
					return nil, err
				}
			}
			lineNext := lineEnd
			if lineEnd == end {
				lineNext = next
			}
			height, err := l.addLine(start, lineEnd, lineNext, y, font, options, &budget)
			if err != nil {
				return nil, err
			}
			if len(l.lines) > 8192 {
				return nil, errors.New("text: layout exceeds 8192 lines")
			}
			y += height
			start = lineEnd
			if start == end {
				break
			}
		}
		if next == end {
			break
		}
		start = next
	}
	h := sha256.New()
	h.Write(content.key[:])
	h.Write(font.key[:])
	writeNumbers(h, options.Width, options.LineSpacing, float64(options.Align))
	if options.TopAlign {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	copy(l.key[:], h.Sum(nil))
	return l, nil
}

func paragraph(text string, start int) (int, int) {
	i := strings.IndexAny(text[start:], "\r\n")
	if i < 0 {
		return len(text), len(text)
	}
	end := start + i
	next := end + 1
	if text[end] == '\r' && next < len(text) && text[next] == '\n' {
		next++
	}
	return end, next
}

func spend(budget *int, n int) error {
	if n > *budget {
		return errors.New("text: layout work budget exceeded")
	}
	*budget -= n
	return nil
}

func runs(content *Content, start, end int) []shapedRun {
	var result []shapedRun
	for _, span := range content.spans {
		a, b := max(start, span.start), min(end, span.end)
		if a >= b {
			continue
		}
		if len(result) > 0 && result[len(result)-1].font.key == span.Font.key && result[len(result)-1].end == a {
			result[len(result)-1].end = b
		} else {
			result = append(result, shapedRun{font: span.Font, start: a, end: b})
		}
	}
	return result
}

func measure(content *Content, start, end int, budget *int) (float64, error) {
	if err := spend(budget, end-start); err != nil {
		return 0, err
	}
	width := 0.0
	for _, run := range runs(content, start, end) {
		if run.font.face != nil {
			w, _ := etext.Measure(content.text[run.start:run.end], run.font.face, 0)
			width += w
			continue
		}
		scale := run.font.options.Size / (run.font.source.metrics.Ascent + run.font.source.metrics.Descent)
		for _, r := range content.text[run.start:run.end] {
			g, ok := run.font.source.glyphs[r]
			if !ok {
				return 0, fmt.Errorf("text: bitmap font has no glyph %U", r)
			}
			width += g.Advance * scale
		}
	}
	return width, nil
}

func wrapEnd(content *Content, start, end int, width float64, budget *int) (int, error) {
	g := uniseg.NewGraphemes(content.text[start:end])
	lastFit, lastBreak := start, start
	for g.Next() {
		_, b := g.Positions()
		candidate := start + b
		w, err := measure(content, start, candidate, budget)
		if err != nil {
			return 0, err
		}
		if w > width {
			if lastBreak > start {
				return lastBreak, nil
			}
			if lastFit > start {
				return lastFit, nil
			}
			return candidate, nil
		}
		lastFit = candidate
		if g.LineBreak() != uniseg.LineDontBreak {
			lastBreak = candidate
		}
	}
	return end, nil
}

func (l *Layout) addLine(start, end, next int, y float64, fallback *Font, o LayoutOptions, budget *int) (float64, error) {
	if err := spend(budget, end-start); err != nil {
		return 0, err
	}
	r := runs(l.content, start, end)
	ascent, descent, gap := fallback.metrics.Ascent, fallback.metrics.Descent, fallback.metrics.LineGap
	width := 0.0
	for i := range r {
		f := r[i].font
		m := f.metrics
		ascent = max(ascent, m.Ascent)
		descent = max(descent, m.Descent)
		gap = max(gap, m.LineGap)
		if f.face != nil {
			r[i].glyphs = etext.AppendLazyGlyphs(nil, l.content.text[r[i].start:r[i].end], f.face, &etext.LayoutOptions{})
			r[i].width = etext.AdvanceAt(l.content.text[r[i].start:r[i].end], r[i].end-r[i].start, f.face)
		} else {
			var err error
			r[i].width, err = measure(l.content, r[i].start, r[i].end, budget)
			if err != nil {
				return 0, err
			}
		}
		width += r[i].width
	}
	if !finite(width) || !finite(ascent+descent+gap) || width > 1<<30 {
		return 0, errors.New("text: invalid or excessive layout extent")
	}
	x := 0.0
	if o.Width > 0 {
		switch o.Align {
		case AlignCenter:
			x = (o.Width - width) / 2
		case AlignEnd:
			x = o.Width - width
		}
	}
	height := ascent + descent
	line := Line{start, end, next, Rect{Point{x, y}, Point{x + width, y + height}}, y + ascent}
	l.lines = append(l.lines, line)
	l.bounds.Max.X = max(l.bounds.Max.X, max(width, o.Width))
	l.bounds.Max.Y = max(l.bounds.Max.Y, y+height)
	for _, run := range r {
		f := run.font
		dy := y + ascent - f.metrics.Ascent
		if o.TopAlign {
			dy = y
		}
		if f.face != nil {
			clusters := make(map[[2]int]Rect)
			for _, g := range run.glyphs {
				a, b := run.start+g.StartIndexInBytes, run.start+g.EndIndexInBytes
				if b <= a {
					continue
				}
				pg := placedGlyph{font: f, glyph: g, x: x, y: dy, start: a, end: b, paint: l.paintAt(a)}
				l.glyphs = append(l.glyphs, pg)
				box := Rect{Point{x + min(g.OriginX, g.OriginX+g.AdvanceX), y}, Point{x + max(g.OriginX, g.OriginX+g.AdvanceX), y + height}}
				key := [2]int{a, b}
				if old, ok := clusters[key]; ok {
					box = union(old, box)
				}
				clusters[key] = box
				if !g.ImageBounds.Empty() {
					l.includeInk(Rect{Point{x + float64(g.ImageBounds.Min.X), dy + float64(g.ImageBounds.Min.Y)}, Point{x + float64(g.ImageBounds.Max.X), dy + float64(g.ImageBounds.Max.Y)}})
				}
			}
			keys := make([][2]int, 0, len(clusters))
			for key := range clusters {
				keys = append(keys, key)
			}
			slices.SortFunc(keys, func(a, b [2]int) int {
				if a[0] != b[0] {
					return a[0] - b[0]
				}
				return a[1] - b[1]
			})
			for _, key := range keys {
				l.clusters = append(l.clusters, cluster{key[0], key[1], clusters[key]})
			}
		} else {
			scale := f.options.Size / (f.source.metrics.Ascent + f.source.metrics.Descent)
			gx := x
			for off, ch := range l.content.text[run.start:run.end] {
				g := f.source.glyphs[ch]
				a := run.start + off
				b := a + utf8.RuneLen(ch)
				pg := placedGlyph{font: f, bitmap: g, isBitmap: true, x: gx + g.BearingX*scale, y: dy + f.metrics.Ascent + g.BearingY*scale, scale: scale, start: a, end: b, paint: l.paintAt(a)}
				l.glyphs = append(l.glyphs, pg)
				l.clusters = append(l.clusters, cluster{a, b, Rect{Point{gx, y}, Point{gx + g.Advance*scale, y + height}}})
				if !g.Region.Empty() {
					l.includeInk(Rect{Point{pg.x, pg.y}, Point{pg.x + float64(g.Region.Dx())*scale, pg.y + float64(g.Region.Dy())*scale}})
				}
				gx += g.Advance * scale
			}
		}
		x += run.width
	}
	if len(l.glyphs) > 131072 {
		return 0, errors.New("text: glyph budget exceeded")
	}
	if o.LineSpacing > 0 {
		return o.LineSpacing, nil
	}
	return height + gap, nil
}

func (l *Layout) paintAt(offset int) string {
	for _, s := range l.content.spans {
		if offset >= s.start && offset < s.end {
			return s.Paint
		}
	}
	return ""
}
func union(a, b Rect) Rect {
	return Rect{Point{min(a.Min.X, b.Min.X), min(a.Min.Y, b.Min.Y)}, Point{max(a.Max.X, b.Max.X), max(a.Max.Y, b.Max.Y)}}
}
func (l *Layout) includeInk(r Rect) {
	if l.ink.Width() == 0 && l.ink.Height() == 0 {
		l.ink = r
	} else {
		l.ink = union(l.ink, r)
	}
}
func (l *Layout) Content() *Content { return l.content }
func (l *Layout) Bounds() Rect      { return l.bounds }
func (l *Layout) InkBounds() Rect   { return l.ink }
func (l *Layout) Lines() []Line     { return slices.Clone(l.lines) }

// Hit is local geometric coverage, not an activation or visibility decision.
type Hit struct {
	Content    *Content
	Start, End int
	Bounds     Rect
	Tags       []string
}

func (l *Layout) HitTestLocal(p Point) []Hit {
	var result []Hit
	if !finite(p.X) || !finite(p.Y) {
		return result
	}
	for _, c := range l.clusters {
		if c.rect.Contains(p) {
			result = append(result, Hit{l.content, c.start, c.end, c.rect, l.tags(c.start, c.end)})
		}
	}
	return result
}
func (l *Layout) tags(start, end int) []string {
	var result []string
	for _, s := range l.content.spans {
		if s.start < end && s.end > start && s.Tag != "" && !slices.Contains(result, s.Tag) {
			result = append(result, s.Tag)
		}
	}
	return result
}

// Coverage reports requested versus represented cluster coverage. A range cutting
// a ligature expands to its cluster; it never silently reshapes a substring.
type Coverage struct {
	RequestedStart, RequestedEnd, Start, End int
	Snapped                                  bool
	Rects                                    []Rect
}

func (l *Layout) RangeRects(start, end int) (Coverage, error) {
	result := Coverage{RequestedStart: start, RequestedEnd: end, Start: start, End: end}
	if !l.boundary(start) || !l.boundary(end) || end < start {
		return result, errors.New("text: invalid UTF-8 range")
	}
	if start == end {
		return result, nil
	}
	for _, c := range l.clusters {
		if c.start < end && c.end > start {
			result.Start = min(result.Start, c.start)
			result.End = max(result.End, c.end)
			result.Rects = append(result.Rects, c.rect)
		}
	}
	result.Snapped = result.Start != start || result.End != end
	return result, nil
}
func (l *Layout) boundary(offset int) bool {
	return offset >= 0 && offset <= len(l.content.text) && (offset == len(l.content.text) || utf8.RuneStart(l.content.text[offset]))
}

// Caret reports a supported visual site, with explicit source snapping. Upstream
// selects the preceding visual line at a soft wrap; default selects the next.
type Caret struct {
	Content                   *Content
	Requested, Offset         int
	Position                  Point
	Height                    float64
	Snapped, Inside, Upstream bool
}

func (l *Layout) CaretAt(offset int, upstream bool) (Caret, error) {
	if !l.boundary(offset) {
		return Caret{}, errors.New("text: invalid caret offset")
	}
	for i, line := range l.lines {
		if offset < line.Start || offset > line.Next {
			continue
		}
		if offset == line.End && line.Next == line.End && i+1 < len(l.lines) && !upstream {
			continue
		}
		if offset == line.Next && line.Next > line.End && i+1 < len(l.lines) {
			continue
		}
		x := line.Bounds.Max.X
		resolved := min(offset, line.End)
		for _, c := range l.clusters {
			if c.start >= line.Start && c.end <= line.End && offset >= c.start && offset < c.end {
				x = c.rect.Min.X
				resolved = c.start
				break
			}
		}
		return Caret{l.content, offset, resolved, Point{x, line.Bounds.Min.Y}, line.Bounds.Height(), resolved != offset, true, upstream}, nil
	}
	return Caret{}, errors.New("text: caret has no line")
}

func (l *Layout) NearestCaret(p Point) (Caret, error) {
	if !finite(p.X) || !finite(p.Y) {
		return Caret{}, errors.New("text: nonfinite point")
	}
	best := math.Inf(1)
	var result Caret
	consider := func(offset int, x float64, line Line, upstream bool) {
		dy := max(line.Bounds.Min.Y-p.Y, 0, p.Y-line.Bounds.Max.Y)
		d := (x-p.X)*(x-p.X) + dy*dy
		if d < best {
			best = d
			result = Caret{Content: l.content, Requested: offset, Offset: offset, Position: Point{x, line.Bounds.Min.Y}, Height: line.Bounds.Height(), Inside: line.Bounds.Contains(p), Upstream: upstream}
		}
	}
	for _, line := range l.lines {
		consider(line.Start, line.Bounds.Min.X, line, false)
		for _, c := range l.clusters {
			if c.start >= line.Start && c.end <= line.End {
				consider(c.start, c.rect.Min.X, line, false)
				consider(c.end, c.rect.Max.X, line, c.end == line.End)
			}
		}
		consider(line.End, line.Bounds.Max.X, line, true)
	}
	return result, nil
}
