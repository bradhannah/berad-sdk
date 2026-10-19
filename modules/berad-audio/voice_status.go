package audio

import "time"

// VoiceState distinguishes transport suspension, release and terminal outcomes.
type VoiceState string

// Live and terminal states do not imply that native output was heard.
const (
	VoicePlaying   VoiceState = "playing"
	VoicePaused    VoiceState = "paused"
	VoiceReleasing VoiceState = "releasing"
	VoiceStopped   VoiceState = "stopped"
	VoiceCompleted VoiceState = "completed"
	VoiceEvicted   VoiceState = "evicted"
	VoiceFailed    VoiceState = "failed"
)

// VoiceStatus is detached. TargetGain excludes the fade envelope, allowing
// consumers to distinguish a muted source from the first silent fade-in frame.
// Terminal status retains no decoder or asset bytes; failures are also reported
// once through Runtime.Update/Close. Status is presentation-only information.
type VoiceStatus struct {
	State       VoiceState
	Cue         CueID
	Recording   RecordingID
	Gain        float64
	TargetGain  float64
	Failure     error
	Position    time.Duration
	Duration    time.Duration
	RequestGain float64
	SourceGain  float64
	Envelope    float64
}

// Status observes the last control/service state without advancing the clock.
func (handle Voice) Status() VoiceStatus {
	if handle.runtime == nil {
		return stoppedVoiceStatus()
	}
	runtime := handle.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.voiceStatus(handle)
}

func (runtime *Runtime) voiceStatus(handle Voice) VoiceStatus {
	if playing := runtime.voices[handle.id]; playing != nil {
		state := VoicePlaying
		if playing.releaseDuration > 0 && runtime.latest.Sub(runtime.origin) >= playing.releaseStart {
			state = VoiceReleasing
		}
		if playing.paused {
			state = VoicePaused
		}

		return VoiceStatus{State: state, Cue: playing.cue, Recording: playing.recording,
			Gain: playing.gain, TargetGain: playing.targetGain(runtime.options.Volume), Failure: nil,
			Position:    boundedPosition(playing, runtime.rate, playing.device.Position()),
			Duration:    voiceDuration(playing, runtime.rate),
			RequestGain: playing.requestGain, SourceGain: playing.sourceGain, Envelope: playing.envelope}
	}
	if handle.result != nil {
		return *handle.result
	}

	return stoppedVoiceStatus()
}

func stoppedVoiceStatus() VoiceStatus {
	var status VoiceStatus
	status.State = VoiceStopped

	return status
}
