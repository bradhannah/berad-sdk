package authoring

import (
	"fmt"
	"math"
)

// CalibrationAuto requests recalibration when the source/family changes.
const CalibrationAuto = "auto"

// CalibrationManual preserves a user-supplied gain through source replacement.
const CalibrationManual = "manual"

func (recording libraryRecording) CalibrationMode() string {
	if recording.CalibrationGainDB == nil {
		return "uncalibrated"
	}
	if recording.CalibrationPolicy == CalibrationAuto {
		return CalibrationAuto
	}

	return CalibrationManual
}

func (recording libraryRecording) validateCalibration() error {
	if value := recording.CalibrationGainDB; value != nil &&
		!finiteRange(*value, calibrationMinGain, calibrationMaxGain) {
		return fmt.Errorf("%w: calibration gain must be in [-48,12] dB", ErrLibrary)
	}
	if recording.CalibrationPolicy != "" && recording.CalibrationPolicy != CalibrationAuto &&
		recording.CalibrationPolicy != CalibrationManual {
		return fmt.Errorf("%w: invalid calibration mode", ErrLibrary)
	}
	if recording.CalibrationPolicy != "" && recording.CalibrationGainDB == nil {
		return fmt.Errorf("%w: calibration mode requires a gain", ErrLibrary)
	}
	if value := recording.CalibrationTargetDB; value != nil &&
		!finiteRange(*value, calibrationMinTarget, calibrationMaxTarget) {
		return fmt.Errorf("%w: calibration target must be in [-60,-6] dB", ErrLibrary)
	}

	return nil
}

func finiteRange(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}

func (e *LibraryEditor) calibrationOptions(family string, override *float64) CalibrationOptions {
	options := e.calibrationPolicy(family)
	if override != nil {
		options.TargetDB = *override
	}

	return options
}

// CalibrationOptionsFor returns the project's actual measurement policy and
// authored target override for a recording, so frontends need no second policy.
func (e *LibraryEditor) CalibrationOptionsFor(recordingID string) (CalibrationOptions, error) {
	recording, exists := e.library.Recordings[recordingID]
	if !exists {
		return CalibrationOptions{}, fmt.Errorf("%w: unknown recording %s", ErrLibrary, recordingID)
	}

	return e.calibrationOptions(e.RecordingFamily(recordingID), recording.CalibrationTargetDB), nil
}
