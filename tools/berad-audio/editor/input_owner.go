package editorui

import tea "github.com/charmbracelet/bubbletea"

// inputOwner is the single priority order for dispatch, foreground presentation
// and contextual hints. Parent drafts remain intact while a child owns input.
type inputOwner uint8

const (
	ownerWorkspace inputOwner = iota
	ownerBusy
	ownerQuit
	ownerField
	ownerInfo
	ownerVolumes
	ownerRoutingPicker
	ownerForm
	ownerAmbientReplacement
	ownerCalibration
	ownerDuplicate
	ownerCrop
	ownerFiles
	ownerAssignment
)

func (m *model) inputOwner() inputOwner {
	switch {
	case m.busy:
		return ownerBusy
	case m.confirmQuit:
		return ownerQuit
	case m.field != "":
		return ownerField
	case m.infoDialog != nil:
		return ownerInfo
	case m.volumeControls:
		return ownerVolumes
	case m.routing.menu != nil:
		return ownerRoutingPicker
	case m.form != nil:
		return ownerForm
	case m.pendingAmbient != nil:
		return ownerAmbientReplacement
	case m.autoGainDraft != nil:
		return ownerCalibration
	case m.duplicate != nil:
		return ownerDuplicate
	case m.crop != nil:
		return ownerCrop
	case m.picker != nil:
		return ownerFiles
	case m.assigning:
		return ownerAssignment
	default:
		return ownerWorkspace
	}
}

// panelFocused projects the current input owner onto a workspace pane. Modal
// owners have their own active frame; telemetry such as transport owns no input.
func (m *model) panelFocused(panel panelFocus) bool {
	switch m.inputOwner() {
	case ownerWorkspace:
		return m.focus == panel
	case ownerField:
		if !m.inlineInput() || m.assigning {
			return false
		}
		if m.field == fieldSearch {
			return panel == focusBrowser
		}

		return panel == focusDetails
	case ownerCrop, ownerDuplicate:
		if m.crop.field == -1 {
			return panel == focusWave
		}

		return panel == focusDetails
	case ownerVolumes:
		return !m.hasDraft() && panel == focusDetails
	case ownerBusy, ownerQuit, ownerInfo, ownerRoutingPicker, ownerForm,
		ownerAmbientReplacement, ownerCalibration, ownerFiles, ownerAssignment:
		return false
	}

	panic("invalid audio editor input owner")
}

func (m *model) typing() bool {
	switch m.inputOwner() {
	case ownerField:
		return m.inputAction == inputTyping
	case ownerForm:
		return m.form.editing
	case ownerRoutingPicker:
		return m.routing.menu.editingSearch
	case ownerWorkspace, ownerBusy, ownerQuit, ownerInfo, ownerVolumes, ownerAmbientReplacement,
		ownerCalibration, ownerDuplicate, ownerCrop, ownerFiles, ownerAssignment:
		return false
	}

	panic("invalid audio editor input owner")
}

func (owner inputOwner) permitsVolumeControls() bool {
	switch owner {
	case ownerWorkspace, ownerForm, ownerRoutingPicker, ownerDuplicate, ownerCrop, ownerFiles, ownerAssignment:
		return true
	case ownerBusy, ownerQuit, ownerField, ownerInfo, ownerVolumes, ownerAmbientReplacement, ownerCalibration:
		return false
	}

	panic("invalid audio editor input owner")
}

//nolint:cyclop // Exhaustive dispatch by the same owner used by presentation and hints.
func (m *model) modalKey(key tea.KeyMsg) (tea.Cmd, bool) {
	owner := m.inputOwner()
	if key.String() == "v" && owner.permitsVolumeControls() && !m.typing() {
		m.openPreviewVolumes()

		return nil, true
	}
	switch owner {
	case ownerField:
		return m.editInput(key), true
	case ownerInfo:
		m.infoKey(key.String())
	case ownerVolumes:
		m.previewVolumeKey(key.String())
	case ownerRoutingPicker:
		return m.routingMenuKey(key), true
	case ownerForm:
		return m.formKey(key), true
	case ownerAmbientReplacement:
		m.ambientReplacementKey(key.String())
	case ownerCalibration:
		return m.autoCalibrationKey(key.String()), true
	case ownerDuplicate:
		return m.duplicateKey(key.String()), true
	case ownerCrop:
		return m.cropKey(key.String()), true
	case ownerFiles:
		return m.pickerKey(key), true
	case ownerAssignment:
		return m.assignmentKey(key.String()), true
	case ownerWorkspace:
		return nil, false
	case ownerBusy, ownerQuit:
		// Global dispatch owns these two non-editing states.
	}

	return nil, true
}

func (m *model) assignmentKey(key string) tea.Cmd {
	switch key {
	case keyEscape:
		return m.cancelAssignment()
	case keyEnter, "y":
		return m.acceptAssignment()
	case "up", "k", keyDown, "j", keyPageUp, keyPageDown, keyShiftUp, keyShiftDown,
		keyPreviousPage, keyNextPage, keyFilter, "f5":
		return m.browserKey(key)
	case "/":
		m.beginSearch()
	case "p":
		m.play()
	case " ":
		m.togglePreview()
	case "?":
		m.openHelp()
	default:
		m.status = "Recording chooser: Enter assigns; Space previews; Escape returns before editing."
	}

	return nil
}
