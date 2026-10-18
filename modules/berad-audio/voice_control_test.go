package audio_test

import (
	"errors"
	"io"
	"testing"
	"time"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

func TestVoicePauseFreezesEnvelopeAndRetainsTransport(t *testing.T) {
	state := newFixture(t, 1, 2)
	options := audio.DefaultPlayOptions()
	options.FadeIn = time.Second
	handle := play(t, state.runtime, options)
	state.time = state.time.Add(500 * time.Millisecond)
	if err := handle.Pause(); err != nil {
		t.Fatal(err)
	}
	state.time = state.time.Add(time.Minute)
	if err := state.runtime.SetVolume(0.5); err != nil {
		t.Fatal(err)
	}
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	status := handle.Status()
	if status.State != audio.VoicePaused || status.Gain != 0.125 || state.sources[0].closed != 0 {
		t.Fatalf("paused transport/envelope lost: %+v", status)
	}
	if err := handle.SetGain(0.5); err != nil {
		t.Fatal(err)
	}
	if err := handle.Resume(); err != nil {
		t.Fatal(err)
	}
	if handle.Status().Gain != 0.0625 {
		t.Fatal(handle.Status())
	}
	state.time = state.time.Add(500 * time.Millisecond)
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	if handle.Status().Gain != 0.125 || len(state.backend.players) != 1 {
		t.Fatal(handle.Status())
	}
}

func TestVoiceReportsDistinctTerminalReasons(t *testing.T) {
	state := newFixture(t, 1, 1)
	first := play(t, state.runtime, audio.DefaultPlayOptions())
	second := play(t, state.runtime, audio.DefaultPlayOptions())
	if first.Status().State != audio.VoiceEvicted {
		t.Fatal(first.Status())
	}
	state.backend.players[1].playing = false
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	if second.Status().State != audio.VoiceCompleted {
		t.Fatal(second.Status())
	}
	if err := second.Resume(); !errors.Is(err, audio.ErrVoiceClosed) {
		t.Fatal(err)
	}
	options := audio.DefaultPlayOptions()
	options.Loop = true
	loop := play(t, state.runtime, options)
	state.backend.players[2].playing = false
	if err := state.runtime.Update(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if loop.Status().State != audio.VoiceFailed || !errors.Is(loop.Status().Failure, io.ErrUnexpectedEOF) {
		t.Fatal(loop.Status())
	}
	last := play(t, state.runtime, options)
	if err := last.Stop(0); err != nil {
		t.Fatal(err)
	}
	if last.Status().State != audio.VoiceStopped {
		t.Fatal(last.Status())
	}
}

func TestVoiceSeekValidatesWindowWithoutRetiringPausedVoice(t *testing.T) {
	state := newFixture(t, 1, 1)
	handle := play(t, state.runtime, audio.DefaultPlayOptions())
	if err := handle.Pause(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Seek(-time.Millisecond); !errors.Is(err, audio.ErrInvalidRequest) {
		t.Fatal(err)
	}
	if err := handle.Seek(time.Second); !errors.Is(err, audio.ErrInvalidRequest) {
		t.Fatal(err)
	}
	if err := handle.Seek(time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if handle.Status().State != audio.VoicePaused {
		t.Fatal(handle.Status())
	}
	if position, err := handle.Position(); err != nil || position != time.Millisecond {
		t.Fatalf("position %v: %v", position, err)
	}
	status := handle.Status()
	if status.Position != time.Millisecond || status.Duration != 2*time.Millisecond || status.RequestGain != 1 || status.SourceGain != 1 {
		t.Fatal("incomplete voice diagnostics", status)
	}
	if cursor, err := state.sources[0].Seek(0, io.SeekCurrent); err != nil || cursor != 4 {
		t.Fatalf("seek did not reach retained source: %d %v", cursor, err)
	}
	if err := handle.Resume(); err != nil {
		t.Fatal(err)
	}
}

func TestPausedVoiceReleaseCanBeShortenedWithoutCountingPausedTime(t *testing.T) {
	state := newFixture(t, 1, 1)
	options := audio.DefaultPlayOptions()
	options.Loop = true
	handle := play(t, state.runtime, options)
	if err := handle.Stop(time.Second); err != nil {
		t.Fatal(err)
	}
	state.time = state.time.Add(200 * time.Millisecond)
	if err := handle.Pause(); err != nil {
		t.Fatal(err)
	}
	state.time = state.time.Add(time.Minute)
	if err := handle.Stop(100 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := handle.Resume(); err != nil {
		t.Fatal(err)
	}
	state.time = state.time.Add(100 * time.Millisecond)
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	if handle.Status().State != audio.VoiceStopped {
		t.Fatal(handle.Status())
	}
}
