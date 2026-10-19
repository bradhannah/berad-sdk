package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path"
	"strings"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

const catalogMeasurementSampleRate = 44100
const catalogMillisecondsPerSecond = 1000
const catalogPCMFrameBytes = 4

func formatName(filename string) string {
	switch strings.ToLower(path.Ext(filename)) {
	case ".wav":
		return "WAV"
	case ".mp3":
		return "MP3"
	case ".ogg":
		return "OGG"
	default:
		return provenanceUnknown
	}
}

// InspectAsset inspects one bounded encoded file without opening a device.
func InspectAsset(id, path string, files fs.FS, options Options) (CatalogAsset, error) {
	var asset CatalogAsset
	asset.ID, asset.Path, asset.Availability = id, options.PathPrefix+path, CatalogPresent
	asset.Format = formatName(path)
	asset.ListeningReview, asset.LoopReview = CatalogUnreviewed, CatalogUnreviewed
	asset.Provenance = RecordingProvenance{
		OriginalFilename: provenanceUnknown, SourceType: provenanceUnknown, SourceURL: provenanceUnknown,
		SourceCreator: provenanceUnknown, License: provenanceUnknown, ImportedFrom: provenanceUnknown,
		Processing: []authoring.ProcessingStep{},
	}
	data, err := readAsset(files, path)
	if err != nil {
		return asset, err
	}
	digest := sha256.Sum256(data)
	asset.SHA256 = hex.EncodeToString(digest[:])
	stream, err := options.Decode(bytes.Clone(data), path, catalogMeasurementSampleRate)
	if err != nil {
		if closer, ok := stream.(io.Closer); ok {
			err = errors.Join(err, closer.Close())
		}

		return asset, fmt.Errorf("inspect audio recording %s: %w", path, err)
	}
	if stream == nil {
		return asset, fmt.Errorf("%w: decoder returned no stream", ErrAudioCatalog)
	}
	length := stream.Length()
	closeStream := func() error {
		if closer, ok := stream.(io.Closer); ok {
			return closer.Close()
		}

		return nil
	}
	if length <= 0 || length%catalogPCMFrameBytes != 0 || length > math.MaxInt64/catalogMillisecondsPerSecond {
		return asset, errors.Join(
			fmt.Errorf("%w: recording %s has no finite decoded length", ErrAudioCatalog, path), closeStream())
	}
	asset.DurationMs = length * catalogMillisecondsPerSecond / (catalogMeasurementSampleRate * catalogPCMFrameBytes)
	if options.Measure {
		peak, rms, err := measureCatalogPCM(stream)
		if err != nil {
			return asset, errors.Join(fmt.Errorf("measure recording %s: %w", path, err), closeStream())
		}
		asset.PeakDBFS, asset.RMSDBFS = amplitudeDB(peak), amplitudeDB(rms)
		asset.PCMMeasured = true
	}

	return asset, closeStream()
}

func readAsset(files fs.FS, path string) ([]byte, error) {
	const maxEncodedBytes = 128 << 20
	info, err := fs.Stat(files, path)
	if err != nil {
		return nil, fmt.Errorf("inspect recording %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxEncodedBytes {
		return nil, fmt.Errorf("%w: bounded regular recording required", ErrAudioCatalog)
	}
	file, err := files.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read audio recording %s: %w", path, err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxEncodedBytes+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, fmt.Errorf("read recording: %w", err)
	}
	if len(data) > maxEncodedBytes {
		return nil, fmt.Errorf("%w: encoded recording exceeds 128 MiB", ErrAudioCatalog)
	}

	return data, nil
}

// measureCatalogPCM streams interleaved signed 16-bit PCM. These measurements
// describe the raw decoded asset, not LUFS, true peak, or perceived acceptance.
func measureCatalogPCM(stream io.Reader) (float64, float64, error) {
	const (
		bufferBytes = 8192
		fullScale   = 32768.0
		sampleBytes = 2
	)
	buffer := make([]byte, bufferBytes)
	var peak, squares float64
	var samples int64
	for {
		count, err := io.ReadFull(stream, buffer)
		if count%sampleBytes != 0 {
			return 0, 0, fmt.Errorf("%w: incomplete PCM sample", ErrAudioCatalog)
		}
		for offset := 0; offset < count; offset += sampleBytes {
			// PCM intentionally reinterprets the unsigned word as a signed sample.
			//nolint:gosec // This is a bit-preserving signed PCM conversion, not a narrowing length.
			sample := float64(int16(binary.LittleEndian.Uint16(buffer[offset:offset+sampleBytes]))) / fullScale
			peak = math.Max(peak, math.Abs(sample))
			squares += sample * sample
			samples++
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return 0, 0, fmt.Errorf("read decoded PCM: %w", err)
		}
	}
	if samples == 0 {
		return 0, 0, fmt.Errorf("%w: empty recording", ErrAudioCatalog)
	}

	return peak, math.Sqrt(squares / float64(samples)), nil
}

func amplitudeDB(amplitude float64) *float64 {
	if amplitude == 0 {
		return nil // Negative infinity is not valid JSON. Never substitute a finite level.
	}
	const amplitudeDecibelFactor = 20
	value := amplitudeDecibelFactor * math.Log10(amplitude)

	return &value
}
