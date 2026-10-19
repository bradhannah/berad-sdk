package authoring

import (
	"fmt"
	"strings"
)

const provenanceUnknown = "unknown"

// RecordingProvenance is authored with the recording in library.json. A source
// credit, an import location, and a conversion performed by the project author
// are distinct facts; none establishes authorship or licensing of the others.
type RecordingProvenance struct {
	OriginalFilename string           `json:"originalFilename"`
	SourceType       string           `json:"sourceType"`
	SourceURL        string           `json:"sourceUrl"`
	SourceCreator    string           `json:"sourceCreator"`
	License          string           `json:"license"`
	ImportedFrom     string           `json:"importedFrom"`
	Processing       []ProcessingStep `json:"processing"`
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
		Processing: []ProcessingStep{},
	}
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
