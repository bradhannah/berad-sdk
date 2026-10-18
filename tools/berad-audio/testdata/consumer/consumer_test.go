package consumer_test

import (
	"testing"

	"github.com/bradhannah/berad-sdk/tools/berad-audio/audition"
)

func TestStandaloneServiceConstructionDoesNotOpenDevice(t *testing.T) {
	voice := audition.NewVoice()
	if voice.Active() || voice.Playing() || voice.PositionMs() != 0 || voice.Failure() != nil {
		t.Fatal("new voice owns playback")
	}
	voice.SetGain(0.5)
	voice.Pause(true)
	if err := voice.SeekMs(100); err != nil {
		t.Fatal(err)
	}
	if err := voice.Close(); err != nil {
		t.Fatal(err)
	}
	if err := voice.Close(); err != nil {
		t.Fatal(err)
	}
}
