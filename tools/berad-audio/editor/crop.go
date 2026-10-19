package editorui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const cropPreviewInterval = 100 * time.Millisecond
const cropBoundCount = 2
const cropActionCount = 2

type cropLayerState struct {
	layer           *mixLayer
	paused, playing bool
}

// cropSession owns a detached region and its one audition voice. The document
// and other uses of the recording stay unchanged until Apply/Create succeeds.
type cropSession struct {
	draft               *audio.RecordingRegionDraft
	source              *audio.RecordingAudio
	recordingID, cueID  string
	revision            uint64
	uses                int
	window              *audio.PlaybackWindow
	fullInitially       bool
	field               int
	preview, audible    audio.RegionPreview
	err                 error
	pending             bool
	nextPreview         time.Time
	audition, paused    bool
	position            int64
	applying, reloading bool
	rebased             bool
	resumeLayers        bool
	layers              []cropLayerState
}

func copyWindow(window *audio.PlaybackWindow) *audio.PlaybackWindow {
	if window == nil {
		return nil
	}
	region := *window

	return &region
}

func (m *model) window() audio.PlaybackWindow {
	if m.crop != nil {
		if m.crop.window != nil {
			return *m.crop.window
		}

		return audio.PlaybackWindow{StartMs: 0, EndMs: m.crop.source.DurationMs()}
	}
	window := audio.PlaybackWindow{StartMs: 0, EndMs: 0}
	if m.clip != nil {
		window.EndMs = m.clip.DurationMs()
	}
	if saved := m.doc.Recording(m.recordingID()).PlaybackWindow; saved != nil {
		window = *saved
	}

	return window
}

func (m *model) selectedClipReady() bool {
	if m.clip == nil || m.sourceStale || m.clipRecordingID != m.recordingID() {
		return false
	}
	path, err := m.doc.RecordingPath(m.recordingID())

	return err == nil && path == m.clip.Path()
}

func (m *model) beginCrop() bool {
	if m.crop != nil {
		return !m.crop.reloading
	}
	if !m.selectedClipReady() || m.assigning || m.picker != nil {
		m.rejectInput("Load/recheck the assigned recording before editing its region.")

		return false
	}
	draft, err := m.doc.BeginRecordingRegion(m.recordingID(), m.clip)
	if err != nil {
		m.report(err)

		return false
	}
	voice := m.previewVoice()
	session := new(cropSession)
	session.draft, session.source = draft, m.clip
	session.recordingID, session.cueID = m.recordingID(), m.activeCue()
	session.revision, session.uses = m.doc.Revision(), len(m.doc.Usages(session.recordingID))
	session.window = copyWindow(m.doc.Recording(session.recordingID).PlaybackWindow)
	session.fullInitially = session.window == nil
	session.audition, session.paused = voice.Playing() || m.previewIsPaused(), m.previewIsPaused()
	session.position = m.position
	if voice.Active() {
		session.position = voice.PositionMs()
	}
	session.resumeLayers = true
	for _, layer := range m.mix {
		if layer.recordingID == session.recordingID && !layer.stopped {
			session.layers = append(session.layers, cropLayerState{
				layer: layer, paused: layer.paused, playing: layer.voice.Playing(),
			})
			layer.paused = true
			layer.voice.Pause(true)
		}
	}
	m.crop, m.sceneFXPreview, m.fullSource = session, false, false
	m.report(m.preview.Close())
	m.focus, m.marker = focusWave, markerStart
	m.scheduleCropPreview()
	m.status, m.statusIsError = "Region draft: Tab selects Apply/Cancel; p auditions; Escape discards.", false

	return true
}

func (m *model) setWindow(window *audio.PlaybackWindow) bool {
	if !m.beginCrop() {
		return false
	}
	if window != nil && m.crop.fullInitially && m.duplicate == nil &&
		window.StartMs == 0 && window.EndMs == m.crop.source.DurationMs() {
		window = nil // Returning to an originally full source is a true no-op.
	}
	m.crop.window = copyWindow(window)
	m.statusIsError, m.inputError = false, ""
	m.scheduleCropPreview()
	m.status = "Region draft updated; Apply/Create commits one edit, Escape discards."

	return true
}

func (m *model) scheduleCropPreview() {
	session := m.crop
	m.rememberCropTransport()
	if !session.pending {
		session.nextPreview = m.now().Add(cropPreviewInterval)
	}
	session.pending = true
	window := m.window()
	if window.StartMs < 0 || window.EndMs <= window.StartMs || window.EndMs > session.source.DurationMs() {
		session.err = fmt.Errorf("%w: use 0 ≤ Start < End ≤ %d ms", audio.ErrAudioLibrary, session.source.DurationMs())
		m.suspendCropPreview()
	} else {
		session.err = nil
	}
}

func (m *model) rememberCropTransport() {
	if !m.preview.Active() {
		return
	}
	m.crop.position, m.crop.paused = m.preview.PositionMs(), m.previewPaused
	if !m.preview.Playing() && !m.previewPaused {
		m.crop.audition = false // Natural completion is not permission to replay on the next edit.
	}
}

func (m *model) suspendCropPreview() {
	if m.preview.Active() {
		m.crop.position, m.crop.paused = m.preview.PositionMs(), m.previewPaused
		m.report(m.preview.Close())
	}
}

func (m *model) resolveCropPreview(force bool) bool {
	session := m.crop
	if session == nil || session.reloading || session.applying {
		return false
	}
	if err := m.doc.CheckRegionRevision(session.draft); err != nil {
		session.err = err
		m.suspendCropPreview()

		return false
	}
	if m.sourceStale {
		session.err = audio.ErrRecordingSourceChanged
		m.suspendCropPreview()

		return false
	}
	if session.pending && (force || !m.now().Before(session.nextPreview)) {
		session.preview, session.err = session.draft.Preview(session.window)
		session.pending = false
		if session.err != nil {
			m.suspendCropPreview()
		}
	}

	return session.err == nil && !session.pending
}

func (m *model) updateCropPreview(force bool) bool {
	if m.crop != nil {
		m.rememberCropTransport()
	}
	pending := m.crop != nil && m.crop.pending
	valid := m.resolveCropPreview(force)
	if valid && pending && m.crop.audition {
		if !m.preview.Active() || !sameRegionPreview(m.crop.preview, m.crop.audible) {
			m.startCropPreview(true)
		}
	}

	return valid
}

func sameRegionPreview(left, right audio.RegionPreview) bool {
	if left.Gain != right.Gain || (left.Window == nil) != (right.Window == nil) {
		return false
	}

	return left.Window == nil || *left.Window == *right.Window
}

func (m *model) cropGain(preview audio.RegionPreview) float64 {
	return preview.Gain * m.auditionGain(m.crop.recordingID, m.crop.cueID, false)
}

func (m *model) startCropPreview(retainPosition bool) bool {
	session := m.crop
	if retainPosition && m.preview.Active() {
		session.position, session.paused = m.preview.PositionMs(), m.previewPaused
	}
	window := session.preview.Window
	if m.fullSource {
		window = nil
	}
	if err := m.preview.Play(session.source, window, m.looping, m.cropGain(session.preview)); err != nil {
		m.report(err)

		return false
	}
	session.audible = session.preview
	m.previewRoute = &previewRoute{recordingID: session.recordingID, cueID: session.cueID, calibrated: true}
	if retainPosition {
		m.report(m.preview.SeekMs(session.position))
	}
	m.previewPaused, m.previewLoop = session.paused, m.looping
	m.preview.Pause(session.paused)
	m.position = m.preview.PositionMs()
	m.previewLabel = fmt.Sprintf("Draft %s · %d–%d ms", session.recordingID, m.window().StartMs, m.window().EndMs)

	return true
}

func (m *model) playCrop() bool {
	if !m.resolveCropPreview(true) {
		m.report(m.crop.err)

		return false
	}
	m.crop.audition, m.crop.paused = true, false

	return m.startCropPreview(false)
}

func (m *model) applyCrop() tea.Cmd {
	if !m.updateCropPreview(true) {
		m.report(m.crop.err)

		return nil
	}
	session := m.crop
	window := copyWindow(session.window)
	newID, appendCue := "", ""
	duplicate := m.duplicate != nil
	if duplicate {
		newID = m.duplicate.id
		if m.duplicate.appendToCue {
			appendCue = m.duplicate.eligibleCue
		}
	}
	session.applying = true
	m.status = "Verifying source content and applying region…"

	return m.documentOperation(false, "Region applied; Ctrl+s saves authored changes.",
		func(document *audio.LibraryEditor) error {
			if duplicate {
				return document.DuplicateRecordingRegion(context.Background(), session.draft, newID, window, appendCue)
			}

			return document.ApplyRecordingRegion(context.Background(), session.draft, window)
		})
}

func (m *model) restoreCropLayers(session *cropSession, applied bool) {
	if !session.resumeLayers || m.sourceStale {
		return
	}
	for _, saved := range session.layers {
		if m.layerAt(saved.layer.slot) != saved.layer || saved.layer.stopped && !session.rebased {
			continue
		}
		layer := saved.layer
		layer.paused = saved.paused
		if !saved.playing && !saved.paused {
			if applied || session.rebased {
				layer.clip = session.source
				m.report(layer.voice.Close())
				layer.stopped = layer.mode != modeRepeat
			}

			continue
		}
		if applied || session.rebased {
			position := layer.voice.PositionMs()
			layer.clip = session.source
			if m.startMixLayer(layer) {
				m.report(layer.voice.SeekMs(position))
			}
		} else {
			layer.voice.Pause(saved.paused)
		}
	}
}

func (m *model) cancelCrop() {
	session := m.crop
	if session == nil {
		return
	}
	m.rememberCropTransport()
	position, paused := session.position, session.paused
	m.statusIsError = false
	m.crop, m.duplicate, m.marker, m.focus = nil, nil, 0, focusBrowser
	m.report(m.preview.Close())
	m.restoreCropLayers(session, false)
	if session.audition && m.selectedClipReady() && len(session.layers) == 0 {
		if m.playStandalone() {
			m.report(m.preview.SeekMs(position))
			m.pausePreview(paused)
		}
	}
	if !m.statusIsError {
		m.status = "Region draft discarded; authored settings are unchanged."
		if m.sourceStale {
			m.status = "Draft discarded. Source changed; F5 reloads the selected recording."
		}
	}
}

func (m *model) finishCropCommit() tea.Cmd {
	session := m.crop
	created := m.duplicate != nil
	if created {
		identity, cue := m.duplicate.id, m.duplicate.eligibleCue
		if m.variantCue == "" || !m.duplicate.appendToCue || cue != m.variantCue {
			m.variantCue, m.workspace = "", workspaceRecordings
		}
		m.search, m.filter = "", filterAll
		m.refreshKeys()
		m.selectID(identity)
		m.clipRecordingID = identity
	}
	m.crop, m.duplicate, m.marker, m.focus = nil, nil, 0, focusBrowser
	m.previewLabel = m.recordingID()
	m.previewRoute = &previewRoute{recordingID: m.recordingID(), cueID: session.cueID, calibrated: true}
	if created && session.cueID != "" {
		m.previewLabel += " · via " + m.doc.Cue(session.cueID).Name
	}
	m.restoreCropLayers(session, !created && m.doc.Revision() != session.revision)
	m.updateSelectedAvailability(loadedMsg{generation: m.generation,
		request: sourceLoad{recordingID: m.recordingID(), path: m.clip.Path(), purpose: sourceSelection,
			autoplay: false, cueID: "", variant: 0}, clip: m.clip, err: nil})
	selected := m.recordingID()
	m.refreshKeys()
	if selected != m.recordingID() {
		return m.loadSelection()
	}

	return nil
}

func (m *model) cropFieldCount() int {
	if m.duplicate != nil {
		return cropBoundCount + 1 // Identity plus the two bounds.
	}

	return cropBoundCount
}

func (m *model) focusCropField(field int) {
	m.crop.field, m.marker = field, 0
	if field == m.cropFieldCount()-cropBoundCount {
		m.marker = markerStart
	} else if field == m.cropFieldCount()-1 {
		m.marker = markerEnd
	}
}

//nolint:cyclop,funlen // One region workflow serves ordinary crops and metadata-only duplicate clips.
func (m *model) cropKey(key string) tea.Cmd {
	count := m.cropFieldCount()
	switch key {
	case keyEscape:
		m.cancelCrop()
	case "q":
		return m.quit()
	case keyTab, keyDown:
		m.focusCropField((m.crop.field + 1) % (count + cropActionCount))
	case keyShiftTab, "up":
		m.focusCropField((m.crop.field + count + 1) % (count + cropActionCount))
	case keyEnter:
		switch m.crop.field {
		case count:
			return m.applyCrop()
		case count + 1:
			m.cancelCrop()
		case count - cropBoundCount:
			m.beginInput(fieldStart, strconv.FormatInt(m.window().StartMs, 10))
		case count - 1:
			m.beginInput(fieldEnd, strconv.FormatInt(m.window().EndMs, 10))
		default:
			if m.duplicate != nil && m.crop.field == 0 {
				m.beginInput(fieldClipID, m.duplicate.id)
			} else {
				m.playCrop()
			}
		}
	case "a":
		if m.duplicate != nil {
			m.duplicate.appendToCue = !m.duplicate.appendToCue && m.duplicate.eligibleCue != ""
		} else {
			return m.applyCrop()
		}
	case "c":
		if m.duplicate != nil {
			return m.applyCrop()
		}
	case "w":
		m.focusCropField(-1)
	case "[":
		m.focusCropField(count - cropBoundCount)
	case "]":
		m.focusCropField(count - 1)
	case keyLeft, keyRight, keyShiftLeft, keyShiftRight:
		if m.crop.field >= count {
			m.focusCropField(count + (m.crop.field-count+1)%cropActionCount)
		} else if m.marker != 0 || m.crop.field == -1 {
			return m.browserKey(key)
		}
	case "p":
		m.playCrop()
	case " ":
		m.togglePreview()
	case "s", "e", "i", "o", "z", "t", "r", "S", "l":
		return m.browserKey(key)
	case "f5":
		m.rememberCropTransport()
		m.crop.reloading = true

		return m.loadSelection()
	case "?":
		m.cropHelp()
	}

	return nil
}

func (m *model) markPosition(start bool) {
	position := m.position
	if !m.beginCrop() {
		return
	}
	window, field := m.window(), m.cropFieldCount()-1
	if start {
		window.StartMs, field = position, m.cropFieldCount()-cropBoundCount
	} else {
		window.EndMs = position
	}
	if m.setWindow(&window) {
		m.focusCropField(field)
	}
}

func (m *model) nudge(delta int64) {
	if !m.selectedClipReady() {
		m.status = "Load/recheck the selected source before seeking or moving a marker."

		return
	}
	if m.marker != 0 {
		if !m.beginCrop() {
			return
		}
		window := m.window()
		if m.marker == markerStart {
			window.StartMs = max(0, min(m.clip.DurationMs(), window.StartMs+delta))
		} else {
			window.EndMs = max(0, min(m.clip.DurationMs(), window.EndMs+delta))
		}
		m.setWindow(&window)

		return
	}
	position := max(0, min(m.clip.DurationMs(), m.position+delta))
	voice := m.previewVoice()
	if err := voice.SeekMs(position); err != nil {
		m.report(err)

		return
	}
	m.position, m.statusIsError = position, false
	m.status = fmt.Sprintf("Source cursor: %d ms. [/]: select a trim marker.", position)
	if voice.Active() {
		m.position = voice.PositionMs()
		m.status = fmt.Sprintf("Preview cursor: %d ms (within the playing selection). Ctrl+x stops.", m.position)
	}
	if m.crop != nil {
		m.crop.position = m.position
	}
}

func (m *model) suggestTrim() {
	if !m.beginCrop() {
		return
	}
	window, ok := m.clip.SuggestedWindow()
	if !ok {
		m.status = "No sound above −40 dB found; set draft bounds manually."

		return
	}
	m.setWindow(window)
	m.status = "Suggested draft region (−40 dB, 10 ms padding); audition, then Apply or Cancel."
}
