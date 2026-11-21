package editorui

import (
	"fmt"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const transportRows = 1

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
		return "▶ playing"
	}
	if m.previewVoice().Active() && m.previewIsPaused() {
		return "Ⅱ paused"
	}
	if m.loading {
		return "… loading"
	}

	return "■ stopped"
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

// One line: what is playing, where in it, and at which volumes.
func (m *model) transportView(width int) string {
	voice := m.previewVoice()
	position, total := m.position, int64(0)
	if voice.Active() {
		position = voice.PositionMs()
	}
	if m.crop != nil {
		total = m.crop.source.DurationMs()
	} else if m.clip != nil {
		total = m.clip.DurationMs()
	}
	style := m.colors.dim
	if voice.Playing() {
		style = m.colors.ready
	}
	at := brief(position, total)
	if m.crop != nil {
		at = clock(position, true)
	}
	left := " " + style.Render(m.transportState()) + "  " + m.colors.title.Render(at) +
		m.colors.dim.Render(" / "+brief(total, total)) + "   " + safe(m.previewDescription()) +
		m.colors.dim.Render(m.transportFlags())
	right := m.previewVolumeStrip() + " "
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return fit(left, width)
	}

	return left + strings.Repeat(" ", gap) + right
}

func (m *model) transportFlags() string {
	flags := ""
	if m.looping {
		flags += " · loop"
	}
	if m.fullSource {
		flags += " · whole file"
	}
	if m.routingWorkspace() {
		flags += fmt.Sprintf(" · %g tiles away", m.routing.distance)
	}

	return flags
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
		m.scrollInfo(-1)
	case keyDown:
		m.scrollInfo(1)
	case keyPageUp:
		m.scrollInfo(-m.infoPage())
	case keyPageDown:
		m.scrollInfo(m.infoPage())
	case "home":
		m.infoDialog.scroll = 0
	case "end":
		m.infoDialog.scroll, _ = m.infoBounds()
	}
}

// infoBounds returns the largest top row and the page size of the open info dialog,
// measured exactly as overlayDialog wraps and fits it.
func (m *model) infoBounds() (int, int) {
	width := min(maxDialogWidth, m.width-editorDialogMargin)
	dialog, _ := m.dialogContent(width - panelEdges)
	layout := m.layoutDialog(dialog, width)

	return max(0, len(layout.rows)-layout.capacity), layout.capacity
}

func (m *model) infoPage() int {
	_, page := m.infoBounds()

	return page
}

func (m *model) scrollInfo(delta int) {
	limit, _ := m.infoBounds()
	m.infoDialog.scroll = max(0, min(limit, m.infoDialog.scroll+delta))
}

func (m *model) openHelp() {
	lines := []string{
		"BROWSING AND PLAYBACK", "1–4 / Tab: workspace · /: filter as you type · Ctrl+F: all, with audio, empty, problems",
		"Enter / p: play or restart · Space: pause/resume · x or Ctrl+x: stop (Ctrl+x also while typing)",
		"n: next clip in Moments, new empty recording in Recordings · Shift+n: recording note",
		"Ctrl+S: save library · u / Ctrl+R: undo / redo · F5: recheck files · q: quit",
		"Shift+I: every number in Details", "",
		"TILES AND LOCATIONS", "f: choose ambient sound or music · r: change recording · i: import recording file",
		"p: preview ambience · Shift+P: preview footstep · n: next footstep",
		"Insert: mark tile families · s: footstep surface / location scope",
		"Shift+S: edit footstep sequence · Shift+D: default surface", "B: sound settings · Ctrl+A: new ambient sound",
		"Left/Right: preview distance · Shift+L: preview leaving range", "F: animation details · U: used by · o: open sound",
		"", "RECORDINGS AND MOMENTS", "f: file (Recordings) or sound (Moments) · d: details · w: waveform · m: Soundscape",
		"+ / - / g: in Moments, that moment's volume (%) · in Recordings, the shared recording's level (dB)",
		"l: loop on/off · Shift+S: whole file / trimmed part · i/o: mark a trim where it is playing",
		"s/e: type a trim time · [/]: choose a trim marker · Left/Right: seek, or nudge the marker (Shift: bigger)",
		"z: zoom · t: trim silence · r: back to the whole file",
		"A trim is a draft: Enter applies, Escape discards, p plays it, Shift+P plays only its ending",
		"F5 in a draft reloads its file and keeps the draft · ? lists the draft's keys",
		"Shift+K: keywords · Shift+H: trigger · Shift+N: note · C: classification",
		"Ctrl+L: edit the selected recording's license (shared provenance)",
		"A: auto level · T: auto target · D: duplicate clip · V: ordered variants",
		"a: add to Soundscape · v: temporary preview volumes", "",
		"EDITING", "A highlighted field owns typing; other workspace shortcuts are suspended.",
		"One value (level, volume, time): type over it · Enter applies · Escape cancels",
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
