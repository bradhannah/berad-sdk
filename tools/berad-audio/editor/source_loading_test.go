package editorui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestMovingFilePickerRejectsLateCandidateAndSpacePreviewsNewSelection(t *testing.T) {
	m := fixture(t)
	data, err := os.ReadFile(m.clip.Path())
	require.NoError(t, err)
	directory := t.TempDir()
	first, second := filepath.Join(directory, "a.wav"), filepath.Join(directory, "b.wav")
	require.NoError(t, os.WriteFile(first, data, 0o644))
	data[len(data)-1] ^= 1
	require.NoError(t, os.WriteFile(second, data, 0o644))
	m.readDirectory(directory)
	m.picker.cursor = 1 // Parent directory occupies row zero.
	command := m.previewPickerEntry(false)
	completed := command() // Decode A completes before selection changes, delivery occurs afterward.
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyDown})
	m.Update(completed)
	require.Nil(t, m.clip)
	require.False(t, m.preview.Active())
	dispatchKey(m, tea.KeyMsg{Type: tea.KeySpace})
	require.Equal(t, second, m.clip.Path())
	require.Contains(t, m.previewDescription(), "b.wav")
	require.Same(t, m.clip, m.preview.(*fakeVoice).clip)
	require.False(t, m.doc.Dirty())
}

func separateVariant(t *testing.T, m *model) string {
	t.Helper()
	data, err := os.ReadFile(m.clip.Path())
	require.NoError(t, err)
	data[len(data)-1] ^= 1
	path := filepath.Join(filepath.Dir(m.clip.Path()), "different.wav")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	recording := m.doc.Recording("healing_02")
	file := "sounds/different.wav"
	recording.File = &file
	require.NoError(t, m.doc.SetRecording("healing_02", recording))
	require.NoError(t, m.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	selectHealMoment(t, m)
	return path
}

func TestFailedOrCanceledNextVariantRetainsActualSelectionAndPlayback(t *testing.T) {
	m := fixture(t)
	path := separateVariant(t, m)
	dispatchRune(m, 'p')
	first := m.clip
	command := m.previewNextClip()
	message := command()
	dispatchRune(m, 'N') // An editor now owns the first recording; a late preview cannot retarget it.
	m.Update(message)
	require.Equal(t, "healing_01", m.recordingID())
	require.Same(t, first, m.clip)
	require.True(t, m.preview.Playing())
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.NoError(t, os.Remove(path))
	finish(m, m.previewNextClip())
	require.Equal(t, "healing_01", m.recordingID())
	require.Same(t, first, m.clip)
	require.True(t, m.preview.Playing())
	require.Equal(t, audio.RecordingMissing, m.availability["healing_02"].State)
	require.Equal(t, audio.RecordingReady, m.availability["healing_01"].State)
}

func TestStopDuringPendingVariantDoesNotAdvanceItsCursor(t *testing.T) {
	m := fixture(t)
	separateVariant(t, m)
	command := m.previewNextClip()
	message := command()
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyCtrlX})
	m.Update(message)
	require.Equal(t, "healing_01", m.recordingID())
	require.False(t, m.preview.Active())
}

func TestSourceProbeStopsChangedSnapshotAndCannotPoisonReloadedSource(t *testing.T) {
	m := fixture(t)
	dispatchRune(m, 'p')
	old := m.clip
	data, err := os.ReadFile(old.Path())
	require.NoError(t, err)
	data[len(data)-1] ^= 1
	require.NoError(t, os.WriteFile(old.Path(), data, 0o644))
	command := m.queueSourceProbe()
	require.NotNil(t, command)
	message := command()
	m.Update(message)
	require.True(t, m.sourceStale)
	require.False(t, m.preview.Active())
	finish(m, m.loadSelection())
	require.False(t, m.sourceStale)
	require.NotEqual(t, old.Fingerprint(), m.clip.Fingerprint())
	m.Update(message)
	require.False(t, m.sourceStale, "late old-snapshot evidence does not invalidate a fresh load")
}

func TestSourceChangeStopsOwnedLayersAndReloadCanRestoreThem(t *testing.T) {
	m := fixture(t)
	m.soundscapeVisible = true
	dispatchRune(m, 'p')
	layer := m.layerAt(sfxSlot)
	dispatchRune(m, '[')
	data, err := os.ReadFile(m.clip.Path())
	require.NoError(t, err)
	data[len(data)-1] ^= 1
	require.NoError(t, os.WriteFile(m.clip.Path(), data, 0o644))
	dispatchRune(m, 'a')
	require.True(t, layer.stopped)
	require.False(t, layer.voice.Active())
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyF5})
	dispatchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	require.True(t, layer.voice.Playing())
	require.Equal(t, m.clip.Fingerprint(), layer.clip.Fingerprint())
}
