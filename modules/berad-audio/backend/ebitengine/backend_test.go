package ebitengine_test

import (
	"encoding/binary"
	"errors"
	"io"
	"testing"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
)

func TestDecodeSyntheticWAVWithoutGameDataOrDevice(t *testing.T) {
	data := make([]byte, 44+400)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 2)
	binary.LittleEndian.PutUint32(data[24:], 44100)
	binary.LittleEndian.PutUint32(data[28:], 44100*4)
	binary.LittleEndian.PutUint16(data[32:], 4)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 400)
	stream, err := ebitengine.Decode(data, "pulse.wav", 44100)
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := io.ReadAll(stream)
	if err != nil || len(pcm) != 400 {
		t.Fatalf("decoded %d bytes: %v", len(pcm), err)
	}
	if _, err := ebitengine.Decode(data, "pulse.flac", 44100); !errors.Is(err, audio.ErrInvalidLibrary) {
		t.Fatal(err)
	}
	host := ebitengine.Backend{}
	if host.SampleRate() != 0 {
		t.Fatal("missing host accepted")
	}
	if _, err := host.NewPlayer(stream); !errors.Is(err, audio.ErrInvalidRequest) {
		t.Fatal(err)
	}
}
