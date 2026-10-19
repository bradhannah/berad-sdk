// Package editorui implements the standalone Bubble Tea audio-library editor.
package editorui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/audition"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/session"
)

const (
	refreshInterval = 50 * time.Millisecond
	defaultWidth    = 100
	defaultHeight   = 36
	msPerSecond     = 1000
	decibelBase     = 10
	decibelDivisor  = 20
	keyEscape       = "esc"
	keyEnter        = "enter"
	keyDown         = "down"
	keyTab          = "tab"
	keyBackspace    = "backspace"
	keyDelete       = "delete"
	keyPageUp       = "pgup"
	keyPageDown     = "pgdown"
	keyPreviousPage = "ctrl+p"
	keyNextPage     = "ctrl+n"
	keyShiftUp      = "shift+up"
	keyShiftDown    = "shift+down"
	keyShiftLeft    = "shift+left"
	keyShiftRight   = "shift+right"
	keyCtrlDown     = "ctrl+down"
	keyShiftTab     = "shift+tab"
	keyCtrlA        = "ctrl+a"
	keyLeft         = "left"
	keyRight        = "right"
	keySave         = "ctrl+s"
	keyFilter       = "ctrl+f"
	fieldNote       = "note"
	fieldGain       = "gain"
	fieldStart      = "start"
	fieldEnd        = "end"
	markerStart     = 1
	markerEnd       = 2
)

type loadedMsg struct {
	generation uint64
	request    sourceLoad
	clip       *audio.RecordingAudio
	err        error
}
type documentMsg struct {
	result       session.Result
	generation   uint64
	baseRevision uint64
	document     *audio.LibraryEditor
	err          error
	summary      string
	reload       bool
	calibration  *calibrationDraft
}
type tickMsg time.Time

type playback interface {
	Play(clip *audio.RecordingAudio, window *audio.PlaybackWindow, loop bool, gain float64) error
	SetGain(gain float64)
	Playing() bool
	Active() bool
	Pause(paused bool)
	PositionMs() int64
	SeekMs(position int64) error
	Close() error
}

type model struct {
	operations          session.Documents
	options             Options
	targets             audio.AuthoringTargets
	doc                 *audio.LibraryEditor
	preview             playback
	clip                *audio.RecordingAudio
	clipRecordingID     string
	sourceStale         bool
	sourceProbePending  bool
	nextSourceProbe     time.Time
	cancelSourceProbe   context.CancelFunc
	keys                []string
	cursor              int
	browserStart        int
	workspace           editorWorkspace
	search              string
	width, height       int
	generation          uint64
	documentGeneration  uint64
	cancelLoad          context.CancelFunc
	loading, busy       bool
	status              string
	field, input        string
	inputText           textCursor
	inputAction         int
	licenseRecordingID  string
	inputError          string
	committingInput     bool
	statusIsError       bool
	form                *editorForm
	infoDialog          *editorInfo
	previewPaused       bool
	previewLoop         bool
	previewEndMs        int64
	picker              *filePicker
	assigning           bool
	assignmentCue       string
	fullSource, looping bool
	autoPreview         bool
	position            int64
	marker              int
	zoom                bool
	previewSliders      [previewCategoryCount]int
	referencePreview    bool
	recordingCategory   previewCategory
	volumeControls      bool
	volumeCursor        int
	confirmQuit         bool
	quitAfterOperation  bool
	mix                 []*mixLayer
	mixCursor           int
	soundscapeVisible   bool
	pendingAmbient      *mixLayer
	sceneFXPreview      bool
	previewLabel        string
	previewRoute        *previewRoute
	now                 func() time.Time
	newVoice            func() playback
	availability        map[string]audio.RecordingAvailability
	filter              availabilityFilter
	scanGeneration      uint64
	scanQueue           []string
	cancelScan          context.CancelFunc
	workspaceState      [workspaceCount]browseState
	assignmentState     browseState
	focus               panelFocus
	detailsScroll       int
	colors              palette
	variantCue          string
	variantState        browseState
	assignmentAppend    bool
	duplicate           *clipDraft
	crop                *cropSession
	currentVariant      map[string]int
	autoGainDraft       *calibrationDraft
	calibrationCache    *calibrationCache
	routing             routingEditor
}

func newModel(doc *audio.LibraryEditor, selected ...Options) *model {
	editor := new(model)
	var options Options
	if len(selected) > 0 {
		options = selected[0]
	}
	editor.options = normalizedOptions(options)
	editor.doc, editor.preview = doc, audition.NewVoice()
	editor.width, editor.height = defaultWidth, defaultHeight
	editor.previewSliders = [previewCategoryCount]int{defaultPreviewSlider, defaultPreviewSlider, defaultAmbientSlider}
	editor.recordingCategory = previewCategoryAuto
	editor.now = time.Now
	editor.newVoice = newVoice
	editor.colors = newPalette()
	editor.currentVariant = make(map[string]int)
	editor.targets = doc.Targets()
	editor.routing.tiles = editor.targets.Tiles
	editor.routing.selected = make(map[string]bool)
	editor.prepareAvailability()
	editor.status = "Enter: play selection. f: choose file. Ctrl+s: save the real manifest."
	editor.refreshKeys()

	return editor
}

//nolint:ireturn // The presentation adapter allows deterministic input tests without opening an audio device.
func newVoice() playback { return audition.NewVoice() }

// Run restores the terminal and closes every player on quit, interrupt, or error.
func Run(doc *audio.LibraryEditor, options Options) error {
	if doc == nil {
		return fmt.Errorf("%w: editor document required", audio.ErrLibrary)
	}
	options = normalizedOptions(options)
	if err := validateOptions(doc, options); err != nil {
		return err
	}
	m := newModel(doc, options)
	defer m.close()
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return fmt.Errorf("audio editor: %w", err)
	}

	return nil
}

func (m *model) Init() tea.Cmd { return tea.Batch(m.loadSelection(), tick(), m.scanNext()) }
func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

//nolint:ireturn // Bubble Tea requires Update to return its Model interface.
func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case loadedMsg:
		m.acceptLoaded(msg)
	case documentMsg:
		return m, m.acceptDocument(msg)
	case availabilityMsg:
		return m, m.acceptAvailability(msg)
	case sourceProbeMsg:
		m.acceptSourceProbe(msg)
	case tickMsg:
		if voice := m.previewVoice(); voice.Active() {
			m.position = voice.PositionMs()
		}
		m.updateMix()
		m.updateRoutingPreview()
		m.updateCropPreview(false)

		return m, tea.Batch(tick(), m.queueSourceProbe())
	case tea.KeyMsg:
		return m, m.key(msg)
	}

	return m, nil
}

func (m *model) selectedID() string {
	if len(m.keys) == 0 {
		return ""
	}

	return m.keys[m.cursor]
}

func (m *model) recordingID() string {
	if menu := m.routing.menu; m.isRecordingMenu() {
		choices := m.routingMenuChoices()
		if len(choices) > 0 && choices[menu.cursor].id != formEmptyRecording {
			return choices[menu.cursor].id
		}

		return ""
	}
	if m.form != nil && m.routing.menu == nil {
		return m.formRecordingID()
	}
	if m.routingWorkspace() || m.routing.menu != nil {
		return m.doc.Cue(m.routingCue()).PrimaryRecording()
	}
	if m.browsingCues() {
		ids := m.doc.Cue(m.selectedID()).Recordings()
		if len(ids) == 0 {
			return ""
		}

		return ids[m.currentVariant[m.selectedID()]%len(ids)]
	}

	return m.selectedID()
}

func (m *model) formRecordingID() string {
	switch {
	case m.form.cue != "":
		return m.doc.Cue(m.form.cue).PrimaryRecording()
	case m.form.kind == formNewAmbient:
		return m.form.fields[newRecordingField].value
	case m.form.kind == formSurface:
		if m.form.cursor < len(m.form.sequence) {
			return m.doc.Cue(m.form.sequence[m.form.cursor]).PrimaryRecording()
		}

		return ""
	default:
		return m.selectedID()
	}
}

func (m *model) gain() float64 {
	if m.crop != nil {
		preview := m.crop.preview
		if m.preview.Active() {
			preview = m.crop.audible
		}

		return m.cropGain(preview)
	}
	if m.form != nil && m.form.kind == formNewAmbient && m.routing.menu == nil {
		return m.newAmbientPreviewGain()
	}
	gain := m.auditionGain(m.recordingID(), m.activeCue(), m.picker == nil)
	if m.form != nil && m.form.previewLevel != nil && m.routing.menu == nil {
		return gain * *m.form.previewLevel
	}
	if m.workspace == workspaceTiles && !m.routing.footsteps && m.routing.menu == nil && m.form == nil {
		_, level := m.doc.Routing().TileAmbientSettings(m.selectedID())
		gain *= level
	}

	return gain
}

func (m *model) play() bool {
	if m.crop != nil {
		return m.playCrop()
	}
	if m.sourceStale {
		m.report(audio.ErrRecordingSourceChanged)

		return false
	}
	if m.picker == nil && !m.selectedClipReady() {
		m.status = "Choose a recording with an audio file; wait for waveform loading."

		return false
	}
	if m.form != nil && m.routing.menu == nil {
		m.previewForm(false, false)

		return m.preview.Playing()
	}
	if (m.routingWorkspace() || m.routing.menu != nil) && m.picker == nil {
		return m.playRouting()
	}
	if m.clip == nil {
		m.status = "Choose a recording with an audio file; wait for waveform loading."

		return false
	}
	if m.shouldPreviewInSoundscape() {
		return m.playSoundscapeEffect()
	}

	return m.playStandalone()
}

func (m *model) shouldPreviewInSoundscape() bool {
	return (m.soundscapeVisible || len(m.mix) > 0) && m.picker == nil && !m.assigning && m.crop == nil &&
		m.categoryForPreview(m.recordingID(), m.activeCue()) == previewSFX
}

func (m *model) playSoundscapeEffect() bool {
	layer := m.selectedMixLayer()
	layer.slot = sfxSlot
	layer.fullSource = m.fullSource
	if m.looping {
		layer.mode = modeLoop
	}
	if !m.installMixLayer(layer) {
		return false
	}
	m.sceneFXPreview = true
	m.previewPaused, m.previewLoop = false, layer.mode == modeLoop
	m.previewLabel = m.layerName(layer)
	m.mixCursor = int(sfxSlot)
	m.status = "SFX triggered over Soundscape: " + m.layerName(layer)

	return true
}

func (m *model) playStandalone() bool {
	m.sceneFXPreview = false
	window := m.doc.Recording(m.recordingID()).PlaybackWindow
	if m.fullSource {
		window = nil
	}
	err := m.preview.Play(m.clip, window, m.looping, m.gain())
	if err == nil {
		m.previewRoute = &previewRoute{recordingID: m.recordingID(), cueID: m.activeCue(), calibrated: m.picker == nil}
		m.previewLabel = m.recordingID()
		if m.picker != nil {
			m.previewLabel = "Candidate " + filepath.Base(m.clip.Path())
		}
		m.status = "Playing " + m.previewLabel
		m.previewPaused, m.previewLoop = false, m.looping
		m.previewEndMs = m.clip.DurationMs()
		if window != nil {
			m.previewEndMs = window.EndMs
		}
	}
	m.report(err)

	return err == nil
}

//nolint:ireturn // The same playback adapter owns standalone and soundscape preview controls.
func (m *model) previewVoice() playback {
	if m.sceneFXPreview {
		if layer := m.layerAt(sfxSlot); layer != nil {
			return layer.voice
		}
	}

	return m.preview
}

func (m *model) pausePreview(paused bool) {
	if m.crop != nil {
		m.crop.paused = paused
	}
	if paused && !m.previewPaused {
		m.routing.pauseStart = m.now()
	} else if !paused && m.previewPaused && m.routing.fading {
		m.routing.fadeStart = m.routing.fadeStart.Add(m.now().Sub(m.routing.pauseStart))
	}
	m.previewPaused = paused
	if m.sceneFXPreview {
		if layer := m.layerAt(sfxSlot); layer != nil {
			layer.paused = paused
		}
	}
	m.previewVoice().Pause(paused)
}

func (m *model) report(err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		m.status = err.Error()
		m.statusIsError = true
		if m.committingInput {
			m.inputError = err.Error()
		}
	}
}

func (m *model) rejectInput(message string) {
	m.status, m.inputError, m.statusIsError = message, message, true
}

func (m *model) close() {
	if m.cancelSourceProbe != nil {
		m.cancelSourceProbe()
	}
	if m.cancelScan != nil {
		m.cancelScan()
	}
	if m.cancelLoad != nil {
		m.cancelLoad()
	}
	m.report(m.preview.Close())
	for _, layer := range m.mix {
		m.report(layer.voice.Close())
	}
}

func (m *model) save() tea.Cmd {
	m.statusIsError = false
	m.confirmQuit = false
	m.status = "Validating and saving library.json…"

	return m.documentOperation(false, m.options.SavedNotice,
		(*audio.LibraryEditor).Save)
}
