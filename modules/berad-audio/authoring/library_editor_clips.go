package authoring

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// NextRecordingID preserves suffix padding and skips every existing identity.
func (e *LibraryEditor) NextRecordingID(sourceID string) (string, error) {
	if _, exists := e.library.Recordings[sourceID]; !exists {
		return "", fmt.Errorf("%w: unknown recording %s", ErrAudioLibrary, sourceID)
	}
	const defaultDigits = 2
	const firstDuplicate = 2
	base, digits, next := sourceID, defaultDigits, uint64(firstDuplicate)
	if split := strings.LastIndex(sourceID, "_"); split >= 0 {
		suffix := sourceID[split+1:]
		if number, err := strconv.ParseUint(suffix, 10, 64); err == nil {
			if number == ^uint64(0) {
				return "", fmt.Errorf("%w: recording suffix is too large", ErrAudioLibrary)
			}
			base, digits, next = sourceID[:split], len(suffix), number+1
		}
	}
	for {
		candidate := fmt.Sprintf("%s_%0*d", base, digits, next)
		if _, exists := e.library.Recordings[candidate]; !exists {
			return candidate, nil
		}
		if next == ^uint64(0) {
			return "", fmt.Errorf("%w: recording suffix is too large", ErrAudioLibrary)
		}
		next++
	}
}

// AppendCueRecording adds one explicit cycle member, retaining the existing order.
func (e *LibraryEditor) AppendCueRecording(cueID, recordingID string) error {
	cue, exists := e.library.Cues[cueID]
	if !exists {
		return fmt.Errorf("%w: unknown cue %s", ErrAudioLibrary, cueID)
	}
	ids := cue.Recordings()
	if slices.Contains(ids, recordingID) {
		return fmt.Errorf("%w: %s is already assigned", ErrAudioLibrary, recordingID)
	}

	return e.SetCue(cueID, cue.WithRecordings(append(ids, recordingID)))
}

// SetCueRecordings changes ordering or membership through production validation.
func (e *LibraryEditor) SetCueRecordings(cueID string, ids []string) error {
	return e.SetCue(cueID, e.Cue(cueID).WithRecordings(ids))
}
