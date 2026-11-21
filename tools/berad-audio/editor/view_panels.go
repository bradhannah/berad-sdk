package editorui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const (
	rowPrefixWidth   = 2
	filterLabelAll   = "All"
	detailIndent     = "   "
	maxListedMoments = 3
	fileHeadingRows  = 2
	editBoxSpare     = 8
)

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
			line = m.colors.dim.Render(" Nothing matches. / changes the filter; Ctrl+f shows more.")
		}
		rows = append(rows, fit(line, width))
	}

	return rows
}

func (m *model) fileLines(width int) []string {
	rows := make([]string, 0, m.listHeight()+fileHeadingRows)
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

	return rows
}

func (m *model) detailLines(width int) []string {
	switch {
	case m.volumeControls:
		return m.previewVolumeLines(width)
	case m.routing.menu != nil || m.routingWorkspace():
		return m.routingDetailLines(width)
	case m.soundscapeVisible:
		return m.soundscapeLines(width, m.layout().top-panelEdges)
	case m.selectedID() == "":
		return []string{" Nothing selected.", " / changes the filter; Ctrl+f shows more."}
	case m.detailsFull:
		return m.fullDetailLines(width)
	}

	return m.summaryLines(width)
}

// detailSheet lays out the summary: a dim heading with the keys that change the
// block at the right edge, then indented values wrapped at spaces.
type detailSheet struct {
	editor *model
	width  int
	lines  []string
}

func (sheet *detailSheet) spread(left, keys string) {
	gap := max(1, sheet.width-ansi.StringWidth(left)-ansi.StringWidth(keys)-1)
	sheet.lines = append(sheet.lines, left+strings.Repeat(" ", gap)+sheet.editor.colors.hintKey.Render(keys))
}

func (sheet *detailSheet) section(label, keys string) {
	sheet.spread(" "+sheet.editor.colors.dim.Render(label), keys)
}

func (sheet *detailSheet) keyed(value, keys string) { sheet.spread(detailIndent+safe(value), keys) }

func (sheet *detailSheet) wrapped(value string, style lipgloss.Style) {
	for _, line := range strings.Split(ansi.Wrap(safe(value), max(1, sheet.width-len(detailIndent)), ""), "\n") {
		sheet.lines = append(sheet.lines, detailIndent+style.Render(line))
	}
}

func (sheet *detailSheet) text(value string)  { sheet.wrapped(value, sheet.editor.colors.detailValue) }
func (sheet *detailSheet) quiet(value string) { sheet.wrapped(value, sheet.editor.colors.dim) }
func (sheet *detailSheet) warn(value string)  { sheet.wrapped(value, sheet.editor.colors.warning) }

// The summary answers a game designer's questions in order: where does this
// play, how loud, how long, which file. Shift+i shows every measurement.
func (m *model) summaryLines(width int) []string {
	sheet := &detailSheet{editor: m, width: width, lines: nil}
	status := m.itemStatus(m.selectedID())
	if status.State == audio.RecordingMissing || status.State == audio.RecordingInvalid {
		sheet.wrapped(status.Reason, m.colors.problem)
	}
	if m.doc.NeedsSync() {
		sheet.warn("Save not confirmed on disk. Ctrl+s retries.")
	}
	m.gameSection(sheet)
	if m.recordingID() == "" {
		return sheet.lines
	}
	m.levelSection(sheet)
	m.lengthSection(sheet)
	m.fileSection(sheet)
	sheet.section("NOTE", "N")
	if note := m.doc.Recording(m.recordingID()).Note; note != "" {
		sheet.text(note)
	} else {
		sheet.quiet(nothingAssigned)
	}

	return sheet.lines
}

func (m *model) gameSection(sheet *detailSheet) {
	sheet.section("IN GAME", "U")
	if m.browsingCues() {
		m.momentSummary(sheet)

		return
	}
	usages := m.doc.Usages(m.recordingID())
	if len(usages) == 0 {
		sheet.quiet("No moment plays this yet.")
	}
	for index, cueID := range usages {
		if index == maxListedMoments {
			sheet.quiet(fmt.Sprintf("and %d more", len(usages)-index))

			break
		}
		cue := m.doc.Cue(cueID)
		sheet.text(cue.Name + " · " + kindWord(m.cuePreviewCategory(cueID)))
		if len(usages) == 1 {
			sheet.quiet("When: " + cue.HowToHear)
		}
	}
}

func (m *model) momentSummary(sheet *detailSheet) {
	cueID := m.selectedID()
	cue := m.doc.Cue(cueID)
	sheet.text(kindWord(m.cuePreviewCategory(cueID)))
	sheet.quiet("When: " + cue.HowToHear)
	sheet.section("SOUND", "f")
	recordings := cue.Recordings()
	switch len(recordings) {
	case 0:
		sheet.quiet("None yet.")
	case 1:
		sheet.text(recordings[0])
	default:
		current := m.currentVariant[cueID]%len(recordings) + 1
		sheet.text(fmt.Sprintf("%s · clip %d of %d", m.recordingID(), current, len(recordings)))
		sheet.quiet("n plays the next · V edits the list")
	}
	target := m.doc.CueClassification(cueID).TargetAcousticFamily
	if family := m.doc.RecordingFamily(m.recordingID()); family != "" && family != target {
		sheet.warn("Filed as " + family + "; this moment expects " + target + ".")
	}
	sheet.section("VOLUME", hintGainKeys+" g")
	if m.field == fieldGain {
		sheet.lines = append(sheet.lines, m.inlineEditLines(sheet.width, "", "%", percent(cue.VolumeModifier))...)
	} else {
		sheet.text(percent(cue.VolumeModifier))
	}
}

func levelWords(recording audio.Recording) string {
	if recording.CalibrationGainDB == nil {
		return "not set · plays as recorded"
	}
	mode := "set by hand"
	if recording.CalibrationMode() == audio.CalibrationAuto {
		mode = audio.CalibrationAuto
	}

	return fmt.Sprintf("%+.1f dB · %s", *recording.CalibrationGainDB, mode)
}

func (m *model) levelSection(sheet *detailSheet) {
	recording := m.doc.Recording(m.recordingID())
	if recording.File == nil {
		return
	}
	keys := "A"
	if !m.browsingCues() {
		keys = hintGainKeys + " g A"
	}
	sheet.section("LEVEL", keys)
	if m.field == fieldCalibrationTarget || m.field == fieldGain && !m.browsingCues() {
		sheet.lines = append(sheet.lines, m.inlineEditLines(sheet.width, "", "dB", levelWords(recording))...)

		return
	}
	sheet.text(levelWords(recording))
}

func (m *model) lengthSection(sheet *detailSheet) {
	recording := m.doc.Recording(m.recordingID())
	if recording.File == nil {
		return
	}
	sheet.section("LENGTH", "i o t")
	if !m.selectedClipReady() {
		sheet.quiet("…")

		return
	}
	total := brief(m.clip.DurationMs(), m.clip.DurationMs())
	if window := recording.PlaybackWindow; window != nil {
		kept := window.EndMs - window.StartMs
		sheet.text(fmt.Sprintf("%s · %s to %s of %s", brief(kept, kept),
			clock(window.StartMs, true), clock(window.EndMs, true), total))

		return
	}
	sheet.text(total + " · whole file")
}

func (m *model) fileSection(sheet *detailSheet) {
	recording := m.doc.Recording(m.recordingID())
	keys := "f"
	if m.browsingCues() {
		keys = ""
	}
	sheet.section("FILE", keys)
	if recording.File == nil {
		sheet.quiet("No audio yet.")

		return
	}
	sheet.text(*recording.File)
	sheet.keyed("License "+recording.Provenance.License, "Ctrl+L")
}

// A value is typed where it is shown, starting from what it is now.
func (m *model) inlineEditLines(width int, label, unit, was string) []string {
	room := max(1, width-len(detailIndent)-editBoxSpare-len(label))
	box := m.colors.hintKey.Render("[ ") + m.inputText.viewport(m.input, room, m.colors.selected.Render) +
		m.colors.hintKey.Render(" ]")
	lines := []string{detailIndent + label + box + " " + unit, detailIndent + m.colors.dim.Render("was "+was),
		detailIndent + m.colors.dim.Render(m.inputGuidance())}
	if m.inputError != "" {
		lines = append(lines, detailIndent+m.colors.problem.Render("! "+safe(m.inputError)))
	}

	return lines
}

// fullDetailLines is every measurement and filing detail, behind Shift+i.
func (m *model) fullDetailLines(width int) []string {
	recording := m.doc.Recording(m.recordingID())
	status := m.itemStatus(m.selectedID())
	lines := []string{m.badge(status)}
	add := func(label, value string) {
		styled := m.colors.detailLabel.Render(label) + m.colors.detailValue.Render(safe(value))
		lines = append(lines, strings.Split(ansi.Wrap(styled, width, ""), "\n")...)
	}
	add("Every number. ", "I goes back.")
	add("", status.Reason)
	add("Recording: ", m.recordingID())
	m.publicationDetails(add)
	m.previewGainDetails(add)
	m.metadataDetails(recording, add)
	add("Acoustic family: ", m.doc.RecordingFamily(m.recordingID()))
	if recording.File != nil {
		add("Expected file: ", *recording.File)
	}
	calibration := "unreviewed (unity)"
	if recording.CalibrationGainDB != nil {
		calibration = fmt.Sprintf("%.2f dB", *recording.CalibrationGainDB)
	}
	add("Calibration: ", calibration)
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
	add("Cue gain: ", fmt.Sprintf("%.3f amplitude", cue.VolumeModifier))
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
		"Classification: " + m.doc.CueProfile(cueID),
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
