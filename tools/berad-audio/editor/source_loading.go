package editorui

import (
	"context"
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

type sourcePurpose uint8

const (
	sourceSelection sourcePurpose = iota
	sourceCandidate
	sourceVariant
)

type sourceLoad struct {
	recordingID, path string
	purpose           sourcePurpose
	autoplay          bool
	cueID             string
	variant           int
}

func (m *model) invalidateSource(discardLoaded bool) {
	if m.cancelLoad != nil {
		m.cancelLoad()
	}
	m.generation++
	m.loading, m.autoPreview = false, false
	if discardLoaded {
		m.clip, m.clipRecordingID, m.previewLabel = nil, "", ""
		m.previewRoute = nil
		m.previewPaused, m.sceneFXPreview, m.sourceStale = false, false, false
		m.report(m.preview.Close())
	}
}

func (m *model) loadSelection() tea.Cmd {
	m.statusIsError, m.routing.fading, m.fullSource = false, false, false
	path, err := m.doc.RecordingPath(m.recordingID())
	if err != nil {
		m.invalidateSource(true)
		m.report(err)
		if m.doc.FileAvailability(m.recordingID()).State == audio.RecordingNoFile {
			m.statusIsError = false
		}
		if m.selectedID() == "" {
			m.status = "No matches. / changes search; Ctrl+f changes the filter."
		} else if m.routingWorkspace() && m.routingCue() == "" {
			m.status = "No sound selected. f assigns; Enter previews."
		}

		return nil
	}

	return m.loadPath(path, false)
}

func (m *model) loadPath(path string, autoplay bool) tea.Cmd {
	purpose := sourceSelection
	if m.picker != nil {
		purpose = sourceCandidate
	}

	return m.loadSource(sourceLoad{recordingID: m.recordingID(), path: path, purpose: purpose,
		autoplay: autoplay, cueID: "", variant: 0})
}

func (m *model) loadSource(request sourceLoad) tea.Cmd {
	path, err := filepath.Abs(request.path)
	if err != nil {
		m.report(fmt.Errorf("resolve preview source: %w", err))

		return nil
	}
	request.path = path
	m.invalidateSource(request.purpose != sourceVariant)
	m.autoPreview, m.loading = request.autoplay, true
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelLoad = cancel
	generation := m.generation
	document := m.doc

	return func() tea.Msg {
		defer cancel()
		clip, err := document.LoadRecording(ctx, request.path)

		return loadedMsg{generation: generation, request: request, clip: clip, err: err}
	}
}

func (m *model) acceptsSource(request sourceLoad) bool {
	switch request.purpose {
	case sourceCandidate:
		return m.picker != nil && request.recordingID == m.recordingID() && request.path == m.pickerPath() &&
			(m.inputOwner() == ownerFiles || m.inputOwner() == ownerVolumes)
	case sourceVariant:
		return m.acceptsVariantSource(request)
	case sourceSelection:
		path, err := m.doc.RecordingPath(request.recordingID)

		return err == nil && request.recordingID == m.recordingID() && request.path == path
	}

	panic("invalid audio source request purpose")
}

func (m *model) acceptsVariantSource(request sourceLoad) bool {
	if !m.browsingCues() || request.cueID != m.selectedID() || !m.autoPreview ||
		(m.inputOwner() != ownerWorkspace && m.inputOwner() != ownerVolumes) {
		return false
	}
	ids := m.doc.Cue(request.cueID).Recordings()

	return request.variant >= 0 && request.variant < len(ids) && ids[request.variant] == request.recordingID
}

func (m *model) acceptLoaded(message loadedMsg) {
	if message.generation != m.generation {
		return
	}
	m.loading = false
	if !m.acceptsSource(message.request) {
		if m.crop != nil && m.crop.reloading {
			m.crop.reloading, m.crop.err = false, audio.ErrRecordingRegionStale
		}

		return
	}
	m.updateSelectedAvailability(message)
	if message.request.purpose == sourceVariant {
		m.acceptVariantSource(message)

		return
	}
	m.clip, m.clipRecordingID, m.sourceStale = message.clip, message.request.recordingID, false
	m.report(message.err)
	if message.err == nil && message.request.purpose == sourceSelection {
		m.refreshMixSource(message.request.recordingID, message.clip)
	}
	if m.crop != nil && m.crop.reloading {
		m.acceptCropSource(message.err)
	}
	if message.err == nil && message.request.autoplay && m.autoPreview {
		m.play()
	}
}

func (m *model) acceptVariantSource(message loadedMsg) {
	if message.err != nil {
		m.report(message.err)

		return // Retain the last successful variant and its transport.
	}
	request := message.request
	previous, previousID, previousIndex := m.clip, m.clipRecordingID, m.currentVariant[request.cueID]
	previousStale := m.sourceStale
	m.sourceStale = false
	m.clip, m.clipRecordingID, m.currentVariant[request.cueID] = message.clip, request.recordingID, request.variant
	if !m.play() {
		m.clip, m.clipRecordingID, m.currentVariant[request.cueID] = previous, previousID, previousIndex
		m.sourceStale = previousStale
	}
}

func (m *model) acceptCropSource(loadErr error) {
	m.crop.reloading = false
	if loadErr != nil {
		m.crop.err, m.sourceStale = loadErr, true

		return
	}
	draft, err := m.doc.BeginRecordingRegion(m.crop.recordingID, m.clip)
	m.crop.err = err
	if err != nil {
		m.report(err)

		return
	}
	m.crop.draft, m.crop.source = draft, m.clip
	m.crop.rebased = true
	m.crop.fullInitially = m.doc.Recording(m.crop.recordingID).PlaybackWindow == nil
	m.crop.revision, m.crop.uses = m.doc.Revision(), len(m.doc.Usages(m.crop.recordingID))
	m.scheduleCropPreview()
	m.status = "Source reloaded; review and audition retained draft bounds before applying."
}
