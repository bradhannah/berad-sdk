package editorui

import (
	"fmt"
	"strings"
)

func (m *model) visibleCropLines(width, capacity int) []string {
	window := m.window()
	labels := []string{"Start", "End"}
	values := []string{fmt.Sprintf("%d ms", window.StartMs), fmt.Sprintf("%d ms", window.EndMs)}
	if m.duplicate != nil {
		labels, values = append([]string{"ID"}, labels...), append([]string{m.duplicate.id}, values...)
	}
	const contextRows = 2 // Source identity and preview state.
	rows := make([]string, 0, contextRows+len(labels)+cropActionCount)
	rows = append(rows, fit("Source: "+m.crop.recordingID, width))
	for index, label := range labels {
		line := label + ": " + values[index]
		if m.crop.field == index {
			line = m.colors.selected.Render("> " + line)
		}
		rows = append(rows, fit(line, width))
	}
	preview := "Uncalibrated (unity)"
	switch {
	case m.crop.err != nil:
		preview = "Invalid/stale: " + strings.TrimPrefix(m.crop.err.Error(), "invalid audio library: ")
	case m.crop.pending:
		preview = "Preview update pending"
	case m.crop.preview.Calibrated:
		preview = fmt.Sprintf("Draft calibration: %+.2f dB", m.crop.preview.CalibrationDB)
	}
	rows = append(rows, fit(preview, width))
	apply := fmt.Sprintf("Apply (%d cues)", m.crop.uses)
	if m.duplicate != nil {
		apply = "Create clip"
		if m.duplicate.appendToCue {
			apply += " + append"
		}
	}
	focus := max(0, m.crop.field+1)
	for index, action := range []string{apply, hintCancel} {
		line := "[ " + action + " ]"
		if m.crop.field == len(labels)+index {
			focus = len(rows)
			line = m.colors.selected.Render("> " + line)
		}
		rows = append(rows, fit(line, width))
	}
	start := listViewportStart(0, focus, len(rows), max(1, capacity))

	return rows[start:]
}

//nolint:goconst // Shortcut tables keep the rendered key names and action labels together.
func (m *model) cropFooter() []string {
	action := "Apply"
	if m.duplicate != nil {
		action = "Create"
	}
	hints := []keyHint{{"Tab/Enter", "Fields/" + action}, {"Esc", hintCancel},
		{"p/Space", "Preview/pause"}, {"[/]", "Markers"}, {"s/e", "Bounds"}, {"?", "Draft help"}}

	return m.hintRows(hints)
}

func (m *model) cropHelp() {
	uses := m.doc.Usages(m.crop.recordingID)
	m.infoDialog = &editorInfo{title: "REGION DRAFT", scroll: 0, lines: []string{
		"Source: " + m.crop.source.Path(), "Loaded fingerprint: " + m.crop.source.Fingerprint(),
		fmt.Sprintf("Shared authored recording: %d cue uses", len(uses)),
		"Adjustments change only this draft and its audition voice.",
		"Tab / Up / Down: field or Apply/Create/Cancel · Enter activates",
		"s/e: type bounds · [/]: select a marker · arrows: 10 ms · Shift+arrows: 100 ms",
		"w: seek cursor · i/o: mark/select a boundary · z: zoom · t: suggest · r: reset",
		"p: restart preview · Space: pause/resume · l: loop · Shift+s: full-source preview",
		"v: temporary monitoring levels · Ctrl+x: stop all · Escape: discard",
		"a: Apply (ordinary crop) or toggle SFX append (duplicate) · c: Create duplicate",
		"F5: explicitly reload the source and review the retained draft bounds",
		"Apply/Create verifies source content and commits one Undo entry; Ctrl+s saves afterward.",
	}}
}
