package editorui

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	minimumWidth       = 60
	minimumHeight      = 24
	headerRows         = 4
	statusRows         = 1
	panelEdges         = 2
	browserLabels      = 3
	browserHeadingRows = 2
	minimumWaveHeight  = 7
	maximumWaveHeight  = 24
	waveHeightDivisor  = 3
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

type editorLayout struct{ width, left, right, top, wave, mix int }

func (m *model) layout() editorLayout {
	width, height := max(minimumWidth, m.width), max(minimumHeight, m.height)
	footerRows := len(m.footerLines())
	wave := max(minimumWaveHeight, min(maximumWaveHeight, height/waveHeightDivisor))
	mix := 1
	minimumBrowserHeight := panelEdges + m.browserLabelRows() + 1
	if m.field != "" && m.inlineInput() && m.field != fieldSearch {
		minimumBrowserHeight = max(minimumBrowserHeight, len(m.inputLines(width/columnSplit))+panelEdges)
	}
	available := height - headerRows - statusRows - footerRows - minimumBrowserHeight - transportRows
	if mix > 1 && available-mix < minimumWaveHeight {
		mix = 1
	}
	wave = max(panelEdges+1, min(wave, available-mix))
	if m.routingWorkspace() {
		wave = 0 // Assignment workspaces give space to settings; trimming stays in Recordings/Moments.
	}
	top := height - headerRows - statusRows - footerRows - wave - mix - transportRows
	left := width * leftPanePercent / percentScale

	return editorLayout{width: width, left: left, right: width - left - paneGap, top: top, wave: wave, mix: mix}
}

func (m *model) listHeight() int { return max(1, m.layout().top-panelEdges-m.browserLabelRows()) }

func (m *model) browserLabelRows() int {
	const compactFooterWidth = 80
	extra := 0
	if m.field == fieldSearch {
		extra = 2
	}
	if m.height <= minimumHeight && m.width < compactFooterWidth {
		return browserHeadingRows + extra
	}

	return browserLabels + extra
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
	const dialogWidth = 84
	width := min(dialogWidth, m.width-editorDialogMargin)
	background := m.workspaceView()
	if dialog, visible := m.dialogContent(width - panelEdges); visible {
		if m.statusIsError && m.field == "" && m.form == nil {
			dialog.rows = append(dialog.rows, m.colors.problem.Render("! "+safe(m.status)))
			dialog.focus = len(dialog.rows) - 1
		}

		return m.overlayDialog(background, dialog, width)
	}

	return background
}

func (m *model) workspaceView() string {
	layout := m.layout()
	state := m.colors.ready.Render("SAVED")
	dirty := m.doc.Dirty()
	if dirty {
		state = m.colors.warning.Render("* UNSAVED")
	}
	if m.doc.NeedsSync() {
		state = m.colors.warning.Render("* DURABILITY PENDING")
		if dirty {
			state += " · UNSAVED"
		}
	}
	if m.crop != nil {
		state += " · REGION DRAFT"
	}
	title := "AUDIO LIBRARY EDITOR  /  " + strings.ToUpper(workspaceName(m.workspace)) + "  " + state
	tabs := m.workspaceTitle()
	left := m.panel(m.browserTitle(), m.browserLines(layout.left-panelEdges), layout.left, layout.top,
		m.panelFocused(focusBrowser))
	panes := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", paneGap), m.rightPane(layout))
	status := safe(m.status)
	if m.statusIsError {
		status = m.colors.problem.Render("! " + status)
	}
	sections := []string{fit(m.colors.title.Render(title), layout.width), fit(tabs, layout.width),
		m.previewVolumeStrip(layout.width), m.filterStrip(layout.width), panes}
	if layout.wave > 0 {
		sections = append(sections, m.waveform(layout.width, layout.wave))
	}
	sections = append(sections, m.soundscapeStatus(layout.width), m.transportView(layout.width),
		fit(status, layout.width), strings.Join(m.footerLines(), "\n"))

	return strings.Join(sections, "\n")
}

func (m *model) rightPane(layout editorLayout) string {
	rightTitle := "DETAILS · d focus / Esc browse"
	if m.soundscapeVisible {
		rightTitle = "SOUNDSCAPE · temporary · m focus"
	}
	if m.volumeControls {
		rightTitle = "PREVIEW VOLUME · temporary"
	}
	var rightLines []string
	if m.crop != nil && !m.volumeControls {
		rightTitle = "REGION DRAFT · Apply/Cancel"
		if m.duplicate != nil {
			rightTitle = "DUPLICATE CLIP · Create/Cancel"
		}
		rightLines = m.visibleCropLines(layout.right-panelEdges, layout.top-panelEdges)
	} else {
		rightLines = m.visibleDetails(layout.right - panelEdges)
	}
	inline := m.field != "" && m.field != fieldSearch && m.inlineInput()
	if inline {
		rightTitle = "EDITING · " + m.fieldLabel()
		rightLines = m.inputLines(layout.right - panelEdges)
	}

	return m.panel(rightTitle, rightLines, layout.right, layout.top,
		m.panelFocused(focusDetails) || m.panelFocused(focusMix))
}

func (m *model) workspaceTitle() string {
	var tabs []string
	for workspace := workspaceRecordings; workspace < workspaceCount; workspace++ {
		label := strconv.Itoa(int(workspace)+1) + " " + workspaceName(workspace)
		if workspace == m.workspace {
			tabs = append(tabs, m.colors.key.Render("▶ "+strings.ToUpper(label)+" ◀"))
		} else {
			tabs = append(tabs, m.colors.dim.Render("["+label+"]"))
		}
	}

	return strings.Join(tabs, " ")
}

func (m *model) browserTitle() string {
	if m.routing.menu != nil {
		return m.routing.menu.title
	}
	if m.workspace == workspaceTiles {
		return "TILES · animation families / audio"
	}
	if m.workspace == workspaceLocations {
		return "LOCATIONS · music / dungeon ambience"
	}
	if m.duplicate != nil {
		return "DUPLICATE NEXT · Enter edits the selected field"
	}
	if m.picker != nil {
		return "FILES · Enter open/import"
	}
	if m.variantCue != "" && !m.assigning {
		return "ORDERED CLIPS · " + m.variantCue
	}
	if m.browsingCues() {
		return "MOMENTS · target family / assignment"
	}

	return "RECORDINGS · family / calibration / uses"
}

func (m *model) panel(title string, lines []string, width, height int, active bool) string {
	style := m.colors.dim
	if active {
		style = m.colors.focus
		title = "ACTIVE · " + title
	}
	inside := max(1, width-panelEdges)
	rows := make([]string, 0, height)
	rows = append(rows, style.Render("┌"+plainFit(" "+title+" ", inside, "─")+"┐"))
	for row := range height - panelEdges {
		line := ""
		if row < len(lines) {
			line = lines[row]
		}
		rows = append(rows, style.Render("│")+fit(line, inside)+style.Render("│"))
	}
	rows = append(rows, style.Render("└"+strings.Repeat("─", inside)+"┘"))

	return strings.Join(rows, "\n")
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
