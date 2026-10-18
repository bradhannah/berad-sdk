package audio

import (
	"errors"
	"fmt"
	"time"
)

// MusicPlayer owns one finite transport, independently of SFX/ambient admission.
// The consumer owns track selection, scheduling, interruptions and history. It
// shares the runtime's decoder, calibration, source ownership and failure boundary.
// Muted finite music is retained so an application can start at zero then fade in.
type MusicPlayer struct {
	runtime *Runtime
	current Voice
	gain    float64
}

// NewMusicPlayer borrows the backend and forces a single exclusive voice slot.
// Options.Volume is category preference; SetGain supplies context/envelope gain.
func NewMusicPlayer(library *Library, options Options) (*MusicPlayer, error) {
	options.VoiceLimit = 1
	runtime, err := NewRuntime(library, options)
	if err != nil {
		return nil, err
	}

	music := new(MusicPlayer)
	music.runtime, music.gain = runtime, 1

	return music, nil
}

// Play prepares a fresh finite recording before replacing any current transport.
// A known empty slot deliberately stops playback; unknown/cycle cues are errors.
func (music *MusicPlayer) Play(cue CueID) error {
	runtime := music.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return music.play(cue, 0)
}

func (music *MusicPlayer) play(cueID CueID, position time.Duration) error {
	runtime := music.runtime
	if runtime.closed {
		return ErrClosed
	}
	cue, exists := runtime.library.definition.Cues[cueID]
	if !exists || len(cue.Recordings) != 1 {
		return fmt.Errorf("%w: music cue %s requires one known recording", ErrInvalidRequest, cueID)
	}
	recordingID := cue.Recordings[0]
	recording := runtime.library.definition.Recordings[recordingID]
	if recording.File == "" {
		music.stop()

		return runtime.drainErrors()
	}
	prepared, err := runtime.prepare(recording, false)
	if err != nil {
		return fmt.Errorf("prepare music %s: %w", cueID, err)
	}
	position = boundedPosition(prepared, runtime.rate, position)
	if position > 0 {
		if err := prepared.device.SetPosition(position); err != nil {
			prepared.device.PauseAndStopReading()

			return errors.Join(fmt.Errorf("restore music %s: %w", cueID, err), closeSource(prepared.stream.source.source))
		}
	}
	music.stop()
	options := DefaultPlayOptions()
	options.Gain = music.gain
	music.current = runtime.startPrepared(prepared, cueID, recordingID, options, nil, 1, runtime.now())

	return runtime.drainErrors()
}

// Pause retains the current transport and captures its bounded region-relative
// position. Failures are reported by Update/Close; empty/closed players return zero.
func (music *MusicPlayer) Pause() time.Duration {
	runtime := music.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return 0
	}
	runtime.service(runtime.now())
	playing := runtime.voices[music.current.id]
	if playing == nil {
		return 0
	}
	if !playing.paused {
		playing.device.Pause()
		playing.paused, playing.pausedAt = true, runtime.now()
	}

	return boundedPosition(playing, runtime.rate, playing.device.Position())
}

// Resume restores the supplied saved position, including after a same-cue preview.
// It reuses a healthy paused transport or transactionally prepares another one.
func (music *MusicPlayer) Resume(cue CueID, position time.Duration) error {
	runtime := music.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	runtime.service(runtime.now())
	playing := runtime.voices[music.current.id]
	if playing == nil || playing.cue != cue || !playing.paused {
		return music.play(cue, position)
	}
	if err := playing.device.SetPosition(boundedPosition(playing, runtime.rate, position)); err != nil {
		return fmt.Errorf("restore retained music position: %w", err)
	}
	playing.paused = false
	playing.device.Play()

	return runtime.drainErrors()
}

// IsPlaying observes device progress; a retained paused transport is not completed.
func (music *MusicPlayer) IsPlaying() bool {
	runtime := music.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	playing := runtime.voices[music.current.id]

	return playing != nil && !playing.paused && playing.stream.Failure() == nil && playing.device.IsPlaying()
}

// Update retires failed/completed transports and reports failures once.
func (music *MusicPlayer) Update() error { return music.runtime.Update() }

// SetGain changes caller-owned context/envelope gain without restarting playback.
func (music *MusicPlayer) SetGain(gain float64) error {
	runtime := music.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	if !unitGain(gain) {
		return fmt.Errorf("%w: music gain must be in [0,1]", ErrInvalidRequest)
	}
	music.gain = gain
	if playing := runtime.voices[music.current.id]; playing != nil {
		playing.requestGain = gain
		runtime.applyGain(playing)
	}

	return nil
}

// SetVolume changes the independent user/category preference.
func (music *MusicPlayer) SetVolume(volume float64) error { return music.runtime.SetVolume(volume) }

// Stop retires the current transport without closing this reusable player.
func (music *MusicPlayer) Stop() error {
	runtime := music.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	music.stop()

	return runtime.drainErrors()
}

func (music *MusicPlayer) stop() {
	if music.runtime.voices[music.current.id] != nil {
		music.runtime.retire(music.current.id)
	}
	music.current = Voice{runtime: nil, id: 0, result: nil}
}

// Close is terminal/idempotent and never closes the borrowed backend.
func (music *MusicPlayer) Close() error { return music.runtime.Close() }

// MusicSnapshot separates current transport, bounded position and preferences.
type MusicSnapshot struct {
	Closed       bool
	Voice        VoiceStatus
	Position     time.Duration
	Duration     time.Duration
	CategoryGain float64
	ContextGain  float64
}

// Snapshot does not service playback or sample presentation time.
func (music *MusicPlayer) Snapshot() MusicSnapshot {
	runtime := music.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	result := MusicSnapshot{Closed: runtime.closed, Voice: runtime.voiceStatus(music.current),
		Position: 0, Duration: 0, CategoryGain: runtime.options.Volume, ContextGain: music.gain}
	if playing := runtime.voices[music.current.id]; playing != nil {
		result.Duration = voiceDuration(playing, runtime.rate)
		result.Position = max(0, min(playing.device.Position(), result.Duration))
	}

	return result
}

func boundedPosition(playing *voice, rate int, position time.Duration) time.Duration {
	return max(0, min(position, voiceDuration(playing, rate)))
}

func voiceDuration(playing *voice, rate int) time.Duration {
	frames := playing.stream.source.size / bytesPerFrame
	const maximum = time.Duration(1<<63 - 1)
	seconds := frames / int64(rate)
	if seconds >= int64(maximum/time.Second) {
		return maximum
	}

	return time.Duration(seconds)*time.Second + time.Duration(frames%int64(rate))*time.Second/time.Duration(rate)
}
