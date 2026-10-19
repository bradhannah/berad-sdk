package editorui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func TestAvailabilityFiltersIncludeMomentFileResolution(t *testing.T) {
	model := fixture(t)
	model.filter = filterExisting
	model.refreshKeys()
	require.Equal(t, []string{"healing_01", "healing_02"}, model.keys)
	model.availability["healing_02"] = audio.RecordingAvailability{State: audio.RecordingInvalid, Exists: true, Reason: "decode failed"}
	model.filter = filterProblems
	model.refreshKeys()
	require.Equal(t, []string{"healing_02"}, model.keys)
	model.filter = filterExisting
	model.refreshKeys()
	require.Contains(t, model.keys, "healing_02", "existing does not falsely mean playable")
	model.workspace = workspaceMoments
	model.search = "SoundHeal"
	model.refreshKeys()
	require.Contains(t, model.keys, "sfx/SoundHeal")
	require.Equal(t, audio.RecordingReady, model.itemStatus("sfx/SoundHeal").State)
	model.filter = filterMissing
	model.refreshKeys()
	require.NotContains(t, model.keys, "sfx/SoundHeal")
	model.search = ""
	model.refreshKeys()
	require.NotEmpty(t, model.keys)
	for _, id := range model.keys {
		require.False(t, model.itemStatus(id).Exists)
	}
}

func TestClassificationFacetsAndEditing(t *testing.T) {
	m := fixture(t)
	m.search = "family:magic category:sfx"
	m.refreshKeys()
	require.Contains(t, m.keys, "healing_01")
	require.NotContains(t, m.keys, "door_open_01")
	m.search = "family:magic category:unassigned"
	m.refreshKeys()
	require.Empty(t, m.keys)
	m.workspace = workspaceMoments
	m.search = "family:mechanical_interaction phase:action outcome:success"
	m.refreshKeys()
	require.Contains(t, m.keys, "sfx/SoundPushObject")
	require.NotContains(t, m.keys, "sfx/SoundHeal")
	m.selectID("sfx/SoundPushObject")
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	require.Equal(t, formClassification, m.form.kind)
	m.form.fields[0].value = "magic_activation"
	m.applyForm()
	require.Equal(t, "magic_activation", m.doc.CueProfile("sfx/SoundPushObject"))
	require.True(t, m.doc.Dirty())
}

func TestWorkspacesAndAssignmentPickerPreserveBrowseState(t *testing.T) {
	model := fixture(t)
	model.search = "healing"
	model.filter = filterExisting
	model.refreshKeys()
	model.selectID("healing_02")
	finish(model, model.switchWorkspace(true))
	model.search = "SoundHeal"
	model.filter = filterExisting
	model.refreshKeys()
	model.selectID("sfx/SoundHeal")
	before := model.currentBrowseState()
	finish(model, model.chooseFileOrAssignment())
	model.filter = filterMissing
	model.search = ""
	model.refreshKeys()
	finish(model, model.cancelAssignment())
	require.Equal(t, before, model.currentBrowseState())
	finish(model, model.switchWorkspace(false))
	require.Equal(t, "healing", model.search)
	require.Equal(t, filterExisting, model.filter)
	require.Equal(t, "healing_02", model.selectedID())
	finish(model, model.switchWorkspace(true))
	require.Equal(t, before, model.currentBrowseState())
}

func TestAvailabilityScanDoesNotMoveSelectionAndRejectsStaleResults(t *testing.T) {
	model := fixture(t)
	selected := model.selectedID()
	message := availabilityMsg{generation: model.scanGeneration, id: "healing_02", signature: sourceSignature(model.doc, "healing_02"),
		status: audio.RecordingAvailability{State: audio.RecordingReady, Exists: true}}
	model.Update(message)
	require.Equal(t, selected, model.selectedID())
	message.generation--
	message.status.State = audio.RecordingInvalid
	model.Update(message)
	require.Equal(t, audio.RecordingReady, model.availability["healing_02"].State)
	message.generation = model.scanGeneration
	message.signature = "old source"
	model.Update(message)
	require.Equal(t, audio.RecordingReady, model.availability["healing_02"].State)
}

func TestFocusedDetailsAndWaveformArrowsDoNotBrowse(t *testing.T) {
	model := fixture(t)
	selected := model.selectedID()
	model.browserKey("d")
	model.key(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, selected, model.selectedID())
	require.Greater(t, model.detailsScroll, 0)
	model.browserKey("w")
	model.key(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, selected, model.selectedID())
	model.browserKey(keyEscape)
	require.Equal(t, focusBrowser, model.focus)
}
