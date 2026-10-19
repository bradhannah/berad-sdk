package editorui

import (
	"fmt"
	"path"
	"strings"

	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

type editorDialog struct {
	title string
	rows  []string
	focus int
	help  []string
}

const (
	editorDialogMargin = 4
	dialogHeaderRows   = 2
	inputPadding       = 4
)

func (m *model) inlineInput() bool {
	switch m.field {
	case fieldSearch, fieldGain, fieldStart, fieldEnd, fieldCalibrationTarget:
		return true
	default:
		return false
	}
}

func (m *model) inputLines(width int) []string {
	rows, _ := m.inputRows(width)

	return rows
}

func (m *model) inputRows(width int) ([]string, int) {
	label := "EDITING " + strings.ToUpper(m.fieldLabel())
	value := m.inputText.render(m.input, m.colors.selected.Render)
	textWidth := max(1, width-inputPadding)
	prefix := m.input[:m.inputText.position(m.input)]
	caretRow := 0
	for _, line := range strings.Split(prefix+"▏", "\n") {
		caretRow += len(strings.Split(ansi.Hardwrap(safe(line), textWidth, true), "\n"))
	}
	var rows []string
	for _, line := range strings.Split(value, "\n") {
		rows = append(rows, strings.Split(ansi.Hardwrap(line, textWidth, true), "\n")...)
	}
	for index := range rows {
		rows[index] = m.colors.key.Render(fit("  "+rows[index], width))
	}
	rows = append([]string{m.colors.focus.Render(fit(label, width)), ""}, rows...)
	focus := caretRow + 1
	if m.field == fieldLicense {
		const subjectRows = 3
		subject := make([]string, 0, subjectRows+len(rows))
		subject = append(subject, m.colors.detailLabel.Render("Recording: ")+
			m.colors.detailValue.Render(safe(m.licenseRecordingID)),
			fmt.Sprintf("Shared provenance · used by %d Moment(s).", len(m.doc.Usages(m.licenseRecordingID))), "")
		rows = append(subject, rows...)
		focus += subjectRows
	}
	if m.inputError != "" {
		for _, line := range strings.Split(ansi.Hardwrap("! "+safe(m.inputError), width, true), "\n") {
			rows = append(rows, m.colors.problem.Render(line))
		}
	}
	rows = append(rows, "", m.colors.dim.Render(fit(m.inputGuidance(), width)))
	if m.field != fieldSearch {
		for index, action := range []string{"Apply", hintCancel} {
			line := "[ " + action + " ]"
			if m.inputAction == index+1 {
				focus = len(rows)
				line = m.colors.selected.Render("> " + line)
			}
			rows = append(rows, line)
		}
	}

	return rows, focus
}

// Modal priority mirrors input ownership, including nested picker/volume dialogs.
func (m *model) dialogContent(width int) (editorDialog, bool) {
	switch m.inputOwner() {
	case ownerBusy:
		return editorDialog{title: "FILE OPERATION", rows: []string{safe(m.status)}, focus: 0,
			help: []string{"Ctrl+x: stop audio · Ctrl+c: quit after publication · Errors keep the editor open."}}, true
	case ownerQuit:
		return editorDialog{title: "UNSAVED CHANGES", rows: []string{
			m.colors.warning.Render("Unsaved work, an open draft, or unconfirmed file durability remains."),
			"y quits. Published/imported files remain on disk. Escape returns to editing.",
		}, focus: 0, help: []string{"y: discard and quit   Escape: keep editing"}}, true
	case ownerField:
		return m.fieldDialog(width)
	case ownerInfo:
		return editorDialog{title: m.infoDialog.title, rows: m.infoDialog.lines, focus: m.infoDialog.scroll,
			help: []string{"Up/Down: scroll   Enter/Escape: close"}}, true
	case ownerVolumes:
		return m.volumeDialog(width)
	case ownerRoutingPicker:
		return m.routingPickerDialog(width), true
	case ownerForm:
		return m.formDialog(width), true
	case ownerFiles:
		return m.filePickerDialog(width), true
	case ownerAssignment:
		return m.recordingAssignmentDialog(width), true
	case ownerDuplicate, ownerCrop:
		// Region drafts use the right pane so the waveform stays visible.
	case ownerCalibration:
		return m.calibrationDialog(), true
	case ownerAmbientReplacement:
		return editorDialog{title: "REPLACE AMBIENT LAYER", rows: []string{
			"Both ambient slots are occupied. Choose which layer to replace.",
			fmt.Sprintf("Selected slot: %d", m.mixCursor+1),
		}, focus: 1, help: []string{"1/2: select slot   Enter: replace   Escape: cancel"}}, true
	case ownerWorkspace:
	}

	return editorDialog{title: "", rows: nil, focus: 0, help: nil}, false
}

func (m *model) fieldDialog(width int) (editorDialog, bool) {
	if m.field == fieldSearch && m.assigning {
		dialog := m.recordingAssignmentDialog(width)
		dialog.focus = 1
		dialog.help = []string{"Enter: apply filter   Escape: cancel filter   Left/Right: caret   Ctrl+x: stop audio"}

		return dialog, true
	}
	if m.inlineInput() {
		return editorDialog{title: "", rows: nil, focus: 0, help: nil}, false
	}
	help := "Enter: apply   Tab: actions   Escape: cancel   Ctrl+U: clear"
	if m.field == fieldNote {
		help = "Enter: new line / activate action   Tab: Apply/Cancel   Escape: cancel"
	}
	rows, focus := m.inputRows(width)

	return editorDialog{title: "EDIT " + strings.ToUpper(m.fieldLabel()), rows: rows,
		focus: focus, help: []string{help}}, true
}

func (m *model) volumeDialog(width int) (editorDialog, bool) {
	if !m.hasDraft() {
		return editorDialog{title: "", rows: nil, focus: 0, help: nil}, false
	}

	return editorDialog{title: "PREVIEW VOLUME — temporary", rows: m.previewVolumeLines(width),
		focus: m.volumeCursor, help: []string{"Up/Down: control   Left/Right: adjust   Escape: return"}}, true
}

//nolint:cyclop,nestif // Fields, inline errors and actions render in order so focus offsets stay aligned.
func (m *model) formDialog(width int) editorDialog {
	form := m.form
	rows := []string{m.colors.warning.Render("DRAFT · Apply updates the library; Ctrl+S saves the file."), ""}
	focus := 0
	if form.kind == formSurface {
		rows = append(rows, "Ordered footsteps · shared by every tile using "+form.surface)
		for index, cue := range form.sequence {
			line := fmt.Sprintf("%d. %s", index+1, m.doc.Cue(cue).Name)
			if index == form.cursor {
				focus = len(rows)
				line = m.colors.key.Render(fit("> "+line, width))
			}
			rows = append(rows, line)
		}
		if len(form.sequence) == 0 {
			rows = append(rows, "Silent surface. a adds a footstep sound.")
		}
	} else {
		if form.kind == formTileAmbient {
			rows = append(rows, "Applies only to this tile family, including all its animation frames.",
				"Other tile types can reuse the same sound with different distance/level.", "")
		}
		if form.kind == formAmbient {
			rows = append(rows, "Shared sound settings: every source using this sound is affected.", "")
		}
		for index, field := range form.fields {
			if index == form.cursor {
				focus = len(rows)
			}
			rows = append(rows, m.formFieldLine(field, index == form.cursor, form.editing, width))
			if index == form.cursor {
				rows = append(rows, m.colors.dim.Render(fit(field.help, width)))
			}
			if index == form.errorField && form.errorText != "" {
				rows = append(rows, m.colors.problem.Render("! "+safe(form.errorText)))
			}
		}
	}
	fieldCount := len(form.fields)
	if form.kind == formSurface {
		fieldCount = len(form.sequence)
	}
	for index, action := range []string{"Apply changes", hintCancel} {
		line := "[ " + action + " ]"
		if form.cursor == fieldCount+index {
			focus = len(rows)
			line = m.colors.key.Render(fit("> "+line, width))
		}
		rows = append(rows, line)
	}
	if form.errorField < 0 && form.errorText != "" {
		rows = append(rows, m.colors.problem.Render("! "+safe(form.errorText)))
		focus = len(rows) - 1
	}

	return editorDialog{title: form.title, rows: rows, focus: focus, help: m.hintRows(m.formKeyHints())}
}

func (m *model) formFieldLine(field formField, active, editing bool, width int) string {
	value := safe(field.value)
	if active && editing {
		available := max(1, width-ansi.StringWidth(field.label+field.unit)-inputPadding-dialogHeaderRows)
		value = m.form.text.viewport(field.value, available, m.colors.selected.Render)
	}
	if active {
		return m.colors.key.Render(fit("> "+field.label+": [ "+value+" ] "+field.unit, width))
	}

	return m.colors.detailLabel.Render(field.label+": ") + m.colors.detailValue.Render("[ "+value+" ] "+field.unit)
}

//nolint:funlen // Keep picker rows, semantic status colours and the focused row in one rendering pass.
func (m *model) routingPickerDialog(width int) editorDialog {
	menu := m.routing.menu
	choices := m.routingMenuChoices()
	const pickerGaps = 2
	remaining := width - rowPrefixWidth - routingStatusWidth - pickerGaps
	nameWidth, recordingWidth := remaining/columnSplit, remaining-remaining/columnSplit
	search := "Search: " + safe(menu.search)
	if menu.editingSearch {
		const label = "EDIT SEARCH: "
		search = m.colors.key.Render(fit(label+
			menu.text.viewport(menu.searchInput, width-len(label), m.colors.selected.Render), width))
	}
	rows := []string{search, m.colors.detailLabel.Render(strings.Repeat(" ", rowPrefixWidth) +
		fit("Name", nameWidth) + " " + fit("Recording / file", recordingWidth) + " " + fit("State", routingStatusWidth))}
	for index, choice := range choices {
		status, recording, style := "— None", "—", m.colors.dim
		if choice.cue != "" {
			status, style = m.routingAssignmentStyle(choice.cue)
			recording = m.doc.Cue(choice.cue).PrimaryRecording()
		}
		if m.isRecordingMenu() && choice.id != formEmptyRecording {
			availability := m.doc.FileAvailability(choice.id)
			if cached, exists := m.availability[choice.id]; exists {
				availability = cached
			}
			status = string(availability.State)
			style = m.colors.warning
			switch availability.State { //nolint:exhaustive // Empty/checking retain the amber default.
			case audio.RecordingReady:
				style = m.colors.ready
			case audio.RecordingInvalid, audio.RecordingMissing:
				style = m.colors.problem
			}
			if file := m.doc.Recording(choice.id).File; file != nil {
				recording = path.Base(*file)
			}
		}
		prefix, nameStyle := "  ", m.colors.detailValue
		if index == menu.cursor {
			prefix, nameStyle = "> ", m.colors.selected
		}
		line := nameStyle.Render(prefix+fit(safe(choice.label), nameWidth)) + " " +
			style.Render(fit(safe(recording), recordingWidth)) + " " + style.Render(fit(status, routingStatusWidth))
		rows = append(rows, line)
	}
	focus := menu.cursor + dialogHeaderRows
	if menu.editingSearch {
		focus = 0
	}
	if len(choices) == 0 {
		rows = append(rows, "No matching sounds. / edits the search.")
	}

	help := []string{m.transportState() + " · " + safe(m.previewDescription()),
		"Enter: choose   Space: preview/pause   /: search   Escape: cancel"}
	if menu.kind == "cue" {
		help = append(help, "B: sound settings   r: recording   Ctrl+A: new ambient sound")
	}

	return editorDialog{title: menu.title, rows: rows, focus: focus, help: help}
}

func (m *model) recordingAssignmentDialog(width int) editorDialog {
	rows := append(m.browserSearchLines(width), m.columnHeader(width))
	for index, identity := range m.keys {
		rows = append(rows, m.itemLine(identity, index == m.cursor, width))
	}

	return editorDialog{title: "CHOOSE RECORDING — " + m.doc.Cue(m.assignmentCue).Name,
		rows: rows, focus: m.cursor + len(m.browserSearchLines(width)) + 1,
		help: []string{m.transportState() + " · " + safe(m.previewDescription()),
			"Enter: assign   Space: preview/pause   /: search   Escape: cancel"}}
}

func (m *model) filePickerDialog(width int) editorDialog {
	rows := make([]string, 0, dialogHeaderRows+len(m.picker.entries))
	rows = append(rows, m.colors.detailLabel.Render(fit(safe(m.picker.directory), width)), "")
	for index, entry := range m.picker.entries {
		line := safe(entry.name)
		if entry.directory {
			line += "/"
		}
		if index == m.picker.cursor {
			line = m.colors.key.Render(fit("> "+line, width))
		}
		rows = append(rows, line)
	}

	return editorDialog{title: "CHOOSE FILE — " + m.recordingID(), rows: rows, focus: m.picker.cursor + dialogHeaderRows,
		help: []string{"Shared recording: all its uses follow this file assignment.",
			"Enter: open/import   Space: preview/pause   /: path   Escape: cancel"}}
}

func (m *model) overlayDialog(background string, dialog editorDialog, width int) string {
	maxHeight := m.height - editorDialogMargin
	help := make([]string, 0, len(dialog.help))
	for _, line := range dialog.help {
		help = append(help, strings.Split(ansi.Wrap(line, width-panelEdges, ""), "\n")...)
	}
	wrapped := make([]string, 0, len(dialog.rows))
	focus := 0
	for index, line := range dialog.rows {
		if index == dialog.focus {
			focus = len(wrapped)
		}
		wrapped = append(wrapped, strings.Split(ansi.Wrap(line, width-panelEdges, ""), "\n")...)
	}
	capacity := max(1, maxHeight-panelEdges-len(help)-1)
	height := min(maxHeight, len(wrapped)+panelEdges+len(help)+1)
	start := listViewportStart(0, focus, len(wrapped), capacity)
	rows := append([]string(nil), wrapped[start:min(len(wrapped), start+capacity)]...)
	rows = append(rows, "")
	rows = append(rows, help...)
	box := strings.Split(m.panel(dialog.title, rows, width, height, true), "\n")
	base := strings.Split(background, "\n")
	left, top := (m.width-width)/columnSplit, max(dialogHeaderRows, (m.height-height)/columnSplit)
	for index, line := range base {
		if index >= dialogHeaderRows {
			base[index] = m.colors.dim.Render(fit(ansi.Strip(line), m.width))
		}
	}
	for index, line := range box {
		row := top + index
		if row >= len(base) {
			break
		}
		base[row] = ansi.Cut(base[row], 0, left) + line + ansi.Cut(base[row], left+width, m.width)
	}

	return strings.Join(base, "\n")
}
