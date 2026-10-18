package pcm

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sync/atomic"
)

// Gain is a serialized PCM read/seek adapter with an atomically supplied gain.
// It borrows its source; the owning runtime/audition performs retirement/closure.
type Gain struct {
	source          io.ReadSeeker
	gain            *atomic.Uint64
	invalidData     error
	buffer, pending []byte
	pendingErr      error
}

func NewGain(source io.ReadSeeker, gain *atomic.Uint64, invalidData error) *Gain {
	return &Gain{source: source, gain: gain, invalidData: invalidData}
}

func (stream *Gain) Read(buffer []byte) (int, error) {
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
	data := stream.buffer[:wanted]
	count, err := Read(stream.source, data, stream.invalidData)
	if count%sampleBytes != 0 {
		return written, fmt.Errorf("%w: partial PCM sample", stream.invalidData)
	}
	const signedScale, maxSample = 32768, 32767
	gain := math.Float64frombits(stream.gain.Load())
	for offset := 0; gain != 1 && offset < count; offset += sampleBytes {
		sample := int16(binary.LittleEndian.Uint16(data[offset:]))
		value := math.Max(-signedScale, math.Min(maxSample, math.Round(float64(sample)*gain)))
		binary.LittleEndian.PutUint16(data[offset:], uint16(int16(value)))
	}
	copied := copy(buffer[written:], data[:count])
	stream.pending = data[copied:count]
	if len(stream.pending) > 0 {
		stream.pendingErr, err = err, nil
	}
	return written + copied, err
}

func (stream *Gain) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekCurrent {
		offset -= int64(len(stream.pending))
	}
	position, err := stream.source.Seek(offset, whence)
	if err == nil {
		stream.pending, stream.pendingErr = nil, nil
	}
	return position, err
}
