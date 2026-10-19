package consumer_test

import (
	"bytes"
	"io"
	"testing"
	"testing/fstest"
	"time"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

type stream struct{ *bytes.Reader }

func (source stream) Length() int64 { return source.Size() }

type device struct{ playing bool }

func (player *device) Play()                    { player.playing = true }
func (player *device) Pause()                   { player.playing = false }
func (player *device) PauseAndStopReading()     { player.playing = false }
func (player *device) IsPlaying() bool          { return player.playing }
func (*device) SetVolume(float64)               {}
func (*device) SetPosition(time.Duration) error { return nil }
func (*device) Position() time.Duration         { return 0 }

type backend struct{}

func (backend) SampleRate() int                                  { return 44100 }
func (backend) NewPlayer(io.ReadSeeker) (audio.Transport, error) { return new(device), nil }

func TestConsumerOwnsCatalogAndVoiceLifetime(t *testing.T) {
	library, err := audio.NewLibrary(audio.Definition{
		Recordings: map[audio.RecordingID]audio.Recording{"motor": {File: "motor.pcm"}},
		Cues:       map[audio.CueID]audio.Cue{"hangar/motor": {Recordings: []audio.RecordingID{"motor"}, Gain: 1}},
	}, fstest.MapFS{"motor.pcm": {Data: []byte{1, 2, 3, 4}}}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := audio.NewRuntime(library, audio.DefaultOptions(backend{}, func(data []byte, _ string, _ int) (audio.PCMStream, error) { return stream{bytes.NewReader(data)}, nil }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	owner := runtime.NewOwner()
	request := audio.DefaultPlayOptions()
	request.Loop = true
	result, err := owner.Play("hangar/motor", request)
	if err != nil || result.Outcome != audio.Admitted {
		t.Fatalf("play: %+v %v", result, err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if len(runtime.Snapshot()) != 0 {
		t.Fatal("owned voice survived closure")
	}
}
