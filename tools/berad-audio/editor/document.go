package editorui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func (m *model) documentOperation(reload bool, summary string, apply func(*audio.LibraryEditor) error) tea.Cmd {
	job, err := m.operations.Begin(m.doc)
	if err != nil {
		m.report(err)

		return nil
	}
	m.documentGeneration = job.Generation()
	m.busy = true

	return func() tea.Msg {
		result := job.Run(apply)

		return documentMsg{result: result, generation: result.Generation(), baseRevision: result.Revision(),
			document: result.Document(), err: result.Error(), summary: summary, reload: reload, calibration: nil}
	}
}

func (m *model) acceptDocument(message documentMsg) tea.Cmd {
	if !m.busy || message.generation != m.documentGeneration {
		return nil
	}
	var approval error
	if message.calibration != nil {
		approval = m.checkCalibrationDraft(message.calibration)
	}
	completion := m.operations.Complete(m.doc, message.result, approval)
	if completion.Ignored {
		return nil
	}
	m.busy = false
	cropCommit := m.crop != nil && m.crop.applying
	if cropCommit {
		m.crop.applying = false
	}
	if completion.Stale {
		m.quitAfterOperation = false
		m.statusIsError = true
		m.status = "Library changed during the file operation; current draft retained. File outcome kept; review Details."
		m.report(completion.Err)

		return nil
	}
	if approval != nil {
		m.quitAfterOperation = false
		m.report(completion.Err)

		return nil
	}
	publicationChanged := completion.PublicationChanged
	m.doc = completion.Document
	message.err = completion.Err
	if message.err != nil {
		return m.failedDocument(message, cropCommit, publicationChanged)
	}

	return m.completeDocument(message, cropCommit, publicationChanged)
}

func (m *model) completeDocument(message documentMsg, cropCommit, publicationChanged bool) tea.Cmd {
	m.status, m.statusIsError = message.summary, false
	if message.calibration != nil {
		m.autoGainDraft = nil
		m.preview.SetGain(m.livePreviewGain())
		m.refreshMixGains()
	}
	selected := m.selectedID()
	m.refreshKeys()
	var refresh tea.Cmd
	switch {
	case cropCommit:
		refresh = m.finishCropCommit()
	case message.reload:
		refresh = m.recheckFiles()
	case selected != m.selectedID():
		refresh = m.loadSelection()
	}
	if m.quitAfterOperation {
		m.quitAfterOperation = false
		if quit := m.quit(); quit != nil {
			return quit
		}
	}
	if publicationChanged {
		if !m.statusIsError {
			m.status = message.summary
		}
		m.appendPublicationReceipt()
	}

	return refresh
}

func (m *model) failedDocument(message documentMsg, cropCommit, publicationChanged bool) tea.Cmd {
	if cropCommit && errors.Is(message.err, audio.ErrRecordingSourceChanged) {
		m.rejectSourceSnapshot(m.crop.source, message.err)
	}
	if message.calibration != nil && errors.Is(message.err, audio.ErrRecordingSourceChanged) {
		m.rejectSourceSnapshot(message.calibration.source, message.err)
	}
	m.quitAfterOperation = false
	var reload tea.Cmd
	if message.reload {
		reload = m.loadSelection()
	}
	m.report(message.err)
	if publicationChanged {
		m.appendPublicationReceipt()
	}

	return reload
}

func (m *model) appendPublicationReceipt() {
	m.status += publicationReceiptText(m.doc.LastPublication(), m.doc.NeedsSync())
}

func publicationReceiptText(receipt audio.PublicationOutcome, needsSync bool) string {
	text := ""
	if receipt.Import && receipt.Created {
		text += " File retained: " + receipt.Path
		if !receipt.Assigned {
			text += " (not assigned to the requested recording; retry import)."
		}
	}
	if needsSync {
		text += " Durability is unconfirmed; Ctrl+s retries the flush."
	} else if !receipt.DirectorySyncSupported && receipt.State == audio.PublicationComplete {
		text += " Directory durability is limited by this platform."
	}

	return text
}
