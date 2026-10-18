package authoring

import (
	"fmt"
	"math"
)

const CalibrationAuto = "auto"
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
		(math.IsNaN(*value) || math.IsInf(*value, 0) || *value < calibrationMinGain || *value > calibrationMaxGain) {
		return fmt.Errorf("%w: calibration gain must be in [-48,12] dB", ErrLibrary)
	}
	if recording.CalibrationPolicy != "" && recording.CalibrationPolicy != CalibrationAuto && recording.CalibrationPolicy != CalibrationManual {
		return fmt.Errorf("%w: invalid calibration mode", ErrLibrary)
	}
	if recording.CalibrationPolicy != "" && recording.CalibrationGainDB == nil {
		return fmt.Errorf("%w: calibration mode requires a gain", ErrLibrary)
	}
	if value := recording.CalibrationTargetDB; value != nil &&
		(math.IsNaN(*value) || math.IsInf(*value, 0) || *value < calibrationMinTarget || *value > calibrationMaxTarget) {
		return fmt.Errorf("%w: calibration target must be in [-60,-6] dB", ErrLibrary)
	}
	return nil
}

func (e *LibraryEditor) calibrationOptions(family string, override *float64) CalibrationOptions {
	options := e.calibrationPolicy(family)
	if override != nil {
		options.TargetDB = *override
	}
	return options
}
