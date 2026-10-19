package editorui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

type palette struct {
	title, focus, ready, warning, problem, dim, selected, key lipgloss.Style
	detailLabel, detailValue                                  lipgloss.Style
}

const (
	hintStopAll  = "Stop all"
	hintCancel   = "Cancel"
	hintPause    = "Pause"
	hintGainKeys = "+/-"
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
		detailLabel: lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true),
		detailValue: lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
	}
}

func (m *model) badge(status audio.RecordingAvailability) string {
	symbol, style := "…", m.colors.dim
	switch status.State {
	case audio.RecordingReady:
		symbol, style = "●", m.colors.ready
	case audio.RecordingNoFile:
		symbol, style = "○", m.colors.warning
	case audio.RecordingMissing, audio.RecordingInvalid:
		symbol, style = "!", m.colors.problem
	case audio.RecordingChecking:
	}

	return style.Render(symbol + " " + string(status.State))
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
			item = m.colors.key.Render("["+hint.key+"]") + " " + item
		}
		separator := ""
		if line != "" {
			separator = "  "
		}
		if line != "" && ansi.StringWidth(line+separator+item) > m.width {
			rows = append(rows, fit(line, m.width))
			line = ""
			separator = ""
		}
		line += separator + item
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
		return m.hintRows([]keyHint{{"y", "Discard and quit"}, {"Esc", "Keep editing"}, {"Ctrl+x", hintStopAll}})
	case ownerField:
		return m.inputFooter()
	case ownerInfo:
		return m.hintRows([]keyHint{{"Up/Down", "Scroll"}, {"Enter/Esc", "Close"}, {"Ctrl+x", hintStopAll}})
	case ownerAmbientReplacement:
		return m.hintRows([]keyHint{{"1/2 or ↑↓", "Ambient slot"}, {"Enter", "Replace"},
			{"Esc", "Cancel"}, {"x", hintStopAll}})
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
				{"Esc", "Cancel search"}, {"Ctrl+u", "Clear"}, {"Ctrl+x", hintStopAll}})
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
		return m.hintRows([]keyHint{{"", "EDIT FILTER · keywords or category:/family:/context:"},
			{"Left/Right", "Caret"}, {"Enter", "Apply filter"}, {"Esc", hintCancel}, {"Ctrl+u", "Clear"}})
	}

	return m.hintRows([]keyHint{{"", m.inputGuidance()},
		{"Tab", "Apply/Cancel"}, {"Left/Right", "Caret/action"}, {"Esc", hintCancel},
		{"Ctrl+u", "Clear"}, {"Ctrl+x", hintStopAll}})
}

//nolint:goconst // Contextual shortcut tables retain the same key and action vocabulary.
func (m *model) selectionFooter() []string {
	if m.picker != nil {
		return m.hintRows([]keyHint{{"↑↓", "Choose"}, {"Enter", "Open/import"}, {"p", "Preview"},
			{"Ctrl+p/n", "Prev/next page"}, {"/", "Path"}, {"Backspace", "Parent"}, {"Esc", hintCancel}, {"x", "Stop"}})
	}
	if m.assigning {
		return m.hintRows([]keyHint{{"↑↓", "Choose"}, {"Enter/y", "Assign"}, {"Space", "Preview/pause"}, {"Esc", hintCancel},
			{"/", "Search"}, {"Ctrl+f", "Filter"}, {"x", hintStopAll}})
	}
	if m.variantCue != "" && m.focus == focusBrowser {
		return m.hintRows([]keyHint{{"↑↓", "Clip"}, {"Enter", "Preview"}, {"a", "Append"}, {"Delete", "Remove"},
			{"Ctrl+↑↓", "Reorder"}, {"Ctrl+p/n", "Prev/next page"}, {"Shift+d", "Duplicate"},
			{"Ctrl+s", "Save"}, {"Esc", "Back"}})
	}

	return m.browseFooter()
}

//nolint:goconst // Each focus surface publishes a complete readable shortcut table.
func (m *model) browseFooter() []string {
	const primaryHintCapacity = 9
	rows := make([]keyHint, 0, primaryHintCapacity)
	rows = append(rows, keyHint{"Tab/1–4", "Workspace"}, keyHint{"/", "Search"},
		keyHint{"f", "Choose sound/file"}, keyHint{"?", "All shortcuts"})
	if m.focus == focusWave {
		rows = []keyHint{{"i/o", "Mark trim"}, {"s/e", "Exact trim"}, {"←→", "Seek/nudge"}, {"Esc", "Browse"}, {"?", "Help"}}
	}
	if m.focus == focusDetails {
		rows = []keyHint{{"↑↓", "Scroll"}, {"f", "Choose sound/file"}, {"Ctrl+L", "License"},
			{"Esc", "Browse"}, {"?", "Help"}}
	}
	global := []keyHint{{"Ctrl+s", "Save"}, {"u", "Undo"}, {"q", "Quit"}}
	if m.focus == focusMix {
		rows = []keyHint{{"↑↓", "Layer"}, {"Enter", "Restart"}, {"Space", "Pause"}, {"u", "Mute"}, {"Esc", "Browse"}}
		global = []keyHint{{"v", "Preview volumes"}, {"x", hintStopAll}, {"Ctrl+s", "Save"}, {"q", "Quit"}}
	}

	return m.hintRows(append(rows, global...))
}

func (m *model) fieldLabel() string {
	if m.field == fieldStart {
		return "Start time (ms)"
	}
	if m.field == fieldEnd {
		return "End time (ms)"
	}
	if m.field == fieldGain && !m.browsingCues() {
		return "Recording calibration (saved dB)"
	}
	if m.field == fieldGain {
		return "Moment gain (saved amplitude)"
	}

	return m.field
}

func (m *model) trimExamples() (int64, int64) {
	const exampleStartMs = 420
	const exampleEndMs = 2350
	const startFraction = 4
	end := int64(exampleEndMs)
	if m.clip != nil {
		end = min(end, m.clip.DurationMs())
		if m.window().StartMs >= end {
			end = m.clip.DurationMs()
		}
	}
	startLimit := end
	if m.clip != nil {
		startLimit = min(startLimit, m.window().EndMs)
	}
	start := min(int64(exampleStartMs), max(0, startLimit)/startFraction)

	return start, end
}

func timeExample(label string, milliseconds int64) string {
	return fmt.Sprintf("%s ms e.g. %d=%.3fs", label, milliseconds, float64(milliseconds)/msPerSecond)
}

func (m *model) inputGuidance() string {
	start, end := m.trimExamples()
	switch m.field {
	case fieldNote:
		return "Note: Enter inserts a line; Tab selects Apply/Cancel; Escape discards."
	case fieldLicense:
		return "Enter documented license terms (e.g. CC BY 4.0), or unknown. Blank is not allowed."
	case fieldKeywords:
		return "Comma-separated keywords or phrases · replaces this item's custom terms"
	case fieldTrigger:
		return "One sentence describing this moment's usual gameplay trigger"
	case fieldStart:
		return timeExample("Start", start) + "; sets the draft only."
	case fieldEnd:
		return timeExample("End", end) + "; sets the draft only."
	default:
		return "Text input · Enter applies"
	}
}
