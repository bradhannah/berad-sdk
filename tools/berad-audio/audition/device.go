package audition

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/ebitengine/oto/v3"
)

// Oto permits one context per process. Voice retirement never closes this host.
//
//nolint:gochecknoglobals // sync.Once owns the single process-wide Oto context required by its API.
var device struct {
	once    sync.Once
	context *oto.Context
	err     error
}

//nolint:ireturn // The injected device boundary supports deterministic audition tests without opening audio hardware.
func newNativePlayer(source io.ReadSeeker) (devicePlayer, error) {
	const stereoChannels = 2
	const deviceReadyTimeout = 5 * time.Second
	device.once.Do(func() {
		var ready chan struct{}
		device.context, ready, device.err = oto.NewContext(&oto.NewContextOptions{
			SampleRate: authoring.EditorSampleRate, ChannelCount: stereoChannels, Format: oto.FormatSignedInt16LE,
			ApplicationName: "Berad Audio Tools", BufferSize: 0,
		})
		if device.err != nil {
			return
		}
		timer := time.NewTimer(deviceReadyTimeout)
		defer timer.Stop()
		select {
		case <-ready:
			device.err = device.context.Err()
		case <-timer.C:
			device.err = fmt.Errorf("%w: audio device readiness timed out", ErrInvalidVoice)
		}
	})
	if device.err != nil {
		return nil, fmt.Errorf("open standalone device: %w", device.err)
	}

	return device.context.NewPlayer(source), nil
}
