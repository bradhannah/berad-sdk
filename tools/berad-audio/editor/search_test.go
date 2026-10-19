package editorui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestFilterSearchesDescriptionsKeywordsAndLinkedUsages(t *testing.T) {
	m := fixture(t)
	for query, expected := range map[string]string{
		"lockpick": "physical_unlock_01", "grating": "stone_steps_01", "injured character": "healing_01",
	} {
		m.search = query
		m.refreshKeys()
		require.Contains(t, m.keys, expected, query)
	}
	m.workspace = workspaceMoments
	m.search = "grating category:sfx family:footstep"
	m.refreshKeys()
	require.Equal(t, []string{"sfx/SoundWalkMetal1"}, m.keys)
	m.search = ""
	m.refreshKeys()
	m.selectID("sfx/SoundHeal")
	m.beginMetadataEdit(false)
	m.input = "warm chime, restorative, WARM CHIME"
	m.commitInput()
	require.Equal(t, []string{"warm chime", "restorative"}, m.doc.Cue("sfx/SoundHeal").SearchTerms)
	m.search = "warm chime"
	m.refreshKeys()
	require.Contains(t, m.keys, "sfx/SoundHeal")
	require.True(t, m.doc.Undo())
	m.refreshKeys()
	require.NotContains(t, m.keys, "sfx/SoundHeal")
}

func TestExistingFilterUsesConventionalBackspaceClearAndCancel(t *testing.T) {
	m := fixture(t)
	m.search = "family:magic"
	m.refreshKeys()
	m.browserKey("/")
	view := ansi.Strip(m.View())
	require.Contains(t, view, "EDIT FILTER")
	require.Contains(t, view, "FILTER › family:magic▏")
	m.key(tea.KeyMsg{Type: tea.KeyBackspace})
	require.Equal(t, "family:magi", m.input)
	m.key(tea.KeyMsg{Type: tea.KeyCtrlU})
	require.Empty(t, m.input)
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("door")})
	m.key(tea.KeyMsg{Type: tea.KeyBackspace})
	require.Equal(t, "doo", m.input)
	m.key(tea.KeyMsg{Type: tea.KeySpace})
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("creak")})
	require.Equal(t, "doo creak", m.input, "terminal space keys must work in multiword searches")
	m.key(tea.KeyMsg{Type: tea.KeyEsc})
	require.Equal(t, "family:magic", m.search, "draft edits do not change the applied filter")
	m.browserKey("/")
	m.key(tea.KeyMsg{Type: tea.KeyCtrlU})
	m.key(tea.KeyMsg{Type: tea.KeyEnter})
	require.Empty(t, m.search)
	require.Len(t, m.keys, len(m.doc.RecordingIDs()))
	m.beginSearch()
	m.input = strings.Repeat("very long keyword ", 20)
	line := m.searchLine(30)
	require.Equal(t, 30, ansi.StringWidth(line))
	require.Contains(t, line, "▏", "long drafts keep the insertion position visible")
}

func TestShiftArrowsPageBrowserAndDetailsWithoutEditingVariants(t *testing.T) {
	m := fixture(t)
	m.cursor = 0
	page := m.listHeight()
	m.key(tea.KeyMsg{Type: tea.KeyShiftDown})
	require.Equal(t, page, m.cursor)
	m.key(tea.KeyMsg{Type: tea.KeyShiftUp})
	require.Zero(t, m.cursor)
	m.selectID("healing_01")
	m.focus = focusDetails
	m.key(tea.KeyMsg{Type: tea.KeyShiftDown})
	require.Positive(t, m.detailsScroll)
	m.key(tea.KeyMsg{Type: tea.KeyShiftUp})
	require.Zero(t, m.detailsScroll)
	require.NoError(t, m.doc.SetCueRecordings("sfx/SoundHeal", []string{"healing_01", "healing_02"}))
	selectHealMoment(t, m)
	finish(m, m.openVariants())
	m.key(tea.KeyMsg{Type: tea.KeyShiftDown})
	require.Equal(t, []string{"healing_01", "healing_02"}, m.doc.Cue("sfx/SoundHeal").Recordings())
	m.key(tea.KeyMsg{Type: tea.KeyCtrlUp})
	require.Equal(t, []string{"healing_02", "healing_01"}, m.doc.Cue("sfx/SoundHeal").Recordings())
}
