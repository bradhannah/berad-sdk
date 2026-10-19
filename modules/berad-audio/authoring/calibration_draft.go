package authoring

import (
	"context"
	"errors"
)

// CalibrationDraft binds a proposal to a document revision and exact immutable
// source. UI selection/request generations remain caller-owned; callers discard
// approvals when their selected source changes, even if the document did not.
type CalibrationDraft struct {
	region     *RecordingRegionDraft
	suggestion CalibrationSuggestion
}

// BeginCalibration computes a proposal without mutating metadata, even when the
// current recording has a manual calibration. Applying requires content proof.
func (e *LibraryEditor) BeginCalibration(recordingID string, source *RecordingAudio) (*CalibrationDraft, error) {
	region, err := e.BeginRecordingRegion(recordingID, source)
	if err != nil {
		return nil, err
	}
	suggestion, err := source.SuggestCalibration(region.recording.PlaybackWindow, region.calibration)
	if err != nil {
		return nil, err
	}
	return &CalibrationDraft{region: region, suggestion: suggestion}, nil
}

// Suggestion is detached and can be rendered or used for audition without I/O.
func (draft *CalibrationDraft) Suggestion() CalibrationSuggestion { return draft.suggestion }

// ApplyCalibration verifies source bytes and publishes exactly the proposed
// calibration and AUTO ownership as one edit. Cancellation authors nothing.
func (e *LibraryEditor) ApplyCalibration(ctx context.Context, draft *CalibrationDraft) error {
	if draft == nil || draft.region == nil {
		return ErrDocumentStale
	}
	if err := e.CheckRegionRevision(draft.region); err != nil {
		return errors.Join(ErrDocumentStale, err)
	}
	if err := draft.region.source.VerifySource(ctx); err != nil {
		return err
	}
	next := cloneAudioValue(e.library)
	recording := draft.region.recording
	recording.CalibrationGainDB = &draft.suggestion.GainDB
	recording.CalibrationPolicy = CalibrationAuto
	next.Recordings[draft.region.recordingID] = recording
	return e.replace(next)
}
