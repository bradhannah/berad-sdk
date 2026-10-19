package editorui

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestTextCursorKeepsGraphemesWholeAndSelectionVisible(t *testing.T) {
	value := "Ae\u0301👩‍🔬界"
	var cursor textCursor
	cursor.reset(value)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyBackspace}, false)
	require.Equal(t, "Ae\u0301👩‍🔬", value)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyShiftLeft}, false)
	require.Contains(t, cursor.render(value, func(parts ...string) string { return "[" + strings.Join(parts, "") + "]" }),
		"▏[👩‍🔬]")
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("B")}, false)
	require.Equal(t, "Ae\u0301B", value)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyLeft}, false)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyBackspace}, false)
	require.Equal(t, "AB", value)
}

func TestTextCursorWordLineAndPasteNavigation(t *testing.T) {
	value := "first word\nsecond line"
	var cursor textCursor
	cursor.reset(value)
	require.Equal(t, value+"▏", cursor.render(value, func(parts ...string) string { return strings.Join(parts, "") }))
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyHome}, true)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyUp}, true)
	require.Zero(t, cursor.offset)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true}, true)
	require.Equal(t, len("first "), cursor.offset)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("left\r\n\x1bX"), Paste: true}, true)
	require.Equal(t, "first left\nXword\nsecond line", value, "pasted key names are data, not commands")
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyCtrlW}, true)
	require.Equal(t, "first left\nword\nsecond line", value)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyEnd}, true)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyCtrlK}, true)
	require.Equal(t, "first left\nwordsecond line", value)
}

func TestTextCursorSelectionCollapsesAndViewportFollowsCaret(t *testing.T) {
	value := strings.Repeat("界", 20)
	var cursor textCursor
	cursor.reset(value)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyShiftLeft}, false)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyShiftLeft}, false)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyRight}, false)
	require.Equal(t, len(value), cursor.offset)
	paint := func(parts ...string) string { return strings.Join(parts, "") }
	require.Contains(t, cursor.viewport(value, 12, paint), "▏")
	require.LessOrEqual(t, ansi.StringWidth(cursor.viewport(value, 12, paint)), 12)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyHome}, false)
	require.True(t, strings.HasPrefix(cursor.viewport(value, 12, paint), "▏"))
}

func TestMultilineCursorRetainsColumnAcrossShortLines(t *testing.T) {
	value := "long line\nx\nlong line"
	var cursor textCursor
	cursor.reset(value)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyUp}, true)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyUp}, true)
	require.Equal(t, len("long line"), cursor.offset)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyDown}, true)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyDown}, true)
	require.Equal(t, len(value), cursor.offset)
}

func TestExistingCRLFRemainsOneNewlineDuringNavigationAndDeletion(t *testing.T) {
	value := "one\r\ntwo"
	var cursor textCursor
	cursor.reset(value)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyHome}, true)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyUp}, true)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyEnd}, true)
	require.Equal(t, len("one"), cursor.offset)
	cursor.edit(&value, tea.KeyMsg{Type: tea.KeyCtrlK}, true)
	require.Equal(t, "onetwo", value)
}

func FuzzTextCursorNeverSplitsUTF8OrGraphemes(f *testing.F) {
	f.Add("e\u0301👩🔬", []byte{0, 1, 4, 2, 6, 3})
	f.Add("a\nb\n", []byte{3, 3, 2, 5, 6, 1})
	f.Add("one\r\ntwo", []byte{7, 5, 8, 9})
	f.Fuzz(func(t *testing.T, source string, actions []byte) {
		if len(source) > 4096 || len(actions) > 128 {
			t.Skip()
		}
		value := strings.ToValidUTF8(source, "�")
		var cursor textCursor
		cursor.reset(value)
		keys := []tea.KeyMsg{
			{Type: tea.KeyLeft}, {Type: tea.KeyRight}, {Type: tea.KeyBackspace}, {Type: tea.KeyDelete},
			{Type: tea.KeyShiftLeft}, {Type: tea.KeyUp}, {Type: tea.KeyRunes, Runes: []rune("\u200de\u0301")},
			{Type: tea.KeyHome}, {Type: tea.KeyEnd}, {Type: tea.KeyCtrlK},
		}
		for _, action := range actions {
			cursor.edit(&value, keys[int(action)%len(keys)], true)
			require.True(t, utf8.ValidString(value))
			require.True(t, slices.Contains(textBoundaries(value), cursor.offset))
			if cursor.anchor >= 0 {
				require.True(t, slices.Contains(textBoundaries(value), cursor.anchor))
			}
		}
	})
}
