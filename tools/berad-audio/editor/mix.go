package editorui

import (
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const maxLayerGain = 2
const repeatInterval = 500 * time.Millisecond
const modeOnce = "once"
const modeLoop = "loop"
const modeRepeat = "repeat"

type soundscapeSlot int

const (
	ambientOne soundscapeSlot = iota
	ambientTwo
	musicSlot
	sfxSlot
	soundscapeSlotCount
)

type mixLayer struct {
	voice                        playback
	clip                         *audio.RecordingAudio
	recordingID, cueID, mode     string
	slot                         soundscapeSlot
	gain                         float64
	muted, solo, paused, stopped bool
	fullSource                   bool
	lastTrigger                  time.Time
}

func (m *model) layerAt(slot soundscapeSlot) *mixLayer {
	for _, layer := range m.mix {
		if layer.slot == slot {
			return layer
		}
	}

	return nil
}

func (m *model) selectedMixLayer() *mixLayer {
	if !m.selectedClipReady() || m.assigning || m.picker != nil || m.crop != nil {
		m.status = "Select a playable Moment, then a adds it to Soundscape."

		return nil
	}
	layer := new(mixLayer)
	layer.clip, layer.recordingID, layer.cueID = m.clip, m.recordingID(), m.activeCue()
	layer.mode, layer.gain = modeOnce, 1

	return layer
}

func (m *model) addMix() {
	layer := m.selectedMixLayer()
	if layer == nil {
		return
	}
	m.soundscapeVisible = true
	switch m.categoryForPreview(layer.recordingID, layer.cueID) {
	case previewAmbience:
		layer.mode, layer.slot = modeLoop, ambientOne
		if m.layerAt(ambientOne) != nil {
			layer.slot = ambientTwo
		}
		if m.layerAt(ambientOne) != nil && m.layerAt(ambientTwo) != nil {
			m.pendingAmbient = layer
			m.mixCursor, m.focus = int(ambientOne), focusMix
			m.status = "Both ambient slots are occupied. Choose 1 or 2; Enter replaces, Esc cancels."

			return
		}
	case previewMusic:
		layer.slot = musicSlot // Music plays once regardless of the standalone preview's loop toggle.
	case previewSFX:
		layer.slot = sfxSlot
	case previewCategoryAuto, previewCategoryCount:
		panic("invalid soundscape category")
	}
	if m.installMixLayer(layer) {
		m.mixCursor = int(layer.slot)
		m.sceneFXPreview = layer.slot == sfxSlot
		m.status = "Soundscape playing. Browse SFX and Enter fires over it; m controls slots; x stops all."
	}
}

// installMixLayer starts a valid replacement before retiring the old slot.
// A failed decode/player creation must not cut off an existing ambience bed.
func (m *model) installMixLayer(layer *mixLayer) bool {
	if old := m.layerAt(layer.slot); old != nil {
		layer.gain, layer.muted, layer.solo = old.gain, old.muted, old.solo
	}
	layer.voice = m.newVoice()
	if !m.startMixLayer(layer) {
		m.report(layer.voice.Close())

		return false
	}
	for index, old := range m.mix {
		if old.slot == layer.slot {
			m.report(old.voice.Close())
			m.mix[index] = layer
			m.report(m.preview.Close())
			m.refreshMixGains()

			return true
		}
	}
	m.mix = append(m.mix, layer)
	m.report(m.preview.Close()) // Do not double an auditioned source when pinning it.
	m.refreshMixGains()

	return true
}

func (m *model) layerGain(layer *mixLayer) float64 {
	if layer.muted {
		return 0
	}
	for _, other := range m.mix {
		if other.solo && !layer.solo {
			return 0
		}
	}

	return layer.gain * m.auditionGain(layer.recordingID, layer.cueID, true)
}

func (m *model) startMixLayer(layer *mixLayer) bool {
	if err := layer.clip.CheckSourceStamp(); err != nil {
		layer.stopped = true
		m.report(layer.voice.Close())
		m.report(err)

		return false
	}
	window := m.doc.Recording(layer.recordingID).PlaybackWindow
	if layer.fullSource {
		window = nil
	}
	if err := layer.voice.Play(layer.clip, window, layer.mode == modeLoop, m.layerGain(layer)); err != nil {
		layer.stopped = true
		m.report(layer.voice.Close())
		m.report(err)

		return false
	}
	layer.voice.Pause(layer.paused)
	layer.lastTrigger, layer.stopped = m.now(), false

	return true
}

func (m *model) updateMix() {
	for _, layer := range m.mix {
		if layer.mode == modeRepeat && !layer.paused && !layer.stopped && m.now().Sub(layer.lastTrigger) >= repeatInterval {
			m.startMixLayer(layer)
		}
	}
}

func (m *model) refreshMixGains() {
	for _, layer := range m.mix {
		layer.voice.SetGain(m.layerGain(layer))
	}
}

func (m *model) ambientReplacementKey(key string) {
	switch key {
	case keyEscape:
		m.pendingAmbient = nil
		m.focus = focusBrowser
		m.status = "Ambient replacement cancelled; existing beds keep playing."
	case "1", "up", keyPreviousPage:
		m.mixCursor = int(ambientOne)
	case "2", keyDown, keyNextPage:
		m.mixCursor = int(ambientTwo)
	case keyEnter:
		layer := m.pendingAmbient
		layer.slot = soundscapeSlot(m.mixCursor)
		if m.installMixLayer(layer) {
			m.pendingAmbient = nil
			m.focus = focusBrowser
			m.status = "Ambient slot replaced; the other bed and music continue."
		}
	}
}

//nolint:cyclop,funlen // One bounded slot dispatch keeps Soundscape controls auditable.
func (m *model) mixKey(key tea.KeyMsg) tea.Cmd {
	layer := m.layerAt(soundscapeSlot(m.mixCursor))
	switch key.String() {
	case "q":
		return m.quit()
	case "ctrl+s":
		return m.save()
	case keyEscape, "m", keyTab:
		m.focus = focusBrowser // The pane remains visible while browsing.
	case "d":
		m.soundscapeVisible, m.focus = false, focusDetails
	case "up":
		m.mixCursor = max(0, m.mixCursor-1)
	case keyDown:
		m.mixCursor = min(int(soundscapeSlotCount)-1, m.mixCursor+1)
	case keyPageUp, keyShiftUp, keyPreviousPage:
		m.mixCursor = int(ambientOne)
	case keyPageDown, keyShiftDown, keyNextPage:
		m.mixCursor = int(sfxSlot)
	case "+", "=", "-":
		if layer != nil {
			delta := cueGainStep
			if key.String() == "-" {
				delta = -delta
			}
			layer.gain = math.Max(0, math.Min(maxLayerGain, layer.gain+delta))
		}
	case " ":
		if layer != nil {
			if layer.stopped || (!layer.paused && !layer.voice.Playing()) {
				layer.paused = false
				m.startMixLayer(layer)
			} else {
				layer.paused = !layer.paused
				layer.voice.Pause(layer.paused)
			}
		}
	case keyEnter:
		if layer != nil {
			layer.paused = false
			m.startMixLayer(layer)
		}
	case "u":
		if layer != nil {
			layer.muted = !layer.muted
		}
	case "s":
		if layer != nil {
			layer.solo = !layer.solo
		}
	case "l":
		if layer != nil {
			m.toggleLayerMode(layer)
		}
	case "delete", keyBackspace:
		if layer != nil {
			m.removeMixLayer(layer)
		}
	case "x":
		m.stopAll()
	}
	m.refreshMixGains()

	return nil
}

func (m *model) toggleLayerMode(layer *mixLayer) {
	switch {
	case layer.mode != modeOnce:
		layer.mode = modeOnce
	case layer.slot == sfxSlot:
		layer.mode = modeRepeat
	default:
		layer.mode = modeLoop
	}
	if !layer.stopped {
		m.startMixLayer(layer)
	}
}

func (m *model) removeMixLayer(layer *mixLayer) {
	for index, existing := range m.mix {
		if existing == layer {
			m.report(layer.voice.Close())
			m.mix = append(m.mix[:index], m.mix[index+1:]...)

			return
		}
	}
}

func (m *model) stopAll() {
	if m.crop != nil {
		m.crop.audition, m.crop.paused, m.crop.resumeLayers = false, false, false
	}
	m.previewPaused, m.routing.fading = false, false
	m.autoPreview, m.sceneFXPreview = false, false
	m.pendingAmbient = nil
	m.report(m.preview.Close())
	for _, layer := range m.mix {
		m.report(layer.voice.Close())
		layer.stopped, layer.paused = true, false
	}
	m.focus = focusBrowser
}

func (m *model) clearSoundscape() {
	m.stopAll()
	m.mix, m.mixCursor = nil, 0
}
