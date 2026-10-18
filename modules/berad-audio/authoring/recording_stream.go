package authoring

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/internal/pcm"
)

const millisecondsPerSecond = 1000
const decodedPCMBytesPerFrame = 4

// ErrLibrary identifies invalid authoring inputs, not listening suitability.
var ErrLibrary = errors.New("invalid audio authoring input")

// ErrStreamClosed rejects operations on a retired authoring decoder.
var ErrStreamClosed = errors.New("authoring stream closed")

// PlaybackWindow is source-relative, with an exclusive end in milliseconds.
// It matches persisted editing units; it is not an authoritative game duration.
type PlaybackWindow struct {
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
}

// Stream owns an independent decoder. Serialize Read, Seek and Close; stop any
// native reader before Close. Close is idempotent and releases the source exactly
// once, including looping windows. It does not close the immutable RecordingAudio.
type Stream struct {
	section *pcm.Section
	source  audio.PCMStream
}

// Stream decodes frozen bytes using the runtime's shared window/loop mechanism.
// The returned stream must be closed even after EOF. Failed preparation also
// closes its decoder; a later request starts with an independent source.
func (clip *RecordingAudio) Stream(window *PlaybackWindow, loop bool) (*Stream, error) {
	return decodeRecording(clip.data, clip.path, clip.decode, window, loop)
}

func decodeRecording(data []byte, path string, decode audio.Decoder, window *PlaybackWindow, loop bool) (*Stream, error) {
	if decode == nil {
		return nil, fmt.Errorf("%w: load a recording before streaming", ErrLibrary)
	}
	source, err := decode(bytes.Clone(data), path, EditorSampleRate)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("decode recording: %w", err), closeDecoded(source))
	}
	if source == nil {
		return nil, fmt.Errorf("%w: decoder returned no recording", ErrLibrary)
	}
	length := source.Length()
	if length <= 0 || length%decodedPCMBytesPerFrame != 0 || length > math.MaxInt64/millisecondsPerSecond {
		return nil, errors.Join(fmt.Errorf("%w: invalid decoded recording length", ErrLibrary), closeDecoded(source))
	}
	start, end := int64(0), length
	if window != nil {
		if window.StartMs < 0 || window.EndMs <= window.StartMs || window.EndMs > length/decodedPCMBytesPerFrame*millisecondsPerSecond/EditorSampleRate {
			return nil, errors.Join(fmt.Errorf("%w: excerpt exceeds recording duration", ErrLibrary), closeDecoded(source))
		}
		start = window.StartMs * EditorSampleRate / millisecondsPerSecond * decodedPCMBytesPerFrame
		end = window.EndMs * EditorSampleRate / millisecondsPerSecond * decodedPCMBytesPerFrame
	}
	selected, err := pcm.Select(source, length, pcm.Options{Start: start, End: end, Loop: loop,
		InvalidData: ErrLibrary, InvalidRequest: ErrLibrary})
	if err != nil {
		return nil, errors.Join(err, closeDecoded(source))
	}
	return &Stream{section: selected, source: source}, nil
}

func (stream *Stream) Read(buffer []byte) (int, error) {
	if stream.source == nil {
		return 0, ErrStreamClosed
	}
	return stream.section.Read(buffer)
}

func (stream *Stream) Seek(offset int64, whence int) (int64, error) {
	if stream.source == nil {
		return 0, ErrStreamClosed
	}
	return stream.section.Seek(offset, whence)
}

// Length reports selected PCM bytes while live; retired streams report zero.
func (stream *Stream) Length() int64 {
	if stream.source == nil {
		return 0
	}
	return stream.section.Length()
}

func (stream *Stream) Close() error {
	err := closeDecoded(stream.source)
	stream.source, stream.section = nil, nil
	return err
}

func closeDecoded(source audio.PCMStream) error {
	if closer, ok := source.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("close decoded recording: %w", err)
		}
	}
	return nil
}
