package authoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"strings"
)

// ErrAudioLibrary is the schema-3 compatibility identity for invalid library data.
var ErrAudioLibrary = ErrLibrary

// LibraryDocument is the detached schema-3 compatibility document. Routing wire
// names are retained; their labels, target identities and policies are consumer-owned.
type LibraryDocument = audioLibrary

// LibraryValidator adds deterministic consumer completeness/vocabulary checks.
// It receives a detached value and cannot mutate the authoritative document.
type LibraryValidator func(LibraryDocument) error

// ParseDocument validates one schema-3 document without opening its files or an
// audio device. The returned maps belong to the caller; validator inputs are
// detached separately. Disk repair and immutable game loading share this parser.
func ParseDocument(data []byte, validate LibraryValidator) (LibraryDocument, error) {
	document, err := parseLibraryDocument(data, validate)
	if err != nil {
		return LibraryDocument{}, err
	}

	return *document, nil
}

type audioLibrary struct {
	SchemaVersion int                         `json:"schemaVersion"`
	Recordings    map[string]libraryRecording `json:"recordings"`
	Cues          map[string]libraryCue       `json:"cues"`
	Taxonomy      Taxonomy                    `json:"taxonomy"`
	Routing       Routing                     `json:"routing"`
}

type libraryRecording struct {
	File                *string             `json:"file"`
	CalibrationGainDB   *float64            `json:"calibrationGainDb,omitempty"`
	CalibrationPolicy   string              `json:"calibrationMode,omitempty"`
	CalibrationTargetDB *float64            `json:"calibrationTargetDb,omitempty"`
	PlaybackWindow      *PlaybackWindow     `json:"playbackWindow,omitempty"`
	Provenance          RecordingProvenance `json:"provenance"`
	Note                string              `json:"note,omitempty"`
	SearchTerms         []string            `json:"searchTerms,omitempty"`
}

type libraryCue struct {
	Name           string   `json:"name"`
	RecordingID    string   `json:"recordingId,omitempty"`
	RecordingIDs   []string `json:"recordingIds,omitempty"`
	Selection      string   `json:"selection,omitempty"`
	VolumeModifier float64  `json:"volumeModifier"`
	HowToHear      string   `json:"howToHear"`
	SearchTerms    []string `json:"searchTerms,omitempty"`
}

func parseLibraryDocument(data []byte, validate LibraryValidator) (*audioLibrary, error) {
	if err := checkLibraryJSON(data); err != nil {
		return nil, err
	}
	var library audioLibrary
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&library); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", ErrAudioLibrary, err)
	}
	if library.SchemaVersion != 3 || library.Recordings == nil || library.Cues == nil {
		return nil, fmt.Errorf("%w: require schemaVersion 3, recordings, cues, taxonomy, and routing", ErrAudioLibrary)
	}
	for id, recording := range library.Recordings {
		if err := validateRecordingProvenance(id, recording.Provenance); err != nil {
			return nil, err
		}
		if err := validateLibraryRecordingMetadata(id, recording); err != nil {
			return nil, err
		}
	}
	if err := validateCoreLibraryCues(&library); err != nil {
		return nil, err
	}
	if err := library.Taxonomy.validateStructure(&library); err != nil {
		return nil, err
	}
	if err := validateRoutingStructure(&library); err != nil {
		return nil, err
	}
	if validate != nil {
		if err := validate(*cloneAudioValue(&library)); err != nil {
			return nil, err
		}
	}

	return &library, nil
}

func validateLibraryRecordingMetadata(recordingID string, recording libraryRecording) error {
	if err := validateSearchTerms(recording.SearchTerms); err != nil {
		return fmt.Errorf("recording %s: %w", recordingID, err)
	}
	if err := recording.validateCalibration(); err != nil {
		return fmt.Errorf("recording %s: %w", recordingID, err)
	}
	if recordingID == "" || strings.Trim(recordingID, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
		return fmt.Errorf("%w: invalid recording ID %q; use lowercase snake_case", ErrAudioLibrary, recordingID)
	}
	if recording.File == nil {
		if recording.PlaybackWindow != nil || recording.CalibrationGainDB != nil {
			return fmt.Errorf("%w: empty slot %s cannot have an excerpt or accepted calibration", ErrAudioLibrary, recordingID)
		}

		return nil
	}
	path := *recording.File
	if !validAssetPath(path) {
		return fmt.Errorf("%w: %s requires an exact relative audio file, got %q", ErrAudioLibrary, recordingID, path)
	}
	if window := recording.PlaybackWindow; window != nil {
		if window.StartMs < 0 || window.EndMs <= window.StartMs {
			return fmt.Errorf("%w: recording %s has invalid excerpt bounds", ErrAudioLibrary, recordingID)
		}
	}

	return nil
}

func validateCoreLibraryCues(library *audioLibrary) error {
	for cueID, cue := range library.Cues {
		if err := validateSearchTerms(cue.SearchTerms); err != nil {
			return fmt.Errorf("cue %s: %w", cueID, err)
		}
		if !fs.ValidPath(cueID) || strings.Contains(cueID, "\\") {
			return fmt.Errorf("%w: invalid cue identity %q", ErrAudioLibrary, cueID)
		}
		if err := validateCueRecordings(library, cueID, cue); err != nil {
			return err
		}
		if strings.TrimSpace(cue.Name) == "" || strings.TrimSpace(cue.HowToHear) == "" ||
			math.IsNaN(cue.VolumeModifier) || math.IsInf(cue.VolumeModifier, 0) ||
			cue.VolumeModifier < 0 || cue.VolumeModifier > 1 {
			return fmt.Errorf("%w: cue %s requires name, howToHear, and a gain in [0,1]", ErrAudioLibrary, cueID)
		}
	}

	return nil
}

// checkLibraryJSON rejects duplicate keys, trailing JSON, and accidentally
// omitted file fields. A null file is deliberate; a misspelled field is not.
func checkLibraryJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueJSONValue(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrAudioLibrary, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing JSON data", ErrAudioLibrary)
	}
	var fields struct {
		Recordings map[string]map[string]json.RawMessage `json:"recordings"`
		Cues       map[string]map[string]json.RawMessage `json:"cues"`
		Routing    map[string]json.RawMessage            `json:"routing"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("%w: inspect file fields: %w", ErrAudioLibrary, err)
	}
	if fields.Routing == nil {
		return fmt.Errorf("%w: require an explicit routing object (empty is allowed)", ErrAudioLibrary)
	}
	for id, fields := range fields.Recordings {
		if _, explicit := fields["file"]; !explicit {
			return fmt.Errorf("%w: recording %s requires explicit file (null is allowed)", ErrAudioLibrary, id)
		}
	}
	for id, fields := range fields.Cues {
		gain, explicit := fields["volumeModifier"]
		if !explicit || bytes.Equal(bytes.TrimSpace(gain), []byte("null")) {
			return fmt.Errorf("%w: cue %s requires explicit volumeModifier", ErrAudioLibrary, id)
		}
	}

	return nil
}

func uniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read JSON value: %w", err)
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	seen := make(map[string]bool)
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("read JSON key: %w", err)
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("%w: duplicate or invalid key %v", ErrAudioLibrary, key)
			}
			seen[name] = true
		}
		if err := uniqueJSONValue(decoder); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("close JSON container: %w", err)
	}

	return nil
}
