package editorui

import (
	tea "github.com/charmbracelet/bubbletea"
)

const fieldClipID = "clip ID"

type clipDraft struct {
	id, eligibleCue string
	appendToCue     bool
}

func (m *model) beginDuplicate() {
	if !m.beginCrop() {
		return
	}
	newID, err := m.doc.NextRecordingID(m.crop.recordingID)
	if err != nil {
		m.report(err)

		return
	}
	window := m.window()
	if window.EndMs < m.clip.DurationMs() {
		window.StartMs, window.EndMs = window.EndMs, m.clip.DurationMs()
	}
	cueID := m.crop.cueID
	if cueID == "" {
		if uses := m.doc.Usages(m.crop.recordingID); len(uses) == 1 {
			cueID = uses[0]
		}
	}
	if cueID != "" && m.cuePreviewCategory(cueID) != previewSFX {
		cueID = ""
	}
	m.duplicate = &clipDraft{id: newID, eligibleCue: cueID, appendToCue: false}
	m.crop.audition = false
	m.report(m.preview.Close())
	m.focusCropField(0)
	m.setWindow(&window)
	m.position = window.StartMs
	m.status = "Duplicate draft: p auditions the exact candidate; Tab selects Create/Cancel."
}

func (m *model) duplicateKey(key string) tea.Cmd { return m.cropKey(key) }

func (m *model) commitDuplicateField(field, value string) {
	if field == fieldClipID {
		m.duplicate.id = value

		return
	}
	m.commitNumber(field, value)
}
