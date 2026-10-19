package consumer_test

import (
	"testing"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/audition"
	editor "github.com/bradhannah/berad-sdk/tools/berad-audio/editor"
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

func TestEditorPublicEntryPointValidatesBeforeOpeningTerminal(t *testing.T) {
	if err := editor.Run(nil, editor.Options{}); err == nil {
		t.Fatal("missing document accepted")
	}
	var launch func(*authoring.LibraryEditor, editor.Options) error = editor.Run
	if launch == nil {
		t.Fatal("public editor entry point missing")
	}
}
