package editorui

import (
	"fmt"
	"math"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const (
	distanceModeField   = 0
	tileDistanceField   = 1
	levelModeField      = 2
	tileLevelField      = 3
	useSoundDefault     = "Sound default"
	useTileOverride     = "Tile override"
	levelPercentScale   = 100
	maxTileLevelPercent = 200
	tileLevelStep       = 5
)

func (m *model) editTileAmbient() tea.Cmd {
	routing := m.doc.Routing()
	identity := m.selectedID()
	assignment := routing.Tiles[identity]
	if assignment.AmbientCue == "" {
		m.status = "Choose an ambient sound first, then set this tile family's distance and intensity."

		return nil
	}
	profile, level := routing.TileAmbientSettings(identity)
	distanceMode, levelMode := useSoundDefault, useSoundDefault
	if assignment.RadiusTiles != nil {
		distanceMode = useTileOverride
	}
	if assignment.Gain != nil {
		levelMode = useTileOverride
	}
	defaultRadius := routing.Ambient[assignment.AmbientCue].RadiusTiles
	m.form = &editorForm{ //nolint:exhaustruct_v5 // Detached per-family source draft.
		kind: formTileAmbient, title: "TILE SOUND — " + m.audioTile(identity).Name,
		cue: assignment.AmbientCue, tileID: identity, errorField: -1,
		fields: []formField{
			{label: "Distance source", value: distanceMode, help: "Left/Right switches default or per-tile override.",
				unit: "", step: 0, options: []string{useSoundDefault, useTileOverride}},
			{label: "Hearing distance", value: fmt.Sprintf("%g", profile.RadiusTiles),
				help: fmt.Sprintf("Sound default: %g tiles. Editing overrides it for this family (greater than 0, up to 32).",
					defaultRadius),
				unit: unitTiles, step: 1, options: nil},
			{label: "Level source", value: levelMode, help: "Default uses the sound's mix level without any tile adjustment.",
				unit: "", step: 0, options: []string{useSoundDefault, useTileOverride}},
			{label: "Relative level", value: fmt.Sprintf("%g", level*levelPercentScale),
				help: "100% = normal; 50% = quieter; 150% = louder. Range 0–200%.",
				unit: "%", step: tileLevelStep, options: nil},
		},
	}

	return m.loadSelection()
}

func (m *model) updateTileOverrideMode() {
	form := m.form
	if form.kind != formTileAmbient {
		return
	}
	switch form.cursor {
	case tileDistanceField:
		form.fields[distanceModeField].value = useTileOverride
	case tileLevelField:
		form.fields[levelModeField].value = useTileOverride
	case distanceModeField:
		if form.fields[distanceModeField].value == useSoundDefault {
			form.fields[tileDistanceField].value = fmt.Sprintf("%g", m.doc.Routing().Ambient[form.cue].RadiusTiles)
		}
	case levelModeField:
		if form.fields[levelModeField].value == useSoundDefault {
			form.fields[tileLevelField].value = "100"
		}
	}
}

func (m *model) tileAmbientDraft() (audio.TileAudio, bool) {
	form := m.form
	assignment := m.doc.Routing().Tiles[form.tileID]
	assignment.RadiusTiles, assignment.Gain = nil, nil
	for _, pair := range [][2]int{{distanceModeField, tileDistanceField}, {levelModeField, tileLevelField}} {
		if form.fields[pair[0]].value == useSoundDefault {
			continue
		}
		value, err := strconv.ParseFloat(form.fields[pair[1]].value, 64)
		invalid := err != nil || math.IsNaN(value) || math.IsInf(value, 0)
		limit := float64(maxTileLevelPercent)
		if pair[1] == tileDistanceField {
			const maxDistance = 32
			limit = maxDistance
			invalid = invalid || value <= 0
		}
		if invalid || value < 0 || value > limit {
			form.errorText = fmt.Sprintf("%s must be %s0 to %g.", form.fields[pair[1]].label,
				map[bool]string{true: "greater than ", false: ""}[pair[1] == tileDistanceField], limit)
			form.cursor, form.errorField = pair[1], pair[1]

			return assignment, false
		}
		if pair[1] == tileDistanceField {
			assignment.RadiusTiles = &value
		} else {
			gain := value / levelPercentScale
			assignment.Gain = &gain
		}
	}

	return assignment, true
}
