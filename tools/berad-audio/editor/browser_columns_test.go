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

func TestRecordingRowsSayWhereTheSoundPlaysAndItsLevelInWords(t *testing.T) {
	m := fixture(t)
	const width = 64 // Recording pane inside a 120-column terminal.
	header := ansi.Strip(m.columnHeader(width))
	for _, label := range []string{"Sound file", "Plays in", "Level dB"} {
		require.Contains(t, header, label)
	}
	recording := m.doc.Recording("healing_01")
	gain := -12.345
	recording.CalibrationGainDB, recording.CalibrationPolicy = &gain, audio.CalibrationAuto
	require.NoError(t, m.doc.SetRecording("healing_01", recording))
	line := ansi.Strip(m.itemLine("healing_01", true, width))
	require.True(t, strings.HasPrefix(line, ">● healing_01 *"), "selection, state and the edited mark lead the row: %q", line)
	require.Contains(t, line, "Restore an injured")
	require.Contains(t, line, "-12.3 auto")
	gain = 0
	recording.CalibrationPolicy = audio.CalibrationManual
	require.NoError(t, m.doc.SetRecording("healing_01", recording))
	require.Contains(t, ansi.Strip(m.itemLine("healing_01", false, width)), "+0.0 set")
	recording.CalibrationGainDB, recording.CalibrationPolicy = nil, ""
	require.NoError(t, m.doc.SetRecording("healing_01", recording))
	require.Contains(t, ansi.Strip(m.itemLine("healing_01", false, width)), "not set")
	cue := m.doc.Cue("sfx/SoundUnlock").WithRecordings([]string{"healing_01"})
	require.NoError(t, m.doc.SetCue("sfx/SoundUnlock", cue))
	require.Contains(t, ansi.Strip(m.itemLine("healing_01", false, width)), "2 moments")
	require.True(t, strings.HasPrefix(ansi.Strip(m.itemLine("door_open_01", false, width)), " · door_open_01"),
		"an empty slot gets the quiet mark")
}

func TestMomentRowsShowSoundVolumeAndKindAndSortByName(t *testing.T) {
	m := fixture(t)
	m.workspace = workspaceMoments
	m.refreshKeys()
	header := ansi.Strip(m.columnHeader(64))
	for _, label := range []string{"Moment", "Sound", "Vol", "Kind"} {
		require.Contains(t, header, label)
	}
	names := make([]string, 0, len(m.keys))
	for _, id := range m.keys {
		names = append(names, strings.ToLower(m.doc.Cue(id).Name))
	}
	require.IsNonDecreasing(t, names, "moments are listed by the name on screen")
	line := ansi.Strip(m.itemLine("sfx/SoundPushObject", false, 64))
	require.Contains(t, line, "push_pull_01")
	require.Contains(t, line, "100%")
	require.Contains(t, line, "effect")
	require.NoError(t, m.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	require.Contains(t, ansi.Strip(m.itemLine("sfx/SoundHeal", true, 64)), "2 clips in turn")
	require.NoError(t, m.doc.SetRecordingFamily("healing_01", "impact"))
	m.selectID("sfx/SoundHeal")
	require.Contains(t, ansi.Strip(strings.Join(m.summaryLines(50), "\n")), "Filed as impact; this moment expects magic.")
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
	require.Contains(t, narrow, "Sound file")
	require.NotContains(t, narrow, "Plays in")
}
