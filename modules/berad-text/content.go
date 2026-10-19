package text

import (
	"crypto/sha256"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

// Span supplies text and typography. Paint names are resolved at draw time;
// annotations are inert host identities, never callbacks. Bitmap font mappings
// can include U+FFFC (or another explicit rune) for inline icons.
type Span struct {
	Text  string
	Font  *Font
	Paint string
	Tag   string
}

type contentSpan struct {
	Span
	start, end int
}

// Content is a detached presentation snapshot, not an editing/undo model.
type Content struct {
	text  string
	spans []contentSpan
	key   [32]byte
}

// NewContent preserves valid UTF-8 including authored spaces/newlines. At most
// 1 MiB of text and 65536 spans are accepted. Empty spans may supply empty-line metrics.
func NewContent(spans []Span) (*Content, error) {
	if len(spans) > 65536 {
		return nil, errors.New("text: too many spans")
	}
	c := &Content{}
	var b strings.Builder
	metadataBytes := 0
	h := sha256.New()
	for _, s := range spans {
		if s.Font == nil || !utf8.ValidString(s.Text) || len(s.Text)+len(s.Paint)+len(s.Tag) > 1<<20-b.Len()-metadataBytes {
			return nil, errors.New("text: content requires fonts, valid UTF-8 and at most 1 MiB")
		}
		s.Text = strings.Clone(s.Text)
		s.Paint = strings.Clone(s.Paint)
		s.Tag = strings.Clone(s.Tag)
		metadataBytes += len(s.Paint) + len(s.Tag)
		start := b.Len()
		b.WriteString(s.Text)
		c.spans = append(c.spans, contentSpan{s, start, b.Len()})
		h.Write(s.Font.key[:])
		writeNumbers(h, float64(len(s.Text)), float64(len(s.Paint)), float64(len(s.Tag)))
		h.Write([]byte(s.Text))
		h.Write([]byte(s.Paint))
		h.Write([]byte(s.Tag))
	}
	c.text = b.String()
	copy(c.key[:], h.Sum(nil))
	return c, nil
}

func (c *Content) Text() string { return c.text }

// Spans returns a detached slice; Fonts are immutable shared values.
func (c *Content) Spans() []Span {
	result := make([]Span, len(c.spans))
	for i, s := range c.spans {
		result[i] = s.Span
	}
	return result
}

// Log retains complete logical entries, independent of wrapping and viewport
// capacity. An oversized entry is rejected, not partially rewritten. Appending
// evicts oldest complete entries and reports how many were dropped.
type Log struct {
	entries                     []*Content
	maxEntries, maxBytes, bytes int
}

func NewLog(maxEntries, maxBytes int) (*Log, error) {
	if maxEntries <= 0 || maxBytes <= 0 {
		return nil, errors.New("text: log limits must be positive")
	}
	return &Log{maxEntries: maxEntries, maxBytes: maxBytes}, nil
}

func (l *Log) Append(content *Content) (int, error) {
	if content == nil || len(content.text) > l.maxBytes {
		return 0, errors.New("text: entry exceeds log budget")
	}
	dropped := 0
	for len(l.entries) > 0 && (len(l.entries) >= l.maxEntries || l.bytes > l.maxBytes-len(content.text)) {
		l.bytes -= len(l.entries[0].text)
		l.entries[0] = nil
		l.entries = l.entries[1:]
		dropped++
	}
	l.entries = append(l.entries, content)
	l.bytes += len(content.text)
	return dropped, nil
}

func (l *Log) Entries() []*Content { return slices.Clone(l.entries) }
func (l *Log) Clear()              { clear(l.entries); l.entries = nil; l.bytes = 0 }

// Panel retains one complete logical snapshot. It never discards earlier rows
// because text wrapped beyond the visible rectangle.
type Panel struct{ content *Content }

func (p *Panel) Set(content *Content) { p.content = content }
func (p *Panel) Content() *Content    { return p.content }
func (p *Panel) Clear()               { p.content = nil }
