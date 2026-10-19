package editorui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const badgeWidth = 11
const rowPrefixWidth = 2
const filterLabelAll = "All"

func (m *model) filterStrip(width int) string {
	if m.statusIsError && m.field == "" && m.form == nil {
		return m.colors.problem.Render(fit("! "+safe(m.status), width))
	}
	if m.routingWorkspace() {
		return fit(m.routingFilterTitle(), width)
	}
	labels := []string{filterLabelAll, "Existing", "Missing", "Problems"}
	counts := [filterCount]int{}
	for _, id := range m.workspaceIDs() {
		for filter := filterAll; filter < filterCount; filter++ {
			if matchesFilter(m.itemStatus(id), filter) {
				counts[filter]++
			}
		}
	}
	parts := make([]string, 0, len(labels))
	for index, label := range labels {
		text := fmt.Sprintf("[%s %d]", label, counts[index])
		if availabilityFilter(index) == m.filter {
			text = m.colors.focus.Render(text)
		} else {
			text = m.colors.dim.Render(text)
		}
		parts = append(parts, text)
	}

	return fit(strings.Join(parts, " "), width)
}

func (m *model) browserLines(width int) []string {
	if m.routing.menu != nil || m.routingWorkspace() {
		return m.routingBrowserLines(width)
	}
	if m.picker != nil {
		return m.fileLines(width)
	}
	rows := append(m.browserSearchLines(width), m.columnHeader(width))
	start := m.browserWindowStart()
	for row := range m.listHeight() {
		index := start + row
		line := ""
		if index < len(m.keys) {
			line = m.itemLine(m.keys[index], index == m.cursor, width)
		}
		if row == 0 && len(m.keys) == 0 {
			line = m.colors.warning.Render("No matches. / search or Ctrl+f filter.")
		}
		rows = append(rows, fit(line, width))
	}
	shown := min(len(m.keys), start+m.listHeight())
	first := 0
	if len(m.keys) > 0 {
		first = start + 1
	}
	legend := "A auto M manual U unset · Uses=cues"
	if m.browsingCues() {
		legend = "! family differs · ↻ cycle"
	}
	position := fmt.Sprintf("%d–%d / %d · %s", first, shown, len(m.keys), legend)
	if m.browserLabelRows() > browserHeadingRows {
		rows = append(rows, m.colors.dim.Render(fit(position, width)))
	}

	return rows
}

func (m *model) itemLine(identity string, selected bool, width int) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}
	edited := m.doc.RecordingChanged(m.itemRecording(identity))
	if m.browsingCues() {
		edited = edited || m.doc.CueChanged(identity)
		for _, id := range m.doc.Cue(identity).Recordings() {
			edited = edited || m.doc.RecordingChanged(id)
		}
	}
	if edited {
		prefix = prefix[:1] + "*"
	}
	if selected {
		prefix = m.colors.focus.Render(prefix)
	}
	columns := m.columnsForWidth(width)
	status := m.badge(m.itemStatus(identity))
	if columns.status == compactStatusWidth {
		status = ansi.Truncate(status, 1, "")
	}
	status = fit(status, columns.status)
	label := m.columnValues(identity, columns)
	if selected {
		label = m.colors.selected.Render(label)
	}

	return fit(prefix+status+label, width)
}

func (m *model) fileLines(width int) []string {
	rows := make([]string, 0, m.listHeight()+browserLabels)
	rows = append(rows, fit(safe(m.picker.directory), width))
	rows = append(rows, m.colors.detailLabel.Render(fit("File / directory", width)))
	start := listViewportStart(m.picker.start, m.picker.cursor, len(m.picker.entries), m.listHeight())
	for row := range m.listHeight() {
		index := start + row
		line := ""
		if index < len(m.picker.entries) {
			entry := m.picker.entries[index]
			line = safe(entry.name)
			if entry.directory {
				line += "/"
			}
			line = "  " + line
			if index == m.picker.cursor {
				line = m.colors.selected.Render(fit("> "+line[2:], width))
			}
		}
		rows = append(rows, fit(line, width))
	}
	if m.browserLabelRows() > browserHeadingRows {
		rows = append(rows, fit("Enter imports · p auditions · Esc cancels", width))
	}

	return rows
}

//nolint:funlen // Details are a single ordered list of labelled authoring fields.
func (m *model) detailLines(width int) []string {
	if m.volumeControls {
		return m.previewVolumeLines(width)
	}
	if m.routing.menu != nil || m.routingWorkspace() {
		return m.routingDetailLines(width)
	}
	if m.soundscapeVisible {
		return m.soundscapeLines(width, m.layout().top-panelEdges)
	}
	if m.selectedID() == "" {
		return []string{"No selected item.", "Use / to change search or Ctrl+f to change filter."}
	}
	recording := m.doc.Recording(m.recordingID())
	status := m.itemStatus(m.selectedID())
	lines := []string{m.badge(status)}
	add := func(label, value string) {
		styled := m.colors.detailLabel.Render(label) + m.colors.detailValue.Render(safe(value))
		lines = append(lines, strings.Split(ansi.Hardwrap(styled, width, true), "\n")...)
	}
	add("", status.Reason)
	if status.State == audio.RecordingNoFile || status.State == audio.RecordingMissing ||
		status.State == audio.RecordingInvalid {
		lines = append(lines, m.colors.warning.Render("f  Choose a file / repair assignment"), "")
	}
	add("Recording: ", m.recordingID())
	m.publicationDetails(add)
	m.previewGainDetails(add)
	m.metadataDetails(recording, add)
	add("Acoustic family [working]: ", m.doc.RecordingFamily(m.recordingID()))
	if recording.File != nil {
		add("Expected file: ", *recording.File)
	}
	calibration := "unreviewed (unity)"
	if recording.CalibrationGainDB != nil {
		calibration = fmt.Sprintf("%.2f dB", *recording.CalibrationGainDB)
	}
	add("Calibration [working]: ", calibration)
	m.calibrationDetails(add)
	if m.workspace == workspaceMoments {
		m.momentDetails(add)
	}
	add("Source: ", recording.Provenance.SourceCreator)
	add("License [Ctrl+L]: ", recording.Provenance.License)
	add("Original: ", recording.Provenance.OriginalFilename)
	add("Note: ", recording.Note)
	usages := m.doc.Usages(m.recordingID())
	add("Shared usages: ", fmt.Sprintf("%d (trim/calibration affects all)", len(usages)))
	for _, id := range usages {
		cue := m.doc.Cue(id)
		add("  • ", cue.Name+" ("+id+")")
		add("    Trigger: ", cue.HowToHear)
	}

	return lines
}

func (m *model) momentDetails(add func(string, string)) {
	cueID := m.activeCue()
	cue := m.doc.Cue(cueID)
	add("Moment: ", cueID)
	for _, classificationLine := range m.classificationDetailLines(cueID) {
		label, value, _ := strings.Cut(classificationLine, ": ")
		add(label+": ", value)
	}
	add("Cue gain [working]: ", fmt.Sprintf("%.3f amplitude", cue.VolumeModifier))
	if profile, exists := m.doc.Routing().Ambient[cueID]; exists {
		add("Ambient [Shift+b]: ", fmt.Sprintf("range %.1f tiles · fade %d/%d ms",
			profile.RadiusTiles, profile.FadeInMs, profile.FadeOutMs))
	}
	add("Configured sources [Shift+u]: ",
		fmt.Sprintf("%d assignments (not placed-map counts)", len(m.routingUsages(cueID))))
	if len(cue.Recordings()) > 1 {
		add("Selection: ", "n previews next; Enter restarts the current clip")
		for index, recordingID := range cue.Recordings() {
			add(fmt.Sprintf("  %d. ", index+1), recordingID)
		}
	}
}

func (m *model) classificationDetailLines(cueID string) []string {
	classification := m.doc.CueClassification(cueID)
	lines := []string{
		"Classification [working]: " + m.doc.CueProfile(cueID),
		"Intent: " + classification.TargetAcousticFamily + " · " + classification.Phase + "/" +
			classification.Outcome + " · " + classification.MixRole,
		"Emitters / contexts: " + strings.Join(classification.Emitters, ", ") + " / " +
			strings.Join(classification.Contexts, ", "),
	}
	if family := m.doc.RecordingFamily(m.recordingID()); family != classification.TargetAcousticFamily && family != "" {
		lines = append(lines, "Assignment review: recording family "+family+
			" differs from target "+classification.TargetAcousticFamily)
	}

	return lines
}

func (m *model) visibleDetails(width int) []string {
	lines := m.detailLines(width)
	if m.soundscapeVisible && !m.volumeControls {
		return lines
	}
	visible := m.layout().top - panelEdges
	start := min(m.detailsScroll, max(0, len(lines)-visible))

	return lines[start:]
}

func (m *model) scrollDetails(key string) bool {
	visible := m.layout().top - panelEdges
	var delta int
	switch key {
	case "up":
		delta = -1
	case keyDown:
		delta = 1
	case keyPageUp, keyShiftUp, keyPreviousPage:
		delta = -visible
	case keyPageDown, keyShiftDown, keyNextPage:
		delta = visible
	default:
		return false
	}
	count := len(m.detailLines(m.layout().right - panelEdges))
	m.detailsScroll = max(0, min(max(0, count-visible), m.detailsScroll+delta))

	return true
}
