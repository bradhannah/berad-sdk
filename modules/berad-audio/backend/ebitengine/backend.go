// Package ebitengine adapts a host-owned Ebitengine audio context and its codecs.
// Device progress requires Ebitengine's running game loop; construction or fake
// transports alone do not establish native playback acceptance.
package ebitengine

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"

	ea "github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

// Backend borrows Context and never constructs a second device.
type Backend struct{ Context *ea.Context }

// SampleRate reports the borrowed host rate, or zero when no host is supplied.
func (backend Backend) SampleRate() int {
	if backend.Context == nil {
		return 0
	}

	return backend.Context.SampleRate()
}

// NewPlayer constructs a stopped player on the borrowed context.
//
//nolint:ireturn // The runtime deliberately depends on a transport rather than a concrete native player.
func (backend Backend) NewPlayer(source io.ReadSeeker) (audio.Transport, error) {
	if backend.Context == nil {
		return nil, fmt.Errorf("%w: host context required", audio.ErrInvalidRequest)
	}
	player, err := backend.Context.NewPlayer(source)
	if err != nil {
		return nil, fmt.Errorf("create Ebitengine player: %w", err)
	}

	return player, nil
}

// Decode selects the exact WAV, MP3 or OGG extension and outputs signed 16-bit
// stereo PCM at the host rate. It does not infer alternate paths/extensions.
//
//nolint:ireturn // Codec-specific stream implementations share one PCM ownership contract.
func Decode(encoded []byte, filename string, sampleRate int) (audio.PCMStream, error) {
	reader := bytes.NewReader(encoded)
	var stream audio.PCMStream
	var err error
	switch strings.ToLower(path.Ext(filename)) {
	case ".wav":
		stream, err = wav.DecodeWithSampleRate(sampleRate, reader)
	case ".mp3":
		stream, err = mp3.DecodeWithSampleRate(sampleRate, reader)
	case ".ogg":
		stream, err = vorbis.DecodeWithSampleRate(sampleRate, reader)
	default:
		return nil, fmt.Errorf("%w: unsupported recording extension %s", audio.ErrInvalidLibrary, filename)
	}
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", filename, err)
	}

	return stream, nil
}
