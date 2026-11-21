package editorui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestClockWritesAndReadsMinutesAndSeconds(t *testing.T) {
	require.Equal(t, "1:41", clock(101120, false))
	require.Equal(t, "1:41.120", clock(101120, true))
	for text, want := range map[string]int64{"1:35": 95000, "1:35.000": 95000, "95.5": 95500, " 0:00.110 ": 110, "3": 3000} {
		got, valid := parseClock(text)
		require.True(t, valid, text)
		require.Equal(t, want, got, text)
	}
	for _, text := range []string{"", "abc", "1:75", "-1", "1:2:3"} {
		_, valid := parseClock(text)
		require.False(t, valid, text)
	}
}

func TestLevelStepsAnswerOnTheMessageLineAndUndoAsOne(t *testing.T) {
	m := fixture(t)
	for range 4 {
		dispatchRune(m, '+')
	}
	require.InDelta(t, 2.0, *m.doc.Recording("healing_01").CalibrationGainDB, 1e-9)
	require.Equal(t, "✓ Level +2.0 dB · u undoes", m.status)
	dispatchRune(m, 'u')
	require.False(t, m.doc.Dirty(), "a run of presses is one history entry")
	dispatchRune(m, '2')
	m.selectID("sfx/SoundHeal")
	for range 3 {
		dispatchRune(m, '-')
	}
	require.Equal(t, 0.85, m.doc.Cue("sfx/SoundHeal").VolumeModifier, "no float noise accumulates")
	require.Equal(t, "✓ Volume 85% · u undoes", m.status)
	dispatchRune(m, 'g')
	require.Equal(t, "85", m.input, "typing starts from the value shown")
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("40")})
	require.Equal(t, "40", m.input, "the old value is selected, so typing replaces it")
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, 0.4, m.doc.Cue("sfx/SoundHeal").VolumeModifier)
}

func TestTrimDraftShowsNowBesideDraftAndEnterApplies(t *testing.T) {
	m := fixture(t)
	voice, ok := m.preview.(*fakeVoice)
	require.True(t, ok)
	dispatchRune(m, 'e')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0:02.900")})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	view := ansi.Strip(m.View())
	for _, text := range []string{"trim draft", "Now", "Draft", "0:03.000   0:02.900   -0.1 s", "Used by: Restore an injured character"} {
		require.Contains(t, view, text)
	}
	dispatchRune(m, 'P')
	require.Equal(t, &audio.PlaybackWindow{StartMs: 900, EndMs: 2900}, voice.window, "only the ending plays")
	require.Nil(t, m.doc.Recording("healing_01").PlaybackWindow, "listening is not applying")
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, m.crop)
	require.Equal(t, &audio.PlaybackWindow{StartMs: 0, EndMs: 2900}, m.doc.Recording("healing_01").PlaybackWindow)
}

func TestTrimSilenceSurvivesAFileThatFailedToReload(t *testing.T) {
	m := fixture(t)
	dispatchRune(m, '[')
	require.NoError(t, os.Remove(m.clip.Path()))
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyF5})
	require.Nil(t, m.clip, "the reload failed")
	require.NotPanics(t, func() { dispatchRune(m, 't') })
	require.NotNil(t, m.crop, "the draft and its unsaved work are still there")
}

func TestDetailsLeadWithTheGameAndKeepNumbersBehindOneKey(t *testing.T) {
	m := fixture(t)
	summary := ansi.Strip(strings.Join(m.detailLines(40), "\n"))
	require.True(t, strings.HasPrefix(strings.TrimSpace(summary), "IN GAME"))
	require.Contains(t, summary, "When: Restore an injured character")
	require.NotContains(t, summary, "[working]")
	require.NotContains(t, summary, "Preview gain")
	for _, line := range strings.Split(summary, "\n") {
		require.LessOrEqual(t, ansi.StringWidth(line), 40)
	}
	dispatchRune(m, 'I')
	require.Contains(t, ansi.Strip(strings.Join(m.detailLines(40), "\n")), "Preview gain")
}
