package audio

import (
	"fmt"
	"time"
)

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

// SetGain changes this voice's request coefficient in [0,1], not authored gain.
func (handle Voice) SetGain(gain float64) error {
	if !unitGain(gain) {
		return fmt.Errorf("%w: voice gain must be in [0,1]", ErrInvalidRequest)
	}

	return handle.control(func(runtime *Runtime, playing *voice) error {
		playing.requestGain = gain
		runtime.envelope(playing, controlTime(runtime, playing))

		return nil
	})
}

// Pause retains the decoder and freezes the voice envelope until Resume.
func (handle Voice) Pause() error {
	return handle.control(func(runtime *Runtime, playing *voice) error {
		if !playing.paused {
			playing.pausedAt = runtime.now()
			runtime.envelope(playing, playing.pausedAt)
			playing.device.Pause()
			playing.paused = true
		}

		return nil
	})
}

// Resume continues a paused voice; a terminal voice cannot be reopened.
func (handle Voice) Resume() error {
	return handle.control(func(runtime *Runtime, playing *voice) error {
		if playing.paused {
			delta := runtime.now() - playing.pausedAt
			playing.started += delta
			playing.releaseStart += delta
			playing.paused = false
			playing.device.Play()
		}

		return nil
	})
}

// Seek uses a region-relative position and preserves paused/playing ownership.
func (handle Voice) Seek(position time.Duration) error {
	if position < 0 {
		return fmt.Errorf("%w: negative position", ErrInvalidRequest)
	}

	return handle.control(func(runtime *Runtime, playing *voice) error {
		if frameOffset(position, runtime.rate) > playing.stream.source.size {
			return fmt.Errorf("%w: position exceeds recording window", ErrInvalidRequest)
		}
		if err := playing.device.SetPosition(position); err != nil {
			return fmt.Errorf("seek voice: %w", err)
		}

		return nil
	})
}

// Position returns the backend estimate, clamped to the selected recording.
func (handle Voice) Position() (time.Duration, error) {
	var position time.Duration
	err := handle.control(func(runtime *Runtime, playing *voice) error {
		position = boundedPosition(playing, runtime.rate, playing.device.Position())

		return nil
	})

	return position, err
}

func (handle Voice) control(action func(*Runtime, *voice) error) error {
	if handle.runtime == nil {
		return ErrVoiceClosed
	}
	runtime := handle.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	playing := runtime.voices[handle.id]
	if playing == nil {
		return ErrVoiceClosed
	}
	if state, err := runtime.terminalReason(playing); state != "" {
		runtime.finish(handle.id, state, err)

		return ErrVoiceClosed
	}

	return action(runtime, playing)
}

func controlTime(runtime *Runtime, playing *voice) time.Duration {
	if playing.paused {
		return playing.pausedAt
	}

	return runtime.now()
}
