package editorui

import (
	"math"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const (
	calibrationStep    = 0.5
	cueGainStep        = 0.05
	minimumCalibration = -48.0
	maximumCalibration = 12.0
	fineSeekMs         = 10
	coarseSeekMs       = 100
)

func (m *model) key(key tea.KeyMsg) tea.Cmd {
	if key.Type == tea.KeyRunes && !key.Alt && !m.typing() && m.inputOwner() != ownerFiles &&
		(key.Paste || len(key.Runes) != 1) {
		m.status, m.statusIsError = "Focus a text field before entering text; no shortcut was executed.", false

		return nil // Pasting outside a text owner never executes a shortcut macro.
	}
	if command, handled := m.globalKey(key); handled {
		return command
	}
	if command, handled := m.modalKey(key); handled {
		return command
	}
	if key.Type == tea.KeyF5 {
		return m.recheckFiles()
	}
	if command, handled := m.focusedKey(key); handled {
		return command
	}
	if m.routingWorkspace() {
		return m.routingKey(key.String())
	}
	if m.variantCue != "" && !m.assigning {
		return m.variantKey(key.String())
	}

	return m.browserKey(key.String())
}

func (m *model) focusedKey(key tea.KeyMsg) (tea.Cmd, bool) {
	if m.focus == focusMix {
		return m.mixKey(key), true
	}
	if m.focus == focusDetails && m.scrollDetails(key.String()) {
		return nil, true
	}
	if m.focus == focusWave && (key.String() == "up" || key.String() == keyDown) {
		return nil, true
	}
	if key.String() == keyEscape && (m.marker != 0 || m.focus != focusBrowser) {
		m.marker, m.focus = 0, focusBrowser

		return nil, true
	}

	return nil, false
}

func (m *model) globalKey(key tea.KeyMsg) (tea.Cmd, bool) {
	name := key.String()
	if m.typing() {
		name = textControlKey(key)
	}
	if name == "ctrl+x" || name == "x" && !m.typing() {
		m.stopAll()
		m.status = "All playback stopped."

		return nil, true
	}
	if m.busy {
		m.busyKey(name)

		return nil, true
	}
	if name == "ctrl+c" {
		m.confirmQuit = true

		return m.quit(), true
	}
	if m.confirmQuit {
		if name == "y" || name == "Y" {
			return tea.Quit, true
		}
		m.confirmQuit = false

		if name == keySave {
			return m.saveFromKeyboard(), true
		}

		return nil, true
	}
	if name == keySave {
		return m.saveFromKeyboard(), true
	}
	if m.width < minimumWidth || m.height < minimumHeight {
		if name == "q" {
			return m.quit(), true
		}

		return nil, true
	}

	return nil, false
}

func (m *model) busyKey(key string) {
	m.status = "Finishing file operation…"
	if key == "ctrl+c" {
		m.quitAfterOperation = true
	}
	if m.quitAfterOperation {
		m.status = "Quit requested; finishing file publication before checking unsaved work."
	}
}

func (m *model) hasDraft() bool {
	return m.field != "" || m.form != nil || m.duplicate != nil || m.assigning || m.picker != nil ||
		m.autoGainDraft != nil || m.routing.menu != nil || m.crop != nil
}

func (m *model) saveFromKeyboard() tea.Cmd {
	if m.hasDraft() {
		m.status, m.statusIsError = "Finish or cancel the current draft before saving the library.", true

		return nil
	}

	return m.save()
}

//nolint:cyclop,funlen,gocyclo,maintidx // One explicit keyboard dispatch table keeps bindings auditable.
func (m *model) browserKey(key string) tea.Cmd {
	switch key {
	case "q":
		return m.quit()
	case "up", "k":
		return m.move(-1)
	case keyDown, "j":
		return m.move(1)
	case keyPageUp, keyShiftUp, keyPreviousPage:
		m.browserStart = m.browserWindowStart() - m.listHeight()

		return m.move(-m.listHeight())
	case keyPageDown, keyShiftDown, keyNextPage:
		m.browserStart = m.browserWindowStart() + m.listHeight()

		return m.move(m.listHeight())
	case keyTab:
		return m.keySwitchWorkspace((m.workspace + 1) % workspaceCount)
	case keyShiftTab:
		return m.keySwitchWorkspace((m.workspace + workspaceCount - 1) % workspaceCount)
	case "1":
		return m.switchWorkspace(false)
	case "2":
		return m.switchWorkspace(true)
	case "3":
		return m.keySwitchWorkspace(workspaceTiles)
	case "4":
		return m.keySwitchWorkspace(workspaceLocations)
	case "U":
		return m.openRoutingUsages()
	case "B":
		return m.editAmbientProfile()
	case keyCtrlA:
		return m.openNewAmbientForm()
	case keyFilter:
		selected := m.selectedID()
		m.filter = (m.filter + 1) % filterCount
		m.refreshKeys()
		if selected != m.selectedID() {
			return m.loadSelection()
		}
	case "f5":
		return m.recheckFiles()
	case "w":
		m.focus = focusWave
	case "d":
		m.soundscapeVisible = false
		m.focus = focusDetails
	case "/":
		m.beginSearch()
	case "K":
		m.beginMetadataEdit(false)
	case "H":
		m.beginMetadataEdit(true)
	case keyEnter, "p":
		if m.assigning && key == keyEnter {
			return m.acceptAssignment()
		}
		m.play()
	case "V":
		return m.openVariants()
	case "D":
		m.beginDuplicate()
	case " ":
		m.togglePreview()
	case "x":
		m.stopAll()
	case "S":
		m.fullSource = !m.fullSource
		m.applyPlaybackToggle(map[bool]string{true: "Whole file.", false: "Trimmed region."}[m.fullSource])
	case "l":
		m.looping = !m.looping
		m.applyPlaybackToggle(map[bool]string{true: "Loop on.", false: "Loop off."}[m.looping])
	case "+", "=":
		m.adjustGain(1)
	case "-":
		m.adjustGain(-1)
	case "g":
		m.beginInput(fieldGain, "")
	case "ctrl+l":
		m.beginLicenseEdit()
	case "v":
		m.openPreviewVolumes()
	case "i":
		m.focus = focusWave
		m.markPosition(true)
	case "o":
		m.focus = focusWave
		m.markPosition(false)
	case "[":
		if m.beginCrop() {
			m.focusCropField(m.cropFieldCount() - cropBoundCount)
		}
	case "]":
		if m.beginCrop() {
			m.focusCropField(m.cropFieldCount() - 1)
		}
	case "s":
		if m.beginCrop() {
			m.focusCropField(m.cropFieldCount() - cropBoundCount)
			m.beginInput(fieldStart, strconv.FormatInt(m.window().StartMs, 10))
		}
	case "e":
		if m.beginCrop() {
			m.focusCropField(m.cropFieldCount() - 1)
			m.beginInput(fieldEnd, strconv.FormatInt(m.window().EndMs, 10))
		}
	case keyLeft:
		m.nudge(-fineSeekMs)
	case keyRight:
		m.nudge(fineSeekMs)
	case keyShiftLeft:
		m.nudge(-coarseSeekMs)
	case keyShiftRight:
		m.nudge(coarseSeekMs)
	case "z":
		m.zoom = !m.zoom
	case "t":
		m.suggestTrim()
	case "r":
		m.setWindow(nil)
	case "u":
		return m.history(false)
	case "ctrl+r":
		return m.history(true)
	case "f":
		return m.chooseFileOrAssignment()
	case "a":
		m.addMix()
	case "m":
		m.soundscapeVisible, m.focus = true, focusMix
	case "n":
		if m.browsingCues() {
			return m.previewNextClip()
		}
		if m.workspace == workspaceRecordings {
			m.beginInput("new", "")
		}
	case "N":
		m.beginInput(fieldNote, m.doc.Recording(m.recordingID()).Note)
	case "C":
		m.beginClassificationEdit()
	case "A":
		m.beginAutoCalibration()
	case "T":
		m.editCalibrationTarget()
	case "y":
		return m.acceptAssignment()
	case keyEscape:
		if m.assigning {
			return m.cancelAssignment()
		}
		if m.routing.hasReturn {
			return m.returnFromRoutingJump()
		}
		m.marker = 0
		m.focus = focusBrowser
	case "?":
		m.openHelp()
	}

	return nil
}

// applyPlaybackToggle makes a loop/whole-file flip take effect: idle previews only
// report it, while a playing one restarts and returns to where it was.
func (m *model) applyPlaybackToggle(notice string) {
	voice := m.previewVoice()
	if !voice.Playing() {
		m.status, m.statusIsError = notice, false

		return
	}
	position := voice.PositionMs()
	if m.play() {
		m.report(m.previewVoice().SeekMs(position))
	}
}

func (m *model) move(delta int) tea.Cmd {
	if m.routingWorkspace() {
		m.routing.footsteps = false
	}
	m.cursor = max(0, min(m.cursor+delta, len(m.keys)-1))
	m.browserStart = m.browserWindowStart()
	m.marker = 0
	m.detailsScroll = 0

	return m.loadSelection()
}

func (m *model) quit() tea.Cmd {
	if m.doc.Dirty() || m.doc.NeedsSync() || m.hasDraft() {
		m.confirmQuit = true
		m.status = "Unsaved work or pending durability: y quits; other keys cancel. Ctrl+s saves/retries."

		return nil
	}

	return tea.Quit
}

// undoOrRedo applies one history step and returns the status text for its outcome.
func (m *model) undoOrRedo(redo bool) (bool, string) {
	if redo {
		if m.doc.Redo() {
			return true, "Redone."
		}

		return false, "Nothing to redo."
	}
	if m.doc.Undo() {
		return true, "Undone."
	}

	return false, "Nothing to undo."
}

func (m *model) history(redo bool) tea.Cmd {
	before, selected, recording := m.sourceSignatures(), m.selectedID(), m.recordingID()
	applied, done := m.undoOrRedo(redo)
	if !applied {
		m.status, m.statusIsError = done, false

		return nil
	}
	m.refreshKeys()
	command := m.recheckChanged(before, m.selectedID() != selected || m.recordingID() != recording)
	m.preview.SetGain(m.livePreviewGain())
	m.status, m.statusIsError = done, false
	if len(m.doc.RetainedImports()) != 0 {
		m.status += " Copied import files remain on disk; Details lists unassigned retained files."
	}

	return command
}

func (m *model) adjustGain(direction float64) {
	if m.browsingCues() {
		cue := m.doc.Cue(m.selectedID())
		m.setGain(math.Max(0, math.Min(1, cue.VolumeModifier+direction*cueGainStep)))

		return
	}
	recording := m.doc.Recording(m.recordingID())
	value := 0.0
	if recording.CalibrationGainDB != nil {
		value = *recording.CalibrationGainDB
	}
	m.setGain(math.Max(minimumCalibration, math.Min(maximumCalibration, value+direction*calibrationStep)))
}

func (m *model) setGain(value float64) {
	if m.assigning {
		m.status = "Confirm the assignment before editing."

		return
	}
	if m.browsingCues() {
		cue := m.doc.Cue(m.selectedID())
		cue.VolumeModifier = value
		m.report(m.doc.SetCue(m.selectedID(), cue))
	} else {
		recording := m.doc.Recording(m.recordingID())
		recording.CalibrationGainDB = &value
		recording.CalibrationPolicy = audio.CalibrationManual
		m.report(m.doc.SetRecording(m.recordingID(), recording))
	}
	m.preview.SetGain(m.livePreviewGain())
	m.refreshMixGains()
}
