package authoring

import (
	"fmt"
	"strings"
)

const provenanceUnknown = "unknown"

// RecordingProvenance is authored with the recording in library.json. A source
// credit, an import location, and a conversion performed by the project author
// are distinct facts; none establishes authorship or licensing of the others.
// CommercialUse, from schema 4, records the author's judgement that the
// recording may be used in a commercial game; it is required in schema 4 and
// absent in schema 3. The library records the judgement; it verifies no licence.
type RecordingProvenance struct {
	OriginalFilename string           `json:"originalFilename"`
	SourceType       string           `json:"sourceType"`
	SourceURL        string           `json:"sourceUrl"`
	SourceCreator    string           `json:"sourceCreator"`
	License          string           `json:"license"`
	ImportedFrom     string           `json:"importedFrom"`
	Processing       []ProcessingStep `json:"processing"`
	CommercialUse    *bool            `json:"commercialUse,omitempty"`
}

// ProcessingStep records a known edit or conversion without attributing
// the underlying composition or recording to the person who processed it.
type ProcessingStep struct {
	Description string `json:"description"`
	Tool        string `json:"tool"`
	PerformedBy string `json:"performedBy"`
}

func unknownRecordingProvenance() RecordingProvenance {
	return RecordingProvenance{
		OriginalFilename: provenanceUnknown, SourceType: provenanceUnknown,
		SourceURL: provenanceUnknown, SourceCreator: provenanceUnknown,
		License: provenanceUnknown, ImportedFrom: provenanceUnknown,
		Processing: []ProcessingStep{}, CommercialUse: nil,
	}
}

// CommercialUseAllowed reports whether the author cleared the recording for a
// commercial game. Unrecorded, as in schema 3, is not cleared.
func (provenance RecordingProvenance) CommercialUseAllowed() bool {
	return provenance.CommercialUse != nil && *provenance.CommercialUse
}

func validateCommercialUse(schema int, recordingID string, provenance RecordingProvenance) error {
	if schema >= LibrarySchemaFour && provenance.CommercialUse == nil {
		return fmt.Errorf("%w: %s provenance.commercialUse is required from schema 4", ErrAudioLibrary, recordingID)
	}
	if schema < LibrarySchemaFour && provenance.CommercialUse != nil {
		return fmt.Errorf("%w: %s provenance.commercialUse needs schema 4", ErrAudioLibrary, recordingID)
	}

	return nil
}

func validateRecordingProvenance(recordingID string, provenance RecordingProvenance) error {
	fields := []struct{ name, value string }{
		{"originalFilename", provenance.OriginalFilename}, {"sourceType", provenance.SourceType},
		{"sourceUrl", provenance.SourceURL}, {"sourceCreator", provenance.SourceCreator},
		{"license", provenance.License}, {"importedFrom", provenance.ImportedFrom},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%w: %s provenance.%s must be explicit (use unknown)", ErrAudioLibrary, recordingID, field.name)
		}
	}
	if provenance.Processing == nil {
		return fmt.Errorf("%w: %s provenance.processing must be an array (empty is allowed)", ErrAudioLibrary, recordingID)
	}
	for _, step := range provenance.Processing {
		if strings.TrimSpace(step.Description) == "" || strings.TrimSpace(step.Tool) == "" ||
			strings.TrimSpace(step.PerformedBy) == "" {
			return fmt.Errorf("%w: %s processing steps require description, tool, and performedBy", ErrAudioLibrary, recordingID)
		}
	}

	return nil
}
