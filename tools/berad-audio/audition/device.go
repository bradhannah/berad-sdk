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
var device struct {
	once    sync.Once
	context *oto.Context
	err     error
}

func newNativePlayer(source io.ReadSeeker) (devicePlayer, error) {
	device.once.Do(func() {
		var ready chan struct{}
		device.context, ready, device.err = oto.NewContext(&oto.NewContextOptions{
			SampleRate: authoring.EditorSampleRate, ChannelCount: 2, Format: oto.FormatSignedInt16LE,
			ApplicationName: "Berad Audio Tools",
		})
		if device.err != nil {
			return
		}
		timer := time.NewTimer(5 * time.Second)
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
