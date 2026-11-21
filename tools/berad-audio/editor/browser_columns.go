package editorui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const (
	compactStatusWidth   = 2
	extraColumnThreshold = 40
	columnGap            = 1
	columnSplit          = 2
	momentColumnGaps     = 2
	listPrefixWidth      = 3
	narrowListWidth      = 30
	mediumListWidth      = 48
	levelColumnWidth     = 10
	volumeColumnWidth    = 5
	kindColumnWidth      = 8
	playsShare           = 2
	playsParts           = 5
	editedSuffix         = " *"
	nothingAssigned      = "—"
)

type listField int

const (
	listName listField = iota
	listPlays
	listLevel
	listSound
	listVolume
	listKind
)

type listColumn struct {
	field listField
	title string
	width int
	right bool
}

// Narrow panes drop the trailing columns first; the name always stays.
func (m *model) listColumns(width int) []listColumn {
	usable := max(0, width-listPrefixWidth-columnGap)
	if m.browsingCues() {
		return momentColumns(usable)
	}

	return recordingColumns(usable)
}

func recordingColumns(usable int) []listColumn {
	name := listColumn{field: listName, title: "Sound file", width: usable, right: false}
	level := listColumn{field: listLevel, title: "Level dB", width: levelColumnWidth, right: true}
	if usable < narrowListWidth {
		return []listColumn{name}
	}
	if usable < mediumListWidth {
		name.width = usable - level.width - columnGap

		return []listColumn{name, level}
	}
	flexible := usable - level.width - columnSplit*columnGap
	plays := listColumn{field: listPlays, title: "Plays in", width: flexible * playsShare / playsParts, right: false}
	name.width = flexible - plays.width

	return []listColumn{name, plays, level}
}

func momentColumns(usable int) []listColumn {
	name := listColumn{field: listName, title: "Moment", width: usable, right: false}
	volume := listColumn{field: listVolume, title: "Vol", width: volumeColumnWidth, right: true}
	if usable < narrowListWidth {
		return []listColumn{name}
	}
	if usable < mediumListWidth {
		name.width = usable - volume.width - columnGap

		return []listColumn{name, volume}
	}
	kind := listColumn{field: listKind, title: "Kind", width: kindColumnWidth, right: false}
	flexible := usable - volume.width - kind.width - (columnSplit+1)*columnGap
	sound := listColumn{field: listSound, title: "Sound", width: flexible / columnSplit, right: false}
	name.width = flexible - sound.width

	return []listColumn{name, sound, volume, kind}
}

func rowText(columns []listColumn, cells []string) string {
	parts := make([]string, 0, len(columns))
	for index, column := range columns {
		if column.right {
			parts = append(parts, rightFit(cells[index], column.width))
		} else {
			parts = append(parts, fit(cells[index], column.width))
		}
	}

	return strings.Join(parts, strings.Repeat(" ", columnGap))
}

func (m *model) columnHeader(width int) string {
	columns := m.listColumns(width)
	titles := make([]string, 0, len(columns))
	for _, column := range columns {
		titles = append(titles, column.title)
	}

	return m.colors.dim.Render(fit(strings.Repeat(" ", listPrefixWidth)+rowText(columns, titles), width))
}

// One row: a ">" on the selection, a mark for the file's state, then the cells.
// Empty slots are dimmed so the sounds that exist stand out.
func (m *model) itemLine(identity string, selected bool, width int) string {
	status := m.itemStatus(identity)
	mark, _ := stateMark(status.State)
	columns := m.listColumns(width)
	cells := make([]string, 0, len(columns))
	for _, column := range columns {
		cells = append(cells, safe(m.listCell(identity, column.field)))
	}
	text := rowText(columns, cells)
	switch {
	case selected:
		return m.colors.selected.Render(fit(">"+mark+" "+text, width))
	case status.State == audio.RecordingNoFile:
		return m.colors.dim.Render(fit(" "+mark+" "+text, width))
	}

	return fit(" "+m.stateStyle(status.State).Render(mark)+" "+text, width)
}

func (m *model) listCell(identity string, field listField) string {
	switch field {
	case listName:
		return m.listName(identity)
	case listPlays:
		return m.playsIn(identity)
	case listLevel:
		return levelCell(m.doc.Recording(identity))
	case listSound:
		return soundCell(m.doc.Cue(identity))
	case listVolume:
		return percent(m.doc.Cue(identity).VolumeModifier)
	case listKind:
		return kindWord(m.cuePreviewCategory(identity))
	}

	panic("invalid list column")
}

// An asterisk marks an entry changed since the last save, including a moment
// whose shared recording was edited.
func (m *model) listName(identity string) string {
	name, edited := identity, m.doc.RecordingChanged(identity)
	if m.browsingCues() {
		cue := m.doc.Cue(identity)
		name, edited = cue.Name, m.doc.CueChanged(identity)
		for _, recordingID := range cue.Recordings() {
			edited = edited || m.doc.RecordingChanged(recordingID)
		}
	}
	if edited {
		name += editedSuffix
	}

	return name
}

func (m *model) playsIn(recordingID string) string {
	usages := m.doc.Usages(recordingID)
	switch len(usages) {
	case 0:
		return nothingAssigned
	case 1:
		return m.doc.Cue(usages[0]).Name
	}

	return fmt.Sprintf("%d moments", len(usages))
}

func levelCell(recording audio.Recording) string {
	if recording.CalibrationGainDB == nil {
		if recording.File == nil {
			return ""
		}

		return "not set"
	}
	mode := "set"
	if recording.CalibrationMode() == audio.CalibrationAuto {
		mode = audio.CalibrationAuto
	}

	return fmt.Sprintf("%+.1f %s", *recording.CalibrationGainDB, mode)
}

func soundCell(cue audio.Cue) string {
	recordings := cue.Recordings()
	switch len(recordings) {
	case 0:
		return nothingAssigned
	case 1:
		return recordings[0]
	}

	return fmt.Sprintf("%d clips in turn", len(recordings))
}

func kindWord(category previewCategory) string {
	switch category {
	case previewMusic:
		return categoryMusic
	case previewAmbience:
		return "ambience"
	case previewSFX:
		return "effect"
	case previewCategoryAuto, previewCategoryCount:
	}

	return ""
}

func rightFit(value string, width int) string {
	value = ansi.Truncate(value, width, "…")

	return strings.Repeat(" ", max(0, width-ansi.StringWidth(value))) + value
}
