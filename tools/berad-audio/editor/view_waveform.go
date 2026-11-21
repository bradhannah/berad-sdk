package editorui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const (
	zoomRadiusMs         = 250
	closeUpRadiusMs      = 2000
	closeUpWidth         = 38
	closeUpMinimumWidth  = 100
	waveformFloorDB      = -48.0
	waveformDBMultiplier = 20.0
	waveformCellSteps    = 8
	waveSlices           = 8
	rulerLabelSpacing    = 14
	rulerPreciseSpanMs   = 10 * msPerSecond
)

func (m *model) waveform(width, height int) string {
	title := m.colors.title.Render("WAVEFORM")
	if m.loading {
		return m.panel(title, []string{m.colors.dim.Render(" Loading…")}, width, height, m.panelFocused(focusWave))
	}
	if m.clip == nil {
		return m.panel(title, []string{m.colors.dim.Render(" No audio to show.")}, width, height,
			m.panelFocused(focusWave))
	}
	// While a trim marker is selected its close-up sits beside the whole file.
	if m.crop != nil && m.marker != 0 && width >= closeUpMinimumWidth {
		overview := m.wavePane(title+m.colors.dim.Render("  dim = cut off"), 0, m.clip.DurationMs(),
			width-closeUpWidth-paneGap, height, false)

		return lipgloss.JoinHorizontal(lipgloss.Top, overview, strings.Repeat(" ", paneGap), m.closeUpPane(height))
	}
	from, until := m.waveRange()

	return m.wavePane(title+m.colors.dim.Render(m.waveNote()), from, until, width, height, m.panelFocused(focusWave))
}

func (m *model) waveNote() string {
	switch {
	case m.zoom:
		return "  zoomed · z shows it all"
	case m.fullSource:
		return "  playing the whole file"
	case m.crop != nil:
		return "  dim = cut off"
	}

	return ""
}

func (m *model) waveRange() (int64, int64) {
	from, until := int64(0), m.clip.DurationMs()
	if !m.zoom {
		return from, until
	}
	center := m.markerPosition(m.position)

	return max(0, center-zoomRadiusMs), min(until, center+zoomRadiusMs)
}

func (m *model) markerPosition(fallback int64) int64 {
	switch m.marker {
	case markerStart:
		return m.window().StartMs
	case markerEnd:
		return m.window().EndMs
	}

	return fallback
}

func (m *model) closeUpPane(height int) string {
	name := "END"
	if m.marker == markerStart {
		name = "START"
	}
	center := m.markerPosition(0)
	from, until := max(0, center-closeUpRadiusMs), min(m.clip.DurationMs(), center+closeUpRadiusMs)

	title := m.colors.title.Render(name) + m.colors.dim.Render("  close-up")

	return m.wavePane(title, from, until, closeUpWidth, height, false)
}

func (m *model) wavePane(title string, from, until int64, width, height int, active bool) string {
	inside := width - panelEdges
	lines := m.drawWaveform(m.window(), m.position, from, until, inside, height-panelEdges-1)
	lines = append(lines, m.colors.dim.Render(timeRuler(from, until, inside)))

	return m.panel(title, lines, width, height, active)
}

func (m *model) drawWaveform(window audio.PlaybackWindow, position, from, until int64, width, height int) []string {
	rows := make([]string, height)
	span := max(int64(1), until-from)
	for column := range width {
		start := from + int64(column)*span/int64(width)
		end := from + int64(column+1)*span/int64(width)
		units := waveformDisplayUnits(m.columnLevel(start, end), height)
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

// columnLevel averages a few peaks across the column. One loudest instant per
// column makes sustained music a solid block; the average shows its shape, and
// a lone click in silence still registers.
func (m *model) columnLevel(start, end int64) float64 {
	span := end - start
	if span < waveSlices {
		return m.clip.PeakBetween(start, end)
	}
	total := 0.0
	for slice := range int64(waveSlices) {
		total += m.clip.PeakBetween(start+slice*span/waveSlices, start+(slice+1)*span/waveSlices)
	}

	return total / waveSlices
}

// The scale starts at -48 dB so music has a shape instead of filling the pane;
// a close-up still shows quiet tails. Any nonzero signal gets at least one
// eighth of a cell and digital zero stays flat. Display only.
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

// timeRuler labels round times under the waveform and the end at the right.
func timeRuler(from, until int64, width int) string {
	span := max(int64(1), until-from)
	precise := span < rulerPreciseSpanMs
	line := []byte(strings.Repeat(" ", max(0, width)))
	last := clock(until, precise)
	limit := width - len(last) - 1
	free := 0
	step := rulerStep(span, width)
	for tick := (from + step - 1) / step * step; tick < until; tick += step {
		label := clock(tick, precise)
		column := int((tick - from) * int64(width) / span)
		if column >= free && column+len(label) <= limit {
			copy(line[column:], label)
			free = column + len(label) + 1
		}
	}
	if limit >= 0 {
		copy(line[width-len(last):], last)
	}

	return string(line)
}

// rulerStep walks 0.1, 0.2, 0.5, 1, 2, 5… seconds until labels have room.
func rulerStep(span int64, width int) int64 {
	const firstStepMs, stepCycle, double, half = 100, 3, 2, 5
	step := int64(firstStepMs)
	for index := 0; step*int64(max(1, width))/span < rulerLabelSpacing; index++ {
		if index%stepCycle == 1 {
			step = step * half / double
		} else {
			step *= double
		}
	}

	return step
}
