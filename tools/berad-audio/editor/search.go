package editorui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const fieldSearch = "search"
const fieldKeywords = "keywords"
const fieldTrigger = "typical trigger"
const searchPromptWidth = 4
const facetCategory = "category"
const facetFamily = "family"

func (m *model) beginSearch() {
	m.searchBefore = m.search
	m.beginInput(fieldSearch, m.search)
	m.focus = focusBrowser
}

// The filter narrows the list while it is typed; Escape puts the old one back.
func (m *model) applySearch(value string) tea.Cmd {
	selected := m.selectedID()
	m.search = strings.TrimSpace(value)
	m.refreshKeys()
	m.status, m.statusIsError = "", false
	var command tea.Cmd
	if selected != m.selectedID() {
		command = m.loadSelection()
	}
	if hint := m.unknownFilterWord(); hint != "" {
		m.status = hint
	}

	return command
}

// A "word:value" term with a word the editor does not know can never match.
func (m *model) unknownFilterWord() string {
	if m.routingWorkspace() {
		return ""
	}
	for _, term := range strings.Fields(strings.ToLower(m.search)) {
		word, _, keyed := strings.Cut(term, ":")
		if keyed && !slices.Contains(filterWords(), word) {
			return "No filter is called \"" + safe(word) + "\". Try " + strings.Join(filterWords(), ", ") + "."
		}
	}

	return ""
}

func filterWords() []string {
	return []string{facetCategory, facetFamily, "phase", "outcome", "role", routingEmitter, "context"}
}

// One row, shown only while a filter is set or being typed.
func (m *model) browserSearchLines(width int) []string {
	if !m.searchRowVisible() {
		return nil
	}
	text, tail := safe(m.search), " · / edits "
	count := fmt.Sprintf("%d of %d", len(m.keys), len(m.workspaceIDs()))
	if m.field == fieldSearch {
		tail = " · Esc clears "
		text = m.inputText.viewport(m.input, max(1, width-ansi.StringWidth(count+tail)-searchPromptWidth),
			m.colors.selected.Render)
	}
	left := " " + m.colors.hintKey.Render("/") + " " + text
	gap := max(1, width-ansi.StringWidth(left)-ansi.StringWidth(count+tail))

	return []string{fit(left+strings.Repeat(" ", gap)+m.colors.dim.Render(count+tail), width)}
}

func (m *model) beginMetadataEdit(trigger bool) {
	if m.selectedID() == "" || m.assigning {
		return
	}
	if trigger {
		if !m.browsingCues() {
			m.status = "Open a Moment to edit its typical trigger; shared recordings may have several uses."

			return
		}
		m.beginInput(fieldTrigger, m.doc.Cue(m.activeCue()).HowToHear)

		return
	}
	terms := m.doc.Recording(m.recordingID()).SearchTerms
	if m.browsingCues() {
		terms = m.doc.Cue(m.activeCue()).SearchTerms
	}
	m.beginInput(fieldKeywords, strings.Join(terms, ", "))
}

func (m *model) commitKeywords(value string) {
	var terms []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(value, ",") {
		term := strings.TrimSpace(part)
		key := strings.ToLower(term)
		if term != "" && !seen[key] {
			terms = append(terms, term)
			seen[key] = true
		}
	}
	if m.browsingCues() {
		cue := m.doc.Cue(m.activeCue())
		cue.SearchTerms = terms
		m.report(m.doc.SetCue(m.activeCue(), cue))
	} else {
		recording := m.doc.Recording(m.recordingID())
		recording.SearchTerms = terms
		m.report(m.doc.SetRecording(m.recordingID(), recording))
	}
}

func (m *model) metadataDetails(recording audio.Recording, add func(string, string)) {
	if len(recording.SearchTerms) > 0 {
		add("Recording keywords: ", strings.Join(recording.SearchTerms, ", "))
	}
	if m.workspace == workspaceMoments {
		cue := m.doc.Cue(m.activeCue())
		add("Typical trigger: ", cue.HowToHear)
		add("Moment keywords: ", strings.Join(cue.SearchTerms, ", "))
		add("All moment search terms: ", strings.Join(m.doc.CueSearchTerms(m.activeCue()), ", "))

		return
	}
	usages := m.doc.Usages(m.recordingID())
	if len(usages) == 1 {
		add("Typical trigger: ", m.doc.Cue(usages[0]).HowToHear)
	} else if len(usages) > 1 {
		names := make([]string, 0, len(usages))
		for _, cueID := range usages {
			names = append(names, m.doc.Cue(cueID).Name)
		}
		add("Shared moments: ", strings.Join(names, ", ")+"; per-use triggers below.")
	}
}
