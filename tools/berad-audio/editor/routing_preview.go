package editorui

import (
	"time"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

func (m *model) playRouting() bool {
	if !m.selectedClipReady() {
		m.status = "No playable recording selected. Open the Moment to assign/import its recording."

		return false
	}
	cue := m.activeCue()
	profile, ambient := m.previewAmbientProfile()
	gain := m.gain()
	m.routing.envelope, m.routing.fading = 1, false
	if ambient && profile.FadeInMs > 0 {
		m.routing.envelope = 0
	}
	if ambient {
		gain *= audio.QuadraticDistanceGain(m.routing.distance, profile.RadiusTiles) * m.routing.envelope
	}
	if err := m.preview.Play(m.clip, m.doc.Recording(m.recordingID()).PlaybackWindow, ambient, gain); err != nil {
		m.report(err)

		return false
	}
	m.sceneFXPreview = false
	m.previewPaused, m.previewLoop = false, ambient
	m.previewEndMs = m.window().EndMs
	m.previewLabel = m.doc.Cue(cue).Name
	if m.previewLabel == "" {
		m.previewLabel = m.recordingID()
	}
	if m.workspace == workspaceTiles && m.routing.menu == nil && m.form == nil {
		if m.routing.footsteps {
			m.previewLabel += " · footstep"
		} else {
			m.previewLabel += " · ambient"
		}
	}
	if m.form != nil && m.form.previewProfile != nil {
		m.previewLabel += " · settings draft"
	}
	if ambient {
		m.startRoutingFade(1, profile.FadeInMs)
	}
	m.status = "Auditioning " + m.doc.Cue(cue).Name + " · preview state is temporary"

	return true
}

func (m *model) startRoutingFade(to float64, duration int64) {
	m.routing.fadeStart = m.now()
	m.routing.fadeFrom, m.routing.fadeTo = m.routing.envelope, to
	m.routing.fadeMs, m.routing.fading = duration, true
}

func (m *model) leaveRoutingRange() {
	if profile, exists := m.previewAmbientProfile(); exists && m.preview.Active() {
		m.startRoutingFade(0, profile.FadeOutMs)
	}
}

func (m *model) updateRoutingPreview() {
	if !m.routing.fading || !m.preview.Playing() {
		return
	}
	progress := 1.0
	if m.routing.fadeMs > 0 {
		duration := time.Duration(m.routing.fadeMs) * time.Millisecond
		progress = min(1, max(0, float64(m.now().Sub(m.routing.fadeStart))/float64(duration)))
	}
	m.routing.envelope = m.routing.fadeFrom + (m.routing.fadeTo-m.routing.fadeFrom)*progress
	m.refreshRoutingPreviewGain()
	if progress == 1 {
		m.routing.fading = false
		if m.routing.fadeTo == 0 {
			m.report(m.preview.Close())
		}
	}
}

func (m *model) refreshRoutingPreviewGain() {
	if !m.preview.Active() {
		return
	}
	gain := m.gain()
	if profile, exists := m.previewAmbientProfile(); exists {
		gain *= audio.QuadraticDistanceGain(m.routing.distance, profile.RadiusTiles) * m.routing.envelope
	}
	m.preview.SetGain(gain)
}
