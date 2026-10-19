package editorui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestInvalidInlineInputRemainsVisibleAndDoesNotMutateLibrary(t *testing.T) {
	m := fixture(t)
	m.browserKey("g")
	m.input = "not-a-number"
	finish(m, m.commitInput())
	require.Equal(t, fieldGain, m.field)
	require.Equal(t, "not-a-number", m.input)
	require.NotEmpty(t, m.inputError)
	require.False(t, m.doc.Dirty())
	view := ansi.Strip(m.View())
	require.Contains(t, view, "EDITING")
	require.Contains(t, view, "not-a-number▏")
	lines := strings.Split(view, "\n")
	require.NotContains(t, strings.Join(lines[len(lines)-3:], "\n"), "not-a-number▏", "typing is never in the footer")
	m.input = "-3"
	finish(m, m.commitInput())
	require.Empty(t, m.field)
	require.Equal(t, -3.0, *m.doc.Recording("healing_01").CalibrationGainDB)
	require.True(t, m.doc.Undo())
	require.False(t, m.doc.Dirty())
}

func TestSoundSettingsDraftValidationPreviewCancelAndAtomicApply(t *testing.T) {
	m := fixture(t)
	cue := m.doc.Cue("ambient/AmbientFountain")
	cue.RecordingID = "healing_01"
	require.NoError(t, m.doc.SetCue("ambient/AmbientFountain", cue))
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("216")
	finish(m, m.loadSelection())
	before := m.doc.Routing()
	finish(m, m.editSharedAmbientProfile())
	require.NotNil(t, m.form)
	m.form.fields[1].value = "-1"
	finish(m, m.applyForm())
	require.NotNil(t, m.form)
	require.Equal(t, 1, m.form.errorField)
	require.Equal(t, "-1", m.form.fields[1].value)
	require.Equal(t, before, m.doc.Routing())
	m.form.fields[1].value = "1.5"
	finish(m, m.previewForm(false, false))
	require.True(t, m.preview.Playing())
	require.Equal(t, before, m.doc.Routing(), "draft preview is not an authored mutation")
	finish(m, m.closeForm())
	require.Equal(t, before, m.doc.Routing())
	require.False(t, m.preview.Active())
	finish(m, m.editSharedAmbientProfile())
	m.form.fields[0].value = "6"
	m.form.fields[1].value = "1.5"
	finish(m, m.applyForm())
	require.Nil(t, m.form)
	require.Equal(t, int64(1500), m.doc.Routing().Ambient["ambient/AmbientFountain"].FadeInMs)
	require.True(t, m.doc.Undo())
	require.Equal(t, before, m.doc.Routing(), "one undo restores the complete form")
}

func TestNewSoundFormCapturesTypingAndCanReuseRecordingAtomically(t *testing.T) {
	m := fixture(t)
	m.openNewAmbientForm()
	finish(m, m.key(tea.KeyMsg{Type: tea.KeyEnter}))
	finish(m, m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X Quiet Cave")}))
	require.Equal(t, "X Quiet Cave", m.form.fields[newNameField].value)
	require.Equal(t, "x_quiet_cave", m.form.fields[newIDField].value)
	require.Zero(t, m.preview.(*fakeVoice).starts, "letters in forms do not trigger playback or workspace shortcuts")
	m.form.fields[newRecordingField].value = "healing_01"
	m.form.fields[newRadiusField].value = "0"
	finish(m, m.applyForm())
	require.Nil(t, m.form)
	require.Equal(t, "healing_01", m.doc.Cue("ambient/x_quiet_cave").RecordingID)
	require.NotContains(t, m.doc.RecordingIDs(), "x_quiet_cave_01", "reusing a recording does not create an orphan slot")
	require.True(t, m.doc.Undo())
	require.False(t, m.doc.Dirty())
}

func TestFootstepSequenceIsAReorderableDraft(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("68")
	before := m.doc.Routing()
	m.editSurfaceCues()
	finish(m, m.surfaceFormKey("ctrl+down"))
	require.Equal(t, before, m.doc.Routing())
	require.NotEqual(t, before.Surfaces["stone"], m.form.sequence)
	finish(m, m.applyForm())
	require.NotEqual(t, before.Surfaces["stone"], m.doc.Routing().Surfaces["stone"])
	require.True(t, m.doc.Undo())
	require.Equal(t, before, m.doc.Routing())
	m.editSurfaceCues()
	for len(m.form.sequence) > 0 {
		finish(m, m.surfaceFormKey("delete"))
	}
	require.Empty(t, m.recordingID())
	finish(m, m.surfaceFormKey("enter"))
	require.False(t, m.preview.Active(), "a silent surface must not preview the tile's ambient recording")
}

func TestEnterRestartsCurrentVariantAndSpaceResumesIt(t *testing.T) {
	m := fixture(t)
	require.NoError(t, m.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	selectHealMoment(t, m)
	m.browserKey("enter")
	m.browserKey("enter")
	require.Equal(t, "healing_01", m.recordingID())
	voice := m.preview.(*fakeVoice)
	require.Equal(t, 2, voice.starts)
	m.browserKey(" ")
	require.False(t, voice.Playing())
	m.browserKey(" ")
	require.True(t, voice.Playing())
	require.Equal(t, 2, voice.starts)
	finish(m, m.browserKey("n"))
	require.Equal(t, "healing_02", m.recordingID())
}

func TestSpacePausesAmbientWithoutMarkingOrAdvancingItsFade(t *testing.T) {
	m := fixture(t)
	for _, id := range []string{"ambient/AmbientFountain", "sfx/SoundWalkGrassLeft"} {
		cue := m.doc.Cue(id)
		cue.RecordingID = "healing_01"
		require.NoError(t, m.doc.SetCue(id, cue))
	}
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("216")
	finish(m, m.loadSelection())
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	m.routingKey("enter")
	voice := m.preview.(*fakeVoice)
	now = now.Add(400 * time.Millisecond)
	m.updateRoutingPreview()
	envelope := m.routing.envelope
	m.routingKey(" ")
	require.False(t, voice.Playing())
	require.Empty(t, m.routing.selected)
	require.Contains(t, m.transportState(), "PAUSED")
	now = now.Add(time.Minute)
	m.updateRoutingPreview()
	require.Equal(t, envelope, m.routing.envelope)
	m.routingKey(" ")
	require.True(t, voice.Playing())
	require.Equal(t, 1, voice.starts)
	now = now.Add(400 * time.Millisecond)
	m.updateRoutingPreview()
	require.InDelta(t, 1.0, m.routing.envelope, 1e-9)
	m.routingKey("enter")
	require.Equal(t, 2, voice.starts, "Enter intentionally restarts")
	m.routingKey("insert")
	require.True(t, m.routing.selected["216"])
	before := m.routingItemLine("216", true, 70)
	finish(m, m.previewTileSound(true, false))
	require.Equal(t, before, m.routingItemLine("216", true, 70), "preview target never changes assignment columns")
	require.Equal(t, "ambient/AmbientFountain", m.routingCueFor("216"))
	require.Contains(t, m.previewDescription(), "footstep")
}

func TestDialogsFitSmallTerminalsAndKeepTabsAndActionsVisible(t *testing.T) {
	m := fixture(t)
	for _, size := range [][2]int{{60, 24}, {80, 30}, {120, 42}} {
		m.width, m.height = size[0], size[1]
		m.openNewAmbientForm()
		m.form.cursor = len(m.form.fields)
		view := ansi.Strip(m.View())
		require.Contains(t, view, "RECORDINGS")
		require.Contains(t, view, "NEW AMBIENT SOUND")
		require.Contains(t, view, "Apply changes")
		require.Contains(t, view, "Escape")
		assertFullScreen(t, m)
		finish(m, m.closeForm())
		m.openHelp()
		assertFullScreen(t, m)
		m.infoKey("esc")
	}
}

func TestPreviewVolumeControlsRemainVisibleAboveAnAssignmentDialog(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("216")
	finish(m, m.openRoutingAssignment("cue"))
	picker := m.routing.menu
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	require.True(t, m.volumeControls)
	require.Contains(t, ansi.Strip(m.View()), "PREVIEW VOLUME")
	m.key(tea.KeyMsg{Type: tea.KeyEsc})
	require.False(t, m.volumeControls)
	require.Same(t, picker, m.routing.menu)
	require.Contains(t, ansi.Strip(m.View()), "CHOOSE AMBIENT SOUND")
}

func TestTileSourceDialogOverridesPreviewAndKeepsOtherFireTilesIndependent(t *testing.T) {
	m := fixture(t)
	cue := m.doc.Cue("ambient/AmbientFire")
	cue.RecordingID = "healing_01"
	require.NoError(t, m.doc.SetCue("ambient/AmbientFire", cue))
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("178")
	finish(m, m.loadSelection())
	before := m.doc.Routing()
	finish(m, m.editAmbientProfile())
	require.Equal(t, formTileAmbient, m.form.kind)
	m.form.cursor = tileDistanceField
	m.form.fields[tileDistanceField].value = "12"
	m.updateTileOverrideMode()
	m.form.cursor = tileLevelField
	m.form.fields[tileLevelField].value = "50"
	m.updateTileOverrideMode()
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	finish(m, m.previewForm(false, false))
	now = now.Add(time.Second)
	m.updateRoutingPreview()
	require.Equal(t, before, m.doc.Routing(), "previewing source overrides never changes the library")
	wantGain := m.auditionGain("healing_01", "ambient/AmbientFire", true) * 0.5
	require.InDelta(t, wantGain, m.preview.(*fakeVoice).gain, 1e-9)
	finish(m, m.applyForm())
	profile, level := m.doc.Routing().TileAmbientSettings("178")
	require.Equal(t, 12.0, profile.RadiusTiles)
	require.Equal(t, 0.5, level)
	require.Equal(t, before.Tiles["269"], m.doc.Routing().Tiles["269"])
	require.Equal(t, before.Ambient, m.doc.Routing().Ambient)
	finish(m, m.editAmbientProfile())
	m.form.cursor = distanceModeField
	m.adjustFormField(true)
	m.form.cursor = levelModeField
	m.adjustFormField(true)
	finish(m, m.applyForm())
	require.Nil(t, m.doc.Routing().Tiles["178"].RadiusTiles)
	require.Nil(t, m.doc.Routing().Tiles["178"].Gain)
	require.True(t, m.doc.Undo(), "one undo restores both source overrides")
	require.NotNil(t, m.doc.Routing().Tiles["178"].Gain)
	finish(m, m.openRoutingAssignment("cue"))
	finish(m, m.applyRoutingChoice(routingNone))
	require.NotContains(t, m.doc.Routing().Tiles, "178", "clearing the sound also clears its obsolete overrides")
}
