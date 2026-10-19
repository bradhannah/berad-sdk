package editorui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func (m *model) browsingCues() bool {
	return m.workspace == workspaceMoments && !m.assigning && m.variantCue == ""
}

func (m *model) activeCue() string {
	if m.form != nil && m.routing.menu == nil {
		if m.form.kind == formSurface && m.form.cursor < len(m.form.sequence) {
			return m.form.sequence[m.form.cursor]
		}

		return m.form.cue
	}
	if m.routingWorkspace() || m.routing.menu != nil {
		return m.routingCue()
	}
	if m.assigning {
		return m.assignmentCue
	}
	if m.variantCue != "" {
		return m.variantCue
	}
	if m.workspace == workspaceMoments {
		return m.selectedID()
	}

	return ""
}

func (m *model) cueAvailability(cueID string) audio.RecordingAvailability {
	ids := m.doc.Cue(cueID).Recordings()
	status := audio.RecordingAvailability{State: audio.RecordingReady, Exists: true,
		Reason: fmt.Sprintf("%d playable clip(s); listening review is separate.", len(ids))}
	for _, recordingID := range ids {
		clip, exists := m.availability[recordingID]
		if !exists {
			clip = audio.RecordingAvailability{State: audio.RecordingNoFile, Exists: false, Reason: "No file assigned."}
		}
		status.Exists = status.Exists && clip.Exists
		if availabilityPriority(clip.State) > availabilityPriority(status.State) {
			status.State, status.Reason = clip.State, recordingID+": "+clip.Reason
		}
	}

	return status
}

//nolint:mnd // Ordered display-severity ranks; these values are not audio gains or gameplay numbers.
func availabilityPriority(state audio.RecordingState) int {
	switch state {
	case audio.RecordingReady:
		return 0
	case audio.RecordingChecking:
		return 1
	case audio.RecordingNoFile:
		return 2
	case audio.RecordingInvalid:
		return 3
	case audio.RecordingMissing:
		return 4
	}

	return 0
}

func (m *model) previewNextClip() tea.Cmd {
	if !m.browsingCues() || len(m.doc.Cue(m.selectedID()).Recordings()) < 2 {
		m.play()

		return nil
	}
	cueID := m.selectedID()
	ids := m.doc.Cue(cueID).Recordings()
	index := (m.currentVariant[cueID] + 1) % len(ids)
	path, err := m.doc.RecordingPath(ids[index])
	if err != nil {
		m.report(err)

		return nil
	}

	return m.loadSource(sourceLoad{recordingID: ids[index], path: path, purpose: sourceVariant,
		autoplay: true, cueID: cueID, variant: index})
}

func (m *model) openVariants() tea.Cmd {
	if !m.browsingCues() || m.cuePreviewCategory(m.selectedID()) != previewSFX {
		m.status = "Open an SFX moment to edit its ordered clips."

		return nil
	}
	m.variantState = m.currentBrowseState()
	m.variantCue = m.selectedID()
	m.search, m.filter = "", filterAll
	m.refreshKeys()
	m.cursor = 0
	m.focus = focusBrowser

	return m.loadSelection()
}

func (m *model) closeVariants() tea.Cmd {
	m.variantCue = ""
	m.restoreBrowseState(m.variantState)
	m.focus = focusBrowser

	return m.loadSelection()
}

func (m *model) beginAppendVariant() tea.Cmd {
	m.assignmentState = m.currentBrowseState()
	m.assigning, m.assignmentAppend, m.assignmentCue = true, true, m.variantCue
	m.search, m.filter = "", filterAll
	m.refreshKeys()
	m.cursor = 0
	m.status = "Enter appends the recording; Space previews/pauses; Escape cancels."

	return m.loadSelection()
}

func (m *model) variantKey(key string) tea.Cmd {
	switch key {
	case keyEscape:
		return m.closeVariants()
	case "a", "f":
		return m.beginAppendVariant()
	case keyDelete:
		return m.editVariants(0, true)
	case "ctrl+up":
		return m.editVariants(-1, false)
	case "ctrl+down":
		return m.editVariants(1, false)
	case "+", "-", keyFilter, keyTab, "1", "2":
		return nil
	}

	return m.browserKey(key)
}

func (m *model) editVariants(direction int, remove bool) tea.Cmd {
	ids := m.doc.Cue(m.variantCue).Recordings()
	index := slices.Index(ids, m.selectedID())
	if index < 0 {
		return nil
	}
	if remove {
		if len(ids) == 1 {
			m.status = "Keep one clip, or replace its assignment from Moments."

			return nil
		}
		ids = append(ids[:index], ids[index+1:]...)
	} else {
		destination := max(0, min(len(ids)-1, index+direction))
		ids[index], ids[destination] = ids[destination], ids[index]
	}
	if err := m.doc.SetCueRecordings(m.variantCue, ids); err != nil {
		m.report(err)

		return nil
	}
	m.currentVariant[m.variantCue] = 0
	m.clearSoundscape()
	m.refreshKeys()

	return m.loadSelection()
}
