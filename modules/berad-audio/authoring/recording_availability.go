package authoring

import (
	"context"
	"errors"
	"os"
)

// RecordingState describes technical availability, not perceptual acceptance.
type RecordingState string

// Availability states never imply that a recording has passed listening review.
const (
	RecordingChecking RecordingState = "CHECKING"
	RecordingReady    RecordingState = "READY"
	RecordingNoFile   RecordingState = "NO FILE"
	RecordingMissing  RecordingState = "MISSING"
	RecordingInvalid  RecordingState = "INVALID"
)

// RecordingAvailability keeps file presence separate from playback validity.
type RecordingAvailability struct {
	State  RecordingState
	Exists bool
	Reason string
}

// FileAvailability performs a cheap filesystem check before background decoding.
func (e *LibraryEditor) FileAvailability(id string) RecordingAvailability {
	path, err := e.RecordingPath(id)
	if err != nil {
		return RecordingAvailability{State: RecordingNoFile, Exists: false,
			Reason: "No audio file assigned. Choose a file with f."}
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return RecordingAvailability{State: RecordingMissing, Exists: false, Reason: "Assigned file is missing: " + path}
	}
	if err != nil {
		return RecordingAvailability{State: RecordingInvalid, Exists: false, Reason: err.Error()}
	}
	if info.IsDir() {
		return RecordingAvailability{State: RecordingInvalid, Exists: true,
			Reason: "Assigned path is a directory, not audio."}
	}

	return RecordingAvailability{State: RecordingChecking, Exists: true, Reason: "Checking audio and selection bounds…"}
}

// InspectRecording validates actual decoding and bounds without opening a player.
// The caller can cancel it when the document or file selection changes.
// Cancellation or deadline expiry leaves the result CHECKING, never INVALID.
func (e *LibraryEditor) InspectRecording(ctx context.Context, recordingID string) RecordingAvailability {
	status := e.FileAvailability(recordingID)
	if status.State != RecordingChecking {
		return status
	}
	path, err := e.RecordingPath(recordingID)
	if err != nil {
		panic(err) // FileAvailability already resolved this immutable document entry.
	}
	clip, err := LoadRecordingContext(ctx, path, e.decode)
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status
	}
	if err == nil {
		var stream *Stream
		stream, err = clip.Stream(e.library.Recordings[recordingID].PlaybackWindow, false)
		if err == nil {
			err = stream.Close()
		}
	}
	if ctx.Err() != nil {
		return status
	}
	if err != nil {
		status.State, status.Reason = RecordingInvalid, err.Error()
		if errors.Is(err, os.ErrNotExist) {
			status.State, status.Exists = RecordingMissing, false
		}

		return status
	}

	return RecordingAvailability{State: RecordingReady, Exists: true,
		Reason: "Playable recording; listening review is separate."}
}
