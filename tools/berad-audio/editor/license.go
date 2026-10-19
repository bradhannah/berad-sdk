package editorui

import "slices"

const fieldLicense = "license"

func (m *model) beginLicenseEdit() {
	if m.assigning || m.picker != nil || m.duplicate != nil {
		m.status = "Choose a recording in Recordings or Moments before editing its license."

		return
	}
	recordingID := m.recordingID()
	if !slices.Contains(m.doc.RecordingIDs(), recordingID) {
		m.status = "Select a recording to edit its license; empty recording slots are supported."

		return
	}
	m.licenseRecordingID = recordingID
	m.beginInput(fieldLicense, m.doc.Recording(recordingID).Provenance.License)
	m.inputError, m.statusIsError = "", false
}
