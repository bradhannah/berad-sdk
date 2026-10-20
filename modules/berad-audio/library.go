package audio

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"slices"
	"strings"
	"time"
)

// ErrInvalidLibrary identifies invalid playback data or decoded format.
var ErrInvalidLibrary = errors.New("invalid audio library")

// CueID is an application-owned semantic playback identity.
type CueID string

// RecordingID identifies a source/clip independently of a cue assignment.
type RecordingID string

// Window selects source-relative audio; End is exclusive. Nil selects all audio.
type Window struct{ Start, End time.Duration }

// Recording names an exact asset. Empty File is an intentional silent slot.
// CalibrationDB is an amplitude correction, restricted to [-48,12] dB.
type Recording struct {
	File          string
	CalibrationDB float64
	Window        *Window
}

// Cue selects one recording or cycles an ordered nonempty list. Only admitted
// requests advance selection. Gain is a linear coefficient in [0,1].
type Cue struct {
	Recordings []RecordingID
	Gain       float64
}

// Definition contains playback data only; callers own authoring metadata/routing.
// It is not a serialized file format or a schema migration of Redux's manifest.
type Definition struct {
	Recordings map[RecordingID]Recording
	Cues       map[CueID]Cue
}

// Library holds frozen metadata and one encoded snapshot per unique asset path.
// Decoded streams belong to individual voices, not to the library.
type Library struct {
	definition Definition
	assets     map[string][]byte
	bytes      int64
}

// NewLibrary freezes files below the supplied fs.FS root. maxBytes is a required
// positive encoded-payload budget (unique paths); it is not a total heap limit.
func NewLibrary(definition Definition, files fs.FS, maxBytes int64) (*Library, error) {
	if files == nil || maxBytes <= 0 {
		return nil, fmt.Errorf("%w: filesystem and positive encoded byte budget required", ErrInvalidLibrary)
	}
	frozen := cloneDefinition(definition)
	if err := validateDefinition(frozen); err != nil {
		return nil, err
	}
	library := &Library{definition: frozen, assets: make(map[string][]byte), bytes: 0}
	for _, recording := range frozen.Recordings {
		if recording.File == "" {
			continue
		}
		if _, exists := library.assets[recording.File]; exists {
			continue
		}
		data, err := readAsset(files, recording.File, maxBytes-library.bytes)
		if err != nil {
			return nil, err
		}
		library.assets[recording.File] = data
		library.bytes += int64(len(data))
	}

	return library, nil
}

// EncodedBytes excludes decoder, backend and PCM buffers.
func (library *Library) EncodedBytes() int64 { return library.bytes }

func cloneDefinition(source Definition) Definition {
	result := Definition{Recordings: make(map[RecordingID]Recording), Cues: make(map[CueID]Cue)}
	for id, recording := range source.Recordings {
		if recording.Window != nil {
			recording.Window = new(*recording.Window)
		}
		result.Recordings[id] = recording
	}
	for id, cue := range source.Cues {
		cue.Recordings = slices.Clone(cue.Recordings)
		result.Cues[id] = cue
	}

	return result
}

func validateDefinition(definition Definition) error {
	for recordingID, recording := range definition.Recordings {
		if recordingID == "" || !finite(recording.CalibrationDB) ||
			recording.CalibrationDB < -48 || recording.CalibrationDB > 12 {
			return fmt.Errorf("%w: recording %q has invalid identity/calibration", ErrInvalidLibrary, recordingID)
		}
		if recording.File != "" && (!fs.ValidPath(recording.File) || strings.Contains(recording.File, "\\")) {
			return fmt.Errorf("%w: recording %s requires an exact relative asset path", ErrInvalidLibrary, recordingID)
		}
		if window := recording.Window; window != nil &&
			(recording.File == "" || window.Start < 0 || window.End <= window.Start) {
			return fmt.Errorf("%w: recording %s has invalid playback window", ErrInvalidLibrary, recordingID)
		}
	}

	return validateCues(definition)
}

func validateCues(definition Definition) error {
	for cueID, cue := range definition.Cues {
		if cueID == "" || !unitGain(cue.Gain) || len(cue.Recordings) == 0 {
			return fmt.Errorf("%w: cue %q requires identity, recordings and gain in [0,1]", ErrInvalidLibrary, cueID)
		}
		for _, recordingID := range cue.Recordings {
			recording, exists := definition.Recordings[recordingID]
			if !exists || (len(cue.Recordings) > 1 && recording.File == "") {
				return fmt.Errorf("%w: cue %s has unknown/empty cycle recording %s", ErrInvalidLibrary, cueID, recordingID)
			}
		}
	}

	return nil
}

func readAsset(files fs.FS, name string, remaining int64) ([]byte, error) {
	file, err := files.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open recording %s: %w", name, err)
	}
	// Avoid overflow even when the caller deliberately supplies MaxInt64.
	limit := remaining
	if limit < math.MaxInt64 {
		limit++
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, fmt.Errorf("read recording %s: %w", name, err)
	}
	if int64(len(data)) > remaining {
		return nil, fmt.Errorf("%w: encoded byte budget exceeded by %s", ErrInvalidLibrary, name)
	}

	return data, nil
}

func finite(value float64) bool   { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func unitGain(value float64) bool { return finite(value) && value >= 0 && value <= 1 }
