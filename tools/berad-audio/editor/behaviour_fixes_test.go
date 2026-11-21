package editorui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func scanned(t *testing.T) *model {
	t.Helper()
	model := fixture(t)
	finish(model, model.scanNext())
	require.Equal(t, audio.RecordingReady, model.availability["healing_01"].State)

	return model
}

func TestUndoReportsAndRechecksOnlyChangedRecordings(t *testing.T) {
	model := scanned(t)
	fillSoundscape(t, model)
	generation := model.scanGeneration
	require.NoError(t, model.doc.SetRecording("healing_02", audio.Recording{Note: "x", File: model.doc.Recording("healing_02").File,
		Provenance: model.doc.Recording("healing_02").Provenance}))
	finish(model, model.history(false))
	require.Equal(t, "Undone.", model.status)
	require.Len(t, model.mix, int(soundscapeSlotCount), "an unrelated undo keeps the Soundscape")
	require.Equal(t, audio.RecordingReady, model.availability["healing_01"].State)
	require.Equal(t, generation, model.scanGeneration, "nothing source-related changed, so nothing is rechecked")
	finish(model, model.history(false))
	require.Equal(t, "Nothing to undo.", model.status)

	window := audio.PlaybackWindow{StartMs: 10, EndMs: 200}
	recording := model.doc.Recording("healing_01")
	recording.PlaybackWindow = &window
	require.NoError(t, model.doc.SetRecording("healing_01", recording))
	finish(model, model.history(false))
	require.Empty(t, model.mix, "layers of a changed recording are closed")
	require.Greater(t, model.scanGeneration, generation)
	require.Equal(t, audio.RecordingReady, model.availability["healing_02"].State)
	finish(model, model.history(true))
	require.Equal(t, "Redone.", model.status)
}

func TestScanRunsAPoolAndCountsResults(t *testing.T) {
	model := scanned(t)
	model.prepareAvailability()
	model.scanQueue = []string{"healing_01", "healing_02", "healing_01", "healing_02", "healing_01", "healing_02"}
	model.scanTotal = len(model.scanQueue)
	command := model.scanNext()
	require.Equal(t, maxConcurrentScans, model.scanActive)
	require.Len(t, model.scanQueue, 2)
	finish(model, command)
	require.Zero(t, model.scanActive)
	require.Empty(t, model.scanQueue)
	require.Equal(t, 6, model.scanDone)
	require.Equal(t, 6, model.scanTotal)
}

func TestStopLeavesFocusWhereItWas(t *testing.T) {
	model := fixture(t)
	model.focus = focusDetails
	dispatchRune(model, 'x')
	require.Equal(t, focusDetails, model.focus)
}

func TestLoopAndWholeFileTogglesOnlyRestartAPlayingPreview(t *testing.T) {
	model := fixture(t)
	voice := model.preview.(*fakeVoice)
	model.browserKey("l")
	require.Zero(t, voice.starts, "an idle toggle must not start playback")
	require.True(t, model.looping)
	require.Equal(t, "Loop on.", model.status)
	model.browserKey("enter")
	voice.position = 120
	model.browserKey("S")
	require.Equal(t, 2, voice.starts)
	require.True(t, model.fullSource)
	require.Equal(t, int64(120), voice.position, "the restart seeks back to where it was")
	voice.playing = false
	model.browserKey("S")
	require.Equal(t, 2, voice.starts)
	require.Equal(t, "Trimmed region.", model.status)
}

func TestFilePickerRemembersDirectoryAndAcceptsPastedPaths(t *testing.T) {
	model := fixture(t)
	finish(model, model.openPicker())
	library := model.picker.directory
	sounds := filepath.Join(library, "sounds")
	model.readDirectory(sounds)
	model.picker = nil
	finish(model, model.openPicker())
	require.Equal(t, sounds, model.picker.directory)

	model.readDirectory(library)
	paste := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("'" + sounds + "'\n"), Paste: true}
	finish(model, model.key(paste))
	require.Equal(t, sounds, model.picker.directory)
	require.Equal(t, `/a b/c`, pastedPath(`"/a\ b/c"`))
	require.NoError(t, os.RemoveAll(sounds))
	model.picker = nil
	finish(model, model.openPicker())
	require.Equal(t, library, model.picker.directory, "a vanished remembered directory falls back")
}

func TestBulkTileAssignmentClearsMarksAndKeepsItsStatus(t *testing.T) {
	model := fixture(t)
	finish(model, model.selectWorkspace(workspaceTiles))
	model.selectID("216")
	finish(model, model.openRoutingAssignment("cue"))
	model.routing.selected["178"] = true
	finish(model, model.applyRoutingChoice(routingNone))
	require.Empty(t, model.routing.selected)
	require.Contains(t, model.status, "Assignment updated")
	require.Empty(t, model.doc.Routing().Tiles["178"].AmbientCue)
}

func TestManualWorkspaceSwitchAbandonsEscReturn(t *testing.T) {
	model := fixture(t)
	model.routing.hasReturn = true
	finish(model, model.browserKey("tab"))
	require.False(t, model.routing.hasReturn)
	model.routing.hasReturn = true
	finish(model, model.routingKey("shift+tab"))
	require.False(t, model.routing.hasReturn)
}

func TestReplacingSeveralClipsNeedsASecondEnter(t *testing.T) {
	model := scanned(t)
	require.NoError(t, model.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	model.workspace = workspaceMoments
	model.refreshKeys()
	model.selectID("sfx/SoundHeal")
	finish(model, model.chooseFileOrAssignment())
	model.selectID("healing_02")
	finish(model, model.acceptAssignment())
	require.True(t, model.assigning)
	require.Contains(t, model.status, "This replaces 2 clips with one. Enter again to confirm; Esc cancels.")
	finish(model, model.acceptAssignment())
	require.False(t, model.assigning)
	require.Equal(t, []string{"healing_02"}, model.doc.Cue("sfx/SoundHeal").Recordings())
}

func TestFormStepsShowNoFloatNoise(t *testing.T) {
	model := fixture(t)
	model.form = &editorForm{fields: []formField{{label: "Fade in", value: "1.1", step: 0.1}, {label: "Radius", value: "7", step: 1}}}
	model.adjustFormField(true)
	require.Equal(t, "1.2", model.form.fields[0].value)
	model.form.cursor = 1
	model.adjustFormField(true)
	require.Equal(t, "8", model.form.fields[1].value)
}

func TestNewAmbientUsesThePresetPrefix(t *testing.T) {
	model := fixture(t)
	model.options.Ambient.CuePrefix = "custom/"
	finish(model, model.openNewAmbientForm())
	model.form.fields[newNameField].value, model.form.fields[newIDField].value = "Cave", "cave"
	finish(model, model.applyForm())
	require.Nil(t, model.form)
	require.Equal(t, "custom/cave", model.selectedID())
}

func TestHelpScrollsByRealRowsWithPagingAndBounds(t *testing.T) {
	model := fixture(t)
	model.openHelp()
	limit, page := model.infoBounds()
	require.Positive(t, limit)
	model.infoKey(keyDown)
	require.Equal(t, 1, model.infoDialog.scroll, "the first Down scrolls")
	model.infoKey("end")
	require.Equal(t, limit, model.infoDialog.scroll)
	model.infoKey(keyDown)
	require.Equal(t, limit, model.infoDialog.scroll, "no dead presses past the end")
	model.infoKey(keyPageUp)
	require.Equal(t, max(0, limit-page), model.infoDialog.scroll)
	model.infoKey("home")
	require.Zero(t, model.infoDialog.scroll)
	model.infoKey(keyDown)
	view := model.View()
	require.False(t, strings.Contains(view, "BROWSING AND PLAYBACK"), "the first row scrolled away")
	require.Contains(t, strings.Join(model.infoDialog.lines, "\n"), "l: loop")
}

func TestInitialSlidersValidateAndSeedPreviewVolume(t *testing.T) {
	model := fixture(t)
	options := testViewOptions()
	options.InitialSliders = &SliderLevels{Music: 3, SFX: 4, Ambience: 5}
	require.NoError(t, ValidateOptions(model.doc, options))
	seeded := newModel(model.doc, options)
	t.Cleanup(seeded.close)
	require.Equal(t, [previewCategoryCount]int{3, 4, 5}, seeded.previewSliders)
	options.InitialSliders = &SliderLevels{Music: 11}
	require.Error(t, ValidateOptions(model.doc, options))
}
