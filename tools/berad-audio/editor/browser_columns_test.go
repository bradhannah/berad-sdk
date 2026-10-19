package editorui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestRecordingColumnsShowCalibrationOwnershipAndLiveUsageCount(t *testing.T) {
	m := fixture(t)
	const width = 53 // Recording pane inside a 100-column terminal.
	header := ansi.Strip(m.columnHeader(width))
	for _, label := range []string{"Recording ID", "Family", "Cal dB", "Uses"} {
		require.Contains(t, header, label)
	}
	recording := m.doc.Recording("healing_01")
	gain := -12.345
	recording.CalibrationGainDB, recording.CalibrationPolicy = &gain, audio.CalibrationAuto
	require.NoError(t, m.doc.SetRecording("healing_01", recording))
	uses := len(m.doc.Usages("healing_01"))
	line := ansi.Strip(m.itemLine("healing_01", true, width))
	require.Contains(t, line, "healing_01")
	require.Contains(t, line, "magic")
	require.Contains(t, line, "-12.3 A")
	require.True(t, strings.HasSuffix(line, fmt.Sprintf("%4d", uses)))
	gain = 0
	recording.CalibrationPolicy = audio.CalibrationManual
	require.NoError(t, m.doc.SetRecording("healing_01", recording))
	require.Contains(t, ansi.Strip(m.itemLine("healing_01", false, width)), "+0.0 M")
	recording.CalibrationGainDB, recording.CalibrationPolicy = nil, ""
	require.NoError(t, m.doc.SetRecording("healing_01", recording))
	require.Contains(t, ansi.Strip(m.itemLine("healing_01", false, width)), "— U")
	cue := m.doc.Cue("sfx/SoundUnlock").WithRecordings([]string{"healing_01"})
	require.NoError(t, m.doc.SetCue("sfx/SoundUnlock", cue))
	line = ansi.Strip(m.itemLine("healing_01", false, width))
	require.True(t, strings.HasSuffix(line, fmt.Sprintf("%4d", uses+1)), "usage count must reflect working-copy assignments")
}

func TestMomentColumnsFlagEveryMismatchedCycleMember(t *testing.T) {
	m := fixture(t)
	m.workspace = workspaceMoments
	m.refreshKeys()
	header := ansi.Strip(m.columnHeader(70))
	for _, label := range []string{"Moment", "Target", "Assignment"} {
		require.Contains(t, header, label)
	}
	line := ansi.Strip(m.itemLine("sfx/SoundPushObject", false, 70))
	require.Contains(t, line, "push_pull_01")
	require.NotContains(t, line, "mechanical !")
	push := m.doc.Cue("sfx/SoundPushObject").WithRecordings([]string{"healing_01"})
	require.NoError(t, m.doc.SetCue("sfx/SoundPushObject", push))
	line = ansi.Strip(m.itemLine("sfx/SoundPushObject", false, 70))
	require.Contains(t, line, "mechanical !")
	require.Contains(t, line, "healing_01")
	require.NoError(t, m.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	require.False(t, m.momentFamilyMismatch("sfx/SoundHeal"))
	require.NoError(t, m.doc.SetRecordingFamily("healing_02", "impact"))
	require.True(t, m.momentFamilyMismatch("sfx/SoundHeal"), "a matching first member must not hide another member's mismatch")
	line = ansi.Strip(m.itemLine("sfx/SoundHeal", true, 70))
	require.Contains(t, line, "2 clips ↻")
	require.Contains(t, line, "!")
	require.NoError(t, m.doc.SetRecordingFamily("healing_01", "mechanical_interaction"))
	require.False(t, m.momentFamilyMismatch("sfx/SoundPushObject"), "warning follows repaired assignments")
}

func TestColumnsFitNarrowAndWidePanesWithColoursAndUnicode(t *testing.T) {
	before := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(before) })
	m := fixture(t)
	cue := m.doc.Cue("sfx/SoundHeal")
	cue.Name = "癒し — Healing with a long descriptive name"
	require.NoError(t, m.doc.SetCue("sfx/SoundHeal", cue))
	for _, width := range []int{20, 31, 39, 40, 53, 70, 90, 120} {
		for _, moments := range []bool{false, true} {
			m.workspace = workspaceRecordings
			if moments {
				m.workspace = workspaceMoments
			}
			identity := "healing_01"
			if moments {
				identity = "sfx/SoundHeal"
			}
			require.Equal(t, width, ansi.StringWidth(m.columnHeader(width)))
			for _, selected := range []bool{false, true} {
				require.Equal(t, width, ansi.StringWidth(m.itemLine(identity, selected, width)))
			}
		}
	}
	m.workspace = workspaceRecordings
	narrow := ansi.Strip(m.columnHeader(31))
	require.Contains(t, narrow, "Family")
	require.NotContains(t, narrow, "Cal dB")
	require.Contains(t, strings.Join(m.detailLines(80), "\n"), "Calibration [working]")
}
