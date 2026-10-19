package editorui

import (
	"context"
	"fmt"
	"math"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const fieldCalibrationTarget = "calibration target"

type calibrationDraft struct {
	audio.CalibrationSuggestion
	recordingID string
	revision    uint64
	source      *audio.RecordingAudio
	err         error
	proposal    *audio.CalibrationDraft
}

type calibrationCache struct {
	clip   *audio.RecordingAudio
	window audio.PlaybackWindow
	family string
	target float64
	result audio.CalibrationSuggestion
	err    error
}

func (m *model) calibrationSuggestion(recording audio.Recording) (audio.CalibrationSuggestion, error) {
	window := audio.PlaybackWindow{StartMs: 0, EndMs: m.clip.DurationMs()}
	if recording.PlaybackWindow != nil {
		window = *recording.PlaybackWindow
	}
	family := m.doc.RecordingFamily(m.recordingID())
	options, err := m.doc.CalibrationOptionsFor(m.recordingID())
	if err != nil {
		return audio.CalibrationSuggestion{}, err
	}
	target := options.TargetDB
	if recording.CalibrationTargetDB != nil {
		target = *recording.CalibrationTargetDB
	}
	if cached := m.calibrationCache; cached != nil && cached.clip == m.clip && cached.window == window &&
		cached.family == family && cached.target == target {
		return cached.result, cached.err
	}
	options.TargetDB = target
	result, err := m.clip.SuggestCalibration(&window, options)
	if err != nil {
		err = fmt.Errorf("suggest calibration: %w", err)
	}
	m.calibrationCache = &calibrationCache{clip: m.clip, window: window, family: family,
		target: target, result: result, err: err}

	return result, err
}

func (m *model) beginAutoCalibration() {
	if !m.selectedClipReady() || m.picker != nil || m.assigning || m.duplicate != nil {
		m.status = "Select a playable recording before recalibrating."

		return
	}
	proposal, err := m.doc.BeginCalibration(m.recordingID(), m.clip)
	if err != nil {
		m.report(err)

		return
	}
	suggestion := proposal.Suggestion()
	m.autoGainDraft = &calibrationDraft{CalibrationSuggestion: suggestion,
		recordingID: m.recordingID(), revision: m.doc.Revision(), source: m.clip, err: nil, proposal: proposal}
	m.status = fmt.Sprintf("Apply %+.2f dB and enable AUTO? Enter confirms; Esc keeps calibration.", suggestion.GainDB)
	m.statusIsError = false
}

func (m *model) autoCalibrationKey(key string) tea.Cmd {
	switch key {
	case keyEscape:
		m.autoGainDraft = nil
		m.status, m.statusIsError = "Calibration unchanged.", false
		if m.sourceStale {
			m.status = "Calibration unchanged. Source changed; F5 reloads before Shift+A recalculates."
		}
	case keyEnter, "y":
		draft := m.autoGainDraft
		if err := m.checkCalibrationDraft(draft); err != nil {
			m.report(err)

			return nil
		}
		m.status = "Verifying source content and applying AUTO calibration…"
		operation := m.documentOperation(false,
			"AUTO enabled. Trim edits follow; Recordings +/- or g locks calibration. Ctrl+s saves.",
			func(document *audio.LibraryEditor) error {
				if err := document.ApplyCalibration(context.Background(), draft.proposal); err != nil {
					return fmt.Errorf("apply AUTO calibration: %w", err)
				}

				return nil
			})

		return func() tea.Msg {
			message, ok := operation().(documentMsg)
			if !ok {
				panic("document operation returned a non-document message")
			}
			message.calibration = draft

			return message
		}
	}

	return nil
}

// Recheck both before starting work and before accepting its detached result.
// A probe or source reload delivered while verification runs invalidates approval.
func (m *model) checkCalibrationDraft(draft *calibrationDraft) error {
	if draft == nil || m.autoGainDraft != draft || draft.recordingID != m.recordingID() ||
		draft.revision != m.doc.Revision() {
		return fmt.Errorf("%w: calibration draft is stale; Escape and recalculate before applying", audio.ErrAudioLibrary)
	}
	if draft.err != nil {
		return draft.err
	}
	if draft.source != m.clip || !m.selectedClipReady() {
		return audio.ErrRecordingSourceChanged
	}

	return nil
}

func (m *model) calibrationDialog() editorDialog {
	dialog := editorDialog{title: "AUTO-GAIN", rows: []string{
		fmt.Sprintf("Suggested recording calibration: %+.2f dB", m.autoGainDraft.GainDB),
		"Verify the source and apply this calibration with automatic gain updates?",
	}, focus: 0, help: []string{"Enter: verify and apply   Escape: keep existing calibration"}}
	if err := m.checkCalibrationDraft(m.autoGainDraft); err != nil {
		dialog.rows = []string{
			m.colors.warning.Render("This calibration approval is stale and cannot be applied."),
			safe(err.Error()), "Escape keeps calibration; F5 reloads, then Shift+A recalculates.",
		}
		dialog.help = []string{"Escape: keep existing calibration   Ctrl+x: stop audio"}
	}

	return dialog
}

//nolint:goconst // Keep each input owner's complete shortcut table readable with the same rendered key labels.
func (m *model) calibrationFooter() []string {
	if m.checkCalibrationDraft(m.autoGainDraft) != nil {
		return m.hintRows([]keyHint{{"Esc", "Keep calibration; reload and recalculate"}, {"x", hintStopAll}})
	}

	return m.hintRows([]keyHint{{"Enter", "Verify and apply auto-gain"}, {"Esc", "Keep calibration"}, {"x", hintStopAll}})
}

func (m *model) followAutoCalibration(recording *audio.Recording) bool {
	if recording.CalibrationMode() != audio.CalibrationAuto {
		return true
	}
	suggestion, err := m.calibrationSuggestion(*recording)
	if err != nil {
		m.report(err)

		return false
	}
	recording.CalibrationGainDB = &suggestion.GainDB

	return true
}

func (m *model) editCalibrationTarget() {
	if m.clip == nil || m.assigning || m.picker != nil {
		return
	}
	if m.doc.Recording(m.recordingID()).CalibrationMode() == audio.CalibrationManual {
		m.status = "Manual calibration is locked. Shift+A recalculates and enables AUTO first."

		return
	}
	options, err := m.doc.CalibrationOptionsFor(m.recordingID())
	if err != nil {
		m.report(err)
		return
	}
	target := options.TargetDB
	if saved := m.doc.Recording(m.recordingID()).CalibrationTargetDB; saved != nil {
		target = *saved
	}
	m.beginInput(fieldCalibrationTarget, strconv.FormatFloat(target, 'f', 1, 64))
}

func (m *model) setCalibrationTarget(value string) {
	target, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(target) || math.IsInf(target, 0) || target < -60 || target > -6 {
		m.rejectInput("Enter a target between -60 and -6 dB.")

		return
	}
	recording := m.doc.Recording(m.recordingID())
	recording.CalibrationTargetDB = &target
	if m.followAutoCalibration(&recording) {
		m.report(m.doc.SetRecording(m.recordingID(), recording))
		m.preview.SetGain(m.livePreviewGain())
		m.refreshMixGains()
	}
}

func (m *model) calibrationDetails(add func(string, string)) {
	recording := m.doc.Recording(m.recordingID())
	mode := recording.CalibrationMode()
	add("Calibration mode: ", mode)
	if mode == audio.CalibrationManual {
		add("Auto-gain: ", "Paused · manual override locked")
		add("Recalculate: ", "Shift+A · confirmation replaces the override")

		return
	}
	if m.clip == nil || m.picker != nil {
		add("Auto-gain: ", "Waiting for playable source")

		return
	}
	suggestion, err := m.calibrationSuggestion(recording)
	if err != nil {
		add("Auto-gain: ", "No suggestion · silent/too quiet region")

		return
	}
	add("Method: ", suggestion.Method)
	add("Measured / target: ", fmt.Sprintf("%.1f / %.1f dB", suggestion.MeasuredDB, suggestion.TargetDB))
	add("Suggested gain: ", fmt.Sprintf("%+.2f dB · Shift+A applies", suggestion.GainDB))
	add("Predicted sample peak: ", fmt.Sprintf("%.1f dBFS", suggestion.PeakDB+suggestion.GainDB))
	if suggestion.Limited {
		add("Headroom: ", "Limited by -3 dBFS peak ceiling or calibration gain range")
	}
	add("Target: ", "Shift+T edits · defaults are starting points, not listening approval")
}
