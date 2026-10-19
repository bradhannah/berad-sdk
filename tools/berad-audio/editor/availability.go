package editorui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func (m *model) updateSelectedAvailability(message loadedMsg) {
	if message.request.purpose == sourceCandidate || errors.Is(message.err, context.Canceled) ||
		errors.Is(message.err, context.DeadlineExceeded) {
		return
	}
	recordingID := message.request.recordingID
	if recordingID == "" {
		return
	}
	status := m.doc.FileAvailability(recordingID)
	if status.State == audio.RecordingChecking {
		failure := m.checkSelectedStream(message)
		if failure != nil {
			status.State, status.Reason = audio.RecordingInvalid, failure.Error()
		} else {
			status.State, status.Reason = audio.RecordingReady, "Playable; listening review is separate."
		}
	}
	m.availability[recordingID] = status
}

func (m *model) checkSelectedStream(message loadedMsg) error {
	if message.err != nil || message.clip == nil {
		return message.err
	}
	stream, err := message.clip.Stream(m.doc.Recording(message.request.recordingID).PlaybackWindow, false)
	if err != nil {
		return fmt.Errorf("check selected stream: %w", err)
	}
	if err := stream.Close(); err != nil {
		return fmt.Errorf("close selected stream check: %w", err)
	}

	return nil
}

type availabilityFilter int

const (
	filterAll availabilityFilter = iota
	filterExisting
	filterMissing
	filterProblems
	filterCount
)

type browseState struct {
	search        string
	filter        availabilityFilter
	selected      string
	start         int
	routingFilter int
	routingScope  int
}

type availabilityMsg struct {
	generation    uint64
	id, signature string
	status        audio.RecordingAvailability
}

func sourceSignature(doc *audio.LibraryEditor, id string) string {
	recording := doc.Recording(id)
	if recording.File == nil {
		return ""
	}
	if recording.PlaybackWindow == nil {
		return *recording.File
	}

	return fmt.Sprintf("%s:%d:%d", *recording.File, recording.PlaybackWindow.StartMs, recording.PlaybackWindow.EndMs)
}

// File presence is cheap to discover; decoding runs one recording at a time,
// with cancellation and source fingerprints to reject stale scan results.
func (m *model) prepareAvailability() {
	if m.cancelScan != nil {
		m.cancelScan()
	}
	m.scanGeneration++
	m.availability = make(map[string]audio.RecordingAvailability)
	m.scanQueue = nil
	for _, id := range m.doc.RecordingIDs() {
		status := m.doc.FileAvailability(id)
		m.availability[id] = status
		if status.State == audio.RecordingChecking {
			m.scanQueue = append(m.scanQueue, id)
		}
	}
}

func (m *model) scanNext() tea.Cmd {
	if len(m.scanQueue) == 0 {
		return nil
	}
	recordingID := m.scanQueue[0]
	m.scanQueue = m.scanQueue[1:]
	document, generation := m.doc.Clone(), m.scanGeneration
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelScan = cancel

	return func() tea.Msg {
		defer cancel()

		return availabilityMsg{generation: generation, id: recordingID, signature: sourceSignature(document, recordingID),
			status: document.InspectRecording(ctx, recordingID)}
	}
}

func (m *model) acceptAvailability(message availabilityMsg) tea.Cmd {
	if message.generation != m.scanGeneration {
		return nil
	}
	if message.signature == sourceSignature(m.doc, message.id) {
		selected := m.selectedID()
		m.availability[message.id] = message.status
		if m.selectionOwned() {
			return m.scanNext() // Do not change the target beneath an active editor or picker.
		}
		m.refreshKeys()
		if selected != m.selectedID() {
			return tea.Batch(m.scanNext(), m.loadSelection())
		}
	}

	return m.scanNext()
}

func (m *model) selectionOwned() bool {
	return m.inputOwner() != ownerWorkspace || m.marker != 0 || m.focus == focusWave
}

func (m *model) recheckFiles() tea.Cmd {
	m.prepareAvailability()
	m.refreshKeys()
	m.status = "Rechecking source files; green means playable, not listening-approved."

	return tea.Batch(m.scanNext(), m.loadSelection())
}

func (m *model) itemRecording(id string) string {
	if m.browsingCues() {
		return m.doc.Cue(id).PrimaryRecording()
	}

	return id
}

func (m *model) itemStatus(id string) audio.RecordingAvailability {
	if m.browsingCues() {
		return m.cueAvailability(id)
	}
	status, exists := m.availability[m.itemRecording(id)]
	if !exists {
		return audio.RecordingAvailability{State: audio.RecordingNoFile, Exists: false, Reason: "No file assigned."}
	}

	return status
}

func matchesFilter(status audio.RecordingAvailability, filter availabilityFilter) bool {
	switch filter {
	case filterAll:
		return true
	case filterExisting:
		return status.Exists
	case filterMissing:
		return status.State == audio.RecordingNoFile || status.State == audio.RecordingMissing
	case filterProblems:
		return status.State == audio.RecordingInvalid || status.State == audio.RecordingMissing
	case filterCount:
		return false
	}

	return false
}

func (m *model) workspaceIDs() []string {
	if m.routingWorkspace() {
		return m.routingIDs()
	}
	if m.variantCue != "" && !m.assigning {
		return m.doc.Cue(m.variantCue).Recordings()
	}
	if m.browsingCues() {
		return m.doc.CueIDs()
	}

	return m.doc.RecordingIDs()
}

func (m *model) refreshKeys() {
	if m.routingWorkspace() {
		m.refreshRoutingKeys()

		return
	}
	selected := m.selectedID()
	m.keys = nil
	for _, identity := range m.workspaceIDs() {
		label := m.doc.SearchText(identity, m.browsingCues())
		matchesSearch := m.matchesSearch(identity, label)
		if matchesSearch && matchesFilter(m.itemStatus(identity), m.filter) {
			m.keys = append(m.keys, identity)
		}
	}
	m.cursor = max(0, min(m.cursor, len(m.keys)-1))
	m.selectID(selected)
	m.browserStart = m.browserWindowStart()
}

// Bare words retain substring search; keyed terms filter authored classifications.
func (m *model) matchesSearch(identity, label string) bool {
	for _, term := range strings.Fields(strings.ToLower(m.search)) {
		key, value, facet := strings.Cut(term, ":")
		if !facet {
			if !strings.Contains(strings.ToLower(label), term) {
				return false
			}

			continue
		}
		if !m.matchesFacet(identity, key, value) {
			return false
		}
	}

	return true
}

func (m *model) matchesFacet(identity, key, value string) bool {
	usages := []string{identity}
	if !m.browsingCues() {
		usages = m.doc.Usages(identity)
	}
	if key == "category" && value == "unassigned" && len(usages) == 0 {
		return true
	}
	if key == "family" && !m.browsingCues() && m.doc.RecordingFamily(identity) == value {
		return true
	}
	for _, cueID := range usages {
		if m.matchesCueFacet(cueID, key, value) {
			return true
		}
	}

	return false
}

func (m *model) matchesCueFacet(cueID, key, value string) bool {
	classification := m.doc.CueClassification(cueID)
	category := categorySFX
	switch m.cuePreviewCategory(cueID) {
	case previewMusic:
		category = categoryMusic
	case previewAmbience:
		category = categoryAmbient
	case previewSFX, previewCategoryCount, previewCategoryAuto:
		// Preserve the default SFX facet for non-music/non-ambient values.
	}
	switch key {
	case "category":
		return category == value
	case "family":
		return classification.TargetAcousticFamily == value
	case "phase":
		return classification.Phase == value
	case "outcome":
		return classification.Outcome == value
	case "role":
		return classification.MixRole == value
	case "emitter":
		return slices.Contains(classification.Emitters, value)
	case "context":
		return slices.Contains(classification.Contexts, value)
	default:
		return false
	}
}

func (m *model) currentBrowseState() browseState {
	return browseState{search: m.search, filter: m.filter, selected: m.selectedID(), start: m.browserWindowStart(),
		routingFilter: m.routing.filter, routingScope: m.routing.scope}
}

func (m *model) restoreBrowseState(state browseState) {
	m.search, m.filter, m.cursor = state.search, state.filter, 0
	m.routing.filter, m.routing.scope = state.routingFilter, state.routingScope
	m.refreshKeys()
	m.selectID(state.selected)
	m.browserStart = listViewportStart(state.start, m.cursor, len(m.keys), m.listHeight())
}

func (m *model) switchWorkspace(moments bool) tea.Cmd {
	if moments {
		return m.selectWorkspace(workspaceMoments)
	}

	return m.selectWorkspace(workspaceRecordings)
}
