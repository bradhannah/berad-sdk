package editorui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

func TestResponsiveLayoutUsesEveryRowAndKeepsHintsAtBottom(t *testing.T) {
	model := fixture(t)
	for _, size := range [][2]int{{60, 24}, {80, 24}, {100, 40}, {160, 70}} {
		for _, focus := range []panelFocus{focusBrowser, focusWave, focusDetails, focusMix} {
			t.Run(fmt.Sprintf("%dx%d-focus%d", size[0], size[1], focus), func(t *testing.T) {
				model.width, model.height, model.focus = size[0], size[1], focus
				fillSoundscape(t, model)
				assertFullScreen(t, model)
				model.clearSoundscape()
				model.focus = focus
				assertFullScreen(t, model)
			})
		}
	}
	model.height = 70
	require.Greater(t, model.listHeight(), 30, "tall terminals must not retain the old ten-row cap")
	model.field = "search"
	model.input = "healing"
	assertFullScreen(t, model)
	model.field = ""
	model.openPicker()
	assertFullScreen(t, model)
	model.picker = nil
	finish(model, model.loadSelection())
	model.width, model.height = 60, 24
	model.beginDuplicate()
	assertFullScreen(t, model)
	footer := ansi.Strip(strings.Join(model.footerLines(), "\n"))
	require.Contains(t, footer, "c create")
	require.Contains(t, footer, "Esc discard")
}

func assertFullScreen(t *testing.T, model *model) {
	t.Helper()
	lines := strings.Split(model.View(), "\n")
	require.Len(t, lines, model.height)
	for index, line := range lines {
		require.Equal(t, model.width, ansi.StringWidth(line), "row %d", index)
	}
	require.NotEmpty(t, strings.TrimSpace(ansi.Strip(lines[len(lines)-1])), "hints stay on the last row")
}

func TestStatusAndKeyColoursRemainDistinctFromLabels(t *testing.T) {
	before := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(before) })
	model := fixture(t)
	model.focus = focusWave
	footer := strings.Join(model.footerLines(), "\n")
	require.Contains(t, footer, "\x1b[")
	require.Contains(t, ansi.Strip(footer), "i o mark trim")
	require.Contains(t, ansi.Strip(footer), "s e type times")
	require.Contains(t, ansi.Strip(footer), "? all keys")
	require.Contains(t, ansi.Strip(model.View()), "● ready")
	model.filter = filterMissing
	model.refreshKeys()
	require.Contains(t, ansi.Strip(model.View()), "· no audio yet")
	require.Contains(t, ansi.Strip(model.View()), "No audio yet.")
	model.focus = focusMix
	require.Contains(t, ansi.Strip(strings.Join(model.footerLines(), "\n")), "u Mute")
	require.NotContains(t, ansi.Strip(strings.Join(model.footerLines(), "\n")), "u Undo")
}
