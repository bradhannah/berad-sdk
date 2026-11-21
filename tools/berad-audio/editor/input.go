package editorui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const minReplaceClips = 2

const (
	inputTyping = iota
	inputApply
	inputCancel
	inputFocusCount
)

func (m *model) beginInput(field, value string) {
	m.field, m.input, m.inputError = field, value, ""
	m.inputAction, m.statusIsError = inputTyping, false
	m.inputText.reset(value)
}

// beginValueInput opens a one-value edit with the current value selected.
func (m *model) beginValueInput(field, value string) {
	m.beginInput(field, value)
	m.inputText.selectAll(value)
}

func (m *model) beginGainInput() {
	if m.selectedID() == "" || m.assigning {
		return
	}
	if m.browsingCues() {
		m.beginValueInput(fieldGain, strings.TrimSuffix(percent(m.doc.Cue(m.selectedID()).VolumeModifier), "%"))

		return
	}
	value := 0.0
	if gain := m.doc.Recording(m.recordingID()).CalibrationGainDB; gain != nil {
		value = *gain
	}
	m.beginValueInput(fieldGain, strconv.FormatFloat(value, 'f', 1, 64))
}

func (m *model) beginBoundInput(field string) {
	position := m.window().StartMs
	if field == fieldEnd {
		position = m.window().EndMs
	}
	m.beginValueInput(field, clock(position, true))
}

func (m *model) cancelInput() {
	m.field, m.input, m.inputError = "", "", ""
	m.inputAction = inputTyping
	m.inputText.reset("")
}

func (m *model) discardInput() {
	m.cancelInput()
	m.status, m.statusIsError = "Field draft discarded.", false
}

func (m *model) editInput(key tea.KeyMsg) tea.Cmd {
	name := textControlKey(key)
	switch name {
	case keyEscape:
		if m.field == fieldSearch {
			before := m.searchBefore
			m.cancelInput()

			return m.applySearch(before)
		}
		m.discardInput()

		return nil
	case keyTab, keyShiftTab:
		if m.inlineInput() {
			return m.commitInput()
		}
		direction := 1
		if name == keyShiftTab {
			direction = inputFocusCount - 1
		}
		m.inputAction = (m.inputAction + direction) % inputFocusCount

		return nil
	case keyEnter:
		switch {
		case m.inputAction == inputCancel:
			m.discardInput()
		case m.inputAction == inputApply || m.field != fieldNote:
			return m.commitInput()
		default:
			m.inputText.edit(&m.input, key, true)
		}

		return nil
	}
	if m.inputAction == inputTyping {
		m.inputText.edit(&m.input, key, m.field == fieldNote)
	} else if name == keyLeft || name == keyRight {
		m.inputAction = inputFocusCount - m.inputAction
	}
	if m.field == fieldSearch {
		return m.applySearch(m.input)
	}

	return nil
}

func (m *model) beginClassificationEdit() {
	if m.selectedID() == "" || m.assigning {
		return
	}
	value, options, cue := m.doc.RecordingFamily(m.recordingID()), m.doc.AcousticFamilies(), ""
	if m.browsingCues() {
		cue = m.activeCue()
		value, options = m.doc.CueProfile(cue), m.doc.ClassificationProfiles()
	}
	m.form = &editorForm{ //nolint:exhaustruct_v5 // Choice draft applies explicitly.
		kind: formClassification, title: "CLASSIFICATION", cue: cue, errorField: -1,
		fields: []formField{{label: "Classification", value: value,
			help: "Left/Right chooses an authored classification.", unit: "", step: 0, options: options}},
	}
}

func (m *model) commitInput() tea.Cmd {
	m.inputError, m.statusIsError = "", false
	m.committingInput = true
	defer func() { m.committingInput = false }()
	command := m.commitInputValue()
	if m.inputError == "" {
		m.cancelInput()
	} else {
		m.inputAction = inputTyping
	}

	return command
}

func (m *model) commitInputValue() tea.Cmd {
	selected := m.selectedID()
	field, value := m.field, normalizedFieldInput(m.field, m.input)
	if m.duplicate != nil {
		m.commitDuplicateField(field, value)

		return nil
	}
	switch field {
	case fieldLicense:
		m.report(m.doc.SetRecordingLicense(m.licenseRecordingID, value))
	case fieldCalibrationTarget:
		m.setCalibrationTarget(value)
	case fieldSearch:
		return m.applySearch(value)
	case "new":
		return m.createEmptyRecording(value)
	case fieldNote:
		m.commitNote(value)
	case "classification":
		m.commitClassification(value)
		m.refreshKeys()
	case "path":
		return m.enterPickerPath(value)
	case fieldKeywords:
		m.commitKeywords(value)
		m.refreshKeys()
	case fieldTrigger:
		cue := m.doc.Cue(m.activeCue())
		cue.HowToHear = value
		m.report(m.doc.SetCue(m.activeCue(), cue))
		m.refreshKeys()
	default:
		m.commitNumber(field, value)
	}
	if selected != m.selectedID() {
		return m.loadSelection()
	}

	return nil
}

func normalizedFieldInput(field, value string) string {
	switch field {
	case fieldNote, "path":
		return value // Preserve authored note whitespace and exact filesystem paths.
	default:
		return strings.TrimSpace(value)
	}
}

func (m *model) commitNote(value string) {
	recording := m.doc.Recording(m.recordingID())
	recording.Note = value
	m.report(m.doc.SetRecording(m.recordingID(), recording))
	m.refreshKeys()
}

func (m *model) commitClassification(value string) {
	if m.browsingCues() {
		m.report(m.doc.SetCueProfile(m.activeCue(), value))

		return
	}
	m.report(m.doc.SetRecordingFamily(m.recordingID(), value))
	m.preview.SetGain(m.livePreviewGain())
	m.refreshMixGains()
}

func (m *model) createEmptyRecording(recordingID string) tea.Cmd {
	if err := m.doc.AddRecording(recordingID); err != nil {
		m.report(err)

		return nil
	}
	m.search, m.filter = "", filterAll
	m.refreshKeys()
	m.selectID(recordingID)

	return m.loadSelection()
}

func (m *model) commitNumber(field, value string) {
	if field == fieldStart || field == fieldEnd {
		m.commitBound(field, value)

		return
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		m.rejectInput("Type a number.")

		return
	}
	if m.browsingCues() {
		if number < 0 || number > percentPerUnit {
			m.rejectInput("Type a volume from 0 to 100.")

			return
		}
		number /= percentPerUnit
	}
	m.setGain(number)
}

func (m *model) commitBound(field, value string) {
	position, valid := parseClock(value)
	if !valid || m.clip == nil || position > m.clip.DurationMs() {
		m.rejectInput("Type a time inside the sound, like 1:35.000 or 95.5.")

		return
	}
	window := m.window()
	if field == fieldStart {
		window.StartMs = position
	} else {
		window.EndMs = position
	}
	m.setWindow(&window)
}

func (m *model) selectID(id string) {
	for index, key := range m.keys {
		if key == id {
			m.cursor = index

			return
		}
	}
}

func (m *model) chooseFileOrAssignment() tea.Cmd {
	if m.selectedID() == "" {
		return nil
	}
	m.focus = focusBrowser
	if m.workspace == workspaceMoments && !m.assigning {
		if m.variantCue != "" {
			return m.beginAppendVariant()
		}
		m.assignmentState = m.currentBrowseState()
		m.assigning, m.assignmentCue = true, m.selectedID()
		m.search = ""
		m.filter = filterAll
		m.refreshKeys()
		m.selectID(m.doc.Cue(m.assignmentCue).PrimaryRecording())
		m.status = "Choose a recording: Enter assigns; Space previews/pauses; Escape cancels."

		return m.loadSelection()
	}

	return m.openPicker()
}

func (m *model) acceptAssignment() tea.Cmd {
	if !m.assigning {
		return nil
	}
	if m.itemStatus(m.selectedID()).State != audio.RecordingReady {
		m.status = "Choose a READY recording; wait for checking or repair its file first."

		return nil
	}
	if m.confirmReplace(m.selectedID()) {
		return nil
	}
	var err error
	if m.assignmentAppend {
		err = m.doc.AppendCueRecording(m.assignmentCue, m.recordingID())
	} else {
		err = m.doc.SetCueRecordings(m.assignmentCue, []string{m.recordingID()})
	}
	if err != nil {
		m.report(err)

		return nil
	}
	m.status = "Recording assigned; Ctrl+s saves."

	return m.cancelAssignment()
}

// confirmReplace guards a plain assignment that would drop several ordered clips; it
// reports true while the first Enter is still waiting for its confirming second one.
func (m *model) confirmReplace(selected string) bool {
	clips := len(m.doc.Cue(m.assignmentCue).Recordings())
	if m.assignmentAppend || clips < minReplaceClips {
		return false
	}
	if m.replaceConfirmed == selected {
		return false
	}
	m.replaceConfirmed = selected
	m.status = fmt.Sprintf("This replaces %d clips with one. Enter again to confirm; Esc cancels.", clips)

	return true
}

func (m *model) cancelAssignment() tea.Cmd {
	m.replaceConfirmed = ""
	m.assigning = false
	m.assignmentAppend = false
	m.assignmentCue = ""
	m.restoreBrowseState(m.assignmentState)

	return m.loadSelection()
}
