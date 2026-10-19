package editorui

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// textCursor edits a caller-owned draft. Offsets are UTF-8 byte boundaries at
// grapheme edges, so deletion never splits an accent or joined emoji. observed
// detects replacement of the entire draft by its owner; rendering is read-only.
type textCursor struct {
	observed string
	offset   int
	anchor   int
	column   int
}

// Printable rune batches can spell key names ("enter", "ctrl+s"). Only the
// decoder's control-key types may perform workflow actions during text entry.
func textControlKey(key tea.KeyMsg) string {
	if key.Type == tea.KeyRunes && !key.Alt {
		return ""
	}

	return key.String()
}

func (cursor *textCursor) reset(value string) {
	cursor.observed, cursor.offset, cursor.anchor, cursor.column = value, len(value), -1, -1
}

func (cursor textCursor) position(value string) int {
	if cursor.observed != value {
		return len(value)
	}

	return min(cursor.offset, len(value))
}

func (cursor textCursor) selection(value string) (int, int) {
	position := cursor.position(value)
	if cursor.observed != value || cursor.anchor < 0 {
		return position, position
	}

	return min(position, cursor.anchor), max(position, cursor.anchor)
}

func (cursor *textCursor) replace(value *string, text string) {
	start, end := cursor.selection(*value)
	*value = (*value)[:start] + text + (*value)[end:]
	cursor.offset, cursor.anchor, cursor.observed, cursor.column = start+len(text), -1, *value, -1
	// Inserting a combining/joining character can merge with the suffix.
	boundaries := textBoundaries(*value)
	index, _ := slices.BinarySearch(boundaries, cursor.offset)
	cursor.offset = boundaries[index]
}

func (cursor *textCursor) move(value string, offset int, extend bool) {
	if extend && cursor.anchor < 0 {
		cursor.anchor = cursor.offset
	} else if !extend {
		cursor.anchor = -1
	}
	cursor.offset = max(0, min(len(value), offset))
}

func textBoundaries(value string) []int {
	boundaries := []int{0}
	graphemes := uniseg.NewGraphemes(value)
	for graphemes.Next() {
		_, end := graphemes.Positions()
		boundaries = append(boundaries, end)
	}

	return boundaries
}

func textNeighbor(value string, position, direction int) int {
	boundaries := textBoundaries(value)
	index, _ := slices.BinarySearch(boundaries, position)

	return boundaries[max(0, min(len(boundaries)-1, index+direction))]
}

func textWord(value string, position, direction int) int {
	boundaries := textBoundaries(value)
	index, _ := slices.BinarySearch(boundaries, position)
	space := func(index int) bool {
		char, _ := utf8.DecodeRuneInString(value[boundaries[index]:])

		return unicode.IsSpace(char)
	}
	if direction < 0 {
		for index > 0 && space(index-1) {
			index--
		}
		for index > 0 && !space(index-1) {
			index--
		}
	} else {
		for index < len(boundaries)-1 && !space(index) {
			index++
		}
		for index < len(boundaries)-1 && space(index) {
			index++
		}
	}

	return boundaries[index]
}

func textLine(value string, position int) (int, int) {
	start := strings.LastIndex(value[:position], "\n") + 1
	end := len(value)
	if newline := strings.IndexByte(value[position:], '\n'); newline >= 0 {
		end = position + newline
		if end > start && value[end-1] == '\r' {
			end-- // Existing CRLF notes retain one indivisible newline grapheme.
		}
	}

	return start, end
}

func textVertical(value string, position, direction, column int) int {
	start, end := textLine(value, position)
	if direction < 0 {
		if start == 0 {
			return position
		}
		start, end = textLine(value, start-1)
	} else {
		if end == len(value) {
			return position
		}
		start, end = textLine(value, textNeighbor(value, end, 1))
	}
	if column == 0 {
		return start
	}
	graphemes, width := uniseg.NewGraphemes(value[start:end]), 0
	for graphemes.Next() {
		width += graphemes.Width()
		if width >= column {
			_, offset := graphemes.Positions()

			return start + offset
		}
	}

	return end
}

func cleanTypedText(text string, multiline bool) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")

	return strings.Map(func(char rune) rune {
		switch {
		case char == '\n' && multiline:
			return char
		case char == '\n' || char == '\t':
			return ' '
		case unicode.IsControl(char):
			return -1
		default:
			return char
		}
	}, text)
}

// edit consumes text, navigation and deletion only. Apply/Cancel and focus
// changes belong to the owning workflow, never to this reusable text component.
func (cursor *textCursor) edit(value *string, key tea.KeyMsg, multiline bool) {
	if cursor.observed != *value {
		cursor.reset(*value)
	}
	if key.Type == tea.KeyRunes && !key.Alt {
		if text := cleanTypedText(string(key.Runes), multiline); text != "" {
			cursor.replace(value, text)
		}

		return
	}
	if cursor.navigate(*value, key.String(), multiline) {
		return
	}
	start, end := cursor.selection(*value)
	switch key.String() {
	case " ":
		cursor.replace(value, " ")
	case keyEnter:
		if multiline {
			cursor.replace(value, "\n")
		}
	case "ctrl+u":
		*value = ""
		cursor.reset("")
	case keyBackspace, "ctrl+h", keyDelete, "ctrl+d", "ctrl+w", "alt+backspace", "alt+d", "ctrl+k":
		if start == end {
			cursor.anchor = cursor.deletionEnd(*value, key.String())
		}
		cursor.replace(value, "")
	}
}

//nolint:cyclop // One text-navigation binding table keeps terminal conventions auditable.
func (cursor *textCursor) navigate(value, key string, multiline bool) bool {
	position, extend := cursor.offset, strings.HasPrefix(key, "shift+")
	start, end := cursor.selection(value)
	if start != end && (key == keyLeft || key == keyRight) {
		cursor.column = -1
		if key == keyLeft {
			cursor.move(value, start, false)
		} else {
			cursor.move(value, end, false)
		}

		return true
	}
	switch key {
	case keyLeft, keyShiftLeft:
		position = textNeighbor(value, position, -1)
	case keyRight, keyShiftRight:
		position = textNeighbor(value, position, 1)
	case "alt+left", "ctrl+left", "alt+b":
		position = textWord(value, position, -1)
	case "alt+right", "ctrl+right", "alt+f":
		position = textWord(value, position, 1)
	case "home", "ctrl+a":
		position, _ = textLine(value, position)
	case "end", "ctrl+e":
		_, position = textLine(value, position)
	case "ctrl+home":
		position = 0
	case "ctrl+end":
		position = len(value)
	case "up", keyShiftUp, keyDown, keyShiftDown:
		if !multiline {
			return false
		}
		direction := 1
		if key == "up" || key == keyShiftUp {
			direction = -1
		}
		if cursor.column < 0 {
			lineStart, _ := textLine(value, position)
			cursor.column = uniseg.StringWidth(value[lineStart:position])
		}
		position = textVertical(value, position, direction, cursor.column)
	default:
		return false
	}
	if key != "up" && key != keyShiftUp && key != keyDown && key != keyShiftDown {
		cursor.column = -1
	}
	cursor.move(value, position, extend)

	return true
}

func (cursor textCursor) deletionEnd(value, key string) int {
	switch key {
	case keyBackspace, "ctrl+h":
		return textNeighbor(value, cursor.offset, -1)
	case "ctrl+w", "alt+backspace":
		return textWord(value, cursor.offset, -1)
	case "alt+d":
		return textWord(value, cursor.offset, 1)
	case "ctrl+k":
		_, end := textLine(value, cursor.offset)
		if end == cursor.offset {
			return textNeighbor(value, end, 1)
		}

		return end
	default:
		return textNeighbor(value, cursor.offset, 1)
	}
}

func (cursor textCursor) render(value string, selected func(...string) string) string {
	start, end := cursor.selection(value)
	if start != end {
		middle := selected(displayText(value[start:end]))
		if cursor.position(value) == start {
			return displayText(value[:start]) + "▏" + middle + displayText(value[end:])
		}

		return displayText(value[:start]) + middle + "▏" + displayText(value[end:])
	}

	return displayText(value[:start]) + "▏" + displayText(value[start:])
}

func displayText(value string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = safe(line)
	}

	return strings.Join(lines, "\n")
}

func (cursor textCursor) viewport(value string, width int, selected func(...string) string) string {
	caretColumn := ansi.StringWidth(safe(value[:cursor.position(value)]))
	start := max(0, caretColumn-max(1, width)+1)
	columns := 0
	graphemes := uniseg.NewGraphemes(safe(value))
	for columns < start && graphemes.Next() {
		columns += graphemes.Width()
	}

	return ansi.Truncate(ansi.Cut(cursor.render(value, selected), columns, columns+max(1, width)), max(1, width), "")
}
