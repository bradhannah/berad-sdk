package authoring

import (
	"fmt"
	"math"
	"slices"
)

const (
	calibrationPeakCeiling    = -3.0
	calibrationMinGain        = -48.0
	calibrationMaxGain        = 12.0
	calibrationSilenceDB      = -70.0
	calibrationMinTarget      = -60.0
	calibrationMaxTarget      = -6.0
	amplitudeToDB             = 20.0
	energyToDB                = 10.0
	calibrationGainTolerance  = 0.01
	integratedWindowBuckets   = 40
	integratedStrideBuckets   = 10
	activeWindowBuckets       = 5
	activePercentile          = 0.95
	activeRelativeEnergyFloor = 1e-4
	absoluteEnergyFloor       = 1e-7
)

// CalibrationMethod selects measurement explicitly, independently of consumer
// family names. Gated selections under 400 ms fall back to active RMS.
type CalibrationMethod string

// Supported calibration measurements retain their persisted method identities.
const (
	ActiveRMS     CalibrationMethod = "active_rms"
	GatedLoudness CalibrationMethod = "gated_loudness"
)

// CalibrationOptions supplies consumer policy. TargetDB is finite in [-60,-6];
// zero is invalid. Values are estimates, never listening approval.
type CalibrationOptions struct {
	Method   CalibrationMethod
	TargetDB float64
}

// DefaultCalibrationOptions supplies a conservative short-effect starting point.
func DefaultCalibrationOptions() CalibrationOptions {
	return CalibrationOptions{Method: ActiveRMS, TargetDB: -20}
}

// CalibrationSuggestion is an offline estimate for the selected region. It
// never changes source bytes, cue gain, or category preferences.
type CalibrationSuggestion struct {
	Method     string
	MeasuredDB float64
	TargetDB   float64
	PeakDB     float64
	GainDB     float64
	Limited    bool
}

type loudnessBucket struct {
	energy, weighted float64
	peak             float64
	frames           int
}

// SuggestCalibration measures cached 10 ms energy buckets, independent of the
// playhead. Boundary buckets are weighted by overlap; peaks are conservative.
// Long sustained audio uses BS.1770-style K weighting and two-stage gating.
// Short effects use the P95 of active 50 ms stereo RMS windows. Neither estimate
// is advertised as a certified LUFS/true-peak meter.
func (clip *RecordingAudio) SuggestCalibration(
	window *PlaybackWindow, options CalibrationOptions,
) (CalibrationSuggestion, error) {
	if !finiteRange(options.TargetDB, calibrationMinTarget, calibrationMaxTarget) ||
		(options.Method != ActiveRMS && options.Method != GatedLoudness) {
		return CalibrationSuggestion{},
			fmt.Errorf("%w: calibration needs a known method and target in [-60,-6] dB", ErrLibrary)
	}
	start, end := int64(0), clip.durationMs
	if window != nil {
		start, end = window.StartMs, window.EndMs
	}
	if start < 0 || end <= start || end > clip.durationMs {
		return CalibrationSuggestion{}, fmt.Errorf("%w: invalid calibration region", ErrLibrary)
	}
	selected, peak := clip.calibrationBuckets(start, end)
	method, measured := "Active P95 RMS (dBFS, estimated)", activeRMSLevel(selected)
	if options.Method == GatedLoudness && len(selected) >= integratedWindowBuckets {
		method, measured = "Gated loudness (LUFS, estimated)", gatedLoudness(selected)
	}
	if peak == 0 || math.IsInf(measured, -1) || measured < calibrationSilenceDB {
		return CalibrationSuggestion{}, fmt.Errorf("%w: selection is silent or below the analysis floor", ErrLibrary)
	}
	targetDB := options.TargetDB
	peakDB := amplitudeToDB * math.Log10(peak)
	wanted := targetDB - measured
	gain := math.Max(calibrationMinGain, math.Min(wanted, math.Min(calibrationMaxGain, calibrationPeakCeiling-peakDB)))

	return CalibrationSuggestion{Method: method, MeasuredDB: measured, TargetDB: targetDB,
		PeakDB: peakDB, GainDB: gain, Limited: math.Abs(gain-wanted) > calibrationGainTolerance}, nil
}

func (clip *RecordingAudio) calibrationBuckets(start, end int64) ([]loudnessBucket, float64) {
	var selected []loudnessBucket
	peak := 0.0
	for index := int(start / WaveformBucketMs); index < len(clip.loudness); index++ {
		from := int64(index) * WaveformBucketMs
		if from >= end {
			break
		}
		bucket := clip.loudness[index]
		fraction := float64(min(end, from+WaveformBucketMs)-max(start, from)) / WaveformBucketMs
		bucket.energy *= fraction
		bucket.weighted *= fraction
		bucket.frames = max(1, int(float64(bucket.frames)*fraction))
		selected = append(selected, bucket)
		peak = math.Max(peak, bucket.peak)
	}

	return selected, peak
}

func windowEnergies(buckets []loudnessBucket, size, stride int, weighted bool) []float64 {
	size = min(size, len(buckets))
	var values []float64
	if size == 0 {
		return values
	}
	for from := 0; from+size <= len(buckets); from += stride {
		energy, frames := 0.0, 0
		for _, bucket := range buckets[from : from+size] {
			if weighted {
				energy += bucket.weighted
			} else {
				energy += bucket.energy
			}
			frames += bucket.frames
		}
		values = append(values, energy/float64(frames))
	}

	return values
}

func activeRMSLevel(buckets []loudnessBucket) float64 {
	values := windowEnergies(buckets, activeWindowBuckets, 1, false)
	maximum := 0.0
	for _, energy := range values {
		maximum = math.Max(maximum, energy)
	}
	// Exclude silence and tails >40 dB below the loudest active window.
	floor := math.Max(absoluteEnergyFloor, maximum*activeRelativeEnergyFloor)
	values = slices.DeleteFunc(values, func(value float64) bool { return value < floor })
	if len(values) == 0 {
		return math.Inf(-1)
	}
	slices.Sort(values)
	index := int(math.Ceil(float64(len(values))*activePercentile)) - 1

	return energyToDB * math.Log10(values[max(0, index)])
}

func gatedLoudness(buckets []loudnessBucket) float64 {
	const offset = -0.691
	values := windowEnergies(buckets, integratedWindowBuckets, integratedStrideBuckets, true)
	absoluteFloor := math.Pow(energyToDB, (calibrationSilenceDB-offset)/energyToDB)
	mean := gatedMean(values, absoluteFloor)
	mean = gatedMean(values, math.Max(absoluteFloor, mean/energyToDB))

	return offset + energyToDB*math.Log10(mean)
}

func gatedMean(values []float64, floor float64) float64 {
	sum, count := 0.0, 0
	for _, value := range values {
		if value >= floor {
			sum += value
			count++
		}
	}
	if count == 0 {
		return 0
	}

	return sum / float64(count)
}

// biquad stores one channel's K-weighting filter state at EditorSampleRate.
type biquad struct{ b0, b1, b2, a1, a2, x1, x2, y1, y2 float64 }

func (filter *biquad) sample(input float64) float64 {
	output := filter.b0*input + filter.b1*filter.x1 + filter.b2*filter.x2 - filter.a1*filter.y1 - filter.a2*filter.y2
	filter.x2, filter.x1, filter.y2, filter.y1 = filter.x1, input, filter.y1, output

	return output
}

//nolint:mnd,exhaustruct_v5 // Published 44.1 kHz filter coefficients; delay elements start at zero.
func kWeightingFilters() [2][2]biquad {
	// BS.1770 K-weighting coefficients at 44.1 kHz (shelf then RLB high-pass).
	shelf := biquad{b0: 1.53084123005035, b1: -2.65097999515473, b2: 1.16907907992159,
		a1: -1.66365511325602, a2: 0.712595428073225}
	highpass := biquad{b0: 1, b1: -2, b2: 1, a1: -1.9891696736298, a2: 0.989199035787039}

	return [2][2]biquad{{shelf, highpass}, {shelf, highpass}}
}
