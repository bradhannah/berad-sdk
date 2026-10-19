package authoring

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
)

// ErrRecordingRegionStale rejects an edit reviewed against another document.
var ErrRecordingRegionStale = errors.New("recording region draft is stale")

// RecordingRegionDraft binds an immutable source and authored settings. Bounds
// remain caller-owned draft values; Preview is pure and never reads the disk.
type RecordingRegionDraft struct {
	recordingID string
	revision    uint64
	source      *RecordingAudio
	recording   Recording
	family      string
	calibration CalibrationOptions
}

// RegionPreview is the exact region/calibration used by both audition and commit.
// Window is detached; nil means the entire source without an authored excerpt.
type RegionPreview struct {
	Window        *PlaybackWindow
	Gain          float64
	CalibrationDB float64
	Calibrated    bool
}

// BeginRecordingRegion captures one authoring transaction without changing state.
func (e *LibraryEditor) BeginRecordingRegion(
	recordingID string, source *RecordingAudio,
) (*RecordingRegionDraft, error) {
	path, err := e.RecordingPath(recordingID)
	if err != nil {
		return nil, err
	}
	if source == nil || len(source.data) == 0 || source.durationMs <= 0 || source.path != path {
		return nil, fmt.Errorf("%w: load the assigned recording before editing", ErrAudioLibrary)
	}

	return &RecordingRegionDraft{recordingID: recordingID, revision: e.revision, source: source,
		recording: e.Recording(recordingID), family: e.RecordingFamily(recordingID),
		calibration: e.calibrationOptions(e.RecordingFamily(recordingID), e.Recording(recordingID).CalibrationTargetDB)}, nil
}

// Preview derives the candidate using cached source analysis, including AUTO
// calibration for duplicates. Invalid intermediate bounds are not publishable.
func (draft *RecordingRegionDraft) Preview(window *PlaybackWindow) (RegionPreview, error) {
	recording, err := draft.candidate(window)
	if err != nil {
		return RegionPreview{}, err
	}
	preview := RegionPreview{Window: recording.PlaybackWindow, Gain: 1, CalibrationDB: 0, Calibrated: false}
	if recording.CalibrationGainDB != nil {
		preview.CalibrationDB, preview.Calibrated = *recording.CalibrationGainDB, true
		const amplitudeBase = 10
		preview.Gain = math.Pow(amplitudeBase, preview.CalibrationDB/amplitudeToDB)
	}

	return preview, nil
}

func (draft *RecordingRegionDraft) candidate(window *PlaybackWindow) (Recording, error) {
	if draft == nil || draft.source == nil {
		return Recording{}, ErrRecordingRegionStale
	}
	recording := draft.recording
	recording.PlaybackWindow = nil
	if window != nil {
		if window.StartMs < 0 || window.EndMs <= window.StartMs || window.EndMs > draft.source.durationMs {
			return Recording{}, fmt.Errorf("%w: use 0 ≤ Start < End ≤ %d ms", ErrAudioLibrary, draft.source.durationMs)
		}
		region := *window
		recording.PlaybackWindow = &region
	}
	if recording.CalibrationMode() == CalibrationAuto {
		suggestion, err := draft.source.SuggestCalibration(window, draft.calibration)
		if err != nil {
			return Recording{}, err
		}
		recording.CalibrationGainDB = &suggestion.GainDB
	}

	return recording, nil
}

// CheckRegionRevision is cheap enough for a UI validity check. Source content
// verification remains mandatory when publishing either kind of region edit.
func (e *LibraryEditor) CheckRegionRevision(draft *RecordingRegionDraft) error {
	if draft == nil || draft.revision != e.revision {
		return ErrRecordingRegionStale
	}
	path, err := e.RecordingPath(draft.recordingID)
	if err != nil || path != draft.source.path {
		return ErrRecordingRegionStale
	}

	return nil
}

// ApplyRecordingRegion publishes bounds and resulting calibration as one edit.
func (e *LibraryEditor) ApplyRecordingRegion(
	ctx context.Context, draft *RecordingRegionDraft, window *PlaybackWindow,
) error {
	if err := e.CheckRegionRevision(draft); err != nil {
		return err
	}
	recording, err := draft.candidate(window)
	if err != nil {
		return err
	}
	if err := draft.source.VerifySource(ctx); err != nil {
		return err
	}
	next := cloneAudioValue(e.library)
	next.Recordings[draft.recordingID] = recording

	return e.replace(next)
}

// DuplicateRecordingRegion creates one metadata-only clip and optional cue
// membership atomically, using exactly the candidate auditioned by Preview.
func (e *LibraryEditor) DuplicateRecordingRegion(
	ctx context.Context, draft *RecordingRegionDraft, newID string, window *PlaybackWindow, appendCue string,
) error {
	if err := e.CheckRegionRevision(draft); err != nil {
		return err
	}
	if _, exists := e.library.Recordings[newID]; exists {
		return fmt.Errorf("%w: recording %s already exists", ErrAudioLibrary, newID)
	}
	recording, err := draft.candidate(window)
	if err != nil {
		return err
	}
	if err := draft.source.VerifySource(ctx); err != nil {
		return err
	}
	recording.Note = ""
	next := cloneAudioValue(e.library)
	next.Recordings[newID] = recording
	next.Taxonomy.RecordingFamilies[draft.family] = append(next.Taxonomy.RecordingFamilies[draft.family], newID)
	slices.Sort(next.Taxonomy.RecordingFamilies[draft.family])
	if appendCue != "" {
		cue, exists := next.Cues[appendCue]
		if !exists {
			return fmt.Errorf("%w: choose a known moment for clip cycling", ErrAudioLibrary)
		}
		next.Cues[appendCue] = cue.WithRecordings(append(cue.Recordings(), newID))
	}

	return e.replace(next)
}
