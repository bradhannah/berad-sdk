package text

import (
	"crypto/sha256"
	"errors"
	"strings"
	"unicode/utf8"
)

const maxContentSpans = 65536

var (
	errContentSpans = errors.New("text: too many spans")
	errContent      = errors.New("text: content requires fonts, valid UTF-8 and at most 1 MiB")
)

// Span supplies text and typography. Paint names are resolved at draw time;
// bitmap font mappings can include U+FFFC (or another explicit rune) for inline icons.
type Span struct {
	Text  string
	Font  *Font
	Paint string
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
			len(span.Text)+len(span.Paint) > 1<<20-builder.Len()-metadataBytes {
			return nil, errContent
		}
		span.Text = strings.Clone(span.Text)
		span.Paint = strings.Clone(span.Paint)
		metadataBytes += len(span.Paint)
		start := builder.Len()
		builder.WriteString(span.Text)
		content.spans = append(content.spans, contentSpan{span, start, builder.Len()})
		digest.Write(span.Font.key[:])
		writeNumbers(digest, float64(len(span.Text)), float64(len(span.Paint)))
		digest.Write([]byte(span.Text))
		digest.Write([]byte(span.Paint))
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
