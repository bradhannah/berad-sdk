package editorui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestAutoCalibrationIsConfirmedUndoableAndManualOverrideStaysLocked(t *testing.T) {
	m := fixture(t)
	id := m.recordingID()
	original := m.doc.Recording(id)
	m.detailLines(60)
	require.False(t, m.doc.Dirty(), "displaying a suggestion does not accept it")
	m.beginAutoCalibration()
	require.NotNil(t, m.autoGainDraft)
	m.autoCalibrationKey(keyEscape)
	require.False(t, m.doc.Dirty())
	m.beginAutoCalibration()
	finish(m, m.autoCalibrationKey(keyEnter))
	require.Equal(t, audio.CalibrationAuto, m.doc.Recording(id).CalibrationMode())
	m.setCalibrationTarget("-30")
	require.Equal(t, -30.0, *m.doc.Recording(id).CalibrationTargetDB)
	window := audio.PlaybackWindow{StartMs: 0, EndMs: m.clip.DurationMs() / 2}
	beforeTrim := m.doc.Recording(id)
	m.setWindow(&window)
	finish(m, m.applyCrop())
	suggestion, err := m.calibrationSuggestion(m.doc.Recording(id))
	require.NoError(t, err)
	require.InDelta(t, suggestion.GainDB, *m.doc.Recording(id).CalibrationGainDB, 0.001)
	require.True(t, m.doc.Undo())
	require.Equal(t, beforeTrim, m.doc.Recording(id), "trim and resulting auto-gain are one undo step")
	m.setGain(2.5)
	cache := m.calibrationCache
	m.setWindow(&window)
	finish(m, m.applyCrop())
	m.position = 100
	m.detailLines(60)
	require.Same(t, cache, m.calibrationCache, "manual override does not trigger measurement")
	require.Equal(t, 2.5, *m.doc.Recording(id).CalibrationGainDB)
	require.Equal(t, audio.CalibrationManual, m.doc.Recording(id).CalibrationMode())
	require.NoError(t, m.doc.Save())
	reopened, err := openTestEditor(m.doc.Path())
	require.NoError(t, err)
	require.Equal(t, audio.CalibrationManual, reopened.Recording(id).CalibrationMode())
	m.beginAutoCalibration()
	finish(m, m.autoCalibrationKey(keyEnter))
	require.Equal(t, audio.CalibrationAuto, m.doc.Recording(id).CalibrationMode())
	require.Equal(t, original.File, m.doc.Recording(id).File)
}

func TestDetailsSeparateColouredLabelsAndValues(t *testing.T) {
	before := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(before) })
	m := fixture(t)
	lines := m.detailLines(100)
	joined := strings.Join(lines, "\n")
	require.Contains(t, joined, m.colors.detailLabel.Render("Recording: "))
	require.Contains(t, joined, m.colors.detailValue.Render("healing_01"))
	require.NotEqual(t, m.colors.detailLabel.Render("same"), m.colors.detailValue.Render("same"))
	for _, line := range m.detailLines(25) {
		require.LessOrEqual(t, ansi.StringWidth(line), 25)
	}
	require.Contains(t, ansi.Strip(joined), "Suggested gain:")
}
