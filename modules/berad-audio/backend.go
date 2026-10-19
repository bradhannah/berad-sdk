package audio

import (
	"io"
	"time"
)

// Transport consumes stereo signed 16-bit little-endian PCM at Backend.SampleRate.
// Creation must not start playback. Read/seek operations must be serialized.
// Pause is resumable; PauseAndStopReading must synchronously wait for all source
// reads before returning. The runtime, not the transport, closes the source.
// Transport calls must not re-enter the runtime. Control operations may block.
type Transport interface {
	Play()
	Pause()
	PauseAndStopReading()
	IsPlaying() bool
	SetVolume(gain float64)
	SetPosition(position time.Duration) error
	Position() time.Duration
}

// Backend is borrowed; Runtime.Close never closes its device. SampleRate must
// remain stable and positive. Shared backends must serialize their own creation.
type Backend interface {
	SampleRate() int
	NewPlayer(source io.ReadSeeker) (Transport, error)
}

// PCMStream is a seekable decoded source with a positive whole-frame length.
// If it implements io.Closer, ownership transfers to the runtime on successful
// decoding, including preparation failures and window/loop adaptation.
type PCMStream interface {
	io.ReadSeeker
	Length() int64
}

// Decoder receives a private encoded copy, so it cannot mutate frozen library
// bytes. Every playback creates an independent decoder/cursor.
type Decoder func(encoded []byte, filename string, sampleRate int) (PCMStream, error)
