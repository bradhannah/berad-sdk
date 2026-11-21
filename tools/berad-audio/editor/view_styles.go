package editorui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

// Cyan is reserved for keys and the pane that has them; green, amber and red
// describe state; labels are dim and values plain.
type palette struct {
	title, focus, ready, warning, problem, dim, selected, key lipgloss.Style
	hintKey, detailLabel, detailValue                         lipgloss.Style
}

const (
	hintStopAll   = "Stop all"
	hintCancel    = "Cancel"
	hintPause     = "Pause"
	hintGainKeys  = "+ -"
	hintSeparator = "   "
	hintArrows    = "← →"
	hintBoundKeys = "s e"
	hintPlay      = "play"
	hintAllKeys   = "all keys"
	keyCancelName = "Esc"
	keyEnterName  = "Enter"
)

func newPalette() palette {
	return palette{
		title: lipgloss.NewStyle().Bold(true), focus: lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true),
		ready:       lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		warning:     lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		problem:     lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
		dim:         lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		selected:    lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("4")).Bold(true),
		key:         lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Bold(true),
		hintKey:     lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true),
		detailLabel: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		detailValue: lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
	}
}

// stateMark is the one-character sign and plain words for a recording's file.
func stateMark(state audio.RecordingState) (string, string) {
	switch state {
	case audio.RecordingReady:
		return "●", "ready"
	case audio.RecordingNoFile:
		return "·", "no audio yet"
	case audio.RecordingMissing:
		return "!", "file missing"
	case audio.RecordingInvalid:
		return "!", "cannot play"
	case audio.RecordingChecking:
	}

	return "…", "checking"
}

func (m *model) stateStyle(state audio.RecordingState) lipgloss.Style {
	switch state {
	case audio.RecordingReady:
		return m.colors.ready
	case audio.RecordingMissing, audio.RecordingInvalid:
		return m.colors.problem
	case audio.RecordingNoFile, audio.RecordingChecking:
	}

	return m.colors.dim
}

func (m *model) badge(status audio.RecordingAvailability) string {
	symbol, words := stateMark(status.State)

	return m.stateStyle(status.State).Render(symbol + " " + words)
}

type keyHint struct{ key, action string }

// hintRows packs complete shortcuts across the real terminal width. A long
// terminal gets fewer footer rows rather than retaining narrow fixed groups.
func (m *model) hintRows(hints []keyHint) []string {
	var rows []string
	line := ""
	for _, hint := range hints {
		item := hint.action
		if hint.key != "" {
			item = m.colors.hintKey.Render(hint.key) + " " + item
		}
		if line != "" && ansi.StringWidth(line+hintSeparator+item) > m.width {
			rows = append(rows, fit(line, m.width))
			line = ""
		}
		if line == "" {
			line = " " + item
		} else {
			line += hintSeparator + item
		}
	}
	if line != "" {
		rows = append(rows, fit(line, m.width))
	}

	return rows
}

//nolint:cyclop,goconst // Exhaustive owner hints keep priority and complete binding tables auditable.
func (m *model) footerLines() []string {
	switch m.inputOwner() {
	case ownerBusy:
		return m.hintRows([]keyHint{{"", "Finishing file operation…"}, {"Ctrl+x", hintStopAll},
			{"Ctrl+c", "Quit after publication"}})
	case ownerQuit:
		return m.hintRows([]keyHint{{"y", "Discard and quit"}, {keyCancelName, "Keep editing"}, {"Ctrl+x", hintStopAll}})
	case ownerField:
		return m.inputFooter()
	case ownerInfo:
		return m.hintRows([]keyHint{{"Up/Down", "Scroll"}, {"Enter/Esc", "Close"}, {"Ctrl+x", hintStopAll}})
	case ownerAmbientReplacement:
		return m.hintRows([]keyHint{{"1/2 or ↑↓", "Ambient slot"}, {"Enter", "Replace"},
			{keyCancelName, hintCancel}, {"x", hintStopAll}})
	case ownerVolumes:
		return m.hintRows([]keyHint{{"↑↓", "Control"}, {"←→ or +/-", "Change"},
			{"r", "Game Mix/Reference"}, {"Enter/Esc", "Close"}, {"x", hintStopAll}})
	case ownerCalibration:
		return m.calibrationFooter()
	case ownerForm:
		return m.hintRows(m.formKeyHints())
	case ownerRoutingPicker:
		if m.routing.menu.editingSearch {
			return m.hintRows([]keyHint{{"Left/Right", "Caret"}, {"Enter", "Apply search"},
				{keyCancelName, "Cancel search"}, {"Ctrl+u", "Clear"}, {"Ctrl+x", hintStopAll}})
		}

		return m.routingFooter()
	case ownerCrop, ownerDuplicate:
		return m.cropFooter()
	case ownerWorkspace:
		if m.routingWorkspace() && m.focus == focusBrowser {
			return m.routingFooter()
		}
	case ownerFiles, ownerAssignment:
	}

	return m.selectionFooter()
}

//nolint:goconst // Complete shortcut tables deliberately repeat rendered keys and action labels.
func (m *model) formKeyHints() []keyHint {
	if m.form.editing {
		return []keyHint{{"Left/Right", "Caret"}, {"Enter/Tab", "Finish field"},
			{"Escape", "Revert field"}, {"Ctrl+u", "Clear"}, {"Ctrl+x", hintStopAll}}
	}
	hints := []keyHint{{"Tab/Up/Down", "Focus"}, {"Enter", "Edit/activate"},
		{"", "Apply changes / Cancel"}, {"Escape", hintCancel}, {"Ctrl+x", hintStopAll}}
	if m.form.kind == formSurface {
		return append(hints, keyHint{"a", "Add"}, keyHint{"Delete", "Remove"},
			keyHint{"Ctrl+Up/Down", "Reorder"}, keyHint{"Space", "Preview/pause"})
	}

	return append(hints, keyHint{"Left/Right", "Adjust"}, keyHint{"p", "Preview"},
		keyHint{"Space", hintPause}, keyHint{"Shift+l", "Departure"})
}

func (m *model) inputFooter() []string {
	if m.field == fieldSearch {
		return m.hintRows([]keyHint{{"", "Type to narrow the list"},
			{keyEnterName, "keep"}, {keyCancelName, "clear"}, {"Ctrl+u", "empty"}})
	}
	if m.inlineInput() {
		return m.hintRows([]keyHint{{"", m.inputGuidance()}, {keyEnterName, "apply"}, {keyCancelName, "cancel"}})
	}

	return m.hintRows([]keyHint{{"", m.inputGuidance()},
		{"Tab", "Apply/Cancel"}, {"Left/Right", "Caret/action"}, {keyCancelName, hintCancel},
		{"Ctrl+u", "Clear"}, {"Ctrl+x", hintStopAll}})
}

//nolint:goconst // Contextual shortcut tables retain the same key and action vocabulary.
func (m *model) selectionFooter() []string {
	if m.picker != nil {
		return m.hintRows([]keyHint{{"↑↓", "Choose"}, {"Enter", "Open/import"}, {"p", "Preview"},
			{"Ctrl+p/n", "Prev/next page"}, {"/", "Path"}, {"Backspace", "Parent"}, {keyCancelName, hintCancel}, {"x", "Stop"}})
	}
	if m.assigning {
		return m.hintRows([]keyHint{{"↑↓", "Choose"}, {"Enter/y", "Assign"}, {"Space", "Preview/pause"},
			{keyCancelName, hintCancel}, {"/", "Search"}, {"Ctrl+f", "Filter"}, {"x", hintStopAll}})
	}
	if m.variantCue != "" && m.focus == focusBrowser {
		return m.hintRows([]keyHint{{"↑↓", "Clip"}, {"Enter", "Preview"}, {"a", "Append"}, {"Delete", "Remove"},
			{"Ctrl+↑↓", "Reorder"}, {"Ctrl+p/n", "Prev/next page"}, {"Shift+d", "Duplicate"},
			{"Ctrl+s", "Save"}, {keyCancelName, "Back"}})
	}

	return m.browseFooter()
}

// The everyday keys for whatever has them; ? lists the rest.
func (m *model) browseFooter() []string {
	switch m.focus {
	case focusWave:
		return m.hintRows([]keyHint{{hintArrows, "seek"}, {"i o", "mark trim"}, {hintBoundKeys, "type times"},
			{keyCancelName, "list"}, {"?", hintAllKeys}})
	case focusDetails:
		return m.hintRows([]keyHint{{"↑↓", "scroll"}, {"I", "all numbers"}, {keyCancelName, "list"}, {"?", hintAllKeys}})
	case focusMix:
		return m.hintRows([]keyHint{{"↑↓", "Layer"}, {keyEnterName, "Restart"}, {"Space", hintPause}, {"u", "Mute"},
			{keyCancelName, "Browse"}, {"v", "Preview volumes"}, {"x", hintStopAll}, {"Ctrl+s", "Save"}, {"q", "Quit"}})
	case focusBrowser:
	}
	level, file := "level", "file"
	if m.browsingCues() {
		level, file = "volume", "sound"
	}

	return m.hintRows([]keyHint{{keyEnterName, hintPlay}, {"Space", "pause"}, {"x", "stop"}, {hintGainKeys, level},
		{"i o", "trim"}, {"f", file}, {"/", "find"}, {"?", hintAllKeys}, {"Ctrl+S", "save"}, {"q", "quit"}})
}

func (m *model) fieldLabel() string {
	switch m.field {
	case fieldStart:
		return "Start"
	case fieldEnd:
		return "End"
	case fieldGain:
		if m.browsingCues() {
			return "Volume"
		}

		return "Level"
	case fieldCalibrationTarget:
		return "Auto target"
	}

	return m.field
}

func (m *model) inputGuidance() string {
	switch m.field {
	case fieldNote:
		return "Note: Enter inserts a line; Tab selects Apply/Cancel; Escape discards."
	case fieldLicense:
		return "Enter documented license terms (e.g. CC BY 4.0), or unknown. Blank is not allowed."
	case fieldKeywords:
		return "Comma-separated keywords or phrases · replaces this item's custom terms"
	case fieldTrigger:
		return "One sentence describing this moment's usual gameplay trigger"
	case fieldStart, fieldEnd:
		return "m:ss.mmm or seconds"
	case fieldGain:
		if m.browsingCues() {
			return "0 to 100"
		}

		return "-48 to +12 · turns auto off"
	case fieldCalibrationTarget:
		return "-60 to -6"
	default:
		return "Text input · Enter applies"
	}
}
