package editorui

import (
	"fmt"
	"strings"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const (
	cropLabelWidth = 9
	cropTimeWidth  = 11
)

// keyLine writes a few shortcuts for use inside a pane.
func (m *model) keyLine(hints ...keyHint) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, m.colors.hintKey.Render(hint.key)+" "+m.colors.dim.Render(hint.action))
	}

	return " " + strings.Join(parts, hintSeparator)
}

// The draft shows what the recording has now beside what it would become, so
// the change and its size are visible before anything is applied.
func (m *model) visibleCropLines(width, capacity int) []string {
	rows, focus := m.cropTable(width)
	if m.crop.err != nil {
		rows = append(rows, " "+m.colors.problem.Render("! "+
			safe(strings.TrimPrefix(m.crop.err.Error(), "invalid audio library: "))))
	}
	rows = append(rows, "", detailIndent+m.cropUses())
	if m.duplicate != nil && m.duplicate.eligibleCue != "" {
		answer := map[bool]string{true: "yes", false: "no"}[m.duplicate.appendToCue]
		moment := safe(m.doc.Cue(m.duplicate.eligibleCue).Name)
		rows = append(rows, m.keyLine(keyHint{"a", "also add to " + moment + ": " + answer}))
	}
	action := " Enter  Apply "
	if m.duplicate != nil {
		action = " c  Create "
	}
	rows = append(rows, "",
		m.keyLine(keyHint{hintArrows, "nudge"}, keyHint{"Shift", "bigger"}, keyHint{hintBoundKeys, "type a time"}),
		m.keyLine(keyHint{"p", hintPlay}, keyHint{"P", "play just the ending"}),
		m.keyLine(keyHint{"t", "trim silence"}, keyHint{"r", "whole file"}),
		"", " "+m.colors.selected.Render(action)+m.colors.dim.Render("   Esc  Discard"))
	start := listViewportStart(0, focus, len(rows), max(1, capacity))

	return rows[start:]
}

func (m *model) cropTable(width int) ([]string, int) {
	now := audio.PlaybackWindow{StartMs: 0, EndMs: m.crop.source.DurationMs()}
	if saved := m.doc.Recording(m.crop.recordingID).PlaybackWindow; saved != nil {
		now = *saved
	}
	draft := m.window()
	rows := []string{m.colors.dim.Render(cropRow("", "Now", "Draft", ""))}
	if m.duplicate != nil {
		rows = append(rows, m.cropField(0, cropRow("Name", "", safe(m.duplicate.id), "")))
	}
	bounds := m.cropFieldCount() - cropBoundCount
	focus := max(0, m.crop.field) + 1
	for index, bound := range [cropBoundCount][2]int64{{now.StartMs, draft.StartMs}, {now.EndMs, draft.EndMs}} {
		label, field := "Start", fieldStart
		if index == 1 {
			label, field = "End", fieldEnd
		}
		if m.field == field {
			focus = len(rows)
			rows = append(rows, m.inlineEditLines(width, fit(label, cropLabelWidth), "", clock(bound[0], true))...)

			continue
		}
		rows = append(rows, m.cropField(bounds+index, cropRow(label, clock(bound[0], true), clock(bound[1], true),
			cropChange(bound[1]-bound[0]))))
	}
	rows = append(rows, cropRow("Length", clock(now.EndMs-now.StartMs, true),
		clock(draft.EndMs-draft.StartMs, true), ""))

	return rows, focus
}

func cropRow(label, now, draft, change string) string {
	return " " + fit("  "+label, cropLabelWidth+columnSplit) + fit(now, cropTimeWidth) + fit(draft, cropTimeWidth) + change
}

func cropChange(milliseconds int64) string {
	if milliseconds == 0 {
		return ""
	}

	return fmt.Sprintf("%+.1f s", float64(milliseconds)/msPerSecond)
}

func (m *model) cropField(field int, row string) string {
	if m.crop.field != field {
		return row
	}

	return m.colors.selected.Render(">" + row[1:])
}

func (m *model) cropUses() string {
	uses := m.doc.Usages(m.crop.recordingID)
	switch len(uses) {
	case 0:
		return m.colors.dim.Render("No moment plays this yet.")
	case 1:
		return "Used by: " + safe(m.doc.Cue(uses[0]).Name)
	}

	return fmt.Sprintf("Used by %d moments", len(uses))
}

func (m *model) cropFooter() []string {
	const cropHintCount = 8
	hints := make([]keyHint, 0, cropHintCount)
	if m.duplicate != nil {
		hints = append(hints, keyHint{"c", "create"}, keyHint{keyEnterName, "rename"})
	} else {
		hints = append(hints, keyHint{keyEnterName, "apply"})
	}

	return m.hintRows(append(hints, keyHint{keyCancelName, "discard"}, keyHint{"p", hintPlay},
		keyHint{"P", "ending only"}, keyHint{hintArrows, "nudge"}, keyHint{hintBoundKeys, "type a time"},
		keyHint{"?", "help"}))
}

func (m *model) cropHelp() {
	m.infoDialog = &editorInfo{title: "TRIM DRAFT", scroll: 0, lines: []string{
		"Nothing changes until you Apply; Escape discards the draft.",
		"Up / Down or [ ]: pick Start or End · Left / Right: nudge 10 ms · Shift: 100 ms",
		"s / e: type a start or end time (m:ss.mmm or seconds) · i / o: mark at the playback position",
		"p: play the draft · P: play only the two seconds at the selected marker · Space: pause",
		"t: trim silence · r: back to the whole file · z: zoom · w: move the playback cursor",
		"l: loop · Shift+s: play the whole file · v: preview volumes · Ctrl+x: stop",
		"Enter: Apply · for a duplicate, c: Create, Enter: rename, a: also add it to its moment",
		"F5: reload the file and keep the draft · Ctrl+s saves afterwards",
		"", "Source: " + m.crop.source.Path(),
		fmt.Sprintf("Used by %d moment(s); every one of them changes.", len(m.doc.Usages(m.crop.recordingID))),
	}}
}
