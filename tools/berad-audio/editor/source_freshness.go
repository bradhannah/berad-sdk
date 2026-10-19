package editorui

import (
	"context"
	"errors"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const sourceProbeInterval = time.Second

type sourceProbeResult struct {
	clip *audio.RecordingAudio
	err  error
}

type sourceProbeMsg []sourceProbeResult

// Probe loaded/playing snapshots off the input loop. A cheap stamp detects
// ordinary replacement; Apply's content verification also catches stamp reuse.
func (m *model) queueSourceProbe() tea.Cmd {
	if m.busy || m.sourceProbePending || m.now().Before(m.nextSourceProbe) {
		return nil
	}
	seen := make(map[*audio.RecordingAudio]bool)
	var sources []*audio.RecordingAudio
	add := func(clip *audio.RecordingAudio) {
		if clip != nil && clip.Path() != "" && !seen[clip] {
			seen[clip] = true
			sources = append(sources, clip)
		}
	}
	if !m.sourceStale {
		add(m.clip)
	}
	for _, layer := range m.mix {
		if !layer.stopped {
			add(layer.clip)
		}
	}
	if len(sources) == 0 {
		return nil
	}
	m.sourceProbePending, m.nextSourceProbe = true, m.now().Add(sourceProbeInterval)
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelSourceProbe = cancel

	return func() tea.Msg {
		defer cancel()
		results := make(sourceProbeMsg, 0, len(sources))
		for _, source := range sources {
			if ctx.Err() != nil {
				break
			}
			results = append(results, sourceProbeResult{clip: source, err: source.CheckSourceStamp()})
		}

		return results
	}
}

func (m *model) acceptSourceProbe(results sourceProbeMsg) {
	m.sourceProbePending = false
	for _, result := range results {
		if result.err == nil {
			continue
		}
		m.rejectSourceSnapshot(result.clip, result.err)
	}
}

func (m *model) rejectSourceSnapshot(clip *audio.RecordingAudio, failure error) {
	affected := m.clip == clip
	if draft := m.autoGainDraft; draft != nil && draft.source == clip {
		draft.err = failure
		affected = true
	}
	if m.clip == clip {
		m.sourceStale = true
		m.markSourceAvailability(m.clipRecordingID, clip, failure)
		if m.crop != nil {
			m.crop.err = failure
			m.suspendCropPreview()
		} else {
			m.report(m.preview.Close())
		}
	}
	for _, layer := range m.mix {
		if layer.clip == clip {
			affected, layer.stopped = true, true
			m.markSourceAvailability(layer.recordingID, clip, failure)
			m.report(layer.voice.Close())
		}
	}
	if affected {
		m.report(failure)
	}
}

func (m *model) markSourceAvailability(recordingID string, clip *audio.RecordingAudio, failure error) {
	path, err := m.doc.RecordingPath(recordingID)
	if err != nil || path != clip.Path() {
		return // An external candidate must not invalidate its destination's assigned file.
	}
	status := audio.RecordingAvailability{State: audio.RecordingInvalid, Exists: true, Reason: failure.Error()}
	if errors.Is(failure, os.ErrNotExist) {
		status.State, status.Exists = audio.RecordingMissing, false
	}
	m.availability[recordingID] = status
}

func (m *model) refreshMixSource(recordingID string, clip *audio.RecordingAudio) {
	for _, layer := range m.mix {
		if layer.recordingID != recordingID {
			continue
		}
		if layer.clip.Fingerprint() != clip.Fingerprint() {
			layer.stopped = true
			m.report(layer.voice.Close())
		}
		layer.clip = clip
	}
}
