package editorui

import (
	tea "github.com/charmbracelet/bubbletea"
)

const formRecordingPicker = "form recording"
const cueRecordingPicker = "sound recording"
const surfaceAddPicker = "add footstep"

func (m *model) isRecordingMenu() bool {
	return m.routing.menu != nil &&
		(m.routing.menu.kind == formRecordingPicker || m.routing.menu.kind == cueRecordingPicker)
}

func (m *model) openFormRecordingPicker() tea.Cmd {
	return m.openRecordingChoices(formRecordingPicker, "")
}

func (m *model) openSoundRecordingPicker() tea.Cmd {
	cue := m.assignmentCueForSelection()
	if cue == "" {
		m.status = "Choose an ambient sound first."

		return nil
	}

	return m.openRecordingChoices(cueRecordingPicker, cue)
}

func (m *model) openRecordingChoices(kind, target string) tea.Cmd {
	menu := &routingMenu{ //nolint:exhaustruct // Fresh child picker retains its parent selection and filter.
		kind: kind, title: "CHOOSE RECORDING", target: target, parent: m.routing.menu,
	}
	if kind == formRecordingPicker {
		menu.choices = append(menu.choices, routingChoice{formEmptyRecording, "Empty slot — supply a file later", "", nil})
	}
	for _, recording := range m.doc.RecordingIDs() {
		menu.choices = append(menu.choices, routingChoice{recording, recording, target, nil})
	}
	m.routing.menu = menu

	return m.loadSelection()
}

func (m *model) openSurfaceAddition() tea.Cmd {
	menu := &routingMenu{ //nolint:exhaustruct // Fresh compatible picker.
		kind: surfaceAddPicker, title: "ADD FOOTSTEP SOUND",
	}
	for _, cue := range m.doc.CueIDs() {
		if m.cuePreviewCategory(cue) == previewSFX {
			menu.choices = append(menu.choices, routingChoice{cue, m.doc.Cue(cue).Name, cue, nil})
		}
	}
	m.routing.menu = menu

	return m.loadSelection()
}

func (m *model) applyFormChoice(choice routingChoice) tea.Cmd {
	menu := m.routing.menu
	switch menu.kind {
	case formRecordingPicker:
		m.form.fields[newRecordingField].value = choice.id
	case surfaceAddPicker:
		m.form.sequence = append(m.form.sequence, choice.id)
		m.form.cursor = len(m.form.sequence) - 1
	case cueRecordingPicker:
		cue := m.doc.Cue(menu.target).WithRecordings([]string{choice.id})
		if err := m.doc.SetCue(menu.target, cue); err != nil {
			m.report(err)

			return nil
		}
	}
	m.routing.menu = menu.parent

	return m.loadSelection()
}

func (m *model) surfaceFormKey(key string) tea.Cmd {
	form := m.form
	const actions = 2
	last := len(form.sequence) + actions - 1
	switch key {
	case keyEscape:
		return m.closeForm()
	case "a":
		return m.openSurfaceAddition()
	case keyShiftTab, "up":
		form.cursor = max(0, form.cursor-1)

		return m.loadSelection()
	case keyTab, keyDown:
		form.cursor = min(last, form.cursor+1)

		return m.loadSelection()
	case "delete":
		if form.cursor < len(form.sequence) {
			form.sequence = append(form.sequence[:form.cursor], form.sequence[form.cursor+1:]...)
			form.cursor = max(0, min(form.cursor, len(form.sequence)-1))
		}

		return m.loadSelection()
	case "ctrl+up", keyCtrlDown:
		m.reorderSurfaceDraft(key == keyCtrlDown)
	case keyEnter:
		if form.cursor == len(form.sequence) {
			return m.applyForm()
		}
		if form.cursor == last {
			return m.closeForm()
		}

		return m.previewForm(false, false)
	case "p":
		return m.previewForm(false, false)
	case " ":
		return m.previewForm(false, true)
	}

	return nil
}

func (m *model) reorderSurfaceDraft(down bool) {
	form := m.form
	if form.cursor >= len(form.sequence) {
		return
	}
	other := max(0, form.cursor-1)
	if down {
		other = min(len(form.sequence)-1, form.cursor+1)
	}
	form.sequence[form.cursor], form.sequence[other] = form.sequence[other], form.sequence[form.cursor]
	form.cursor = other
}

func (m *model) importAssignedRecording() tea.Cmd {
	cue := m.assignmentCueForSelection()
	if cue == "" {
		return nil
	}
	recording := m.doc.Cue(cue).PrimaryRecording()
	m.jumpToAudioUsage(routingUsage{Workspace: "recordings", Target: recording, Label: recording})

	return m.openPicker()
}
