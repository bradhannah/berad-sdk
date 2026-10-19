package editorui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func (m *model) routingFilterTitle() string {
	filters := []string{"All", "Assigned", "Silent / none", "Needs audio", "Surface overrides"}
	if m.workspace == workspaceLocations {
		filters[4] = "Inherited"
	}
	text := "FILTER: " + filters[m.routing.filter] + " · Ctrl+F changes"
	if m.workspace == workspaceLocations {
		scopes := []string{"All", m.groupLabel(routingMusic), m.groupLabel(routingDungeon),
			"Dynamic emitters", m.groupLabel(routingNaval)}
		scope := scopes[m.routing.scope]
		text += "  SCOPE: " + scope + " · s changes"
	}

	return text
}

func (m *model) routingBrowserLines(width int) []string {
	if menu := m.routing.menu; menu != nil {
		choices := m.routingMenuChoices()
		rows := []string{fit(menu.title, width), fit("/ Search: "+menu.search, width)}
		start := listViewportStart(0, menu.cursor, len(choices), m.listHeight())
		for index := start; index < min(len(choices), start+m.listHeight()); index++ {
			choice := choices[index]
			label := safe(choice.label)
			if choice.cue != "" && m.cueAvailability(choice.cue).State != audio.RecordingReady {
				label += " [needs audio]"
			}
			line := "  " + label
			if index == menu.cursor {
				line = m.colors.selected.Render(fit("> "+label, width))
			}
			rows = append(rows, fit(line, width))
		}

		return rows
	}
	rows := append(m.browserSearchLines(width), m.routingColumnHeader(width))
	start := m.browserWindowStart()
	for row := range m.listHeight() {
		index := start + row
		line := ""
		if index < len(m.keys) {
			line = m.routingItemLine(m.keys[index], index == m.cursor, width)
		} else if row == 0 && len(m.keys) == 0 {
			line = "No matches. / search; Ctrl+F filter."
		}
		rows = append(rows, fit(line, width))
	}
	if m.browserLabelRows() > browserHeadingRows {
		rows = append(rows, m.routingLegend(width))
	}

	return rows
}

func (m *model) routingDetailLines(width int) []string {
	var rows []string
	add := func(text string) { rows = append(rows, strings.Split(ansi.Hardwrap(safe(text), width, true), "\n")...) }
	cue := m.assignmentCueForSelection()
	if m.routing.menu != nil {
		add(m.routing.menu.title)
		add("Enter chooses · p previews · Esc returns")
	} else {
		rows = append(rows, m.colors.focus.Render(fit(strings.ToUpper(m.routingName(m.selectedID())), width)))
		state, style := m.routingAssignmentStyle(cue)
		rows = append(rows, style.Render(state))
		if m.workspace != workspaceTiles {
			m.routingLocationDetails(add)
		}
	}
	if cue == "" {
		label := "Ambient sound: None / silence"
		group, target, _ := strings.Cut(m.selectedID(), "|")
		if m.workspace == workspaceTiles {
			group = ""
		}
		if m.targetPolicy(group, target).Category == categoryMusic {
			label = "Music: None / silence"
		}
		rows = append(rows, m.colors.dim.Render(label))
		add("f: choose a sound. Unassigned is not a missing asset.")
		if m.workspace == workspaceTiles {
			m.routingTileDetails(add)
		}

		return m.colourRoutingDetails(rows)
	}
	_, assignmentStyle := m.routingAssignmentStyle(cue)
	rows = append(rows, m.colors.detailLabel.Render("Sound: ")+assignmentStyle.Render(safe(m.doc.Cue(cue).Name)))
	rows = append(rows, m.colors.detailLabel.Render("Recording: ")+
		m.colors.detailValue.Render(safe(m.doc.Cue(cue).PrimaryRecording())))
	rows = append(rows, m.colors.focus.Render("[f Change sound] [r Recording]"))
	if m.workspace == workspaceTiles {
		rows = append(rows, m.colors.focus.Render("[B Tile settings] [b Sound defaults]"))
	} else {
		rows = append(rows, m.colors.focus.Render("[i Choose file] [B Sound settings]"))
	}
	m.appendAmbientDetails(cue, add, func(heading string) { rows = append(rows, m.colors.focus.Render(heading)) })
	if m.workspace == workspaceTiles {
		m.routingTileDetails(add)
	}
	add(fmt.Sprintf("Used by: %d configured assignments (U opens)", len(m.routingUsages(cue))))

	return m.colourRoutingDetails(rows)
}

func (m *model) appendAmbientDetails(cue string, add, heading func(string)) {
	if profile, ambient := m.doc.Routing().Ambient[cue]; ambient {
		switch {
		case m.workspace == workspaceTiles:
			heading("TILE SOUND · EFFECTIVE SETTINGS")
			m.tileAmbientDetails(add)
		case profile.RadiusTiles == 0:
			heading("SOUND SETTINGS · SHARED")
			add("Non-spatial ambient background")
		default:
			heading("SOUND SETTINGS · SHARED")
			add(fmt.Sprintf("Hearing range: %g tiles", profile.RadiusTiles))
		}
		add(fmt.Sprintf("Fade in/out: %g / %g seconds",
			float64(profile.FadeInMs)/msPerSecond, float64(profile.FadeOutMs)/msPerSecond))
	}
}

func (m *model) tileAmbientDetails(add func(string)) {
	routing := m.doc.Routing()
	assignment := routing.Tiles[m.selectedID()]
	profile, level := routing.TileAmbientSettings(m.selectedID())
	distanceOrigin, levelOrigin := "sound default", "sound default"
	if assignment.RadiusTiles != nil {
		distanceOrigin = "tile override"
	}
	if assignment.Gain != nil {
		levelOrigin = "tile override"
	}
	add(fmt.Sprintf("Hearing distance: %g tiles (%s)", profile.RadiusTiles, distanceOrigin))
	add(fmt.Sprintf("Relative level: %g%% (%s)", level*levelPercentScale, levelOrigin))
}

func (m *model) colourRoutingDetails(rows []string) []string {
	for index, line := range rows {
		if !strings.Contains(line, "\x1b[") {
			rows[index] = m.colourDetailText(line)
		}
	}

	return rows
}

func (m *model) routingLocationDetails(add func(string)) {
	kind, target, _ := strings.Cut(m.selectedID(), "|")
	_, explicit := m.doc.ResolveAssignment(kind, target)
	if explicit || kind == routingEmitter || m.targets.Groups[kind].DefaultTarget == "" {
		add("Assignment: explicit [working]")
	} else {
		add("Assignment: inherits default [working]")
	}
	if help := m.targetPolicy(kind, target).Help; help != "" {
		add(help)
	} else {
		add("The consuming application owns activation and scheduling; this assignment chooses its sound.")
	}
}

func (m *model) routingTileDetails(add func(string)) {
	tile := m.audioTile(m.selectedID())
	add("FOOTSTEPS")
	routing := m.doc.Routing()
	surface := routing.Tiles[tile.ID].Surface
	if surface == "" {
		surface = "Inherit → " + routing.DefaultSurface
	}
	add("Footstep surface: " + surface + " [working]")
	add("p: preview ambient · P: preview footstep")
	if len(tile.Frames) > 1 {
		add("ANIMATION · READ ONLY")
		add(fmt.Sprintf("%d frames · one sound assignment", len(tile.Frames)))
		add("F: view animation details")
	}
	selected := 0
	for _, active := range m.routing.selected {
		if active {
			selected++
		}
	}
	if selected > 0 {
		add(fmt.Sprintf("%d marked families: assignment edits include these and the cursor. Esc clears.", selected))
	}
}

//nolint:goconst // Keyboard help uses literal key names and short action labels like the existing editor.
func (m *model) routingFooter() []string {
	if m.routing.menu != nil {
		return m.hintRows([]keyHint{{"↑↓", "Choose"}, {"/", "Search"}, {"p", "Preview"},
			{"Enter", "Select"}, {"Esc", "Back"}, {"x", "Stop"}})
	}
	hints := []keyHint{{"Tab/1–4", "Workspace"}, {"/", "Search"}, {"f", "Choose sound"},
		{"B", "Settings"}, {"?", "All shortcuts"}}
	if m.workspace == workspaceTiles {
		hints = append(hints, keyHint{"Insert", "Mark"}, keyHint{"s", "Surface"}, keyHint{"P", "Preview step"})
	} else {
		hints = append(hints, keyHint{"s", "Scope"})
	}
	hints = append(hints, keyHint{"Ctrl+S", "Save"}, keyHint{"u", "Undo"}, keyHint{"q", "Quit"})

	return m.hintRows(hints)
}

func (m *model) colourDetailText(line string) string {
	if line == "FOOTSTEPS" || line == "ANIMATION · READ ONLY" {
		return m.colors.focus.Render(line)
	}
	label, value, labelled := strings.Cut(line, ": ")
	if labelled {
		style := m.colors.detailValue
		if strings.Contains(value, "Inherit") {
			style = m.colors.focus
		}

		return m.colors.detailLabel.Render(label+": ") + style.Render(value)
	}

	return m.colors.dim.Render(line)
}
