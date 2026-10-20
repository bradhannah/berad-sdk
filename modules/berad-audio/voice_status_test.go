package audio_test

import (
	"errors"
	"io"
	"testing"
	"time"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

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

func TestVoiceStatusObservesBoundedPositionWithoutServicingFade(t *testing.T) {
	state := newFixture(t, 1, 1)
	options := audio.DefaultPlayOptions()
	options.FadeIn = time.Second
	handle := play(t, state.runtime, options)
	state.time = state.time.Add(time.Second)
	state.backend.players[0].position = time.Hour
	status := handle.Status()
	if status.Position != 2*time.Millisecond || status.Duration != 2*time.Millisecond ||
		status.RequestGain != 1 || status.SourceGain != 1 || status.Envelope != 0 || status.Gain != 0 {
		t.Fatal("status advanced a fade or lost bounded diagnostics", status)
	}
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	if handle.Status().Envelope != 1 {
		t.Fatal("update did not service the fade", handle.Status())
	}
}
