package editorui

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func cropClock(m *model) func(time.Duration) {
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	return func(delta time.Duration) {
		now = now.Add(delta)
		m.Update(tickMsg(now))
	}
}

func TestRegionDraftPublishesOnlyOnApplyAndOneUndoRestoresIt(t *testing.T) {
	m := fixture(t)
	before, revision := m.doc.Recording(m.recordingID()), m.doc.Revision()
	dispatchRune(m, '[')
	for range 20 {
		dispatchKey(m, tea.KeyMsg{Type: tea.KeyRight})
	}
	require.Equal(t, int64(200), m.window().StartMs)
	require.Equal(t, before, m.doc.Recording(m.recordingID()))
	require.Equal(t, revision, m.doc.Revision())
	require.Contains(t, ansi.Strip(m.View()), "REGION DRAFT")
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyTab})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyTab})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, m.crop)
	require.Equal(t, revision+1, m.doc.Revision())
	require.Equal(t, int64(200), m.doc.Recording(m.recordingID()).PlaybackWindow.StartMs)
	dispatchRune(m, 'u')
	require.Equal(t, before, m.doc.Recording(m.recordingID()))
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	require.Equal(t, int64(200), m.doc.Recording(m.recordingID()).PlaybackWindow.StartMs)
}

func TestCropFocusMatchesTheSingleVisibleActivePane(t *testing.T) {
	m := fixture(t)
	for _, key := range []rune{'p', '[', 'w', 's'} {
		dispatchRune(m, key)
		require.Equal(t, 1, strings.Count(ansi.Strip(m.View()), "ACTIVE ·"), "key %c", key)
	}
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	dispatchRune(m, 'v')
	require.Equal(t, 1, strings.Count(ansi.Strip(m.View()), "ACTIVE ·"))
}

func TestRegionCancelAndNoOpApplyRetainRedo(t *testing.T) {
	m := fixture(t)
	m.setGain(-2)
	require.True(t, m.doc.Undo())
	revision := m.doc.Revision()
	dispatchRune(m, '[')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRight})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.Nil(t, m.crop)
	require.Equal(t, revision, m.doc.Revision())
	dispatchRune(m, '[')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRight})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyLeft})
	dispatchRune(m, 'a')
	require.Equal(t, revision, m.doc.Revision(), "unchanged bounds are not a new history entry")
	require.True(t, m.doc.Redo())
	require.Equal(t, -2.0, *m.doc.Recording(m.recordingID()).CalibrationGainDB)
}

func TestDraftKeyFeedbackDoesNotReadTheSourceOrChangeHistory(t *testing.T) {
	m := fixture(t)
	dispatchRune(m, '[')
	revision := m.doc.Revision()
	require.NoError(t, os.Remove(m.clip.Path()))
	for range 30 {
		dispatchKey(m, tea.KeyMsg{Type: tea.KeyRight})
	}
	require.Equal(t, int64(300), m.window().StartMs, "draft keys operate on loaded bounds even before a freshness probe")
	require.Equal(t, revision, m.doc.Revision())
	dispatchRune(m, 'a')
	require.NotNil(t, m.crop)
	require.True(t, m.sourceStale, "publication still requires source verification")
	require.Equal(t, revision, m.doc.Revision())
}

func TestCropRejectsUnreviewedDocumentChangeUntilExplicitReload(t *testing.T) {
	m := fixture(t)
	dispatchRune(m, '[')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRight})
	require.NoError(t, m.doc.SetRecordingLicense(m.recordingID(), "unknown"))
	revision := m.doc.Revision()
	dispatchRune(m, 'a')
	require.NotNil(t, m.crop)
	require.Equal(t, revision, m.doc.Revision())
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyF5})
	dispatchRune(m, 'a')
	require.Nil(t, m.crop)
	require.Equal(t, "unknown", m.doc.Recording(m.recordingID()).Provenance.License)
	require.Equal(t, int64(10), m.doc.Recording(m.recordingID()).PlaybackWindow.StartMs)
}

func TestEmptyDuplicateIdentityNeverAppliesToItsSource(t *testing.T) {
	m := fixture(t)
	before := m.doc.Recording(m.recordingID())
	dispatchRune(m, 'D')
	m.setWindow(&audio.PlaybackWindow{StartMs: 100, EndMs: 700})
	m.duplicate.id = ""
	dispatchRune(m, 'c')
	require.NotNil(t, m.duplicate)
	require.Equal(t, before, m.doc.Recording(m.recordingID()))
	require.False(t, m.doc.Dirty())
	m.duplicate.id = "region_test_01"
	dispatchRune(m, 'c')
	require.Nil(t, m.duplicate)
	require.Contains(t, m.doc.RecordingIDs(), "region_test_01")
}

func TestCrossedDraftBoundsStopPreviewAndRemainRecoverable(t *testing.T) {
	m := fixture(t)
	advance := cropClock(m)
	m.setWindow(&audio.PlaybackWindow{StartMs: 100, EndMs: 500})
	dispatchRune(m, 'p')
	voice := m.preview.(*fakeVoice)
	dispatchRune(m, 's')
	m.input = "600"
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	require.Empty(t, m.field, "crossed draft bounds are retained for correction")
	require.Equal(t, int64(600), m.window().StartMs)
	require.False(t, voice.Active(), "an invalid draft cannot keep auditioning old bounds as if current")
	dispatchRune(m, 'a')
	require.NotNil(t, m.crop)
	require.False(t, m.doc.Dirty())
	dispatchRune(m, 'e')
	m.input = "900"
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	advance(cropPreviewInterval)
	require.True(t, voice.Playing())
	require.Equal(t, &audio.PlaybackWindow{StartMs: 600, EndMs: 900}, voice.window)
	require.False(t, m.doc.Dirty())
}

func TestFinishingEndInputKeepsEndAsTheArrowTarget(t *testing.T) {
	m := fixture(t)
	dispatchRune(m, 'e')
	m.input = "700"
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, int64(690), m.window().EndMs)
	require.Zero(t, m.window().StartMs)
	require.Equal(t, markerEnd, m.marker)
	require.False(t, m.doc.Dirty())
}

func TestCropRepeatCoalescesPreviewAndNeverAuthorsPerKey(t *testing.T) {
	m := fixture(t)
	advance := cropClock(m)
	dispatchRune(m, '[')
	dispatchRune(m, 'p')
	voice := m.preview.(*fakeVoice)
	starts, revision := voice.starts, m.doc.Revision()
	for range 30 {
		dispatchKey(m, tea.KeyMsg{Type: tea.KeyRight})
	}
	require.Equal(t, starts, voice.starts)
	require.Equal(t, revision, m.doc.Revision())
	advance(cropPreviewInterval)
	require.Equal(t, starts+1, voice.starts)
	require.Equal(t, int64(300), voice.window.StartMs)
	advance(cropPreviewInterval)
	require.Equal(t, starts+1, voice.starts, "idle ticks do not reconstruct the preview")
	dispatchKey(m, tea.KeyMsg{Type: tea.KeySpace})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRight})
	advance(cropPreviewInterval)
	require.True(t, m.previewIsPaused())
	require.False(t, voice.Playing())
	require.Equal(t, int64(310), voice.window.StartMs)
}

func TestImmediateCancelRestoresTransportBeforeFirstPreviewTick(t *testing.T) {
	for _, paused := range []bool{false, true} {
		m := fixture(t)
		cropClock(m)
		dispatchRune(m, 'p')
		voice := m.preview.(*fakeVoice)
		require.NoError(t, voice.SeekMs(200))
		if paused {
			dispatchKey(m, tea.KeyMsg{Type: tea.KeySpace})
		}
		dispatchRune(m, '[')
		dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
		require.True(t, voice.Active())
		require.Equal(t, !paused, voice.Playing())
		require.Equal(t, paused, m.previewIsPaused())
		require.Equal(t, int64(200), voice.PositionMs())
	}
}

func TestCompletedPreviewDoesNotRestartOnCropEditOrCancel(t *testing.T) {
	m := fixture(t)
	advance := cropClock(m)
	dispatchRune(m, 'p')
	voice := m.preview.(*fakeVoice)
	voice.playing = false // Finite EOF, not an explicit pause.
	starts := voice.starts
	dispatchRune(m, '[')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRight})
	advance(cropPreviewInterval)
	require.Equal(t, starts, voice.starts)
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, starts, voice.starts)
	require.False(t, voice.Active())
}

func TestDraftAutoPreviewEqualsCreatedCalibrationAndSourceBytes(t *testing.T) {
	m := fixture(t)
	m.beginAutoCalibration()
	finish(m, m.autoCalibrationKey(keyEnter))
	before := m.doc.Recording(m.recordingID())
	selectHealMoment(t, m)
	data, err := os.ReadFile(m.clip.Path())
	require.NoError(t, err)
	dispatchRune(m, 'D')
	m.setWindow(&audio.PlaybackWindow{StartMs: 100, EndMs: 700})
	dispatchRune(m, 'p')
	preview, identity := m.crop.preview, m.duplicate.id
	auditionGain := m.preview.(*fakeVoice).gain
	require.True(t, preview.Calibrated)
	require.InDelta(t, m.cropGain(preview), m.preview.(*fakeVoice).gain, 1e-9)
	dispatchRune(m, 'c')
	require.Nil(t, m.crop)
	require.Equal(t, preview.CalibrationDB, *m.doc.Recording(identity).CalibrationGainDB)
	require.Equal(t, preview.Window, m.doc.Recording(identity).PlaybackWindow)
	require.Equal(t, before, m.doc.Recording("healing_01"))
	m.refreshPreviewVolumes()
	require.InDelta(t, auditionGain, m.preview.(*fakeVoice).gain, 1e-9,
		"creating a recording does not silently switch the retained audition to a different cue mix")
	after, err := os.ReadFile(m.clip.Path())
	require.NoError(t, err)
	require.True(t, bytes.Equal(data, after))
}

func TestCancelRestoresOwnedMixWithoutResurrectingExplicitStop(t *testing.T) {
	for _, stop := range []bool{false, true} {
		m := fixture(t)
		m.soundscapeVisible = true
		dispatchRune(m, 'p')
		layer := m.layerAt(sfxSlot)
		require.NotNil(t, layer)
		dispatchRune(m, '[')
		require.True(t, layer.paused)
		dispatchRune(m, 'p')
		if stop {
			dispatchKey(m, tea.KeyMsg{Type: tea.KeyCtrlX})
		}
		dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
		require.Nil(t, m.crop)
		require.Equal(t, !stop, layer.voice.Playing())
		require.False(t, m.preview.Active(), "cancel does not double the restored Soundscape source")
	}
}

func TestChangedSourceRejectsApplyAndF5RebasesRetainedDraft(t *testing.T) {
	m := fixture(t)
	dispatchRune(m, '[')
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyRight})
	data, err := os.ReadFile(m.clip.Path())
	require.NoError(t, err)
	data[len(data)-1] ^= 1
	require.NoError(t, os.WriteFile(m.clip.Path(), data, 0o644))
	dispatchRune(m, 'a')
	require.NotNil(t, m.crop)
	require.True(t, m.sourceStale)
	require.False(t, m.doc.Dirty())
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyF5})
	require.False(t, m.sourceStale)
	require.Equal(t, int64(10), m.window().StartMs)
	dispatchRune(m, 'a')
	require.Nil(t, m.crop)
	require.Equal(t, int64(10), m.doc.Recording(m.recordingID()).PlaybackWindow.StartMs)
}
