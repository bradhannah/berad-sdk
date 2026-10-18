package audio

// Adapted from Redux's recording_stream, gain_player and runtime_stream. Keep
// source ownership through windows/loops and the retirement read barrier.
import (
	"encoding/binary"
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
		return nil, err
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
	source          *section
	gain            atomic.Uint64
	failure         atomic.Pointer[readFailure]
	buffer, pending []byte
	pendingErr      error
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
	count, err := stream.readPCM(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		stream.failure.CompareAndSwap(nil, &readFailure{err: fmt.Errorf("read audio: %w", err)})

		return count, io.EOF
	}

	return count, err
}

func (stream *pcmBoundary) readPCM(buffer []byte) (int, error) {
	written := copy(buffer, stream.pending)
	stream.pending = stream.pending[written:]
	if len(stream.pending) == 0 && stream.pendingErr != nil {
		err := stream.pendingErr
		stream.pendingErr = nil

		return written, err
	}
	if written == len(buffer) {
		return written, nil
	}
	const sampleBytes = 2
	wanted := (len(buffer) - written + sampleBytes - 1) / sampleBytes * sampleBytes
	if cap(stream.buffer) < wanted {
		stream.buffer = make([]byte, wanted)
	}
	pcm := stream.buffer[:wanted]
	count, err := readPCM(stream.source, pcm)
	if count%sampleBytes != 0 {
		return written, fmt.Errorf("%w: partial PCM sample", ErrInvalidLibrary)
	}
	const signedScale, maxSample = 32768, 32767
	gain := math.Float64frombits(stream.gain.Load())
	for offset := 0; gain != 1 && offset < count; offset += sampleBytes {
		//nolint:gosec // Bit-preserving signed PCM reinterpretation.
		sample := int16(binary.LittleEndian.Uint16(pcm[offset:]))
		value := math.Max(-signedScale, math.Min(maxSample, math.Round(float64(sample)*gain)))
		//nolint:gosec // Saturated signed PCM stored as unsigned sample bits.
		binary.LittleEndian.PutUint16(pcm[offset:], uint16(int16(value)))
	}
	copied := copy(buffer[written:], pcm[:count])
	stream.pending = pcm[copied:count]
	if len(stream.pending) > 0 {
		stream.pendingErr, err = err, nil
	}

	return written + copied, err
}

func (stream *pcmBoundary) Seek(offset int64, whence int) (int64, error) {
	if err := stream.Failure(); err != nil {
		return 0, err
	}
	if whence == io.SeekCurrent {
		offset -= int64(len(stream.pending))
	}
	position, err := stream.source.Seek(offset, whence)
	if err != nil {
		stream.failure.CompareAndSwap(nil, &readFailure{err: fmt.Errorf("seek audio: %w", err)})

		return position, err
	}
	stream.pending, stream.pendingErr = nil, nil

	return position, nil
}

//nolint:wrapcheck // Preserve one-time decoder errors, including errors with a full buffer.
func readPCM(source io.Reader, buffer []byte) (int, error) {
	return pcm.Read(source, buffer, ErrInvalidLibrary)
}

func closeSource(source PCMStream) error {
	if closer, ok := source.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("close decoded recording: %w", err)
		}
	}

	return nil
}
