package audition

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

type source struct {
	*bytes.Reader
	close func()
}

func (source *source) Length() int64 { return source.Size() }
func (source *source) Close() error  { source.close(); return nil }

type player struct {
	source  io.ReadSeeker
	playing bool
	starts  int
	volume  float64
	barrier func()
}

func (player *player) Play()                  { player.playing = true; player.starts++ }
func (player *player) Pause()                 { player.playing = false }
func (player *player) PauseAndStopReading()   { player.playing = false; player.barrier() }
func (player *player) IsPlaying() bool        { return player.playing }
func (player *player) SetVolume(gain float64) { player.volume = gain }
func (*player) BufferedSize() int             { return 0 }
func (player *player) Seek(offset int64, whence int) (int64, error) {
	return player.source.Seek(offset, whence)
}
func (*player) Err() error { return nil }

func TestStandaloneControlsAndRetirementPreserveSourceOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signal.raw")
	data := bytes.Repeat([]byte{1, 0, 2, 0}, authoring.EditorSampleRate/10)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var events []string
	decode := func(data []byte, _ string, _ int) (audio.PCMStream, error) {
		return &source{Reader: bytes.NewReader(data), close: func() { events = append(events, "close") }}, nil
	}
	clip, err := authoring.LoadRecording(path, decode)
	if err != nil {
		t.Fatal(err)
	}
	events = nil
	device := &player{barrier: func() { events = append(events, "barrier") }}
	voice := NewVoice()
	created := 0
	voice.create = func(source io.ReadSeeker) (devicePlayer, error) {
		created++
		device.source = source
		return device, nil
	}
	if err := voice.Play(clip, &authoring.PlaybackWindow{StartMs: 20, EndMs: 60}, false, 2); err != nil {
		t.Fatal(err)
	}
	if created != 1 || !voice.Playing() || device.volume != 1 {
		t.Fatal("voice did not start with PCM boost")
	}
	voice.Pause(true)
	voice.SetGain(0.5)
	if err := voice.SeekMs(40); err != nil {
		t.Fatal(err)
	}
	if voice.PositionMs() != 40 || voice.Playing() || created != 1 || device.starts != 1 || device.volume != 0.5 {
		t.Fatal("gain/seek restarted or retargeted paused voice")
	}
	voice.Pause(false)
	if !voice.Playing() || !voice.Active() {
		t.Fatal("resume failed")
	}
	if err := voice.Close(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0] != "barrier" || events[1] != "close" {
		t.Fatal("source closed before retirement", events)
	}
	if err := voice.Close(); err != nil || len(events) != 2 || voice.Active() {
		t.Fatal("duplicate retirement")
	}
}

func TestFailedPreparationRetainsWorkingVoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signal.raw")
	if err := os.WriteFile(path, bytes.Repeat([]byte{1, 0, 2, 0}, 441), 0o600); err != nil {
		t.Fatal(err)
	}
	closed := 0
	clip, err := authoring.LoadRecording(path, func(data []byte, _ string, _ int) (audio.PCMStream, error) {
		return &source{Reader: bytes.NewReader(data), close: func() { closed++ }}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	device := &player{barrier: func() {}}
	voice := NewVoice()
	voice.create = func(source io.ReadSeeker) (devicePlayer, error) { device.source = source; return device, nil }
	if err := voice.Play(clip, nil, true, 0.5); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("device refused replacement")
	voice.create = func(io.ReadSeeker) (devicePlayer, error) { return nil, failure }
	if err := voice.Play(clip, nil, false, 1); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if !voice.Playing() || device.starts != 1 || closed != 2 {
		t.Fatal("failed replacement disturbed working source", closed)
	}
	if err := voice.Close(); err != nil || closed != 3 {
		t.Fatal("working source not retired", err, closed)
	}
}
