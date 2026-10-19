package editorui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const routingStatusThreshold = 60
const routingStatusWidth = 10

type routingColumns struct {
	state, source, assignment, surface int
}

func routingColumnsForWidth(width int) routingColumns {
	state := compactStatusWidth
	if width >= routingStatusThreshold {
		state = routingStatusWidth
	}
	remaining := max(0, width-rowPrefixWidth-state-momentColumnGaps*columnGap)
	source := remaining / columnSplit

	return routingColumns{state: state, source: source, assignment: remaining - source, surface: 0}
}

func (m *model) routingColumns(width int) routingColumns {
	columns := routingColumnsForWidth(width)
	if m.workspace == workspaceTiles {
		const surfaceDivisor = 3
		columns.surface = max(1, (columns.source+columns.assignment)/surfaceDivisor)
		remaining := columns.source + columns.assignment - columns.surface - columnGap
		columns.source, columns.assignment = remaining/columnSplit, remaining-remaining/columnSplit
	}

	return columns
}

func (m *model) routingColumnHeader(width int) string {
	columns := m.routingColumns(width)
	state := "S"
	if columns.state == routingStatusWidth {
		state = "State"
	}

	assignment := "Assignment"
	if columns.surface > 0 {
		assignment = "Ambient"
	}
	line := strings.Repeat(" ", rowPrefixWidth) + fit(state, columns.state) + " " +
		fit("Source", columns.source) + " " + fit(assignment, columns.assignment)
	if columns.surface > 0 {
		line += " " + fit("Footsteps", columns.surface)
	}

	return m.colors.detailLabel.Render(line)
}

func (m *model) routingAssignmentStyle(cue string) (string, lipgloss.Style) {
	if cue == "" {
		return "— None", m.colors.dim
	}
	switch m.cueAvailability(cue).State {
	case audio.RecordingReady:
		return "● Assigned", m.colors.ready
	case audio.RecordingNoFile:
		return "○ No file", m.colors.warning
	case audio.RecordingMissing:
		return "! Missing", m.colors.problem
	case audio.RecordingInvalid:
		return "! Invalid", m.colors.problem
	case audio.RecordingChecking:
		return "… Checking", m.colors.warning
	}

	return "! Unknown", m.colors.problem
}

func (m *model) routingItemLine(identity string, selected bool, width int) string {
	columns := m.routingColumns(width)
	cue := m.routingCueFor(identity)
	state, assignmentStyle := m.routingAssignmentStyle(cue)
	name := m.doc.Cue(cue).Name
	if cue == "" {
		name = "None"
		if m.workspace == workspaceLocations {
			state, name = "— Silent", "Silence"
		}
	}
	if columns.state == compactStatusWidth {
		state, _, _ = strings.Cut(state, " ")
	}
	cursor, marked := " ", " "
	if m.routing.selected[identity] {
		marked = "✓"
	}
	sourceStyle := m.colors.detailValue
	if selected {
		cursor = ">"
		sourceStyle = m.colors.selected
	}
	// Keep semantic colours visible on the selected row; only its source cell
	// and fixed-width cursor/mark cells receive the selection background.
	line := sourceStyle.Render(cursor+marked) + assignmentStyle.Render(fit(state, columns.state)) + " " +
		sourceStyle.Render(fit(safe(m.routingName(identity)), columns.source)) + " " +
		assignmentStyle.Render(fit(safe(name), columns.assignment))
	if columns.surface > 0 {
		surface, style := m.doc.Routing().Tiles[identity].Surface, m.colors.detailValue
		if surface == "" {
			surface, style = "Default", m.colors.focus
		}
		line += " " + style.Render(fit(safe(surface), columns.surface))
	}

	return line
}

func (m *model) routingLegend(width int) string {
	if width < extraColumnThreshold {
		return fit(m.colors.ready.Render("● set")+" "+m.colors.warning.Render("○ empty")+" "+
			m.colors.problem.Render("! bad")+" "+m.colors.dim.Render("— none"), width)
	}

	return fit(m.colors.ready.Render("● assigned")+"  "+m.colors.warning.Render("○ empty")+"  "+
		m.colors.problem.Render("! problem")+"  "+m.colors.dim.Render("— none"), width)
}
