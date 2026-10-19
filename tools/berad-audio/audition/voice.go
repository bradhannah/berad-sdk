// Package audition owns standalone tool voices on a process-wide Oto device.
// It is for standalone tool processes, not a second device inside an Ebitengine
// game. Controls are caller-serialized; only native reading and gain are concurrent.
package audition

import (
	"errors"
	"fmt"
	"io"
	"math"
	"sync/atomic"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const millisecondsPerSecond = 1000
const pcmBytesPerFrame = 4

// ErrInvalidVoice identifies invalid voice inputs or failed device preparation.
var ErrInvalidVoice = errors.New("invalid standalone audio voice")

type devicePlayer interface {
	Play()
	Pause()
	PauseAndStopReading()
	IsPlaying() bool
	SetVolume(gain float64)
	BufferedSize() int
	Seek(offset int64, whence int) (int64, error)
	Err() error
}

type cursor struct {
	source   *authoring.Stream
	position atomic.Int64
}

func (source *cursor) Read(buffer []byte) (int, error) {
	count, err := source.source.Read(buffer)
	source.position.Add(int64(count))

	return count, err //nolint:wrapcheck // The device requires the source's exact io.Reader EOF sentinel.
}

func (source *cursor) Seek(offset int64, whence int) (int64, error) {
	position, err := source.source.Seek(offset, whence)
	if err == nil {
		source.position.Store(position)
	}

	//nolint:wrapcheck // Preserve the contextual authoring-stream seek result through this cursor adapter.
	return position, err
}

// Voice is one seekable audition. It retains healthy paused/completed playback
// for seeking. Close retires only that voice; the shared host device survives.
type Voice struct {
	create         func(io.ReadSeeker) (devicePlayer, error)
	player         devicePlayer
	cursor         *cursor
	startMs, endMs int64
	loop           bool
}

// NewVoice configures a voice without opening a device. Device creation is lazy.
func NewVoice() *Voice {
	voice := new(Voice)
	voice.create = newNativePlayer

	return voice
}

// Play prepares a replacement before retiring the current voice. Caller-owned
// source metadata never changes. PCM boost/saturation is SDK-owned, not duplicated.
func (voice *Voice) Play(
	clip *authoring.RecordingAudio, window *authoring.PlaybackWindow, loop bool, gain float64,
) error {
	if voice.create == nil || clip == nil || math.IsNaN(gain) || math.IsInf(gain, 0) || gain < 0 {
		return fmt.Errorf("%w: initialized voice, source and finite nonnegative gain required", ErrInvalidVoice)
	}
	stream, err := clip.Stream(window, loop)
	if err != nil {
		return fmt.Errorf("prepare audition stream: %w", err)
	}
	if err := stream.SetGain(math.Max(1, gain)); err != nil {
		return errors.Join(err, stream.Close())
	}
	tracked := new(cursor)
	tracked.source = stream
	player, err := voice.create(tracked)
	if err != nil || player == nil {
		if player != nil {
			player.PauseAndStopReading()
		}
		if err == nil {
			err = ErrInvalidVoice
		}

		return errors.Join(fmt.Errorf("create standalone player: %w", err), stream.Close())
	}
	if err := voice.Close(); err != nil {
		player.PauseAndStopReading()

		return errors.Join(err, stream.Close())
	}
	voice.player, voice.cursor = player, tracked
	voice.startMs, voice.endMs, voice.loop = 0, clip.DurationMs(), loop
	if window != nil {
		voice.startMs, voice.endMs = window.StartMs, window.EndMs
	}
	player.SetVolume(math.Min(1, gain))
	player.Play()

	return nil
}

// SetGain changes monitoring without replacing the stream. Invalid display/input
// values normalize to silence, matching the established standalone editor contract.
func (voice *Voice) SetGain(gain float64) {
	if voice.player == nil {
		return
	}
	if math.IsNaN(gain) || math.IsInf(gain, 0) {
		gain = 0
	}
	gain = math.Max(0, gain)
	if err := voice.cursor.source.SetGain(math.Max(1, gain)); err != nil {
		panic(err)
	}
	voice.player.SetVolume(math.Min(1, gain))
}

// Playing reports current device playback without servicing it.
func (voice *Voice) Playing() bool { return voice.player != nil && voice.player.IsPlaying() }

// Active includes paused or completed voices retained for seeking.
func (voice *Voice) Active() bool { return voice.player != nil }

// Pause changes playback while retaining the current voice and position.
func (voice *Voice) Pause(paused bool) {
	if voice.player == nil {
		return
	}
	if paused {
		voice.player.Pause()
	} else {
		voice.player.Play()
	}
}

// PositionMs is source-relative, accounting for device buffering and loop bounds.
func (voice *Voice) PositionMs() int64 {
	if voice.player == nil {
		return 0
	}
	played := max(0, voice.cursor.position.Load()-int64(voice.player.BufferedSize()))
	position := played * millisecondsPerSecond / (authoring.EditorSampleRate * pcmBytesPerFrame)
	if voice.loop && voice.endMs > voice.startMs {
		position %= voice.endMs - voice.startMs
	}

	return min(voice.endMs, voice.startMs+position)
}

// SeekMs clamps a source-relative position to the selected window.
func (voice *Voice) SeekMs(position int64) error {
	if voice.player == nil {
		return nil
	}
	position = max(voice.startMs, min(voice.endMs, position)) - voice.startMs
	offset := position * authoring.EditorSampleRate / millisecondsPerSecond * pcmBytesPerFrame
	if _, err := voice.player.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek standalone voice: %w", err)
	}

	return nil
}

// Failure is observational; it does not service or retire playback.
func (voice *Voice) Failure() error {
	if voice.player != nil {
		if err := voice.player.Err(); err != nil {
			return fmt.Errorf("standalone voice: %w", err)
		}
	}

	return nil
}

// Close stops native reads before retiring the owned stream; repeated calls are harmless.
func (voice *Voice) Close() error {
	if voice.player == nil {
		return nil
	}
	voice.player.PauseAndStopReading()
	err := errors.Join(voice.Failure(), voice.cursor.source.Close())
	voice.player, voice.cursor = nil, nil

	return err
}
