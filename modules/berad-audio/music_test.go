package audio_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/fstest"
	"time"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

func musicFixture(t *testing.T) (*audio.MusicPlayer, *backend, *[]*pcm) {
	t.Helper()
	definition := audio.Definition{Recordings: map[audio.RecordingID]audio.Recording{
		"theme": {File: "theme.pcm"}, "preview": {File: "preview.pcm"}, "empty": {}},
		Cues: map[audio.CueID]audio.Cue{"score": {Recordings: []audio.RecordingID{"theme"}, Gain: 0.5},
			"preview": {Recordings: []audio.RecordingID{"preview"}, Gain: 1}, "silent": {Recordings: []audio.RecordingID{"empty"}, Gain: 1}}}
	library, err := audio.NewLibrary(definition, fstest.MapFS{"theme.pcm": {Data: make([]byte, 4000)}, "preview.pcm": {Data: make([]byte, 4000)}}, 8000)
	if err != nil {
		t.Fatal(err)
	}
	host := new(backend)
	var sources []*pcm
	options := audio.DefaultOptions(host, func(data []byte, _ string, _ int) (audio.PCMStream, error) {
		source := &pcm{Reader: bytes.NewReader(data), length: int64(len(data))}
		sources = append(sources, source)
		return source, nil
	})
	music, err := audio.NewMusicPlayer(library, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := music.Close(); err != nil {
			t.Error(err)
		}
	})
	return music, host, &sources
}

func TestMusicStartsMutedAndComposesLiveCategoryContext(t *testing.T) {
	music, host, _ := musicFixture(t)
	if err := music.SetGain(0); err != nil {
		t.Fatal(err)
	}
	if err := music.Play("score"); err != nil {
		t.Fatal(err)
	}
	if !music.IsPlaying() || len(host.players) != 1 || host.players[0].gain != 0 {
		t.Fatal("zero envelope dropped finite music")
	}
	if err := music.SetGain(0.5); err != nil {
		t.Fatal(err)
	}
	if err := music.SetVolume(0.5); err != nil {
		t.Fatal(err)
	}
	if host.players[0].gain != 0.125 || len(host.players) != 1 {
		t.Fatal(music.Snapshot())
	}
}

func TestMusicPauseResumeAndSameCuePreviewRestoreExactPosition(t *testing.T) {
	for _, preview := range []audio.CueID{"score", "preview"} {
		t.Run(string(preview), func(t *testing.T) {
			music, host, sources := musicFixture(t)
			if err := music.Play("score"); err != nil {
				t.Fatal(err)
			}
			host.players[0].position = 400 * time.Millisecond
			position := music.Pause()
			if position != 400*time.Millisecond || music.IsPlaying() {
				t.Fatal(position)
			}
			if err := music.Update(); err != nil {
				t.Fatal(err)
			}
			if (*sources)[0].closed != 0 {
				t.Fatal("pause retired source")
			}
			if err := music.Resume("score", position); err != nil {
				t.Fatal(err)
			}
			if len(host.players) != 1 || music.Snapshot().Position != position {
				t.Fatal("retained resume restarted")
			}
			music.Pause()
			if err := music.Play(preview); err != nil {
				t.Fatal(err)
			}
			host.players[1].position = 100 * time.Millisecond
			if err := music.Resume("score", position); err != nil {
				t.Fatal(err)
			}
			if music.Snapshot().Position != position || music.Snapshot().Voice.Cue != "score" {
				t.Fatal(music.Snapshot())
			}
			if (*sources)[0].closed != 1 || (*sources)[1].closed != 1 {
				t.Fatal("preview replacement leaked")
			}
		})
	}
}

func TestMusicFailedReplacementPreservesTransportAndEmptySlotStops(t *testing.T) {
	music, host, sources := musicFixture(t)
	if err := music.Play("score"); err != nil {
		t.Fatal(err)
	}
	broken := errors.New("device unavailable")
	host.failure = broken
	if err := music.Play("preview"); !errors.Is(err, broken) {
		t.Fatal(err)
	}
	if !music.IsPlaying() || (*sources)[0].closed != 0 || (*sources)[1].closed != 1 {
		t.Fatal("failed replacement changed voice")
	}
	if err := music.Play("unknown"); !errors.Is(err, audio.ErrInvalidRequest) {
		t.Fatal(err)
	}
	if err := music.Play("silent"); err != nil {
		t.Fatal(err)
	}
	if music.IsPlaying() || (*sources)[0].closed != 1 {
		t.Fatal("empty slot retained score")
	}
}

func TestMusicCompletionAndPausedFailureRetireOnce(t *testing.T) {
	music, host, sources := musicFixture(t)
	if err := music.Play("score"); err != nil {
		t.Fatal(err)
	}
	host.players[0].playing = false
	if err := music.Update(); err != nil {
		t.Fatal(err)
	}
	if music.Snapshot().Voice.State != audio.VoiceCompleted || (*sources)[0].closed != 1 {
		t.Fatal(music.Snapshot())
	}
	if err := music.Play("score"); err != nil {
		t.Fatal(err)
	}
	music.Pause()
	broken := errors.New("paused decoder failed")
	(*sources)[1].failure = broken
	if _, err := host.players[1].read(4); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if err := music.Update(); !errors.Is(err, broken) {
		t.Fatal(err)
	}
	if err := music.Update(); err != nil {
		t.Fatal("failure repeated", err)
	}
	if (*sources)[1].closed != 1 || music.Snapshot().Voice.State != audio.VoiceFailed {
		t.Fatal(music.Snapshot())
	}
}

func TestMusicPositionsAreBoundedAndCloseIsTerminal(t *testing.T) {
	music, host, _ := musicFixture(t)
	if err := music.Play("score"); err != nil {
		t.Fatal(err)
	}
	host.players[0].position = 2 * time.Second
	if got := music.Pause(); got != time.Second {
		t.Fatal(got)
	}
	if err := music.Resume("score", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if music.Snapshot().Position != time.Second {
		t.Fatal(music.Snapshot())
	}
	if err := music.Close(); err != nil {
		t.Fatal(err)
	}
	if err := music.Play("score"); !errors.Is(err, audio.ErrClosed) {
		t.Fatal(err)
	}
	if err := music.SetGain(1); !errors.Is(err, audio.ErrClosed) {
		t.Fatal(err)
	}
}
