package editorui

import (
	"fmt"
	"strings"
)

func (slot soundscapeSlot) label() string {
	return [...]string{"Ambient 1", "Ambient 2", "Music", "SFX"}[slot]
}

func (slot soundscapeSlot) shortLabel() string {
	return [...]string{"A1", "A2", "Mus", "FX"}[slot]
}

func (m *model) layerName(layer *mixLayer) string {
	if layer == nil {
		return "Empty"
	}
	if layer.cueID != "" {
		return m.doc.Cue(layer.cueID).Name
	}

	return layer.recordingID
}

func (m *model) layerState(layer *mixLayer) (string, string) {
	switch {
	case layer == nil:
		return "—", "EMPTY"
	case layer.stopped || !layer.voice.Active():
		return "■", "STOPPED"
	case layer.paused:
		return "Ⅱ", "PAUSED"
	case !layer.voice.Playing():
		return "✓", "FINISHED"
	case m.layerGain(layer) == 0:
		return "○", "MUTED"
	default:
		return "▶", "PLAYING"
	}
}

func (m *model) soundscapeLines(width, height int) []string {
	var lines []string
	count := int(soundscapeSlotCount)
	if m.pendingAmbient != nil {
		lines = append(lines, fit("Replace with: "+m.layerName(m.pendingAmbient), width),
			m.colors.warning.Render(fit("1/2 chooses · Enter replaces", width)))
		count = int(ambientTwo) + 1
	}
	available := max(1, height-len(lines))
	rowsPerSlot := 1
	if available >= count*2 {
		rowsPerSlot = 2
	}
	visible := max(1, available/rowsPerSlot)
	first := soundscapeSlot(listViewportStart(0, min(m.mixCursor, count-1), count, visible))
	for slot := first; int(slot) < min(count, int(first)+visible); slot++ {
		layer := m.layerAt(slot)
		icon, state := m.layerState(layer)
		label := slot.shortLabel()
		if rowsPerSlot > 1 {
			label = slot.label()
		}
		prefix := "  "
		if m.focus == focusMix && int(slot) == m.mixCursor {
			prefix = "> "
		}
		line := fit(fmt.Sprintf("%s%s %s %s", prefix, label, icon, m.layerName(layer)), width)
		if rowsPerSlot == 1 && layer != nil {
			mode := "1×"
			if layer.mode != modeOnce {
				mode = "↻"
			}
			line = fit(fmt.Sprintf("%s%s %s %s %s", prefix, label, icon, mode, m.layerName(layer)), width)
		}
		if prefix == "> " {
			line = m.colors.selected.Render(line)
		}
		lines = append(lines, line)
		if rowsPerSlot > 1 {
			description := m.soundscapeSlotDescription(slot, layer, state)
			lines = append(lines, m.colors.dim.Render(fit(description, width)))
		}
	}
	if len(lines) < height && m.pendingAmbient == nil {
		lines = append(lines, fit("Esc browses · d shows Details", width))
	}

	return lines
}

func (m *model) soundscapeSlotDescription(slot soundscapeSlot, layer *mixLayer, state string) string {
	if layer == nil {
		switch slot {
		case sfxSlot:
			return "Browse SFX; Enter fires over the beds"
		case musicSlot:
			return "a sets music · plays once by default"
		case ambientOne, ambientTwo:
			return "a adds ambience · loops by default"
		case soundscapeSlotCount:
			panic("invalid soundscape slot")
		}
	}
	description := fmt.Sprintf("%s · %s · %s · ×%.2f", state, strings.ToUpper(layer.mode),
		m.previewRouteLabel(layer.recordingID, layer.cueID), layer.gain)
	if layer.slot == musicSlot && layer.mode == modeLoop {
		description = "LOOP (audition only) · " + state
	}
	if layer.solo {
		description += " · SOLO"
	}
	if layer.fullSource {
		description += " · FULL SOURCE"
	}

	return description
}

func (m *model) soundscapeStatus(width int) string {
	if len(m.mix) == 0 {
		if m.preview.Playing() {
			return fit("PREVIEW ▶ "+m.previewLabel+" · a pins it · m opens Soundscape", width)
		}

		return fit(m.colors.dim.Render("SOUNDSCAPE · empty · a adds sound · m opens slots"), width)
	}
	const prefix = "SOUNDSCAPE "
	const separator = " | "
	count := int(soundscapeSlotCount)
	if m.preview.Playing() {
		count++
	}
	cellWidth := max(1, (width-len(prefix)-(count-1)*len(separator))/count)
	parts := make([]string, 0, soundscapeSlotCount)
	for slot := ambientOne; slot < soundscapeSlotCount; slot++ {
		layer := m.layerAt(slot)
		icon, _ := m.layerState(layer)
		name := strings.TrimPrefix(m.layerName(layer), "Nearby ")
		parts = append(parts, fit(slot.shortLabel()+" "+icon+" "+name, cellWidth))
	}
	if m.preview.Playing() {
		parts = append(parts, fit("P ▶ "+m.previewLabel, cellWidth))
	}

	return fit(prefix+strings.Join(parts, separator), width)
}
