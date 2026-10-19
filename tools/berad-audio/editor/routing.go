package editorui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

type editorWorkspace int

const (
	workspaceRecordings editorWorkspace = iota
	workspaceMoments
	workspaceTiles
	workspaceLocations
	workspaceCount
)

const (
	routingMusic        = "music"
	routingDungeon      = "dungeon"
	routingNaval        = "naval"
	routingEmitter      = "emitter"
	routingMoments      = "moments"
	routingSurface      = "surface"
	fieldNewAmbient     = "new ambient"
	fieldAmbientProfile = "ambient radius, fade-in ms, fade-out ms"
	routingFilterCount  = 5
	routingScopeCount   = 5
)

type routingChoice struct {
	id, label, cue string
	usage          *routingUsage
}

type routingMenu struct {
	kind, title, target string
	choices             []routingChoice
	cursor              int
	search              string
	searchInput         string
	text                textCursor
	editingSearch       bool
	parent              *routingMenu
}

type routingEditor struct {
	tiles       []audio.TileTarget
	selected    map[string]bool
	menu        *routingMenu
	distance    float64
	footsteps   bool
	step        int
	filter      int
	scope       int
	fading      bool
	fadeStart   time.Time
	pauseStart  time.Time
	fadeFrom    float64
	fadeTo      float64
	fadeMs      int64
	envelope    float64
	returnSpace editorWorkspace
	returnState browseState
	hasReturn   bool
}

func (m *model) routingWorkspace() bool {
	return m.workspace == workspaceTiles || m.workspace == workspaceLocations
}

func (m *model) selectWorkspace(workspace editorWorkspace) tea.Cmd {
	if m.assigning || m.variantCue != "" || m.picker != nil || m.crop != nil {
		return nil
	}
	m.workspaceState[m.workspace] = m.currentBrowseState()
	m.workspace = workspace
	m.routing.menu = nil
	m.routing.fading = false
	m.soundscapeVisible = false
	m.focus, m.detailsScroll, m.marker = focusBrowser, 0, 0
	m.restoreBrowseState(m.workspaceState[workspace])
	m.status = "Workspace: " + workspaceName(workspace) + " · Tab/Shift+Tab or 1–4 switches"

	return m.loadSelection()
}

func workspaceName(workspace editorWorkspace) string {
	return [...]string{"Recordings", "Moments", "Tiles", "Locations"}[workspace]
}

func (m *model) routingIDs() []string {
	var ids []string
	if m.workspace == workspaceTiles {
		for _, tile := range m.routing.tiles {
			ids = append(ids, tile.ID)
		}

		return ids
	}
	if m.routing.scope == 0 || m.routing.scope == 1 {
		for target := range m.targets.Groups[routingMusic].Targets {
			ids = append(ids, "music|"+target)
		}
	}
	if m.routing.scope == 0 || m.routing.scope == 2 {
		for target := range m.targets.Groups[routingDungeon].Targets {
			ids = append(ids, "dungeon|"+target)
		}
	}
	if m.routing.scope == 0 || m.routing.scope == 3 {
		for emitter := range m.doc.Routing().Emitters {
			ids = append(ids, "emitter|"+emitter)
		}
	}
	if m.routing.scope == 0 || m.routing.scope == 4 {
		for situation := range m.targets.Groups[routingNaval].Targets {
			ids = append(ids, "naval|"+situation)
		}
	}
	slices.Sort(ids)

	return ids
}

func (m *model) audioTile(id string) audio.TileTarget {
	for _, tile := range m.routing.tiles {
		if tile.ID == id {
			return tile
		}
	}

	return audio.TileTarget{ID: "", Name: "", Frames: nil}
}

func (m *model) routingName(identity string) string {
	if m.workspace == workspaceTiles {
		tile := m.audioTile(identity)
		label := tile.Name + " · " + identity
		if len(tile.Frames) > 1 {
			label += fmt.Sprintf(" ↻%d", len(tile.Frames))
		}

		return label
	}
	kind, target, _ := strings.Cut(identity, "|")
	if kind == routingMusic {
		return m.targets.Groups[kind].Targets[target] + " · " + m.targets.Groups[kind].Label
	}
	if kind == routingDungeon {
		return m.targets.Groups[kind].Targets[target] + " · " + m.targets.Groups[kind].Label
	}
	if kind == routingNaval {
		return m.targets.Groups[kind].Targets[target] + " · " + m.targets.Groups[kind].Label
	}

	return target + " · dynamic emitter"
}

func (m *model) routingCueFor(identity string) string {
	routing := m.doc.Routing()
	if m.workspace == workspaceTiles {
		return routing.Tiles[identity].AmbientCue
	}
	kind, target, _ := strings.Cut(identity, "|")
	if kind == routingEmitter {
		return routing.Emitters[target]
	}
	cue, _ := m.doc.ResolveAssignment(kind, target)

	return cue
}

func (m *model) tileSurface(identity string) string {
	routing := m.doc.Routing()
	if surface := routing.Tiles[identity].Surface; surface != "" {
		return surface
	}

	return routing.DefaultSurface
}

func (m *model) footstepCue(identity string) string {
	cues := m.doc.Routing().Surfaces[m.tileSurface(identity)]
	if len(cues) == 0 {
		return ""
	}

	return cues[m.routing.step%len(cues)]
}

func (m *model) assignmentCueForSelection() string {
	if m.routing.menu != nil {
		return m.routingCue()
	}
	if m.routingWorkspace() {
		return m.routingCueFor(m.selectedID())
	}

	return m.activeCue()
}

func (m *model) routingCue() string {
	if menu := m.routing.menu; menu != nil {
		choices := m.routingMenuChoices()
		if len(choices) == 0 {
			return ""
		}

		return choices[menu.cursor].cue
	}
	if m.workspace == workspaceTiles && m.routing.footsteps {
		return m.footstepCue(m.selectedID())
	}

	return m.routingCueFor(m.selectedID())
}

func (m *model) refreshRoutingKeys() {
	selected := m.selectedID()
	m.keys = nil
	for _, identity := range m.routingIDs() {
		cueID := m.routingCueFor(identity)
		label := m.routingName(identity) + " " + cueID + " " + m.doc.Cue(cueID).Name
		matches := true
		for _, term := range strings.Fields(strings.ToLower(m.search)) {
			matches = matches && strings.Contains(strings.ToLower(label), term)
		}
		if matches && m.routingMatchesFilter(identity, cueID) {
			m.keys = append(m.keys, identity)
		}
	}
	m.cursor = max(0, min(m.cursor, len(m.keys)-1))
	m.selectID(selected)
	m.browserStart = m.browserWindowStart()
}

//nolint:mnd // Filter indices correspond directly to the ordered filter labels.
func (m *model) routingMatchesFilter(identity, cue string) bool {
	switch m.routing.filter {
	case 1:
		return cue != ""
	case 2:
		return cue == ""
	case 3:
		return cue != "" && m.cueAvailability(cue).State != audio.RecordingReady
	case 4:
		if m.workspace == workspaceTiles {
			return m.doc.Routing().Tiles[identity].Surface != ""
		}
		kind, target, _ := strings.Cut(identity, "|")
		_, explicit := m.doc.ResolveAssignment(kind, target)

		return !explicit && m.targets.Groups[kind].DefaultTarget != ""
	default:
		return true
	}
}

//nolint:cyclop,funlen,gocyclo // Auditable keyboard table; labels match the existing editor bindings.
func (m *model) routingKey(key string) tea.Cmd {
	if m.focus == focusDetails && m.scrollDetails(key) {
		return nil
	}
	switch key {
	case "1", "2", "3", "4":
		return m.selectWorkspace(editorWorkspace(key[0] - '1'))
	case keyTab:
		return m.selectWorkspace((m.workspace + 1) % workspaceCount)
	case "shift+tab":
		return m.selectWorkspace((m.workspace + workspaceCount - 1) % workspaceCount)
	case "up", "k":
		return m.move(-1)
	case "down", "j":
		return m.move(1)
	case keyPageUp, keyPreviousPage:
		return m.move(-m.listHeight())
	case keyPageDown, keyNextPage:
		return m.move(m.listHeight())
	case "/":
		m.beginSearch()
	case "ctrl+f":
		m.routing.filter = (m.routing.filter + 1) % routingFilterCount
		m.refreshKeys()

		return m.loadSelection()
	case "s":
		if m.workspace == workspaceLocations {
			m.routing.scope = (m.routing.scope + 1) % routingScopeCount
			m.refreshKeys()

			return m.loadSelection()
		}

		return m.openRoutingAssignment(routingSurface)
	case "f":
		return m.openRoutingAssignment("cue")
	case "r":
		return m.openSoundRecordingPicker()
	case "i":
		return m.importAssignedRecording()
	case "D":
		return m.openRoutingAssignment("default surface")
	case "S":
		return m.editSurfaceCues()
	case "insert":
		if m.workspace == workspaceTiles && m.selectedID() != "" {
			m.routing.selected[m.selectedID()] = !m.routing.selected[m.selectedID()]
		}
	case "F":
		m.showAnimationInfo()
	case "p":
		return m.previewTileSound(false, false)
	case "P":
		return m.previewTileSound(true, false)
	case "n":
		return m.previewTileSound(true, true)
	case keyEnter:
		m.playRouting()
	case " ":
		m.togglePreview()
	case "left", "right":
		if key == "right" {
			m.routing.distance++
		} else {
			m.routing.distance = max(0, m.routing.distance-1)
		}
		m.refreshRoutingPreviewGain()
	case "L":
		m.leaveRoutingRange()
	case "B":
		return m.editAmbientProfile()
	case "b":
		return m.editSharedAmbientProfile()
	case "ctrl+a":
		return m.openNewAmbientForm()
	case "d":
		m.focus = focusDetails
	case "U":
		return m.openRoutingUsages()
	case "o":
		return m.openRoutingMoment()
	case "u":
		return m.history(false)
	case "ctrl+r":
		return m.history(true)
	case "ctrl+s":
		return m.save()
	case "q":
		return m.quit()
	case "?":
		m.openHelp()
	case keyEscape:
		m.focus = focusBrowser
		clear(m.routing.selected)
		if m.routing.hasReturn {
			return m.returnFromRoutingJump()
		}
	}

	return nil
}
