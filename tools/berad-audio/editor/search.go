package editorui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const fieldSearch = "search"
const fieldKeywords = "keywords"
const fieldTrigger = "typical trigger"

func (m *model) beginSearch() {
	m.beginInput(fieldSearch, m.search)
	m.focus = focusBrowser
}

func (m *model) browserSearchLines(width int) []string {
	if m.field != fieldSearch {
		return []string{m.searchLine(width)}
	}

	return []string{m.colors.focus.Render(fit("┏ EDIT FILTER — type below", width)),
		m.colors.key.Render(m.searchLine(width)), m.colors.focus.Render(fit("┗ Enter: apply · Escape: cancel", width))}
}

func (m *model) searchLine(width int) string {
	if m.field != fieldSearch {
		return fit(m.colors.detailLabel.Render("Filter: ")+m.colors.detailValue.Render(safe(m.search)), width)
	}
	const label = "FILTER › "
	available := max(1, width-ansi.StringWidth(label)-1)
	input := m.inputText.viewport(m.input, available, m.colors.selected.Render)

	return m.colors.selected.Render(fit(label+input, width))
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
