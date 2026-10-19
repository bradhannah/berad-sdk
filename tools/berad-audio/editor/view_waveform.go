package editorui

import (
	"fmt"
	"math"
	"strings"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const waveLabelRows = 3
const zoomRadiusMs = 250
const waveformFloorDB = -72.0
const waveformDBMultiplier = 20.0
const waveformCellSteps = 8

func (m *model) waveform(width, height int) string {
	inside := width - panelEdges
	if m.loading {
		return m.panel("WAVEFORM · analyzing", []string{"Decoding in background; browsing remains responsive."},
			width, height, m.panelFocused(focusWave))
	}
	if m.clip == nil {
		return m.panel("WAVEFORM · no playable source", []string{
			"Choose a recording with an audio file. Missing items can be repaired with f."},
			width, height, m.panelFocused(focusWave))
	}
	window := m.window()
	from, until := int64(0), m.clip.DurationMs()
	if m.zoom {
		center := m.position
		if m.marker == markerStart {
			center = window.StartMs
		}
		if m.marker == markerEnd {
			center = window.EndMs
		}
		from, until = max(0, center-zoomRadiusMs), min(until, center+zoomRadiusMs)
	}
	mode := "selection"
	if m.fullSource {
		mode = "full source"
	}
	state := "stopped"
	if m.previewVoice().Playing() {
		state = "playing"
	} else if m.previewVoice().Active() {
		state = "paused/ended"
	}
	focus := "source cursor"
	if m.previewVoice().Active() {
		focus = "preview selection"
	}
	if m.marker == markerStart {
		focus = "START marker"
	}
	if m.marker == markerEnd {
		focus = "END marker"
	}
	if m.crop != nil && m.crop.field >= m.cropFieldCount() {
		focus = "Apply/Cancel"
	}
	title := fmt.Sprintf("WAVEFORM · %s · %s · arrows: %s (10 ms)", mode, state, focus)
	label := fmt.Sprintf("Start %d · End %d ms · Selected %d ms · Position %.3fs", window.StartMs, window.EndMs,
		window.EndMs-window.StartMs, float64(m.position)/msPerSecond)
	lines := []string{fit(label, inside)}
	rows := height - panelEdges - waveLabelRows
	if rows > 0 {
		lines = append(lines, m.overview(window, from, until, inside))
		lines = append(lines, m.drawWaveform(window, m.position, from, until, inside, rows)...)
	}
	lines = append(lines, fit(fmt.Sprintf("%.3fs — %.3fs · %s · source dB view −72…0 (display only)",
		float64(from)/msPerSecond, float64(until)/msPerSecond, zoomLabel(m.zoom)), inside))

	return m.panel(title, lines, width, height, m.panelFocused(focusWave))
}

func zoomLabel(zoom bool) string {
	if zoom {
		return "ZOOM · follows arrow target"
	}

	return "FULL"
}

func (m *model) overview(window audio.PlaybackWindow, from, until int64, width int) string {
	var line strings.Builder
	for column := range width {
		position := int64(column) * max(1, m.clip.DurationMs()) / int64(width)
		glyph, style := "─", m.colors.dim
		if position >= window.StartMs && position < window.EndMs {
			glyph, style = "━", m.colors.ready
		}
		if m.zoom && position >= from && position < until {
			glyph, style = "═", m.colors.focus
		}
		_, _ = line.WriteString(style.Render(glyph))
	}

	return line.String()
}

func (m *model) drawWaveform(window audio.PlaybackWindow, position, from, until int64, width, height int) []string {
	rows := make([]string, height)
	span := max(int64(1), until-from)
	for column := range width {
		start := from + int64(column)*span/int64(width)
		end := from + int64(column+1)*span/int64(width)
		peak := m.clip.PeakBetween(start, end)
		units := waveformDisplayUnits(peak, height)
		for row := range height {
			glyph := waveformCell(units, row, height)
			style := m.colors.dim
			if start >= window.StartMs && start < window.EndMs {
				style = m.colors.focus
			}
			if position >= start && position < max(start+1, end) {
				glyph, style = "│", m.colors.warning
			}
			if window.StartMs >= start && window.StartMs < max(start+1, end) {
				glyph, style = "[", m.colors.warning
			}
			if window.EndMs >= start && window.EndMs < max(start+1, end) || column == width-1 && window.EndMs == until {
				glyph, style = "]", m.colors.warning
			}
			rows[row] += style.Render(glyph)
		}
	}

	return rows
}

// Logarithmic display makes quiet onsets/tails visible without changing the
// source peaks, audio gain, or silence-suggestion threshold. Any nonzero signal
// receives at least one eighth of a terminal cell; digital zero stays flat.
func waveformDisplayUnits(peak float64, height int) int {
	if peak <= 0 || math.IsNaN(peak) || height <= 0 {
		return 0
	}
	normalized := math.Max(0, math.Min(1, (waveformDBMultiplier*math.Log10(peak)-waveformFloorDB)/-waveformFloorDB))

	return max(1, int(math.Ceil(normalized*float64(height*waveformCellSteps))))
}

func waveformCell(units, row, height int) string {
	filled := max(0, min(waveformCellSteps, units-(height-row-1)*waveformCellSteps))
	if filled == 0 {
		if row == height-1 {
			return "─"
		}

		return " "
	}
	glyphs := [waveformCellSteps]rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	return string(glyphs[filled-1])
}
