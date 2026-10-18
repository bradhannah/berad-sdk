package authoring

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/internal/pcm"
)

// Editor analysis/audition uses fixed-rate stereo; runtime adapters decode at
// the host context's rate. Authored windows remain source-relative milliseconds.
const (
	EditorSampleRate   = 44100
	WaveformBucketMs   = 10
	maxEditorFileBytes = 128 << 20
	signedPCMScale     = 32768.0
)

// RecordingAudio is immutable decoded metadata plus encoded bytes. Each player
// gets an independent decoder. Peak buckets preserve transients in both channels.
type RecordingAudio struct {
	path        string
	durationMs  int64
	peaks       []float64
	data        []byte
	loudness    []loudnessBucket
	fingerprint [sha256.Size]byte
	fileInfo    os.FileInfo
	decode      audio.Decoder
}

// Path returns the absolute source pathname captured at loading.
func (clip *RecordingAudio) Path() string { return clip.path }

// DurationMs returns the full decoded source duration in milliseconds.
func (clip *RecordingAudio) DurationMs() int64 { return clip.durationMs }

// Peaks returns a detached copy of the source's stereo peak buckets. Editing the
// returned slice cannot change silence suggestions, calibration or playback.
func (clip *RecordingAudio) Peaks() []float64 { return slices.Clone(clip.peaks) }

// PeakBetween returns the greatest peak in the 10 ms buckets intersecting the
// inclusive source-time range. Bounds are clipped to the source; empty/reversed
// ranges return zero. Renderers can inspect a range without copying all peaks.
func (clip *RecordingAudio) PeakBetween(startMs, endMs int64) float64 {
	if endMs < startMs || endMs < 0 {
		return 0
	}
	peak := 0.0
	limit := min(endMs/WaveformBucketMs+1, int64(len(clip.peaks)))
	for bucket := max(0, startMs/WaveformBucketMs); bucket < limit; bucket++ {
		peak = math.Max(peak, clip.peaks[bucket])
	}

	return peak
}

// LoadRecording can run outside the UI loop. It streams analysis rather than
// retaining the entire decoded PCM of a long music track.
func LoadRecording(path string, decode audio.Decoder) (*RecordingAudio, error) {
	return LoadRecordingContext(context.Background(), path, decode)
}

// LoadRecordingContext cancels obsolete waveform requests during fast browsing.
// It checks cancellation between bounded file reads and decoded analysis buckets;
// it cannot interrupt an in-flight filesystem or codec call.
func LoadRecordingContext(ctx context.Context, path string, decode audio.Decoder) (*RecordingAudio, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load recording canceled: %w", err)
	}
	if decode == nil {
		return nil, fmt.Errorf("%w: recording decoder is required", ErrLibrary)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve recording path: %w", err)
	}
	data, info, err := readRecordingSource(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load recording canceled: %w", err)
	}
	stream, err := decode(bytes.Clone(data), path, EditorSampleRate)
	if cancelErr := ctx.Err(); cancelErr != nil {
		return nil, errors.Join(fmt.Errorf("load recording canceled: %w", cancelErr), closeDecoded(stream))
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("decode recording: %w", err), closeDecoded(stream))
	}
	if stream == nil {
		return nil, fmt.Errorf("%w: decoder returned no recording", ErrLibrary)
	}
	length := stream.Length()
	if length <= 0 || length%decodedPCMBytesPerFrame != 0 || length > math.MaxInt64/millisecondsPerSecond {
		return nil, errors.Join(fmt.Errorf("%w: invalid decoded recording length", ErrLibrary), closeDecoded(stream))
	}
	peaks, loudness, analysisErr := recordingAnalysis(ctx, stream)
	if err := errors.Join(analysisErr, closeDecoded(stream)); err != nil {
		return nil, err
	}

	fingerprint := sha256.Sum256(data)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load recording canceled: %w", err)
	}

	return &RecordingAudio{path: path, data: data, peaks: peaks, loudness: loudness,
		durationMs:  length * millisecondsPerSecond / (EditorSampleRate * decodedPCMBytesPerFrame),
		fingerprint: fingerprint, fileInfo: info, decode: decode}, nil
}

func readRecordingSource(ctx context.Context, path string) ([]byte, os.FileInfo, error) {
	initialInfo, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect recording: %w", err)
	}
	if !initialInfo.Mode().IsRegular() || initialInfo.Size() > maxEditorFileBytes {
		return nil, nil, fmt.Errorf("%w: select a regular recording no larger than 128 MiB", ErrLibrary)
	}
	file, err := os.Open(path) //nolint:gosec // Explicit local recording chosen in the authoring tool.
	if err != nil {
		return nil, nil, fmt.Errorf("open recording: %w", err)
	}
	data, info, readErr := readRecordingFile(ctx, file, initialInfo)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, nil, fmt.Errorf("read recording source: %w", err)
	}
	return data, info, nil
}

func readRecordingFile(ctx context.Context, file *os.File, initialInfo os.FileInfo) ([]byte, os.FileInfo, error) {
	data, err := readRecordingBytes(ctx, file)
	if err != nil {
		return nil, nil, fmt.Errorf("read recording: %w", err)
	}
	if len(data) > maxEditorFileBytes {
		return nil, nil, fmt.Errorf("%w: recording exceeds 128 MiB", ErrLibrary)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("inspect recording source: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(data)) || !os.SameFile(initialInfo, info) ||
		!initialInfo.ModTime().Equal(info.ModTime()) {
		return nil, nil, fmt.Errorf("%w: recording changed during loading", ErrRecordingSourceChanged)
	}

	return data, info, nil
}

func recordingAnalysis(ctx context.Context, stream io.Reader) ([]float64, []loudnessBucket, error) {
	buffer := make([]byte, EditorSampleRate*WaveformBucketMs/millisecondsPerSecond*decodedPCMBytesPerFrame)
	var peaks []float64
	var loudness []loudnessBucket
	filters := kWeightingFilters()
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, fmt.Errorf("analyze waveform: %w", err)
		}
		count, err := pcm.Read(analysisReader{ctx: ctx, source: stream}, buffer, ErrLibrary)
		if cancelErr := ctx.Err(); cancelErr != nil {
			return nil, nil, fmt.Errorf("analyze waveform: %w", cancelErr)
		}
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, nil, fmt.Errorf("analyze waveform: %w", err)
		}
		if count%decodedPCMBytesPerFrame != 0 {
			return nil, nil, fmt.Errorf("%w: partial PCM frame during analysis", ErrLibrary)
		}
		peak := 0.0
		bucket := loudnessBucket{frames: count / decodedPCMBytesPerFrame, energy: 0, weighted: 0, peak: 0}
		const stereoChannels = 2
		const sampleBytes = 2
		for offset := 0; offset+1 < count; offset += 2 {
			//nolint:gosec // PCM bit reinterpretation of an unsigned little-endian word as signed 16-bit audio.
			sample := int16(binary.LittleEndian.Uint16(buffer[offset:]))
			peak = math.Max(peak, math.Abs(float64(sample)/signedPCMScale))
			value := float64(sample) / signedPCMScale
			channel := (offset / sampleBytes) % stereoChannels
			weighted := filters[channel][1].sample(filters[channel][0].sample(value))
			bucket.energy += value * value / stereoChannels // Stereo mean energy for RMS.
			bucket.weighted += weighted * weighted          // BS.1770 sums channel energy.
		}
		if count > 0 {
			peaks = append(peaks, peak)
			bucket.peak = peak
			loudness = append(loudness, bucket)
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return peaks, loudness, nil
		}
	}
}

type analysisReader struct {
	ctx    context.Context
	source io.Reader
}

func (reader analysisReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.source.Read(buffer)
}

// SuggestedWindow proposes -40 dB boundaries with one bucket of edge padding.
// It never changes the document. All-silent files have no useful suggestion.
func (clip *RecordingAudio) SuggestedWindow() (*PlaybackWindow, bool) {
	const threshold = 0.01
	first, last := -1, -1
	for index, peak := range clip.peaks {
		if peak >= threshold {
			if first == -1 {
				first = index
			}
			last = index
		}
	}
	if first < 0 {
		return nil, false
	}
	start := int64(max(0, first-1)) * WaveformBucketMs
	const endPaddingBuckets = 2 // End of the last audible bucket plus one padding bucket.
	end := min(clip.durationMs, int64(last+endPaddingBuckets)*WaveformBucketMs)

	return &PlaybackWindow{StartMs: start, EndMs: end}, end > start
}
