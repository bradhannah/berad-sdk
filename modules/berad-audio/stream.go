package audio

// Adapted from Redux's recording_stream, gain_player and runtime_stream. Keep
// source ownership through windows/loops and the retirement read barrier.
import (
	"errors"
	"fmt"
	"io"
	"math"
	"sync/atomic"
	"time"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/internal/pcm"
)

const bytesPerFrame = 4

type section struct {
	*pcm.Section
	source PCMStream
	size   int64
}

func selectSource(source PCMStream, rate int, window *Window, loop bool) (*section, error) {
	length := source.Length()
	if length <= 0 || length%bytesPerFrame != 0 {
		return nil, fmt.Errorf("%w: positive whole-frame PCM length required", ErrInvalidLibrary)
	}
	start, end := int64(0), length
	if window != nil {
		start = frameOffset(window.Start, rate)
		end = frameOffset(window.End, rate)
		if start < 0 || end <= start || end > length {
			return nil, fmt.Errorf("%w: window exceeds decoded recording", ErrInvalidLibrary)
		}
	}
	selected, err := pcm.Select(source, length, pcm.Options{Start: start, End: end, Loop: loop,
		InvalidData: ErrInvalidLibrary, InvalidRequest: ErrInvalidRequest})
	if err != nil {
		return nil, fmt.Errorf("select playback region: %w", err)
	}

	return &section{Section: selected, source: source, size: selected.Length()}, nil
}

func frameOffset(duration time.Duration, rate int) int64 {
	seconds, remainder := duration/time.Second, duration%time.Second

	return (int64(seconds)*int64(rate) + int64(remainder)*int64(rate)/int64(time.Second)) * bytesPerFrame
}

type readFailure struct{ err error }

// pcmBoundary is read/seek serialized by the backend; gain/failure are concurrent.
type pcmBoundary struct {
	source  *section
	gain    atomic.Uint64
	failure atomic.Pointer[readFailure]
	filter  *pcm.Gain
}

func newPCMBoundary(source *section) *pcmBoundary {
	stream := new(pcmBoundary)
	stream.source = source
	stream.gain.Store(math.Float64bits(1))
	stream.filter = pcm.NewGain(source, &stream.gain, ErrInvalidLibrary)

	return stream
}

func (stream *pcmBoundary) Failure() error {
	if failure := stream.failure.Load(); failure != nil {
		return failure.err
	}

	return nil
}

func (stream *pcmBoundary) Read(buffer []byte) (int, error) {
	if stream.Failure() != nil {
		return 0, io.EOF
	}
	count, err := stream.filter.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		stream.failure.CompareAndSwap(nil, &readFailure{err: fmt.Errorf("read audio: %w", err)})

		return count, io.EOF
	}

	return count, err //nolint:wrapcheck // Only nil or EOF reaches the device; other errors are retained in Failure above.
}

func (stream *pcmBoundary) Seek(offset int64, whence int) (int64, error) {
	if err := stream.Failure(); err != nil {
		return 0, err
	}
	position, err := stream.filter.Seek(offset, whence)
	if err != nil {
		stream.failure.CompareAndSwap(nil, &readFailure{err: fmt.Errorf("seek audio: %w", err)})

		return position, err //nolint:wrapcheck // Failure retains context; preserve the decoder's immediate seek result.
	}

	return position, nil
}

func closeSource(source PCMStream) error {
	if closer, ok := source.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("close decoded recording: %w", err)
		}
	}

	return nil
}
