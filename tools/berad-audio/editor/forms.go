package editorui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const (
	formAmbient        = "ambient settings"
	formTileAmbient    = "tile ambient settings"
	formNewAmbient     = "new ambient sound"
	formSurface        = "footstep sequence"
	formClassification = "classification"
	newNameField       = 0
	newIDField         = 1
	newRecordingField  = 2
	newRadiusField     = 3
	formEmptyRecording = "<empty slot>"
	unitTiles          = "tiles"
)

type formField struct {
	label, value, help, unit string
	step                     float64
	options                  []string
}

type editorForm struct {
	kind, title, cue, surface string
	fields                    []formField
	sequence                  []string
	cursor                    int
	editing                   bool
	text                      textCursor
	textBefore                string
	identifierBefore          bool
	errorText                 string
	errorField                int
	parentMenu                *routingMenu
	previewProfile            *audio.AmbientProfile
	previewLevel              *float64
	tileID                    string
	identifierEdited          bool
}

type editorInfo struct {
	title  string
	lines  []string
	scroll int
}

func ambientFields(profile audio.AmbientProfile) []formField {
	const fadeStepSeconds = 0.1

	return []formField{
		{label: "Hearing distance", value: strconv.FormatFloat(profile.RadiusTiles, 'f', -1, 64),
			help: "0 = non-spatial background; 1–32 = nearby source range.", unit: unitTiles, step: 1, options: nil},
		{label: "Fade in", value: fmt.Sprintf("%.3g", float64(profile.FadeInMs)/msPerSecond),
			help: "How quickly the sound appears (0–60 seconds).", unit: "seconds", step: fadeStepSeconds, options: nil},
		{label: "Fade out", value: fmt.Sprintf("%.3g", float64(profile.FadeOutMs)/msPerSecond),
			help: "How quickly the sound disappears (0–60 seconds).", unit: "seconds", step: fadeStepSeconds, options: nil},
	}
}

func (m *model) editAmbientProfile() tea.Cmd {
	if m.workspace == workspaceTiles && m.routing.menu == nil {
		return m.editTileAmbient()
	}

	return m.editSharedAmbientProfile()
}

func (m *model) editSharedAmbientProfile() tea.Cmd {
	cue := m.assignmentCueForSelection()
	profile, exists := m.doc.Routing().Ambient[cue]
	if !exists {
		m.status = "Choose an ambient sound before opening Sound settings."

		return nil
	}
	m.form = &editorForm{ //nolint:exhaustruct // Fresh draft: no preview, input or validation state yet.
		kind: formAmbient, title: "SHARED SOUND DEFAULTS — " + m.doc.Cue(cue).Name,
		cue: cue, fields: ambientFields(profile), errorField: -1, parentMenu: m.routing.menu,
	}
	m.routing.menu = nil

	return m.loadSelection()
}

func (m *model) openNewAmbientForm() tea.Cmd {
	const initialFadeInMs, initialFadeOutMs, initialRadius = 800, 1200, 8
	const newFieldCount = 6
	fields := make([]formField, 0, newFieldCount)
	fields = append(fields, []formField{
		{label: "Name", value: "", help: "A friendly name, such as Fountain or Cave.", unit: "", step: 0, options: nil},
		{label: "Identifier", value: "", help: "Generated from name: lowercase letters, digits, underscores.",
			unit: "", step: 0, options: nil},
		{label: "Recording", value: formEmptyRecording, help: "Enter chooses a recording. Empty slots can be filled later.",
			unit: "", step: 0, options: nil},
	}...)
	profile := audio.AmbientProfile{RadiusTiles: initialRadius, FadeInMs: initialFadeInMs, FadeOutMs: initialFadeOutMs}
	fields = append(fields, ambientFields(profile)...)
	m.form = &editorForm{ //nolint:exhaustruct // Draft is detached until Apply.
		kind: formNewAmbient, title: "NEW AMBIENT SOUND", fields: fields, errorField: -1, parentMenu: m.routing.menu,
	}
	m.routing.menu = nil

	return m.loadSelection()
}

func (m *model) editSurfaceCues() tea.Cmd {
	routing := m.doc.Routing()
	surface := m.tileSurface(m.selectedID())
	m.form = &editorForm{ //nolint:exhaustruct // Ordered draft is detached; Apply commits once.
		kind: formSurface, title: "FOOTSTEP SEQUENCE — " + surface, surface: surface,
		sequence: append([]string(nil), routing.Surfaces[surface]...), errorField: -1,
	}

	return m.loadSelection()
}

func (m *model) closeForm() tea.Cmd {
	parent := m.form.parentMenu
	m.form = nil
	m.routing.menu = parent
	m.routing.fading = false
	m.report(m.preview.Close())

	return m.loadSelection()
}

func (m *model) formProfile() (audio.AmbientProfile, bool) {
	form := m.form
	offset := 0
	if form.kind == formNewAmbient {
		offset = newRadiusField
	}
	var values [3]float64
	for index := range values {
		value, err := strconv.ParseFloat(strings.TrimSpace(form.fields[offset+index].value), 64)
		limit := 60.0
		if index == 0 {
			limit = 32
		}
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > limit {
			form.errorText = fmt.Sprintf("%s must be a number between 0 and %g.", form.fields[offset+index].label, limit)
			form.errorField, form.cursor = offset+index, offset+index

			return audio.AmbientProfile{RadiusTiles: 0, FadeInMs: 0, FadeOutMs: 0}, false
		}
		values[index] = value
	}

	return audio.AmbientProfile{RadiusTiles: values[0], FadeInMs: int64(math.Round(values[1] * msPerSecond)),
		FadeOutMs: int64(math.Round(values[2] * msPerSecond))}, true
}

//nolint:funlen // One apply boundary validates each draft kind before changing document history.
func (m *model) applyForm() tea.Cmd {
	form := m.form
	form.errorText, form.errorField = "", -1
	var err error
	switch form.kind {
	case formTileAmbient:
		assignment, valid := m.tileAmbientDraft()
		if !valid {
			return nil
		}
		routing := m.doc.Routing()
		routing.Tiles[form.tileID] = assignment
		err = m.doc.SetRouting(routing)
	case formAmbient:
		profile, valid := m.formProfile()
		if !valid {
			return nil
		}
		routing := m.doc.Routing()
		routing.Ambient[form.cue] = profile
		err = m.doc.SetRouting(routing)
	case formNewAmbient:
		profile, valid := m.formProfile()
		if !valid {
			return nil
		}
		recording := form.fields[newRecordingField].value
		if recording == formEmptyRecording {
			recording = ""
		}
		err = m.addAmbient(strings.TrimSpace(form.fields[newIDField].value),
			strings.TrimSpace(form.fields[newNameField].value), profile, recording)
	case formSurface:
		routing := m.doc.Routing()
		routing.Surfaces[form.surface] = append([]string(nil), form.sequence...)
		err = m.doc.SetRouting(routing)
	case formClassification:
		if form.cue != "" {
			err = m.doc.SetCueProfile(form.cue, form.fields[0].value)
		} else {
			err = m.doc.SetRecordingFamily(m.recordingID(), form.fields[0].value)
		}
	}
	if err != nil {
		form.errorText = err.Error()

		return nil
	}
	if form.kind == formNewAmbient {
		cueID := "ambient/" + strings.TrimSpace(form.fields[newIDField].value)
		m.form, m.routing.menu = nil, nil
		m.selectWorkspace(workspaceMoments)
		m.search, m.filter = "", filterAll
		m.refreshKeys()
		m.selectID(cueID)
		m.status = "Ambient sound created. f selects its recording; Ctrl+S saves the library."

		return m.loadSelection()
	}
	m.status, m.statusIsError = "Changes applied to the library draft. Ctrl+S saves; u undoes.", false
	m.refreshKeys()

	return m.closeForm()
}

//nolint:cyclop // Form navigation, text capture and explicit actions share one focus owner.
func (m *model) formKey(key tea.KeyMsg) tea.Cmd {
	form := m.form
	if form.editing {
		return m.formTextKey(key)
	}
	if form.kind == formSurface {
		return m.surfaceFormKey(key.String())
	}
	const formActions = 2
	last := len(form.fields) + formActions - 1
	switch key.String() {
	case keyEscape:
		return m.closeForm()
	case keyTab, keyDown:
		form.cursor = (form.cursor + 1) % (last + 1)
	case keyShiftTab, "up":
		form.cursor = (form.cursor + last) % (last + 1)
	case keyLeft, keyRight:
		m.adjustFormField(key.String() == keyRight)
	case "p", " ":
		return m.previewForm(false, key.String() == " ")
	case "L":
		return m.previewForm(true, false)
	case "x":
		m.stopAll()
	case "v":
		m.openPreviewVolumes()
	case keyEnter:
		if form.cursor == len(form.fields) {
			return m.applyForm()
		}
		if form.cursor > len(form.fields) {
			return m.closeForm()
		}
		if form.kind == formNewAmbient && form.cursor == newRecordingField {
			return m.openFormRecordingPicker()
		}
		if len(form.fields[form.cursor].options) > 0 {
			m.adjustFormField(true)
		} else {
			form.editing = true
			form.textBefore = form.fields[form.cursor].value
			form.identifierBefore = form.identifierEdited
			form.text.reset(form.textBefore)
		}
	}

	return nil
}

func (m *model) adjustFormField(increase bool) {
	form := m.form
	if form.cursor >= len(form.fields) {
		return
	}
	field := &form.fields[form.cursor]
	direction := -1
	if increase {
		direction = 1
	}
	if len(field.options) > 0 {
		for index, option := range field.options {
			if option == field.value {
				field.value = field.options[(index+direction+len(field.options))%len(field.options)]
				m.updateTileOverrideMode()

				return
			}
		}
	}
	if field.step > 0 {
		value, err := strconv.ParseFloat(field.value, 64)
		if err == nil {
			field.value = strconv.FormatFloat(math.Max(0, value+float64(direction)*field.step), 'f', -1, 64)
			m.updateTileOverrideMode()
		}
	}
}

func (m *model) formTextKey(key tea.KeyMsg) tea.Cmd {
	form := m.form
	field := &form.fields[form.cursor]
	before := field.value
	switch textControlKey(key) {
	case keyEscape:
		field.value, form.identifierEdited = form.textBefore, form.identifierBefore
		form.editing = false
	case keyEnter, keyTab:
		form.editing = false
		form.cursor = min(len(form.fields), form.cursor+1)
	default:
		form.text.edit(&field.value, key, false)
		if field.value != before && form.kind == formNewAmbient && form.cursor == newIDField {
			form.identifierEdited = true
		}
	}
	if field.value != before {
		m.updateTileOverrideMode()
	}
	form.updateIdentifier()

	return nil
}

func (form *editorForm) updateIdentifier() {
	if form.kind == formNewAmbient {
		if !form.identifierEdited {
			form.fields[newIDField].value = ambientIdentifier(form.fields[newNameField].value)
		}
	}
}

func ambientIdentifier(name string) string {
	return strings.Trim(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		if unicode.IsSpace(r) || r == '-' {
			return '_'
		}

		return -1
	}, strings.ToLower(name)), "_")
}
