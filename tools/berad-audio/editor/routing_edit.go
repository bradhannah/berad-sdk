package editorui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const routingNone = "<silence>"
const routingInherit = "<inherit>"

func (m *model) openRoutingAssignment(kind string) tea.Cmd {
	if m.selectedID() == "" {
		return nil
	}
	menu := &routingMenu{ //nolint:exhaustruct_v5 // Picker starts at the first choice with no search.
		kind: kind, title: "ASSIGN " + strings.ToUpper(kind), target: m.selectedID(),
	}
	if kind == "cue" {
		owner, target, _ := strings.Cut(menu.target, "|")
		if m.workspace == workspaceTiles {
			owner = ""
		}
		menu.title = m.targetPolicy(owner, target).Title
	}
	if kind == routingSurface || kind == "default surface" {
		menu.choices = m.surfaceChoices(kind)
	} else {
		menu.choices = m.routingAssignmentChoices(menu.target)
	}
	m.routing.menu = menu

	return m.loadSelection()
}

func (m *model) routingAssignmentChoices(destination string) []routingChoice {
	owner, target, _ := strings.Cut(destination, "|")
	if m.workspace == workspaceTiles {
		owner = ""
	}
	var choices []routingChoice
	if fallback := m.targets.Groups[owner].DefaultTarget; m.workspace == workspaceLocations &&
		fallback != "" && target != fallback {
		choices = append(choices, routingChoice{routingInherit, "Inherit default", "", nil})
	}
	if owner != routingEmitter {
		label := "Silence / no ambient source"
		if m.targetPolicy(owner, target).Category == categoryMusic {
			label = "No music"
		}
		choices = append(choices, routingChoice{routingNone, label, "", nil})
	}
	for _, cueID := range m.doc.CueIDs() {
		if m.compatibleRoutingCue(cueID, owner, target) {
			choices = append(choices, routingChoice{cueID, m.doc.Cue(cueID).Name, cueID, nil})
		}
	}

	return choices
}

func (m *model) surfaceChoices(kind string) []routingChoice {
	routing := m.doc.Routing()
	var choices []routingChoice
	if kind == routingSurface {
		choices = append(choices, routingChoice{routingInherit, "Inherit default → " + routing.DefaultSurface, "", nil})
	}
	for _, surface := range sortedRoutingKeys(routing.Surfaces) {
		cue := ""
		if cues := routing.Surfaces[surface]; len(cues) > 0 {
			cue = cues[0]
		}
		choices = append(choices, routingChoice{surface, surface, cue, nil})
	}

	return choices
}

func (m *model) compatibleRoutingCue(cueID, owner, target string) bool {
	profile, ambient := m.doc.Routing().Ambient[cueID]
	policy := m.targetPolicy(owner, target)
	if policy.Category == "music" {
		return m.cuePreviewCategory(cueID) == previewMusic && len(m.doc.Cue(cueID).Recordings()) == 1
	}
	if policy.Category == categorySFX {
		return m.cuePreviewCategory(cueID) == previewSFX
	}

	return ambient && (!policy.Background || profile.RadiusTiles == 0) && (!policy.Spatial || profile.RadiusTiles > 0)
}

func sortedRoutingKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
}

func (m *model) routingMenuChoices() []routingChoice {
	menu := m.routing.menu
	var choices []routingChoice
	for _, choice := range menu.choices {
		if strings.Contains(strings.ToLower(choice.label+" "+choice.id), strings.ToLower(menu.search)) {
			choices = append(choices, choice)
		}
	}
	menu.cursor = max(0, min(menu.cursor, len(choices)-1))

	return choices
}

//nolint:cyclop,funlen // Picker search and navigation are a single input state machine.
func (m *model) routingMenuKey(key tea.KeyMsg) tea.Cmd {
	menu := m.routing.menu
	if menu.editingSearch {
		return m.routingSearchKey(key)
	}
	choices := m.routingMenuChoices()
	switch key.String() {
	case keyEscape:
		m.routing.menu = menu.parent

		return m.loadSelection()
	case "/":
		menu.editingSearch = true
		menu.searchInput = menu.search
		menu.text.reset(menu.searchInput)
	case "up", "k":
		menu.cursor = max(0, menu.cursor-1)

		return m.loadSelection()
	case "down", "j":
		menu.cursor = max(0, min(len(choices)-1, menu.cursor+1))

		return m.loadSelection()
	case keyPageUp, keyPreviousPage:
		menu.cursor = max(0, menu.cursor-m.listHeight())

		return m.loadSelection()
	case keyPageDown, keyNextPage:
		menu.cursor = max(0, min(len(choices)-1, menu.cursor+m.listHeight()))

		return m.loadSelection()
	case "p":
		m.playRouting()
	case " ":
		m.togglePreview()
	case "B":
		return m.editAmbientProfile()
	case "r":
		return m.openSoundRecordingPicker()
	case keyCtrlA:
		return m.openNewAmbientForm()
	case keyEnter:
		if len(choices) == 0 {
			return nil
		}
		choice := choices[menu.cursor]
		if m.isRecordingMenu() || menu.kind == surfaceAddPicker {
			return m.applyFormChoice(choice)
		}
		if choice.usage != nil {
			return m.jumpToAudioUsage(*choice.usage)
		}

		return m.applyRoutingChoice(choice.id)
	}

	return nil
}

func (m *model) routingSearchKey(key tea.KeyMsg) tea.Cmd {
	menu := m.routing.menu
	switch textControlKey(key) {
	case keyEscape:
		menu.editingSearch = false
	case keyEnter, keyTab:
		menu.search, menu.editingSearch, menu.cursor = menu.searchInput, false, 0

		return m.loadSelection()
	default:
		menu.text.edit(&menu.searchInput, key, false)
	}

	return nil
}

func (m *model) applyRoutingChoice(identity string) tea.Cmd {
	menu := m.routing.menu
	routing := m.doc.Routing()
	switch {
	case menu.kind == "default surface":
		routing.DefaultSurface = identity
	case m.workspace == workspaceTiles:
		m.applyTileRoutingChoice(&routing, identity)
	default:
		kind, target, _ := strings.Cut(menu.target, "|")
		switch kind {
		case routingEmitter:
			routing.Emitters[target] = identity
		default:
			if routing.Groups == nil && identity != routingInherit {
				routing.Groups = make(map[string]map[string]*string)
			}
			assignments := routing.Groups[kind]
			if assignments == nil && identity != routingInherit {
				assignments = make(map[string]*string)
				routing.Groups[kind] = assignments
			}
			switch identity {
			case routingInherit:
				delete(assignments, target)
			case routingNone:
				assignments[target] = nil
			default:
				assignments[target] = &identity
			}
		}
	}
	if err := m.doc.SetRouting(routing); err != nil {
		m.report(err)

		return nil
	}
	m.routing.menu = nil
	m.refreshKeys()
	m.status = "Assignment updated. Ctrl+S saves; u undoes the entire edit."

	return m.loadSelection()
}

func (m *model) applyTileRoutingChoice(routing *audio.Routing, identity string) {
	if routing.Tiles == nil {
		routing.Tiles = make(map[string]audio.TileAudio)
	}
	menu := m.routing.menu
	targets := []string{menu.target}
	for target, selected := range m.routing.selected {
		if selected && target != menu.target {
			targets = append(targets, target)
		}
	}
	value := identity
	if identity == routingNone || identity == routingInherit {
		value = ""
	}
	for _, target := range targets {
		assignment := routing.Tiles[target]
		if menu.kind == routingSurface {
			assignment.Surface = value
		} else {
			assignment = assignment.WithAmbient(value)
		}
		if assignment.AmbientCue == "" && assignment.Surface == "" {
			delete(routing.Tiles, target)
		} else {
			routing.Tiles[target] = assignment
		}
	}
}

func (m *model) openRoutingUsages() tea.Cmd {
	cues := []string{m.assignmentCueForSelection()}
	if cues[0] == "" {
		cues = m.doc.Usages(m.recordingID())
	}
	menu := &routingMenu{ //nolint:exhaustruct_v5 // Fresh read-only navigation list.
		kind: "uses", title: "USED BY · configured assignments", target: m.selectedID(),
	}
	for _, cue := range cues {
		if cue == "" {
			continue
		}
		usage := routingUsage{Workspace: routingMoments, Target: cue, Label: m.doc.Cue(cue).Name}
		menu.choices = append(menu.choices, routingChoice{cue, "Moment: " + usage.Label, cue, &usage})
		for _, usage := range m.routingUsages(cue) {
			menu.choices = append(menu.choices, routingChoice{usage.Target, usage.Label, cue, &usage})
		}
	}
	m.routing.menu = menu

	return m.loadSelection()
}

func (m *model) rememberRoutingJump() {
	m.routing.returnSpace, m.routing.returnState = m.workspace, m.currentBrowseState()
	m.routing.hasReturn = true
}

func (m *model) openRoutingMoment() tea.Cmd {
	cue := m.assignmentCueForSelection()
	if cue == "" {
		return nil
	}

	return m.jumpToAudioUsage(routingUsage{Workspace: routingMoments, Target: cue, Label: m.doc.Cue(cue).Name})
}

func (m *model) jumpToAudioUsage(usage routingUsage) tea.Cmd {
	m.rememberRoutingJump()
	m.routing.menu = nil
	workspace, target := workspaceLocations, usage.Workspace+"|"+usage.Target
	switch usage.Workspace {
	case unitTiles:
		workspace, target = workspaceTiles, usage.Target
	case routingMoments:
		workspace, target = workspaceMoments, usage.Target
	case "recordings":
		workspace, target = workspaceRecordings, usage.Target
	case routingNaval:
		workspace, target = workspaceLocations, routingNaval+"|"+usage.Target
	}
	m.selectWorkspace(workspace)
	m.search, m.filter, m.routing.filter, m.routing.scope = "", filterAll, 0, 0
	m.refreshKeys()
	m.selectID(target)
	m.status = "Opened " + usage.Label + ". Esc returns."

	return m.loadSelection()
}

func (m *model) returnFromRoutingJump() tea.Cmd {
	workspace, state := m.routing.returnSpace, m.routing.returnState
	m.routing.hasReturn = false
	m.selectWorkspace(workspace)
	m.restoreBrowseState(state)

	return m.loadSelection()
}
