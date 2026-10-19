package editorui

import (
	"fmt"
	"math"
)

type previewCategory int

type previewRoute struct {
	recordingID, cueID string
	calibrated         bool
}

const previewMusicLabel = "Music"

const (
	previewMusic previewCategory = iota
	previewSFX
	previewAmbience
	previewCategoryCount
	previewCategoryAuto  previewCategory = -1
	defaultPreviewSlider                 = 7
	defaultAmbientSlider                 = 7
	maximumPreviewSlider                 = 10
	volumeModeRow                        = 0
	volumeRecordingRow                   = 4
)

func (category previewCategory) label() string {
	switch category {
	case previewMusic:
		return previewMusicLabel
	case previewSFX:
		return "SFX"
	case previewAmbience:
		return "Ambience"
	case previewCategoryAuto:
		return "Auto"
	case previewCategoryCount:
		panic("category count is not a playback category")
	default:
		panic("unknown preview category")
	}
}

func (m *model) cuePreviewCategory(cueID string) previewCategory {
	if category, exists := m.targets.CueCategories[cueID]; exists {
		switch category {
		case categoryMusic:
			return previewMusic
		case categoryAmbient:
			return previewAmbience
		case categorySFX:
			return previewSFX
		default:
			panic("validated project has an unknown preview category")
		}
	}
	if _, ambient := m.doc.Routing().Ambient[cueID]; ambient {
		return previewAmbience
	}

	return previewSFX
}

func (m *model) categoryForPreview(recordingID, cueID string) previewCategory {
	if cueID != "" {
		return m.cuePreviewCategory(cueID)
	}
	if m.recordingCategory != previewCategoryAuto {
		return m.recordingCategory
	}
	usages := m.doc.Usages(recordingID)
	if len(usages) > 0 {
		category := m.cuePreviewCategory(usages[0])
		for _, usage := range usages[1:] {
			if m.cuePreviewCategory(usage) != category {
				return previewSFX // Mixed-use source: labelled default, override in v panel.
			}
		}

		return category
	}
	if m.options.RecordingCategory != nil {
		switch m.options.RecordingCategory(recordingID, m.doc.Recording(recordingID), m.doc.RecordingFamily(recordingID)) {
		case "music":
			return previewMusic
		case "ambient":
			return previewAmbience
		case "sfx", "":
			return previewSFX
		default:
			panic("project returned an invalid recording category")
		}
	}

	return previewSFX
}

// auditionGain shares the game's calibrated source × cue × category formula.
// Reference deliberately bypasses cue/category gains. Request and distance
// gains are unity in the editor; live game positioning/fades are not simulated.
func (m *model) auditionGain(recordingID, cueID string, calibrated bool) float64 {
	gain := 1.0
	if recording := m.doc.Recording(recordingID); calibrated && recording.CalibrationGainDB != nil {
		gain = math.Pow(decibelBase, *recording.CalibrationGainDB/decibelDivisor)
	}
	if m.referencePreview {
		return gain
	}
	if cueID != "" {
		gain *= m.doc.Cue(cueID).VolumeModifier
	}
	category := m.categoryForPreview(recordingID, cueID)

	return gain * m.sliderGain(m.previewSliders[category])
}

func (m *model) refreshPreviewVolumes() {
	if m.routingWorkspace() || m.routing.menu != nil {
		m.refreshRoutingPreviewGain()
	} else {
		m.preview.SetGain(m.livePreviewGain())
	}
	m.refreshMixGains()
}

func (m *model) livePreviewGain() float64 {
	if m.crop != nil || m.previewRoute == nil || !m.preview.Active() {
		return m.gain()
	}
	route := m.previewRoute

	return m.auditionGain(route.recordingID, route.cueID, route.calibrated)
}

func (m *model) openPreviewVolumes() {
	m.volumeControls = true
	m.volumeCursor = int(m.categoryForPreview(m.recordingID(), m.activeCue())) + 1
	m.detailsScroll = 0
}

func (m *model) previewVolumeKey(key string) {
	switch key {
	case keyEscape, keyEnter, "v":
		m.volumeControls = false
	case "up", "k":
		m.volumeCursor = max(volumeModeRow, m.volumeCursor-1)
	case keyDown, "j":
		m.volumeCursor = min(volumeRecordingRow, m.volumeCursor+1)
	case "left", "-":
		m.adjustPreviewControl(-1)
	case keyRight, "+", "=":
		m.adjustPreviewControl(1)
	case "r":
		m.referencePreview = !m.referencePreview
		m.refreshPreviewVolumes()
	}
}

func (m *model) adjustPreviewControl(delta int) {
	switch m.volumeCursor {
	case volumeModeRow:
		m.referencePreview = !m.referencePreview
	case volumeRecordingRow:
		m.recordingCategory = max(previewCategoryAuto, min(previewAmbience, m.recordingCategory+previewCategory(delta)))
	default:
		category := previewCategory(m.volumeCursor - 1)
		m.previewSliders[category] = max(0, min(maximumPreviewSlider, m.previewSliders[category]+delta))
	}
	m.refreshPreviewVolumes()
}

func (m *model) previewModeLabel() string {
	if m.referencePreview {
		return "REFERENCE"
	}

	return m.options.MixLabel
}

func (m *model) previewRouteLabel(recordingID, cueID string) string {
	if m.referencePreview {
		return "Reference"
	}
	category := m.categoryForPreview(recordingID, cueID)

	return fmt.Sprintf("%s %d/10", category.label(), m.previewSliders[category])
}

func (m *model) previewVolumeStrip(width int) string {
	if m.referencePreview {
		return fit(m.colors.focus.Render("REFERENCE · calibrated source ×1 · v controls"), width)
	}

	return fit(m.colors.focus.Render(fmt.Sprintf("%s · Music %d/10 · SFX %d/10 · Amb %d/10 · v controls",
		m.options.MixLabel, m.previewSliders[previewMusic], m.previewSliders[previewSFX],
		m.previewSliders[previewAmbience])), width)
}

func (m *model) previewVolumeLines(width int) []string {
	values := []string{"Mode: " + m.previewModeLabel()}
	for category := previewMusic; category < previewCategoryCount; category++ {
		slider := m.previewSliders[category]
		values = append(values, fmt.Sprintf("%s %d/10 · ×%.2f",
			category.label(), slider, m.sliderGain(slider)))
	}
	values = append(values, "Recording bus: "+m.recordingCategory.label())
	for index, value := range values {
		prefix := "  "
		if index == m.volumeCursor {
			prefix = "> "
			values[index] = m.colors.selected.Render(fit(prefix+value, width))
		} else {
			values[index] = fit(prefix+value, width)
		}
	}

	return append(values, "", fit("Temporary · not saved", width), fit("r toggles Reference", width))
}

func (m *model) previewGainDetails(add func(string, string)) {
	add("Preview mode: ", m.previewModeLabel()+" (temporary)")
	if m.preview.Active() && m.previewRoute != nil && m.previewRoute.cueID != m.activeCue() {
		add("Playing mix route: ", m.previewRoute.cueID+" · Enter restarts the selected mix")
	}
	if m.picker != nil {
		add("Candidate preview: ", "Uncalibrated file; existing recording calibration is bypassed.")
	}
	if !m.referencePreview {
		category := m.categoryForPreview(m.recordingID(), m.activeCue())
		slider := m.previewSliders[category]
		add("Preview category: ", fmt.Sprintf("%s %d/10 = ×%.2f",
			category.label(), slider, m.sliderGain(slider)))
		if m.activeCue() == "" {
			add("Recording audition: ", "No cue gain; Auto uses SFX for mixed-category sources (v overrides).")
		}
		add("Preview conditions: ", "Request/distance gain ×1; no game fades or suppression.")
	} else {
		add("Reference level: ", "Calibration only; cue and category gains bypassed.")
	}
	add("Preview gain: ", fmt.Sprintf("×%.3f", m.gain()))
}
