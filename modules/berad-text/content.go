package text

import (
	"crypto/sha256"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

const maxContentSpans = 65536

var (
	errContentSpans = errors.New("text: too many spans")
	errContent      = errors.New("text: content requires fonts, valid UTF-8 and at most 1 MiB")
	errLogLimits    = errors.New("text: log limits must be positive")
	errLogEntry     = errors.New("text: entry exceeds log budget")
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
	if len(spans) > maxContentSpans {
		return nil, errContentSpans
	}
	content := new(Content)
	var builder strings.Builder
	metadataBytes := 0
	digest := sha256.New()
	for _, span := range spans {
		if span.Font == nil || !utf8.ValidString(span.Text) ||
			len(span.Text)+len(span.Paint)+len(span.Tag) > 1<<20-builder.Len()-metadataBytes {
			return nil, errContent
		}
		span.Text = strings.Clone(span.Text)
		span.Paint = strings.Clone(span.Paint)
		span.Tag = strings.Clone(span.Tag)
		metadataBytes += len(span.Paint) + len(span.Tag)
		start := builder.Len()
		builder.WriteString(span.Text)
		content.spans = append(content.spans, contentSpan{span, start, builder.Len()})
		digest.Write(span.Font.key[:])
		writeNumbers(digest, float64(len(span.Text)), float64(len(span.Paint)), float64(len(span.Tag)))
		digest.Write([]byte(span.Text))
		digest.Write([]byte(span.Paint))
		digest.Write([]byte(span.Tag))
	}
	content.text = builder.String()
	copy(content.key[:], digest.Sum(nil))

	return content, nil
}

// Text returns the unchanged UTF-8 source.
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

// NewLog requires positive complete-entry and source-text byte limits.
func NewLog(maxEntries, maxBytes int) (*Log, error) {
	if maxEntries <= 0 || maxBytes <= 0 {
		return nil, errLogLimits
	}

	return &Log{maxEntries: maxEntries, maxBytes: maxBytes, entries: nil, bytes: 0}, nil
}

// Append retains a complete snapshot and reports the number of oldest entries evicted.
func (l *Log) Append(content *Content) (int, error) {
	if content == nil || len(content.text) > l.maxBytes {
		return 0, errLogEntry
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

// Entries returns a detached slice of immutable snapshots.
func (l *Log) Entries() []*Content { return slices.Clone(l.entries) }

// Clear releases all retained entries.
func (l *Log) Clear() { clear(l.entries); l.entries = nil; l.bytes = 0 }

// Panel retains one complete logical snapshot. It never discards earlier rows
// because text wrapped beyond the visible rectangle.
type Panel struct{ content *Content }

// Set replaces the retained snapshot; nil clears the panel.
func (p *Panel) Set(content *Content) { p.content = content }

// Content returns the retained immutable snapshot, or nil.
func (p *Panel) Content() *Content { return p.content }

// Clear releases the retained snapshot.
func (p *Panel) Clear() { p.content = nil }
