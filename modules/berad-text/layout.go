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

const (
	defaultLayoutWork = 8 << 20
	maxLayoutLines    = 8192
	maxLayoutGlyphs   = 131072
	alignDivisor      = 2
)

var (
	errLayoutOptions = errors.New("text: invalid content/layout options")
	errDefaultFont   = errors.New("text: empty layout requires a default font")
	errLineLimit     = errors.New("text: layout exceeds 8192 lines")
	errLayoutWork    = errors.New("text: layout work budget exceeded")
	errLayoutExtent  = errors.New("text: invalid or excessive layout extent")
	errGlyphLimit    = errors.New("text: glyph budget exceeded")
	errRange         = errors.New("text: invalid UTF-8 range")
	errCaretOffset   = errors.New("text: invalid caret offset")
	errCaretLine     = errors.New("text: caret has no line")
	errPoint         = errors.New("text: nonfinite point")
)

// Point and Rect use layout-local pixels, independent of camera/window coordinates.
type Point struct{ X, Y float64 }

// Rect is a half-open rectangle in layout-local pixels.
type Rect struct{ Min, Max Point }

// Width returns the horizontal extent.
func (r Rect) Width() float64 { return r.Max.X - r.Min.X }

// Height returns the vertical extent.
func (r Rect) Height() float64 { return r.Max.Y - r.Min.Y }

// Contains reports half-open point coverage.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.Min.X && p.Y >= r.Min.Y && p.X < r.Max.X && p.Y < r.Max.Y
}

// Align selects horizontal placement within a supplied width.
type Align uint8

// Alignment values retain source order; they do not select text direction.
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
	if content == nil || !validLayoutOptions(options) {
		return nil, errLayoutOptions
	}
	font := options.DefaultFont
	if font == nil && len(content.spans) > 0 {
		font = content.spans[0].Font
	}
	if font == nil {
		return nil, errDefaultFont
	}
	budget := options.MaxWork
	if budget == 0 {
		budget = defaultLayoutWork
	}
	layout := new(Layout)
	layout.content = content
	if err := layout.buildLines(font, options, &budget); err != nil {
		return nil, err
	}
	digest := sha256.New()
	digest.Write(content.key[:])
	digest.Write(font.key[:])
	writeNumbers(digest, options.Width, options.LineSpacing, float64(options.Align))
	if options.TopAlign {
		digest.Write([]byte{1})
	} else {
		digest.Write([]byte{0})
	}
	copy(layout.key[:], digest.Sum(nil))

	return layout, nil
}

func validLayoutOptions(options LayoutOptions) bool {
	return finite(options.Width) && options.Width >= 0 && finite(options.LineSpacing) &&
		options.LineSpacing >= 0 && options.Align <= AlignEnd && options.MaxWork >= 0
}

func (l *Layout) buildLines(font *Font, options LayoutOptions, budget *int) error {
	start := 0
	positionY := 0.0
	for {
		end, next := paragraph(l.content.text, start)
		for {
			lineEnd := end
			if options.Width > 0 && start < end {
				var err error
				lineEnd, err = wrapEnd(l.content, start, end, options.Width, budget)
				if err != nil {
					return err
				}
			}
			lineNext := lineEnd
			if lineEnd == end {
				lineNext = next
			}
			height, err := l.addLine(start, lineEnd, lineNext, positionY, font, options, budget)
			if err != nil {
				return err
			}
			if len(l.lines) > maxLayoutLines {
				return errLineLimit
			}
			positionY += height
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

	return nil
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
		return errLayoutWork
	}
	*budget -= n

	return nil
}

func runs(content *Content, start, end int) []shapedRun {
	var result []shapedRun
	for _, span := range content.spans {
		spanStart, spanEnd := max(start, span.start), min(end, span.end)
		if spanStart >= spanEnd {
			continue
		}
		if len(result) > 0 && result[len(result)-1].font.key == span.Font.key && result[len(result)-1].end == spanStart {
			result[len(result)-1].end = spanEnd
		} else {
			result = append(result, shapedRun{font: span.Font, start: spanStart, end: spanEnd, width: 0, glyphs: nil})
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
				return 0, fmt.Errorf("%w %U", errMissingGlyph, r)
			}
			width += g.Advance * scale
		}
	}

	return width, nil
}

func wrapEnd(content *Content, start, end int, width float64, budget *int) (int, error) {
	graphemes := uniseg.NewGraphemes(content.text[start:end])
	lastFit, lastBreak := start, start
	for graphemes.Next() {
		_, b := graphemes.Positions()
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
		if graphemes.LineBreak() != uniseg.LineDontBreak {
			lastBreak = candidate
		}
	}

	return end, nil
}

func (l *Layout) shapeRuns(start, end int, fallback *Font, budget *int) ([]shapedRun, Metrics, float64, error) {
	if err := spend(budget, end-start); err != nil {
		return nil, fallback.metrics, 0, err
	}
	shaped := runs(l.content, start, end)
	ascent, descent, gap := fallback.metrics.Ascent, fallback.metrics.Descent, fallback.metrics.LineGap
	width := 0.0
	for index := range shaped {
		run := &shaped[index]
		metrics := run.font.metrics
		ascent = max(ascent, metrics.Ascent)
		descent = max(descent, metrics.Descent)
		gap = max(gap, metrics.LineGap)
		if run.font.face != nil {
			value := l.content.text[run.start:run.end]
			run.glyphs = etext.AppendLazyGlyphs(nil, value, run.font.face, new(etext.LayoutOptions))
			run.width = etext.AdvanceAt(value, len(value), run.font.face)
		} else {
			var err error
			run.width, err = measure(l.content, run.start, run.end, budget)
			if err != nil {
				return nil, fallback.metrics, 0, err
			}
		}
		width += run.width
	}
	if !finite(width) || !finite(ascent+descent+gap) || width > 1<<30 {
		return nil, fallback.metrics, 0, errLayoutExtent
	}

	return shaped, Metrics{ascent, descent, gap}, width, nil
}

func (l *Layout) addLine(
	start, end, next int, positionY float64, fallback *Font, options LayoutOptions, budget *int,
) (float64, error) {
	shaped, metrics, width, err := l.shapeRuns(start, end, fallback, budget)
	if err != nil {
		return 0, err
	}
	positionX := 0.0
	if options.Width > 0 {
		switch options.Align {
		case AlignStart:
			// Keep the natural origin.
		case AlignCenter:
			positionX = (options.Width - width) / alignDivisor
		case AlignEnd:
			positionX = options.Width - width
		}
	}
	height := metrics.Ascent + metrics.Descent
	bounds := Rect{Point{positionX, positionY}, Point{positionX + width, positionY + height}}
	line := Line{start, end, next, bounds, positionY + metrics.Ascent}
	l.lines = append(l.lines, line)
	l.bounds.Max.X = max(l.bounds.Max.X, max(width, options.Width))
	l.bounds.Max.Y = max(l.bounds.Max.Y, positionY+height)
	for _, run := range shaped {
		offsetY := positionY + metrics.Ascent - run.font.metrics.Ascent
		if options.TopAlign {
			offsetY = positionY
		}
		if run.font.face != nil {
			l.placeOutlineRun(run, positionX, offsetY, line)
		} else {
			l.placeBitmapRun(run, positionX, offsetY, line)
		}
		positionX += run.width
	}
	if len(l.glyphs) > maxLayoutGlyphs {
		return 0, errGlyphLimit
	}
	if options.LineSpacing > 0 {
		return options.LineSpacing, nil
	}

	return height + metrics.LineGap, nil
}

func (l *Layout) placeOutlineRun(run shapedRun, positionX, offsetY float64, line Line) {
	clusters := make(map[[2]int]Rect)
	for _, glyph := range run.glyphs {
		start, end := run.start+glyph.StartIndexInBytes, run.start+glyph.EndIndexInBytes
		if end <= start {
			continue
		}
		var placed placedGlyph
		placed.font, placed.glyph = run.font, glyph
		placed.x, placed.y, placed.start, placed.end, placed.paint = positionX, offsetY, start, end, l.paintAt(start)
		l.glyphs = append(l.glyphs, placed)
		box := Rect{
			Point{positionX + min(glyph.OriginX, glyph.OriginX+glyph.AdvanceX), line.Bounds.Min.Y},
			Point{positionX + max(glyph.OriginX, glyph.OriginX+glyph.AdvanceX), line.Bounds.Max.Y},
		}
		key := [2]int{start, end}
		if old, ok := clusters[key]; ok {
			box = union(old, box)
		}
		clusters[key] = box
		if !glyph.ImageBounds.Empty() {
			l.includeInk(Rect{
				Point{positionX + float64(glyph.ImageBounds.Min.X), offsetY + float64(glyph.ImageBounds.Min.Y)},
				Point{positionX + float64(glyph.ImageBounds.Max.X), offsetY + float64(glyph.ImageBounds.Max.Y)},
			})
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
}

func (l *Layout) placeBitmapRun(run shapedRun, positionX, offsetY float64, line Line) {
	font := run.font
	scale := font.options.Size / (font.source.metrics.Ascent + font.source.metrics.Descent)
	for offset, character := range l.content.text[run.start:run.end] {
		glyph := font.source.glyphs[character]
		start := run.start + offset
		end := start + utf8.RuneLen(character)
		var placed placedGlyph
		placed.font, placed.bitmap, placed.isBitmap, placed.scale = font, glyph, true, scale
		placed.x, placed.y = positionX+glyph.BearingX*scale, offsetY+font.metrics.Ascent+glyph.BearingY*scale
		placed.start, placed.end, placed.paint = start, end, l.paintAt(start)
		l.glyphs = append(l.glyphs, placed)
		box := Rect{Point{positionX, line.Bounds.Min.Y}, Point{positionX + glyph.Advance*scale, line.Bounds.Max.Y}}
		l.clusters = append(l.clusters, cluster{start, end, box})
		if !glyph.Region.Empty() {
			l.includeInk(Rect{Point{placed.x, placed.y},
				Point{placed.x + float64(glyph.Region.Dx())*scale, placed.y + float64(glyph.Region.Dy())*scale}})
		}
		positionX += glyph.Advance * scale
	}
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

// Content returns the retained immutable source snapshot.
func (l *Layout) Content() *Content { return l.content }

// Bounds returns logical placement extents.
func (l *Layout) Bounds() Rect { return l.bounds }

// InkBounds includes glyph bearings and overhangs.
func (l *Layout) InkBounds() Rect { return l.ink }

// Lines returns detached source ranges and placement metrics.
func (l *Layout) Lines() []Line { return slices.Clone(l.lines) }

// Hit is local geometric coverage, not an activation or visibility decision.
type Hit struct {
	Content    *Content
	Start, End int
	Bounds     Rect
	Tags       []string
}

// HitTestLocal returns geometric candidates; host routing decides eligibility.
func (l *Layout) HitTestLocal(point Point) []Hit {
	var result []Hit
	if !finite(point.X) || !finite(point.Y) {
		return result
	}
	for _, c := range l.clusters {
		if c.rect.Contains(point) {
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

// RangeRects expands requested UTF-8 ranges to the represented shaping clusters.
func (l *Layout) RangeRects(start, end int) (Coverage, error) {
	result := Coverage{RequestedStart: start, RequestedEnd: end, Start: start, End: end, Snapped: false, Rects: nil}
	if !l.boundary(start) || !l.boundary(end) || end < start {
		return result, errRange
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
	return offset >= 0 && offset <= len(l.content.text) &&
		(offset == len(l.content.text) || utf8.RuneStart(l.content.text[offset]))
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

// CaretAt resolves an offset to a supported site with explicit wrap affinity.
func (l *Layout) CaretAt(offset int, upstream bool) (Caret, error) {
	if !l.boundary(offset) {
		return Caret{}, errCaretOffset
	}
	for index, line := range l.lines {
		if !lineAcceptsCaret(line, index+1 < len(l.lines), offset, upstream) {
			continue
		}
		positionX := line.Bounds.Max.X
		resolved := min(offset, line.End)
		for _, c := range l.clusters {
			if c.start >= line.Start && c.end <= line.End && offset >= c.start && offset < c.end {
				positionX = c.rect.Min.X
				resolved = c.start

				break
			}
		}

		return Caret{l.content, offset, resolved, Point{positionX, line.Bounds.Min.Y},
			line.Bounds.Height(), resolved != offset, true, upstream}, nil
	}

	return Caret{}, errCaretLine
}

func lineAcceptsCaret(line Line, hasNext bool, offset int, upstream bool) bool {
	return offset >= line.Start && offset <= line.Next &&
		(offset != line.End || line.Next != line.End || !hasNext || upstream) &&
		(offset != line.Next || line.Next <= line.End || !hasNext)
}

// NearestCaret returns the nearest supported cluster site, including outside points.
func (l *Layout) NearestCaret(point Point) (Caret, error) {
	if !finite(point.X) || !finite(point.Y) {
		return Caret{}, errPoint
	}
	best := math.Inf(1)
	var result Caret
	consider := func(offset int, x float64, line Line, upstream bool) {
		dy := max(line.Bounds.Min.Y-point.Y, 0, point.Y-line.Bounds.Max.Y)
		d := (x-point.X)*(x-point.X) + dy*dy
		if d < best {
			best = d
			result = Caret{Content: l.content, Requested: offset, Offset: offset, Position: Point{x, line.Bounds.Min.Y},
				Height: line.Bounds.Height(), Inside: line.Bounds.Contains(point), Upstream: upstream, Snapped: false}
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
