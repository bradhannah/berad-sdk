package editorui

import (
	"fmt"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const transportRows = 4

func (m *model) newAmbientPreviewGain() float64 {
	gain := 1.0
	if calibration := m.doc.Recording(m.recordingID()).CalibrationGainDB; calibration != nil {
		gain = math.Pow(decibelBase, *calibration/decibelDivisor)
	}
	if m.referencePreview {
		return gain
	}

	if m.options.Ambient == nil {
		return 0
	}

	return gain * m.options.Ambient.Gain * m.sliderGain(m.previewSliders[previewAmbience])
}

func (m *model) togglePreview() {
	voice := m.previewVoice()
	switch {
	case voice.Playing():
		m.pausePreview(true)
	case voice.Active() && m.previewIsPaused():
		m.pausePreview(false)
	default:
		m.play()
	}
}

func (m *model) previewIsPaused() bool {
	if m.sceneFXPreview {
		if layer := m.layerAt(sfxSlot); layer != nil {
			return layer.paused
		}
	}

	return m.previewPaused
}

func (m *model) previewTileSound(footsteps, next bool) tea.Cmd {
	if m.workspace != workspaceTiles {
		m.playRouting()

		return nil
	}
	changed := m.routing.footsteps != footsteps || next
	m.routing.footsteps = footsteps
	if next {
		m.routing.step++
	}
	if !changed && m.clip != nil {
		m.playRouting()

		return nil
	}

	return m.loadAndPreviewSelection()
}

func (m *model) loadAndPreviewSelection() tea.Cmd {
	path, err := m.doc.RecordingPath(m.recordingID())
	if err != nil {
		m.report(err)

		return nil
	}

	return m.loadPath(path, true)
}

func (m *model) previewForm(departure, toggle bool) tea.Cmd {
	if toggle && m.preview.Active() {
		if m.preview.Playing() || m.previewPaused {
			m.pausePreview(m.preview.Playing())

			return nil
		}
	}
	form := m.form
	if form.kind == formTileAmbient {
		assignment, valid := m.tileAmbientDraft()
		if !valid {
			return nil
		}
		routing := m.doc.Routing()
		routing.Tiles[form.tileID] = assignment
		profile, level := routing.TileAmbientSettings(form.tileID)
		form.previewProfile, form.previewLevel = &profile, &level
	}
	if form.kind == formAmbient || form.kind == formNewAmbient {
		profile, valid := m.formProfile()
		if !valid {
			return nil
		}
		form.previewProfile = &profile
	}
	if !m.selectedClipReady() {
		form.errorText = "Choose a playable recording before previewing."

		return nil
	}
	m.playRouting()
	if departure {
		m.leaveRoutingRange()
	}

	return nil
}

func (m *model) previewAmbientProfile() (audio.AmbientProfile, bool) {
	if m.form != nil && m.form.previewProfile != nil && m.routing.menu == nil {
		return *m.form.previewProfile, true
	}
	if m.workspace == workspaceTiles && !m.routing.footsteps && m.routing.menu == nil && m.form == nil {
		profile, _ := m.doc.Routing().TileAmbientSettings(m.selectedID())

		return profile, m.routingCueFor(m.selectedID()) != ""
	}
	profile, ambient := m.doc.Routing().Ambient[m.activeCue()]

	return profile, ambient
}

func (m *model) transportState() string {
	if m.previewVoice().Playing() {
		return "▶ PLAYING"
	}
	if m.previewVoice().Active() && m.previewIsPaused() {
		return "Ⅱ PAUSED"
	}
	if m.loading {
		return "… LOADING"
	}

	return "■ STOPPED"
}

func (m *model) previewDescription() string {
	if m.previewVoice().Active() && m.previewLabel != "" {
		return m.previewLabel
	}
	if cue := m.activeCue(); cue != "" {
		return m.doc.Cue(cue).Name
	}

	return m.recordingID()
}

func (m *model) transportView(width int) string {
	state := m.transportState()
	style := m.colors.focus
	if m.previewVoice().Playing() {
		style = m.colors.ready
	}
	label := m.previewDescription()
	if label == "" {
		label = "No sound selected"
	}
	mode := "One-shot"
	if m.previewLoop {
		mode = "Looping"
	}
	line := fmt.Sprintf("%s · %s · %.1f s", safe(label), mode, float64(m.previewVoice().PositionMs())/msPerSecond)
	if m.routingWorkspace() {
		line += fmt.Sprintf(" · Distance %g tiles (temporary)", m.routing.distance)
	}

	return m.panel("PREVIEW · "+state, []string{style.Render(fit(line, width-panelEdges)),
		fit("Enter: play/restart   Space: pause/resume   x / Ctrl+x: stop", width-panelEdges)}, width, transportRows, false)
}

func (m *model) showAnimationInfo() {
	tile := m.audioTile(m.selectedID())
	lines := []string{tile.Name, "", fmt.Sprintf("Member tile IDs: %v", tile.Frames), "",
		"All animation frames share one sound assignment.",
		"Membership comes from the shared animation catalog and is read-only."}
	m.infoDialog = &editorInfo{title: "ANIMATION DETAILS", lines: lines, scroll: 0}
}

func (m *model) infoKey(key string) {
	switch key {
	case keyEscape, keyEnter, "?":
		m.infoDialog = nil
	case "up":
		m.infoDialog.scroll = max(0, m.infoDialog.scroll-1)
	case keyDown:
		m.infoDialog.scroll = min(len(m.infoDialog.lines)-1, m.infoDialog.scroll+1)
	}
}

func (m *model) openHelp() {
	lines := []string{
		"BROWSING AND PLAYBACK", "1–4 / Tab: workspace · /: search · Ctrl+F: filter",
		"Enter / p: play or restart · Space: pause/resume · Ctrl+x: stop all (including while typing)",
		"n: next clip in Moments · Shift+n: recording note",
		"Ctrl+S: save library · u / Ctrl+R: undo / redo", "",
		"TILES AND LOCATIONS", "f: choose ambient sound or music · r: change recording · i: import recording file",
		"p: preview ambience · Shift+P: preview footstep · n: next footstep",
		"Insert: mark tile families · s: footstep surface / location scope",
		"Shift+S: edit footstep sequence · Shift+D: default surface", "B: sound settings · Ctrl+A: new ambient sound",
		"Left/Right: preview distance · Shift+L: preview leaving range", "F: animation details · U: used by · o: open sound",
		"", "RECORDINGS AND MOMENTS", "f: file / recording picker · d: details · w: waveform · m: Soundscape",
		"g / +/-: shared gain · i/o: trim markers · s/e: exact trim times",
		"[/]: choose trim marker · arrows: nudge · z: zoom · t: suggest trim · r: reset trim",
		"Regions are drafts: Tab/Enter selects Apply or Cancel; p auditions; Escape discards",
		"F5 in a region reloads its source while retaining bounds for review · ? shows draft/source details",
		"Shift+K: keywords · Shift+H: trigger · Shift+N: note · C: classification",
		"Ctrl+L: edit the selected recording's license (shared provenance)",
		"A: auto-gain · T: calibration target · D: duplicate clip · V: ordered variants",
		"a: add to Soundscape · v: temporary preview volumes", "",
		"EDITING", "A highlighted field owns typing; other workspace shortcuts are suspended.",
		"Dialogs: Tab moves focus · Enter edits/activates Apply or Cancel · Escape cancels",
		"Text: arrows move the caret · Home/End: line bounds · Ctrl+U: clear · Shift+arrows: select",
		"Notes: Enter inserts a line · Tab selects Apply/Cancel · Enter activates the selected action",
		"Forms: Enter finishes a field · Escape restores the field; another Escape cancels the form",
		"Apply changes the library draft; Ctrl+S writes the file. Existing playback snapshots are unchanged until reloaded.",
	}
	m.infoDialog = &editorInfo{
		title: "EDITOR HELP — " + strings.ToUpper(workspaceName(m.workspace)), lines: lines, scroll: 0,
	}
}
