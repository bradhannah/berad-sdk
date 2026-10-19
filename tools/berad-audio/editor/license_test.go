package editorui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestLicenseDialogAppliesOnlyProvenanceAndRoundTrips(t *testing.T) {
	m := fixture(t)
	before := m.doc.Recording("healing_01")
	m.play()
	voice := m.preview.(*fakeVoice)
	starts := voice.starts
	m.key(tea.KeyMsg{Type: tea.KeyCtrlL})
	require.Equal(t, fieldLicense, m.field)
	require.Equal(t, "healing_01", m.licenseRecordingID)
	require.Equal(t, before.Provenance.License, m.input)
	for _, size := range [][2]int{{60, 24}, {100, 36}} {
		m.width, m.height = size[0], size[1]
		view := ansi.Strip(m.View())
		require.Contains(t, view, "EDIT LICENSE")
		require.Contains(t, view, "healing_01")
		require.Contains(t, view, "Shared provenance")
		assertFullScreen(t, m)
	}
	m.key(tea.KeyMsg{Type: tea.KeyCtrlU})
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("CC BY 4.0")})
	require.Equal(t, before, m.doc.Recording("healing_01"), "typing only changes the draft")
	finish(m, m.key(tea.KeyMsg{Type: tea.KeyEnter}))
	require.Empty(t, m.field)
	expected := before
	expected.Provenance.License = "CC BY 4.0"
	require.Equal(t, expected, m.doc.Recording("healing_01"))
	require.Equal(t, starts, voice.starts, "license editing must not restart or recalibrate playback")
	require.True(t, m.doc.Undo())
	require.Equal(t, before, m.doc.Recording("healing_01"))
	require.True(t, m.doc.Redo())
	finish(m, m.save())
	require.False(t, m.doc.Dirty())
	reopened, err := openTestEditor(m.doc.Path())
	require.NoError(t, err)
	require.Equal(t, expected, reopened.Recording("healing_01"))
}

func TestLicenseDialogRejectsBlankRetainsInputAndCancels(t *testing.T) {
	m := fixture(t)
	m.beginLicenseEdit()
	m.input = "   "
	finish(m, m.commitInput())
	require.Equal(t, fieldLicense, m.field)
	require.Equal(t, "   ", m.input)
	require.Contains(t, m.inputError, "license")
	require.Contains(t, m.inputError, "unknown")
	require.False(t, m.doc.Dirty())
	view := ansi.Strip(m.View())
	require.Contains(t, view, "EDIT LICENSE")
	require.Contains(t, view, "must be explicit")
	m.input = "CC0-1.0"
	m.key(tea.KeyMsg{Type: tea.KeyEsc})
	require.Empty(t, m.field)
	require.False(t, m.doc.Dirty())
}

func TestLicenseDialogUsesSelectedMomentVariantAndSupportsEmptySlots(t *testing.T) {
	m := fixture(t)
	previousLicense := m.doc.Recording("healing_01").Provenance.License
	require.NoError(t, m.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	selectHealMoment(t, m)
	finish(m, m.previewNextClip())
	require.Equal(t, "healing_02", m.recordingID())
	m.key(tea.KeyMsg{Type: tea.KeyCtrlL})
	require.Equal(t, "healing_02", m.licenseRecordingID)
	m.input = "Documented custom license"
	finish(m, m.commitInput())
	require.Equal(t, "Documented custom license", m.doc.Recording("healing_02").Provenance.License)
	require.Equal(t, previousLicense, m.doc.Recording("healing_01").Provenance.License)
	require.Contains(t, strings.Join(m.detailLines(80), "\n"), "License [Ctrl+L]")
	finish(m, m.selectWorkspace(workspaceRecordings))
	m.selectID("fountain_drink_01")
	finish(m, m.loadSelection())
	require.Nil(t, m.clip)
	m.key(tea.KeyMsg{Type: tea.KeyCtrlL})
	m.input = "unknown"
	finish(m, m.commitInput())
	require.Empty(t, m.field)
	require.Nil(t, m.doc.Recording("fountain_drink_01").File)
}
