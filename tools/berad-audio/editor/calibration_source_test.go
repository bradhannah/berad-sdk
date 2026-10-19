package editorui

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

// Equal-length, distinguishable PCM sources make stale gain observable, rather
// than merely changing an unused byte in a shared production recording fixture.
func calibrationWAV(amplitude int16) []byte {
	frames := audio.EditorSampleRate / 2
	data := make([]byte, 44+frames*4)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 2)
	binary.LittleEndian.PutUint32(data[24:], audio.EditorSampleRate)
	binary.LittleEndian.PutUint32(data[28:], audio.EditorSampleRate*4)
	binary.LittleEndian.PutUint16(data[32:], 4)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(frames*4))
	for frame := range frames {
		sample := amplitude
		if frame%100 < 50 {
			sample = -sample
		}
		binary.LittleEndian.PutUint16(data[44+frame*4:], uint16(sample))
		binary.LittleEndian.PutUint16(data[46+frame*4:], uint16(sample))
	}
	return data
}

func calibrationSourceFixture(t *testing.T) *model {
	t.Helper()
	m := fixture(t)
	require.NoError(t, os.WriteFile(m.clip.Path(), calibrationWAV(4000), 0o644))
	finish(m, m.loadSelection())
	require.True(t, m.selectedClipReady())
	return m
}

func replaceCalibrationSource(t *testing.T, m *model, sameStamp bool) []byte {
	t.Helper()
	info, err := os.Stat(m.clip.Path())
	require.NoError(t, err)
	data := calibrationWAV(8000)
	require.Equal(t, info.Size(), int64(len(data)))
	require.NoError(t, os.WriteFile(m.clip.Path(), data, info.Mode()))
	stamp := info.ModTime().Add(2 * time.Second)
	if sameStamp {
		stamp = info.ModTime()
	}
	require.NoError(t, os.Chtimes(m.clip.Path(), stamp, stamp))
	if sameStamp {
		require.NoError(t, m.clip.CheckSourceStamp(), "only content verification can detect this replacement")
	} else {
		require.ErrorIs(t, m.clip.CheckSourceStamp(), audio.ErrRecordingSourceChanged)
	}
	return data
}

func TestAutoCalibrationConfirmationRejectsReplacedSourceThroughRouting(t *testing.T) {
	for _, sameStamp := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed stamp", true: "same size and stamp"}[sameStamp], func(t *testing.T) {
			m := calibrationSourceFixture(t)
			// Keep a redo entry; rejection must preserve history as well as metadata.
			m.setGain(-3)
			manual := m.doc.Recording(m.recordingID())
			dispatchRune(m, 'u')
			before, revision := m.doc.Recording(m.recordingID()), m.doc.Revision()
			dispatchRune(m, 'p')
			dispatchRune(m, 'A')
			require.NotNil(t, m.autoGainDraft)
			_, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			require.NotNil(t, command, "confirmation verifies the source off the input loop")
			require.True(t, m.busy)
			require.Equal(t, revision, m.doc.Revision())
			replacement := replaceCalibrationSource(t, m, sameStamp)
			finish(m, command)
			require.False(t, m.busy)
			require.True(t, m.sourceStale)
			require.True(t, m.statusIsError)
			require.Contains(t, m.status, "source changed")
			require.ErrorIs(t, m.autoGainDraft.err, audio.ErrRecordingSourceChanged)
			require.Equal(t, audio.RecordingInvalid, m.availability[m.recordingID()].State)
			require.False(t, m.preview.Active())
			require.Equal(t, before, m.doc.Recording(m.recordingID()))
			require.Equal(t, revision, m.doc.Revision())
			require.False(t, m.doc.Dirty())
			dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
			require.Equal(t, revision, m.doc.Revision(), "a rejected draft cannot be confirmed again")
			data, err := os.ReadFile(m.clip.Path())
			require.NoError(t, err)
			require.Equal(t, replacement, data, "calibration never rewrites source bytes")
			require.True(t, m.doc.Redo(), "failed verification must preserve Redo")
			require.Equal(t, manual, m.doc.Recording(m.recordingID()))
		})
	}
}

func TestAutoCalibrationSourceProbeInvalidatesApproval(t *testing.T) {
	m := calibrationSourceFixture(t)
	dispatchRune(m, 'p')
	dispatchRune(m, 'A')
	before, revision := m.doc.Recording(m.recordingID()), m.doc.Revision()
	replaceCalibrationSource(t, m, false)
	finish(m, m.queueSourceProbe())
	require.True(t, m.sourceStale)
	require.ErrorIs(t, m.autoGainDraft.err, audio.ErrRecordingSourceChanged)
	require.False(t, m.preview.Active())
	dialog, visible := m.dialogContent(100)
	require.True(t, visible)
	require.Contains(t, ansi.Strip(strings.Join(dialog.rows, "\n")), "stale and cannot be applied")
	require.NotContains(t, ansi.Strip(strings.Join(m.footerLines(), "\n")), "Enter")
	_, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, command, "known-invalid approval cannot start a transaction")
	require.True(t, m.statusIsError)
	require.Equal(t, before, m.doc.Recording(m.recordingID()))
	require.Equal(t, revision, m.doc.Revision())
}

func TestAutoCalibrationMissingSourcePreservesFailureAndHistory(t *testing.T) {
	m := calibrationSourceFixture(t)
	dispatchRune(m, 'A')
	revision := m.doc.Revision()
	require.NoError(t, os.Remove(m.clip.Path()))
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.statusIsError)
	require.True(t, m.sourceStale)
	require.ErrorIs(t, m.autoGainDraft.err, os.ErrNotExist)
	require.Equal(t, audio.RecordingMissing, m.availability[m.recordingID()].State)
	require.Equal(t, revision, m.doc.Revision())
	require.False(t, m.doc.Dirty())
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.Nil(t, m.autoGainDraft)
	require.False(t, m.statusIsError, "cancelling clears the dialog error, not source invalidity")
	require.True(t, m.sourceStale)
	require.Contains(t, m.status, "F5")
}

func TestAutoCalibrationReloadRequiresFreshSuggestionAndOneUndo(t *testing.T) {
	m := calibrationSourceFixture(t)
	before := m.doc.Recording(m.recordingID())
	dispatchRune(m, 'A')
	oldGain, oldSource := m.autoGainDraft.GainDB, m.clip
	replacement := replaceCalibrationSource(t, m, true)
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.sourceStale)
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyF5})
	require.True(t, m.selectedClipReady())
	require.NotEqual(t, oldSource.Fingerprint(), m.clip.Fingerprint())
	require.Nil(t, m.autoGainDraft, "refresh cannot silently approve recalculated gain")
	require.Equal(t, before, m.doc.Recording(m.recordingID()))
	dispatchRune(m, 'A')
	gain := m.autoGainDraft.GainDB
	require.NotEqual(t, oldGain, gain, "the replacement has different measured loudness")
	revision := m.doc.Revision()
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.False(t, m.statusIsError)
	require.Nil(t, m.autoGainDraft)
	require.Equal(t, revision+1, m.doc.Revision())
	after := m.doc.Recording(m.recordingID())
	require.Equal(t, audio.CalibrationAuto, after.CalibrationMode())
	require.Equal(t, gain, *after.CalibrationGainDB)
	dispatchRune(m, 'u')
	require.Equal(t, before, m.doc.Recording(m.recordingID()))
	require.False(t, m.doc.Dirty())
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	require.Equal(t, after, m.doc.Recording(m.recordingID()))
	data, err := os.ReadFile(m.clip.Path())
	require.NoError(t, err)
	require.Equal(t, replacement, data)
}

func TestAutoCalibrationCompletionRejectsChangedIdentityOrRevision(t *testing.T) {
	for _, change := range []string{"selection", "revision", "source reload", "draft identity"} {
		t.Run(change, func(t *testing.T) {
			m := calibrationSourceFixture(t)
			id := m.recordingID()
			dispatchRune(m, 'A')
			_, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			require.NotNil(t, command)
			message := command() // Successful detached work, delayed delivery.
			switch change {
			case "selection":
				m.selectID("healing_02")
			case "revision":
				require.NoError(t, m.doc.SetRecordingLicense(id, "newer license"))
			case "source reload":
				finish(m, m.loadSelection())
			case "draft identity":
				m.beginAutoCalibration()
			}
			before, revision := m.doc.Recording(id), m.doc.Revision()
			_, followup := m.Update(message)
			finish(m, followup)
			require.False(t, m.busy)
			require.True(t, m.statusIsError)
			require.NotNil(t, m.autoGainDraft)
			require.Equal(t, before, m.doc.Recording(id))
			require.Equal(t, revision, m.doc.Revision())
			require.Nil(t, m.doc.Recording("healing_02").CalibrationGainDB)
		})
	}
}

func TestAutoCalibrationProbeDuringVerificationRejectsCompletedResult(t *testing.T) {
	m := calibrationSourceFixture(t)
	dispatchRune(m, 'A')
	probe := m.queueSourceProbe() // Already in flight when confirmation starts.
	require.NotNil(t, probe)
	_, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, command)
	message := command()
	replaceCalibrationSource(t, m, false)
	finish(m, probe)
	_, followup := m.Update(message)
	finish(m, followup)
	require.False(t, m.busy)
	require.True(t, m.sourceStale)
	require.True(t, m.statusIsError)
	require.False(t, m.doc.Dirty(), "a detached success cannot overrule later invalidation")
	require.Nil(t, m.doc.Recording(m.recordingID()).CalibrationGainDB)
}

func TestAutoCalibrationAsyncSuccessPreservesTransportAndRejectsReplay(t *testing.T) {
	m := calibrationSourceFixture(t)
	id := m.recordingID()
	before := m.doc.Recording(id)
	sourceBytes, err := os.ReadFile(m.clip.Path())
	require.NoError(t, err)
	manifestBytes, err := os.ReadFile(m.doc.Path())
	require.NoError(t, err)
	dispatchRune(m, 'p')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeySpace})
	voice := m.preview.(*fakeVoice)
	require.NoError(t, voice.SeekMs(150))
	starts := voice.starts
	dispatchRune(m, 'A')
	_, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, command)
	require.Equal(t, before, m.doc.Recording(id))
	message := command()
	m.Update(message)
	after := m.doc.Recording(id)
	require.Nil(t, m.autoGainDraft)
	require.Equal(t, audio.CalibrationAuto, after.CalibrationMode())
	require.False(t, m.statusIsError)
	require.Equal(t, starts, voice.starts)
	require.True(t, m.previewIsPaused())
	require.Equal(t, int64(150), voice.PositionMs())
	require.InDelta(t, m.livePreviewGain(), voice.gain, 1e-9)
	data, err := os.ReadFile(m.clip.Path())
	require.NoError(t, err)
	require.Equal(t, sourceBytes, data)
	data, err = os.ReadFile(m.doc.Path())
	require.NoError(t, err)
	require.Equal(t, manifestBytes, data, "confirmation authors in memory; Save remains explicit")
	dispatchRune(m, 'u')
	require.Equal(t, before, m.doc.Recording(id))
	dispatchRune(m, 'A')
	_, next := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, next)
	draft := m.autoGainDraft
	m.Update(message)
	require.True(t, m.busy, "replayed completion cannot finish a newer operation")
	require.Same(t, draft, m.autoGainDraft)
	require.Equal(t, before, m.doc.Recording(id))
	finish(m, next)
	require.Equal(t, after, m.doc.Recording(id))
}
