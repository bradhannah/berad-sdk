package editorui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

func TestRoutingWorkspacesRetainSelectionAndShowDistinctTabs(t *testing.T) {
	m := fixture(t)
	before := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(before) })
	for workspace := workspaceRecordings; workspace < workspaceCount; workspace++ {
		finish(m, m.selectWorkspace(workspace))
		tabs := m.workspaceTitle()
		require.Contains(t, tabs, "\x1b[", "tabs must use terminal styling")
		require.Contains(t, ansi.Strip(tabs), fmt.Sprintf("▶ %d %s ◀", workspace+1, strings.ToUpper(workspaceName(workspace))))
		require.Contains(t, ansi.Strip(m.View()), "AUDIO LIBRARY EDITOR  /  "+strings.ToUpper(workspaceName(workspace)))
		for _, width := range []int{60, 80, 120} {
			for _, height := range []int{24, 36} {
				m.width, m.height = width, height
				view := m.View()
				require.Len(t, strings.Split(view, "\n"), m.height)
				for _, line := range strings.Split(view, "\n") {
					require.LessOrEqual(t, ansi.StringWidth(line), width)
				}
			}
		}
	}
	finish(m, m.selectWorkspace(workspaceTiles))
	m.search = "fountain"
	m.refreshKeys()
	require.Equal(t, []string{"216"}, m.keys, "four frames have one editable family")
	finish(m, m.selectWorkspace(workspaceLocations))
	finish(m, m.selectWorkspace(workspaceTiles))
	require.Equal(t, "fountain", m.search)
	require.Equal(t, "216", m.selectedID())
}

func TestTileBulkAssignmentIsAtomicAndUsageNavigationReturns(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("216")
	m.routing.selected["178"] = true
	before := m.doc.Routing()
	finish(m, m.openRoutingAssignment("cue"))
	finish(m, m.applyRoutingChoice("ambient/AmbientForest"))
	for _, tile := range []string{"216", "178"} {
		require.Equal(t, "ambient/AmbientForest", m.doc.Routing().Tiles[tile].AmbientCue)
	}
	require.True(t, m.doc.Undo())
	require.Equal(t, before, m.doc.Routing())
	m.routing.selected = make(map[string]bool)
	m.openRoutingUsages()
	choices := m.routingMenuChoices()
	require.NotEmpty(t, choices)
	finish(m, m.jumpToAudioUsage(*choices[0].usage))
	require.Equal(t, workspaceMoments, m.workspace)
	require.Equal(t, "ambient/AmbientFountain", m.selectedID())
	finish(m, m.returnFromRoutingJump())
	require.Equal(t, workspaceTiles, m.workspace)
	require.Equal(t, "216", m.selectedID())
}

func TestLocationSilenceAndInheritanceAndNewBackground(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceLocations))
	m.selectID("music|location/2")
	finish(m, m.openRoutingAssignment("cue"))
	finish(m, m.applyRoutingChoice(routingNone))
	cue, explicit := m.doc.ResolveAssignment("music", "location/2")
	require.Empty(t, cue)
	require.True(t, explicit)
	finish(m, m.openRoutingAssignment("cue"))
	finish(m, m.applyRoutingChoice(routingInherit))
	cue, explicit = m.doc.ResolveAssignment("music", "location/2")
	require.Equal(t, "music/track/overworld", cue)
	require.False(t, explicit)
	m.openNewAmbientForm()
	m.form.fields[newNameField].value = "Cave"
	m.form.fields[newIDField].value = "cave"
	m.form.fields[newRadiusField].value = "0"
	finish(m, m.applyForm())
	require.Equal(t, workspaceMoments, m.workspace)
	require.Equal(t, "ambient/cave", m.selectedID())
	finish(m, m.selectWorkspace(workspaceLocations))
	m.selectID("dungeon|default")
	finish(m, m.openRoutingAssignment("cue"))
	choices := m.routingMenuChoices()
	require.True(t, len(choices) >= 2)
	for _, choice := range choices {
		require.NotEqual(t, "ambient/AmbientFountain", choice.id, "spatial sound cannot be a dungeon bed")
	}
	finish(m, m.applyRoutingChoice("ambient/cave"))
	// Explicitly choose inheritance for the fixture's second target.
	m.selectID("dungeon|location/33")
	finish(m, m.openRoutingAssignment("cue"))
	finish(m, m.applyRoutingChoice(routingInherit))
	finish(m, m.save())
	require.False(t, m.doc.Dirty())
	reopened, err := openTestEditor(m.doc.Path())
	require.NoError(t, err)
	cue, explicit = reopened.ResolveAssignment("dungeon", "location/33")
	require.Equal(t, "ambient/cave", cue)
	require.False(t, explicit)
}

func TestNavalSituationPickerSeparatesSailingMusicFromOnboardAmbience(t *testing.T) {
	m := fixture(t)
	finish(m, m.selectWorkspace(workspaceLocations))
	m.routing.scope = 4
	m.refreshKeys()
	require.Contains(t, m.keys, "naval|frigate")
	require.Contains(t, m.keys, "naval|skiff")
	require.Contains(t, m.keys, "naval|sailing")
	require.Contains(t, m.doc.CueIDs(), "ambient/frigate_onboard")
	require.Contains(t, m.doc.CueIDs(), "ambient/skiff_onboard")
	require.Equal(t, "ambient/frigate_onboard", m.routingCueFor("naval|frigate"))
	require.Equal(t, "ambient/skiff_onboard", m.routingCueFor("naval|skiff"))
	m.selectID("naval|sailing")
	require.Equal(t, "music/track/hornpipe", m.routingCue())
	finish(m, m.openRoutingAssignment("cue"))
	require.Equal(t, "CHOOSE SAILING MUSIC", m.routing.menu.title)
	for _, choice := range m.routingMenuChoices() {
		if choice.cue != "" {
			require.Contains(t, choice.cue, "music/track/")
		}
	}
	finish(m, m.routingMenuKey(tea.KeyMsg{Type: tea.KeyEsc}))
	m.selectID("naval|frigate")
	finish(m, m.openRoutingAssignment("cue"))
	require.Equal(t, "CHOOSE ONBOARD AMBIENCE", m.routing.menu.title)
	for _, choice := range m.routingMenuChoices() {
		if choice.cue != "" {
			require.Contains(t, choice.cue, "ambient/")
			require.Zero(t, m.doc.Routing().Ambient[choice.cue].RadiusTiles)
		}
	}
	finish(m, m.routingMenuKey(tea.KeyMsg{Type: tea.KeyEsc}))
	m.openNewAmbientForm()
	m.form.fields[newNameField].value = "Deck creak"
	m.form.fields[newIDField].value = "deck_creak"
	m.form.fields[newRadiusField].value = "0"
	finish(m, m.applyForm())
	finish(m, m.selectWorkspace(workspaceLocations))
	m.routing.scope = 4
	m.refreshKeys()
	m.selectID("naval|frigate")
	finish(m, m.openRoutingAssignment("cue"))
	require.Contains(t, m.routingMenuChoices(), routingChoice{id: "ambient/deck_creak", label: "Deck creak", cue: "ambient/deck_creak"})
	finish(m, m.applyRoutingChoice("ambient/deck_creak"))
	require.Equal(t, "ambient/deck_creak", m.routingCueFor("naval|frigate"))
	require.Equal(t, "ambient/skiff_onboard", m.routingCueFor("naval|skiff"))
	require.True(t, m.doc.Undo())
	require.Equal(t, "ambient/frigate_onboard", *m.doc.Routing().Groups["naval"]["frigate"])
}

func TestDistanceAuditionFadesWithoutRestartAndNeverPersists(t *testing.T) {
	m := fixture(t)
	cue := m.doc.Cue("ambient/AmbientFountain")
	cue.RecordingID = "healing_01"
	require.NoError(t, m.doc.SetCue("ambient/AmbientFountain", cue))
	finish(m, m.selectWorkspace(workspaceTiles))
	m.selectID("216")
	finish(m, m.loadSelection())
	require.NotNil(t, m.clip)
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	before := m.doc.Routing()
	require.True(t, m.playRouting())
	voice := m.preview.(*fakeVoice)
	require.True(t, voice.loop)
	require.Zero(t, voice.gain)
	now = now.Add(time.Second)
	m.updateRoutingPreview()
	closeGain := voice.gain
	m.routing.distance = 4
	m.refreshRoutingPreviewGain()
	require.Less(t, voice.gain, closeGain)
	require.Equal(t, 1, voice.starts)
	distantGain := voice.gain
	m.refreshPreviewVolumes()
	require.InDelta(t, distantGain, voice.gain, 1e-9, "category refresh retains distance and envelope")
	m.openPreviewVolumes()
	require.Equal(t, m.previewVolumeLines(80), m.detailLines(80), "routing details must not cover volume controls")
	m.volumeControls = false
	m.leaveRoutingRange()
	now = now.Add(2 * time.Second)
	m.updateRoutingPreview()
	require.False(t, voice.Active())
	require.Equal(t, before, m.doc.Routing())
	finish(m, m.openRoutingAssignment("cue"))
	finish(m, m.routingMenuKey(tea.KeyMsg{Type: tea.KeyEsc}))
	require.Equal(t, before, m.doc.Routing(), "canceling a picker never assigns")
}
