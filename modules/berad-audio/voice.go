package audio

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"
)

type voice struct {
	device                                         Transport
	stream                                         *pcmBoundary
	cue                                            CueID
	recording                                      RecordingID
	owner                                          *Owner
	slot                                           string
	loop                                           bool
	authoredGain, requestGain, sourceGain          float64
	envelope, gain, fadeFrom                       float64
	started, fadeIn, releaseStart, releaseDuration time.Duration
	releaseGain                                    float64
	paused                                         bool
	pausedAt                                       time.Duration
	result                                         *VoiceStatus
}

// Voice is an opaque cleanup capability. Zero/stale values stop nothing.
type Voice struct {
	runtime *Runtime
	id      uint64
	result  *VoiceStatus
}

// Stop releases only this instance; repeated requests cannot extend its tail.
func (handle Voice) Stop(fade time.Duration) error {
	if fade < 0 {
		return fmt.Errorf("%w: negative release", ErrInvalidRequest)
	}
	if handle.runtime == nil {
		return nil
	}
	runtime := handle.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return nil
	}
	runtime.stop(handle.id, fade, runtime.now())

	return nil
}

// Owner groups live voices without retaining completed handles. Release closes
// the lifetime to new playback and releases current voices; Close finishes tails.
type Owner struct {
	runtime *Runtime
	closed  bool
}

// NewOwner creates a lifetime, already expired if the runtime has closed.
func (runtime *Runtime) NewOwner() *Owner {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return &Owner{runtime: runtime, closed: runtime.closed}
}

// Play admits a voice in this lifetime and optionally replaces its named slot.
func (owner *Owner) Play(cue CueID, options PlayOptions) (PlayResult, error) {
	if owner == nil || owner.runtime == nil {
		return PlayResult{}, ErrOwnerClosed
	}

	return owner.runtime.play(cue, options, owner)
}

// Release rejects future playback and begins release of all this owner's voices.
func (owner *Owner) Release(fade time.Duration) error {
	if fade < 0 {
		return fmt.Errorf("%w: negative release", ErrInvalidRequest)
	}
	if owner == nil || owner.runtime == nil {
		return nil
	}
	runtime := owner.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	owner.closed = true
	if runtime.closed {
		return nil
	}
	now := runtime.now()
	for id, playing := range runtime.voices {
		if playing.owner == owner {
			runtime.stop(id, fade, now)
		}
	}

	return nil
}

// Close immediately finishes this owner's voices, including pending tails.
func (owner *Owner) Close() error { return owner.Release(0) }

// StopCue deliberately releases every instance, across owners and slots.
func (runtime *Runtime) StopCue(cue CueID, fade time.Duration) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	if fade < 0 {
		return fmt.Errorf("%w: negative release", ErrInvalidRequest)
	}
	now := runtime.now()
	for id, playing := range runtime.voices {
		if playing.cue == cue {
			runtime.stop(id, fade, now)
		}
	}

	return nil
}

func (runtime *Runtime) stop(voiceID uint64, fade, now time.Duration) {
	playing := runtime.voices[voiceID]
	if playing == nil {
		return
	}
	if fade == 0 {
		runtime.retire(voiceID)

		return
	}
	if playing.paused {
		now = playing.pausedAt
	}
	if playing.releaseDuration > 0 && playing.releaseDuration-(now-playing.releaseStart) <= fade {
		return
	}
	runtime.envelope(playing, now)
	playing.releaseStart, playing.releaseDuration, playing.releaseGain = now, fade, playing.envelope
	playing.fadeIn = 0
}

func (runtime *Runtime) service(now time.Duration) {
	for voiceID, playing := range runtime.voices {
		if reason, err := runtime.terminalReason(playing); reason != "" {
			runtime.finish(voiceID, reason, err)

			continue
		}
		if playing.paused {
			runtime.applyGain(playing)

			continue
		}
		runtime.envelope(playing, now)
		released := playing.releaseDuration > 0 && now-playing.releaseStart >= playing.releaseDuration
		if released {
			runtime.retire(voiceID)
		}
	}
}

func (runtime *Runtime) envelope(playing *voice, now time.Duration) {
	if playing.fadeIn > 0 {
		progress := min(1, float64(max(0, now-playing.started))/float64(playing.fadeIn))
		playing.envelope = playing.fadeFrom + (1-playing.fadeFrom)*progress
	}
	if playing.releaseDuration > 0 && now >= playing.releaseStart {
		progress := min(1, float64(max(0, now-playing.releaseStart))/float64(playing.releaseDuration))
		playing.envelope = playing.releaseGain * (1 - progress)
	}
	runtime.applyGain(playing)
}

func (runtime *Runtime) applyGain(playing *voice) {
	playing.gain = playing.targetGain(runtime.options.Volume) * playing.envelope
	playing.stream.gain.Store(math.Float64bits(math.Max(1, playing.gain)))
	playing.device.SetVolume(math.Min(1, playing.gain))
}

func (runtime *Runtime) retire(voiceID uint64) {
	runtime.finish(voiceID, VoiceStopped, nil)
}

func (runtime *Runtime) finish(voiceID uint64, reason VoiceState, failure error) {
	playing := runtime.voices[voiceID]
	playing.device.PauseAndStopReading()
	if failure == nil {
		failure = playing.stream.Failure()
	}
	failure = errors.Join(failure, closeSource(playing.stream.source.source))
	if failure != nil {
		reason = VoiceFailed
		failure = fmt.Errorf("cue %s recording %s: %w", playing.cue, playing.recording, failure)
		runtime.remember(failure)
	}
	*playing.result = stoppedVoiceStatus()
	playing.result.State, playing.result.Cue, playing.result.Recording = reason, playing.cue, playing.recording
	playing.result.Failure = failure
	delete(runtime.voices, voiceID)
}

func (playing *voice) targetGain(category float64) float64 {
	return playing.authoredGain * playing.requestGain * playing.sourceGain * category
}

// attack revives a controller's releasing voice, never a terminal Owner/Voice.
func (runtime *Runtime) attack(playing *voice, now, fade time.Duration) {
	runtime.envelope(playing, now)
	playing.releaseDuration = 0
	playing.started, playing.fadeIn, playing.fadeFrom = now, fade, playing.envelope
	if fade == 0 {
		playing.envelope = 1
	}
	runtime.applyGain(playing)
}

// VoiceSnapshot is detached and reports the last serviced gain. Reading it does
// not sample the clock, service a fade, consume PCM or advance variant selection.
type VoiceSnapshot struct {
	ID          uint64
	Cue         CueID
	Recording   RecordingID
	Loop        bool
	Gain        float64
	RequestGain float64
	Envelope    float64
	TargetGain  float64
	State       VoiceState
	Position    time.Duration
	Duration    time.Duration
}

// Snapshot returns a stable voice-ID-ordered observational view.
func (runtime *Runtime) Snapshot() []VoiceSnapshot {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	result := make([]VoiceSnapshot, 0, len(runtime.voices))
	for id, playing := range runtime.voices {
		status := runtime.voiceStatus(Voice{runtime: runtime, id: id, result: playing.result})
		result = append(result, VoiceSnapshot{ID: id, Cue: playing.cue, Recording: playing.recording,
			Loop: playing.loop, Gain: playing.gain, RequestGain: playing.requestGain, Envelope: playing.envelope,
			TargetGain: playing.targetGain(runtime.options.Volume), State: status.State,
			Position: boundedPosition(playing, runtime.rate, playing.device.Position()),
			Duration: voiceDuration(playing, runtime.rate)})
	}
	slices.SortFunc(result, func(a, b VoiceSnapshot) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}

		return 0
	})

	return result
}
