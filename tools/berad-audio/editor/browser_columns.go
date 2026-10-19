package editorui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const (
	compactStatusWidth     = 2
	fullStatusThreshold    = 70
	fullFamilyThreshold    = 90
	extraColumnThreshold   = 40
	compactFamilyWidth     = 10
	fullFamilyWidth        = 24
	calibrationColumnWidth = 8
	usesColumnWidth        = 4
	columnGap              = 1
	columnSplit            = 2
	momentColumnGaps       = 2
	recordingColumnGaps    = 3
	mismatchMarkerWidth    = 2
	targetColumnTitle      = "Target"
)

type browserColumns struct {
	status, name, family, calibration, uses, assignment int
}

func (m *model) columnsForWidth(width int) browserColumns {
	var columns browserColumns
	columns.status, columns.family = compactStatusWidth, compactFamilyWidth
	if width >= fullStatusThreshold {
		columns.status = badgeWidth
	}
	if width >= fullFamilyThreshold {
		columns.family = fullFamilyWidth
	}
	remaining := max(0, width-rowPrefixWidth-columns.status)
	if width < extraColumnThreshold {
		columns.family = min(columns.family, remaining/columnSplit)
		columns.name = max(0, remaining-columns.family-columnGap)

		return columns
	}
	if m.browsingCues() {
		columns.family += mismatchMarkerWidth
		remaining -= columns.family + momentColumnGaps*columnGap
		columns.assignment = remaining / columnSplit
		columns.name = remaining - columns.assignment
	} else {
		columns.calibration, columns.uses = calibrationColumnWidth, usesColumnWidth
		columns.name = remaining - columns.family - columns.calibration - columns.uses - recordingColumnGaps*columnGap
	}

	return columns
}

func (m *model) columnHeader(width int) string {
	columns := m.columnsForWidth(width)
	state, name, family := "State", "Recording ID", "Family"
	if columns.status == compactStatusWidth {
		state = "S"
	}
	if m.browsingCues() {
		name, family = "Moment", targetColumnTitle
	}
	parts := []string{fit(name, columns.name), fit(family, columns.family)}
	if columns.calibration > 0 {
		parts = append(parts, rightFit("Cal dB", columns.calibration), rightFit("Uses", columns.uses))
	}
	if columns.assignment > 0 {
		parts = append(parts, fit("Assignment", columns.assignment))
	}

	return m.colors.detailLabel.Render(fit(strings.Repeat(" ", rowPrefixWidth)+
		fit(state, columns.status)+strings.Join(parts, " "), width))
}

func familyDisplayName(family string, width int) string {
	if width < fullFamilyWidth {
		switch family {
		case "mechanical_interaction":
			return "mechanical"
		case "special_event":
			return "special"
		}
	}

	return family
}

func calibrationCell(recording audio.Recording) string {
	if recording.CalibrationGainDB == nil {
		return "— U"
	}
	mode := "M"
	if recording.CalibrationMode() == audio.CalibrationAuto {
		mode = "A"
	}

	return fmt.Sprintf("%+.1f %s", *recording.CalibrationGainDB, mode)
}

func (m *model) momentFamilyMismatch(cueID string) bool {
	classification := m.doc.CueClassification(cueID)
	for _, recordingID := range m.doc.Cue(cueID).Recordings() {
		if m.doc.RecordingFamily(recordingID) != classification.TargetAcousticFamily {
			return true
		}
	}

	return false
}

func (m *model) columnValues(identity string, columns browserColumns) string {
	if m.browsingCues() {
		cue := m.doc.Cue(identity)
		family := familyDisplayName(m.doc.CueClassification(identity).TargetAcousticFamily, columns.family)
		familyCell := fit(family, columns.family)
		if m.momentFamilyMismatch(identity) {
			familyCell = m.colors.warning.Render(fit(family, columns.family-mismatchMarkerWidth) + " !")
		}
		parts := []string{fit(safe(cue.Name), columns.name), familyCell}
		if columns.assignment > 0 {
			assignment := cue.PrimaryRecording()
			if len(cue.Recordings()) > 1 {
				assignment = fmt.Sprintf("%d clips ↻", len(cue.Recordings()))
			}
			parts = append(parts, fit(safe(assignment), columns.assignment))
		}

		return strings.Join(parts, " ")
	}
	parts := []string{fit(safe(identity), columns.name),
		fit(familyDisplayName(m.doc.RecordingFamily(identity), columns.family), columns.family)}
	if columns.calibration > 0 {
		parts = append(parts, rightFit(calibrationCell(m.doc.Recording(identity)), columns.calibration),
			rightFit(strconv.Itoa(len(m.doc.Usages(identity))), columns.uses))
	}

	return strings.Join(parts, " ")
}

func rightFit(value string, width int) string {
	value = ansi.Truncate(value, width, "…")

	return strings.Repeat(" ", max(0, width-ansi.StringWidth(value))) + value
}
