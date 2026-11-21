package editorui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	minimumWidth       = 60
	minimumHeight      = 24
	headerRows         = 1
	statusRows         = 1
	panelEdges         = 2
	edgeLabelPadding   = 3
	minimumWaveHeight  = 6
	maximumWaveHeight  = 12
	smallestWaveHeight = 4
	waveHeightDivisor  = 5
	paneGap            = 1
	leftPanePercent    = 55
	percentScale       = 100
)

type panelFocus int

const (
	focusBrowser panelFocus = iota
	focusWave
	focusDetails
	focusMix
)

type editorLayout struct{ width, left, right, top, wave int }

// The list owns whatever height the fixed rows and a modest waveform leave.
func (m *model) layout() editorLayout {
	width, height := max(minimumWidth, m.width), max(minimumHeight, m.height)
	fixed := headerRows + statusRows + len(m.footerLines()) + transportRows + m.soundscapeRows()
	wave := max(minimumWaveHeight, min(maximumWaveHeight, height/waveHeightDivisor))
	minimumBrowserHeight := panelEdges + m.browserLabelRows() + 1
	wave = max(smallestWaveHeight, min(wave, height-fixed-minimumBrowserHeight))
	if m.routingWorkspace() {
		wave = 0 // Assignment workspaces give space to settings; trimming stays in Recordings/Moments.
	}
	left := width * leftPanePercent / percentScale

	return editorLayout{width: width, left: left, right: width - left - paneGap, top: height - fixed - wave, wave: wave}
}

func (m *model) listHeight() int { return max(1, m.layout().top-panelEdges-m.browserLabelRows()) }

func (m *model) searchRowVisible() bool { return m.field == fieldSearch || m.search != "" }

// Column titles, plus the filter row while one is set and the routing legend.
func (m *model) browserLabelRows() int {
	rows := 1
	if m.searchRowVisible() {
		rows++
	}
	if m.routingWorkspace() && m.routing.menu == nil {
		rows++
	}

	return rows
}

func (m *model) View() string {
	if m.width < minimumWidth || m.height < minimumHeight {
		message := "Audio Library Editor — enlarge terminal to at least 60 × 24.\n" +
			"Editing paused; Ctrl+x stops audio; Ctrl+s saves; q quits."
		if m.confirmQuit {
			message += "\nUnsaved work: y discards and quits; Escape keeps editing."
		}

		return message + "\n" + safe(m.status)
	}
	width := min(maxDialogWidth, m.width-editorDialogMargin)
	background := m.workspaceView()
	if dialog, visible := m.dialogContent(width - panelEdges); visible {
		if m.statusIsError && m.field == "" && m.form == nil && dialog.focus >= 0 {
			dialog.rows = append(dialog.rows, m.colors.problem.Render("! "+safe(m.status)))
			dialog.focus = len(dialog.rows) - 1 // Scrolling dialogs keep their own position.
		}

		return m.overlayDialog(background, dialog, width)
	}

	return background
}

func (m *model) workspaceView() string {
	layout := m.layout()
	left := m.panelWithFoot(m.browserTitle(), m.browserLines(layout.left-panelEdges), layout.left, layout.top,
		m.panelFocused(focusBrowser), m.browserFoot())
	panes := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", paneGap), m.rightPane(layout))
	sections := []string{m.headerLine(layout.width), panes}
	if layout.wave > 0 {
		sections = append(sections, m.waveform(layout.width, layout.wave))
	}
	if m.soundscapeRows() > 0 {
		sections = append(sections, m.soundscapeStatus(layout.width))
	}
	sections = append(sections, m.transportView(layout.width), m.statusLine(layout.width),
		strings.Join(m.footerLines(), "\n"))

	return strings.Join(sections, "\n")
}

// One row: where you are, which file, and whether it is saved.
func (m *model) headerLine(width int) string {
	tabs := " " + m.workspaceTitle()
	title := m.colors.title.Render(" AUDIO EDITOR") + "  " + tabs
	// Narrow terminals give up the program name, then the file name, never the state.
	for _, choice := range [][2]string{{title, m.saveState(true)}, {tabs, m.saveState(true)}, {tabs, m.saveState(false)}} {
		if gap := width - ansi.StringWidth(choice[0]) - ansi.StringWidth(choice[1]) - 1; gap > 0 {
			return choice[0] + strings.Repeat(" ", gap) + choice[1] + " "
		}
	}

	return fit(tabs, width)
}

func (m *model) saveState(withFile bool) string {
	state := m.colors.ready.Render("● saved")
	if m.doc.Dirty() {
		state = m.colors.warning.Render("* unsaved")
	}
	if m.doc.NeedsSync() {
		state += m.colors.warning.Render(" · save not confirmed")
	}
	if m.duplicate != nil {
		state += m.colors.warning.Render(" · duplicate draft")
	} else if m.crop != nil {
		state += m.colors.warning.Render(" · trim draft")
	}

	if !withFile {
		return state
	}
	file := filepath.Base(m.doc.Path())
	if m.scanDone < m.scanTotal {
		file = fmt.Sprintf("checking %d of %d · %s", m.scanDone, m.scanTotal, file)
	}

	return m.colors.dim.Render(file) + " " + state
}

func (m *model) statusLine(width int) string {
	if m.statusIsError {
		return m.colors.problem.Render(fit(" ! "+safe(m.status), width))
	}

	return fit(" "+safe(m.status), width)
}

func (m *model) rightPane(layout editorLayout) string {
	lines := m.visibleDetails(layout.right - panelEdges)
	if m.crop != nil && !m.volumeControls {
		lines = m.visibleCropLines(layout.right-panelEdges, layout.top-panelEdges)
	}

	return m.panel(m.rightTitle(), lines, layout.right, layout.top,
		m.panelFocused(focusDetails) || m.panelFocused(focusMix))
}

func (m *model) rightTitle() string {
	draft := m.colors.warning.Render("  draft")
	switch {
	case m.volumeControls:
		return m.colors.title.Render("PREVIEW VOLUME") + m.colors.dim.Render("  temporary")
	case m.duplicate != nil:
		return m.colors.title.Render("DUPLICATE") + "  " + safe(m.crop.recordingID) + draft
	case m.crop != nil:
		return m.colors.title.Render("TRIM") + "  " + safe(m.crop.recordingID) + draft
	case m.soundscapeVisible:
		return m.colors.title.Render("SOUNDSCAPE") + m.colors.dim.Render("  temporary")
	case m.routingWorkspace() || m.routing.menu != nil || m.selectedID() == "":
		return m.colors.title.Render("DETAILS")
	}
	name := m.selectedID()
	if m.browsingCues() {
		name = m.doc.Cue(name).Name
	}

	return m.colors.title.Render(safe(name)) + "  " + m.badge(m.itemStatus(m.selectedID()))
}

func (m *model) workspaceTitle() string {
	var tabs []string
	for workspace := workspaceRecordings; workspace < workspaceCount; workspace++ {
		label := strconv.Itoa(int(workspace)+1) + " " + workspaceName(workspace)
		if workspace == m.workspace {
			tabs = append(tabs, m.colors.selected.Render("["+label+"]"))
		} else {
			tabs = append(tabs, m.colors.dim.Render(" "+label+" "))
		}
	}

	return strings.Join(tabs, " ")
}

func (m *model) browserTitle() string {
	name := strings.ToUpper(workspaceName(m.workspace))
	switch {
	case m.routing.menu != nil:
		return m.colors.dim.Render(safe(m.routing.menu.title))
	case m.routingWorkspace():
		return m.colors.title.Render(name) + "  " + m.colors.dim.Render(m.routingFilterTitle())
	case m.picker != nil:
		return m.colors.title.Render("FILES")
	case m.variantCue != "" && !m.assigning:
		return m.colors.title.Render("CLIPS IN TURN") + "  " + safe(m.doc.Cue(m.variantCue).Name)
	case m.assigning:
		name = strings.ToUpper(workspaceName(workspaceRecordings))
	}

	return m.colors.title.Render(name) + "  " + m.filterChips()
}

// The chips double as the Ctrl+F filter: the lit one is what the list shows.
func (m *model) filterChips() string {
	labels := [filterCount]string{filterLabelAll, "With audio", "Empty", "Problems"}
	counts := [filterCount]int{}
	for _, id := range m.workspaceIDs() {
		status := m.itemStatus(id)
		for filter := filterAll; filter < filterCount; filter++ {
			if matchesFilter(status, filter) {
				counts[filter]++
			}
		}
	}
	parts := make([]string, 0, filterCount)
	for filter := filterAll; filter < filterCount; filter++ {
		text := fmt.Sprintf("%s %d", labels[filter], counts[filter])
		if filter == m.filter {
			text = m.colors.selected.Render(" " + text + " ")
		} else {
			text = m.colors.dim.Render(text)
		}
		parts = append(parts, text)
	}

	return strings.Join(parts, "  ")
}

func (m *model) browserFoot() string {
	if m.picker != nil || m.routing.menu != nil || len(m.keys) == 0 {
		return ""
	}
	foot := fmt.Sprintf("%d of %d", m.cursor+1, len(m.keys))
	if m.browsingCues() {
		foot += " · A–Z"
	}

	return m.colors.dim.Render(foot)
}

func (m *model) panel(title string, lines []string, width, height int, active bool) string {
	return m.panelWithFoot(title, lines, width, height, active, "")
}

// The pane with the keys gets a heavy cyan frame, so focus reads without colour.
func (m *model) panelWithFoot(title string, lines []string, width, height int, active bool, foot string) string {
	style, side := m.colors.dim, "│"
	corners := [...]string{"┌", "─", "┐", "└", "┘"}
	if active {
		style, side = m.colors.focus, "┃"
		corners = [...]string{"┏", "━", "┓", "┗", "┛"}
	}
	inside := max(1, width-panelEdges)
	rows := make([]string, 0, height)
	rows = append(rows, panelEdge(style, corners[0], corners[1], corners[2], title, inside))
	for row := range height - panelEdges {
		line := ""
		if row < len(lines) {
			line = lines[row]
		}
		rows = append(rows, style.Render(side)+fit(line, inside)+style.Render(side))
	}
	rows = append(rows, panelEdge(style, corners[3], corners[1], corners[4], foot, inside))

	return strings.Join(rows, "\n")
}

func panelEdge(style lipgloss.Style, left, fill, right, label string, inside int) string {
	if label == "" || inside <= edgeLabelPadding {
		return style.Render(left + strings.Repeat(fill, inside) + right)
	}
	label = ansi.Truncate(label, inside-edgeLabelPadding, "…")
	rest := inside - ansi.StringWidth(label) - edgeLabelPadding

	return style.Render(left+fill+" ") + label + style.Render(" "+strings.Repeat(fill, rest)+right)
}

func plainFit(text string, width int, pad string) string {
	text = ansi.Truncate(text, width, "…")

	return text + strings.Repeat(pad, max(0, width-ansi.StringWidth(text)))
}

func fit(text string, width int) string { return plainFit(text, max(0, width), " ") }

func safe(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}

		return r
	}, text)
}
