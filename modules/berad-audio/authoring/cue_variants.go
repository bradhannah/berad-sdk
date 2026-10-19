package authoring

import (
	"fmt"
	"slices"
)

// SelectionCycle selects the next recording after each successful cue start.
const SelectionCycle = "cycle"

// Recordings returns a detached ordered list for a single assignment or cycle.
func (cue libraryCue) Recordings() []string {
	if len(cue.RecordingIDs) > 0 {
		return slices.Clone(cue.RecordingIDs)
	}
	if cue.RecordingID == "" {
		return nil
	}

	return []string{cue.RecordingID}
}

// PrimaryRecording is the first clip shown before an audition cycle starts.
func (cue libraryCue) PrimaryRecording() string {
	if len(cue.RecordingIDs) > 0 {
		return cue.RecordingIDs[0]
	}

	return cue.RecordingID
}

// WithRecordings returns a single assignment or an explicit ordered cycle.
func (cue libraryCue) WithRecordings(ids []string) libraryCue {
	cue.RecordingID, cue.RecordingIDs, cue.Selection = "", nil, ""
	if len(ids) == 1 {
		cue.RecordingID = ids[0]
	} else {
		cue.RecordingIDs = slices.Clone(ids)
		cue.Selection = SelectionCycle
	}

	return cue
}

func validateCueRecordings(library *audioLibrary, cueID string, cue libraryCue) error {
	if len(cue.RecordingIDs) > 0 {
		if cue.RecordingID != "" || len(cue.RecordingIDs) < 2 ||
			cue.Selection != SelectionCycle {
			return fmt.Errorf("%w: %s requires either recordingId or a cycle of at least two clips", ErrAudioLibrary, cueID)
		}
	} else if cue.RecordingID == "" || cue.Selection != "" {
		return fmt.Errorf("%w: %s requires one recordingId or an ordered cycle", ErrAudioLibrary, cueID)
	}
	seen := make(map[string]bool)
	for _, recordingID := range cue.Recordings() {
		recording, exists := library.Recordings[recordingID]
		if !exists || seen[recordingID] {
			return fmt.Errorf("%w: %s has unknown or duplicate recording %q", ErrAudioLibrary, cueID, recordingID)
		}
		if cue.Selection == SelectionCycle && recording.File == nil {
			return fmt.Errorf("%w: %s cycle member %s needs an assigned file", ErrAudioLibrary, cueID, recordingID)
		}
		seen[recordingID] = true
	}

	return nil
}
