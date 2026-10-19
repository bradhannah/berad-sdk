package editorui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func dispatchKey(model *model, key tea.KeyMsg) {
	_, command := model.Update(key)
	finish(model, command)
}

func dispatchRune(model *model, key rune) {
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
}

func TestCropMarkersShareArrowStepsThroughInputRouting(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "recording", true: "duplicate"}[duplicate], func(t *testing.T) {
			model := fixture(t)
			if duplicate {
				dispatchRune(model, 'D')
			}
			dispatchRune(model, '[')
			dispatchKey(model, tea.KeyMsg{Type: tea.KeyRight})
			require.Equal(t, int64(10), model.window().StartMs)
			dispatchKey(model, tea.KeyMsg{Type: tea.KeyShiftRight})
			require.Equal(t, int64(110), model.window().StartMs)
			dispatchRune(model, 's')
			dispatchKey(model, tea.KeyMsg{Type: tea.KeyLeft})
			require.Equal(t, "110", model.input, "arrows move the caret once text editing owns input")
			dispatchKey(model, tea.KeyMsg{Type: tea.KeyEsc})
			if duplicate {
				require.False(t, model.doc.Dirty(), "duplicate bounds remain a draft until Create")
			}
		})
	}
}

func TestPausedPreviewSeekAgreesWithNextTransportTick(t *testing.T) {
	model := fixture(t)
	model.setWindow(&audio.PlaybackWindow{StartMs: 100, EndMs: 500})
	finish(model, model.applyCrop())
	dispatchRune(model, 'p')
	dispatchKey(model, tea.KeyMsg{Type: tea.KeySpace})
	model.position = 100
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, int64(100), model.position, "active preview must immediately show its clamped position")
	model.Update(tickMsg(time.Time{}))
	require.Equal(t, int64(100), model.position)
	require.Contains(t, model.status, "selection")
}

func TestCropPreservesPausedTransportPositionAndResume(t *testing.T) {
	model := fixture(t)
	model.setWindow(&audio.PlaybackWindow{StartMs: 40, EndMs: 500})
	dispatchRune(model, 'p')
	voice := model.preview.(*fakeVoice)
	require.NoError(t, voice.SeekMs(200))
	model.Update(tickMsg(time.Time{}))
	dispatchKey(model, tea.KeyMsg{Type: tea.KeySpace})
	dispatchRune(model, '[')
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, int64(50), model.window().StartMs)
	require.True(t, model.previewIsPaused())
	require.Equal(t, int64(200), voice.PositionMs())
	require.Contains(t, model.transportState(), "PAUSED")
	starts := voice.starts
	dispatchKey(model, tea.KeyMsg{Type: tea.KeySpace})
	require.True(t, voice.Playing())
	require.Equal(t, starts, voice.starts, "Space resumes the edited preview rather than restarting it")
}

func TestMarkingBoundarySelectsItsArrowTarget(t *testing.T) {
	model := fixture(t)
	model.position = 40
	dispatchRune(model, 'i')
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, int64(50), model.window().StartMs)
	model.position = 300
	dispatchRune(model, 'o')
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, int64(290), model.window().EndMs)
	require.Equal(t, int64(50), model.window().StartMs)
}

func TestDuplicateVolumeDialogMatchesInputOwner(t *testing.T) {
	model := fixture(t)
	dispatchRune(model, 'D')
	dispatchRune(model, 'v')
	require.True(t, model.volumeControls)
	dialog, visible := model.dialogContent(80)
	require.True(t, visible)
	require.Contains(t, dialog.title, "PREVIEW VOLUME")
}

func TestFailedImportRestoresSourceBeforeEditing(t *testing.T) {
	model := fixture(t)
	originalPath := model.clip.Path()
	candidatePath := filepath.Join(t.TempDir(), "candidate.wav")
	require.NoError(t, os.WriteFile(candidatePath, calibrationWAV(4000), 0o644))
	candidate, err := model.doc.LoadRecording(context.Background(), candidatePath)
	require.NoError(t, err)
	model.clip = candidate
	operation := model.documentOperation(true, "imported", func(*audio.LibraryEditor) error { return errors.New("import failed") })
	_, command := model.Update(operation())
	finish(model, command)
	require.Equal(t, originalPath, model.clip.Path())
	require.True(t, model.statusIsError)
	require.Contains(t, model.status, "import failed")
}

func TestLoadingAutoRecordingDoesNotAuthorOrDestroyRedo(t *testing.T) {
	model := fixture(t)
	recording := model.doc.Recording(model.recordingID())
	initialGain := -12.0
	recording.CalibrationPolicy, recording.CalibrationGainDB = audio.CalibrationAuto, &initialGain
	require.NoError(t, model.doc.SetRecording(model.recordingID(), recording))
	require.True(t, model.doc.Undo())
	require.True(t, model.doc.Redo())
	before := model.doc.Recording(model.recordingID())
	finish(model, model.loadSelection())
	require.Equal(t, before, model.doc.Recording(model.recordingID()), "loading is observational")
	finish(model, model.history(false))
	require.Nil(t, model.doc.Recording(model.recordingID()).CalibrationGainDB)
	finish(model, model.history(true))
	require.Equal(t, before, model.doc.Recording(model.recordingID()))
}

func TestFamilyAndAutoCalibrationUndoAsOneEditThroughReload(t *testing.T) {
	model := fixture(t)
	model.beginAutoCalibration()
	finish(model, model.autoCalibrationKey(keyEnter))
	before, family := model.doc.Recording(model.recordingID()), model.doc.RecordingFamily(model.recordingID())
	dispatchRune(model, 'C')
	model.form.fields[0].value = "nature"
	finish(model, model.applyForm())
	after := model.doc.Recording(model.recordingID())
	require.Equal(t, "nature", model.doc.RecordingFamily(model.recordingID()))
	require.NotEqual(t, *before.CalibrationGainDB, *after.CalibrationGainDB)
	dispatchRune(model, 'u')
	require.Equal(t, family, model.doc.RecordingFamily(model.recordingID()))
	require.Equal(t, before, model.doc.Recording(model.recordingID()))
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyCtrlR})
	require.Equal(t, "nature", model.doc.RecordingFamily(model.recordingID()))
	require.Equal(t, after, model.doc.Recording(model.recordingID()))
}

func TestStaleDocumentCompletionCannotReplaceNewerDraft(t *testing.T) {
	model := fixture(t)
	command := model.documentOperation(false, "saved", func(*audio.LibraryEditor) error { return nil })
	recording := model.doc.Recording(model.recordingID())
	recording.Note = "newer edit"
	require.NoError(t, model.doc.SetRecording(model.recordingID(), recording))
	finish(model, command)
	require.False(t, model.busy)
	require.Equal(t, "newer edit", model.doc.Recording(model.recordingID()).Note)
	require.True(t, model.statusIsError)
	require.Contains(t, model.status, "current draft retained")
}

func TestDuplicateDocumentCompletionIsIgnoredDuringNextOperation(t *testing.T) {
	model := fixture(t)
	first := model.documentOperation(false, "first", func(*audio.LibraryEditor) error { return nil })()
	model.Update(first)
	second := model.documentOperation(false, "second", func(*audio.LibraryEditor) error { return nil })
	model.Update(first)
	require.True(t, model.busy)
	finish(model, second)
	require.Equal(t, "second", model.status)
}

func TestCalibrationConfirmationRejectsRetargetedDraft(t *testing.T) {
	model := fixture(t)
	model.beginAutoCalibration()
	model.selectID("healing_02")
	finish(model, model.autoCalibrationKey(keyEnter))
	require.False(t, model.doc.Dirty())
	require.Contains(t, model.status, "stale")
}

func TestWrongSourceCannotBeCroppedOrPreviewed(t *testing.T) {
	model := fixture(t)
	model.selectID("healing_02") // Same physical file, different editing identity.
	require.False(t, model.setWindow(&audio.PlaybackWindow{StartMs: 10, EndMs: 200}))
	require.False(t, model.play())
	require.False(t, model.doc.Dirty())
	require.False(t, model.preview.Active())
}

func TestVariantFocusAndEscapeDoNotSilentlySwitchRecording(t *testing.T) {
	model := fixture(t)
	require.NoError(t, model.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	selectHealMoment(t, model)
	dispatchRune(model, 'V')
	dispatchRune(model, '[')
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, "healing_01", model.recordingID())
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyEsc})
	require.NotEmpty(t, model.variantCue, "first Escape leaves marker focus")
	require.Zero(t, model.marker)
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyEsc})
	require.Empty(t, model.variantCue, "second Escape leaves the variant list")
}

func TestSaveShortcutProtectsDraftAndWorksFromQuitConfirmation(t *testing.T) {
	model := fixture(t)
	dispatchRune(model, 's')
	model.input = "100"
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyCtrlS})
	require.Equal(t, fieldStart, model.field)
	require.False(t, model.doc.Dirty())
	require.Contains(t, model.status, "Finish or cancel")
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	dispatchRune(model, 'a')
	dispatchRune(model, 'q')
	require.True(t, model.confirmQuit)
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyCtrlS})
	require.False(t, model.doc.Dirty())
	require.False(t, model.confirmQuit)
	dispatchRune(model, 'D')
	_, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	require.Nil(t, command, "a clean document can still contain an unsaved duplicate draft")
	require.True(t, model.confirmQuit)
}

func TestTooSmallTerminalDoesNotAcceptInvisibleCropEdits(t *testing.T) {
	model := fixture(t)
	dispatchRune(model, '[')
	model.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyRight})
	require.False(t, model.doc.Dirty())
	require.Contains(t, model.View(), "Editing paused")
}

func TestCropKeepsStandalonePreviewOwnershipWhenSoundscapeIsVisible(t *testing.T) {
	model := fixture(t)
	dispatchRune(model, 'p')
	voice := model.preview.(*fakeVoice)
	require.NoError(t, voice.SeekMs(200))
	model.soundscapeVisible = true
	model.setWindow(&audio.PlaybackWindow{StartMs: 10, EndMs: 500})
	require.False(t, model.sceneFXPreview)
	require.Empty(t, model.mix, "editing an existing preview does not create a different transport owner")
	require.Equal(t, int64(200), voice.PositionMs())
}

func TestDuplicateIsolatesPreviousPreviewOwnersAndUsesItsDraftWindow(t *testing.T) {
	model := fixture(t)
	dispatchRune(model, 'p')
	standalone := model.preview.(*fakeVoice)
	model.soundscapeVisible = true
	dispatchRune(model, 'p')
	layer := model.layerAt(sfxSlot)
	require.NotNil(t, layer)
	layer.mode = modeRepeat
	require.True(t, layer.voice.Active())
	dispatchRune(model, 'D')
	require.False(t, standalone.Active())
	require.False(t, layer.voice.Playing())
	require.True(t, layer.paused, "the old repeating SFX stays suspended under the duplicate preview")
	dispatchRune(model, 'p')
	dispatchKey(model, tea.KeyMsg{Type: tea.KeySpace})
	dispatchRune(model, 's')
	model.input = "100"
	dispatchKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	model.updateCropPreview(true)
	require.Equal(t, int64(100), standalone.window.StartMs)
	require.True(t, model.previewIsPaused())
	require.False(t, model.doc.Dirty())
}
