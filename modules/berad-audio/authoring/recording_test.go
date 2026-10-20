package authoring_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
)

func sourceFixture(t *testing.T, milliseconds int) (string, []byte) {
	t.Helper()
	frames := authoring.EditorSampleRate * milliseconds / 1000
	data := make([]byte, 44+frames*4)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 2)
	binary.LittleEndian.PutUint32(data[24:], authoring.EditorSampleRate)
	binary.LittleEndian.PutUint32(data[28:], authoring.EditorSampleRate*4)
	binary.LittleEndian.PutUint16(data[32:], 4)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(frames*4))
	for frame := frames / 5; frame < frames*4/5; frame++ {
		sample := int16(4000)
		if frame%100 < 50 {
			sample = -sample
		}
		binary.LittleEndian.PutUint16(data[44+frame*4+2:], uint16(sample))
	}
	path := filepath.Join(t.TempDir(), "signal.wav")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data
}

func TestImmutableAnalysisAndSourceVerification(t *testing.T) {
	path, encoded := sourceFixture(t, 1000)
	clip, err := authoring.LoadRecording(path, ebitengine.Decode)
	if err != nil {
		t.Fatal(err)
	}
	if clip.Path() != path || clip.DurationMs() != 1000 {
		t.Fatal("source metadata changed")
	}
	digest := sha256.Sum256(encoded)
	if clip.Fingerprint() != hex.EncodeToString(digest[:]) {
		t.Fatal("fingerprint does not identify encoded source")
	}
	window, ok := clip.SuggestedWindow()
	if !ok || window.StartMs != 190 || window.EndMs != 810 {
		t.Fatalf("wrong source-relative suggestion: %+v", window)
	}
	before, err := clip.SuggestCalibration(window, authoring.DefaultCalibrationOptions())
	if err != nil {
		t.Fatal(err)
	}
	wantRMS := 20 * math.Log10(4000.0/32768/math.Sqrt2)
	if math.Abs(before.MeasuredDB-wantRMS) > 1e-8 {
		t.Fatalf("stereo mean-energy calibration changed: got %v want %v", before.MeasuredDB, wantRMS)
	}
	window.StartMs, window.EndMs = 0, clip.DurationMs()
	unchanged, ok := clip.SuggestedWindow()
	if !ok || unchanged.StartMs != 190 || unchanged.EndMs != 810 {
		t.Fatal("a returned window altered authoritative analysis", unchanged)
	}
	after, err := clip.SuggestCalibration(unchanged, authoring.DefaultCalibrationOptions())
	if err != nil || before != after || clip.PeakBetween(200, 400) == 0 {
		t.Fatal("a returned window altered authoritative analysis")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	encoded[len(encoded)-1] ^= 1
	if err := os.WriteFile(path, encoded, info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := clip.CheckSourceStamp(); err != nil {
		t.Fatal("stamp should be unchanged", err)
	}
	if err := clip.VerifySource(context.Background()); !errors.Is(err, authoring.ErrRecordingSourceChanged) {
		t.Fatalf("same-stamp content replacement accepted: %v", err)
	}
}

type ownedStream struct {
	audio.PCMStream
	closes int
}

type stalledStream struct{ closes int }

func (*stalledStream) Read([]byte) (int, error)       { return 0, nil }
func (*stalledStream) Seek(int64, int) (int64, error) { return 0, nil }
func (*stalledStream) Length() int64                  { return 4 }
func (stream *stalledStream) Close() error            { stream.closes++; return nil }

func TestNonprogressingAnalysisFailsAndRetiresDecoder(t *testing.T) {
	path, _ := sourceFixture(t, 100)
	stream := new(stalledStream)
	_, err := authoring.LoadRecording(path, func([]byte, string, int) (audio.PCMStream, error) {
		return stream, nil
	})
	if !errors.Is(err, io.ErrNoProgress) || stream.closes != 1 {
		t.Fatal("nonprogressing decoder was not rejected and retired", err)
	}
}

func (stream *ownedStream) Close() error { stream.closes++; return nil }

func TestIndependentWindowStreamsAndExactOnceRetirement(t *testing.T) {
	path, _ := sourceFixture(t, 100)
	var streams []*ownedStream
	decode := func(encoded []byte, path string, rate int) (audio.PCMStream, error) {
		stream, err := ebitengine.Decode(encoded, path, rate)
		if err != nil {
			return nil, err
		}
		owned := &ownedStream{PCMStream: stream}
		streams = append(streams, owned)
		return owned, nil
	}
	clip, err := authoring.LoadRecording(path, decode)
	if err != nil || len(streams) != 1 || streams[0].closes != 1 {
		t.Fatalf("analysis failed to close decoder: %v", err)
	}
	window := &authoring.PlaybackWindow{StartMs: 20, EndMs: 60}
	first, err := clip.Stream(window, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := clip.Stream(window, true)
	if err != nil {
		t.Fatal(err)
	}
	finite, err := io.ReadAll(first)
	if err != nil || len(finite) != authoring.EditorSampleRate*40/1000*4 {
		t.Fatalf("incorrect window length: %d, %v", len(finite), err)
	}
	looped := make([]byte, len(finite)*2)
	if _, err := io.ReadFull(second, looped); err != nil || !bytes.Equal(looped[:len(finite)], finite) ||
		!bytes.Equal(looped[len(finite):], finite) {
		t.Fatalf("loop does not match the selected finite PCM: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil || streams[1].closes != 1 || streams[2].closes != 0 {
		t.Fatal("retirement changed another stream or closed twice")
	}
	if _, err := first.Seek(0, io.SeekStart); !errors.Is(err, authoring.ErrStreamClosed) {
		t.Fatal("retired source reopened", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = clip.Stream(&authoring.PlaybackWindow{StartMs: 0, EndMs: 101}, false)
	if !errors.Is(err, authoring.ErrLibrary) || streams[3].closes != 1 {
		t.Fatal("failed window preparation leaked its decoder", err)
	}
}

func TestCancellationAndDecoderMutationDoNotRetargetSnapshot(t *testing.T) {
	path, encoded := sourceFixture(t, 100)
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()
	if _, err := authoring.LoadRecordingContext(ctx, "never-opened", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	decode := func(private []byte, path string, rate int) (audio.PCMStream, error) {
		copyForDecoder := bytes.Clone(private)
		clear(private)
		return ebitengine.Decode(copyForDecoder, path, rate)
	}
	clip, err := authoring.LoadRecording(path, decode)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	if clip.Fingerprint() != hex.EncodeToString(digest[:]) || clip.VerifySource(context.Background()) != nil {
		t.Fatal("decoder mutated frozen encoded bytes")
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	var owned *ownedStream
	_, err = authoring.LoadRecordingContext(ctx, path, func(data []byte, path string, rate int) (audio.PCMStream, error) {
		stream, err := ebitengine.Decode(data, path, rate)
		owned = &ownedStream{PCMStream: stream}
		cancel()
		return owned, err
	})
	if !errors.Is(err, context.Canceled) || owned == nil || owned.closes != 1 {
		t.Fatal("cancelled preparation did not retire decoded source", err)
	}
}

func TestCalibrationPolicyIsExplicitAndObservational(t *testing.T) {
	path, _ := sourceFixture(t, 1000)
	clip, err := authoring.LoadRecording(path, ebitengine.Decode)
	if err != nil {
		t.Fatal(err)
	}
	options := authoring.CalibrationOptions{Method: authoring.GatedLoudness, TargetDB: -23}
	suggestion, err := clip.SuggestCalibration(nil, options)
	if err != nil || suggestion.TargetDB != -23 || suggestion.Method != "Gated loudness (LUFS, estimated)" {
		t.Fatalf("explicit measurement policy not honored: %+v, %v", suggestion, err)
	}
	short, err := clip.SuggestCalibration(&authoring.PlaybackWindow{StartMs: 200, EndMs: 300}, options)
	if err != nil || short.Method != "Active P95 RMS (dBFS, estimated)" {
		t.Fatalf("short selection fallback changed: %+v, %v", short, err)
	}
	for _, invalid := range []authoring.CalibrationOptions{{}, {Method: "score", TargetDB: -23},
		{Method: authoring.ActiveRMS, TargetDB: math.NaN()}} {
		if _, err := clip.SuggestCalibration(nil, invalid); !errors.Is(err, authoring.ErrLibrary) {
			t.Fatal("invalid policy accepted", err)
		}
	}
	if err := clip.VerifySource(context.Background()); err != nil {
		t.Fatal("calibration altered source bytes", err)
	}
}

func TestCalibrationConstrainsRareSamplePeak(t *testing.T) {
	path, data := sourceFixture(t, 1000)
	binary.LittleEndian.PutUint16(data[44+authoring.EditorSampleRate/2*4+2:], 29491)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	clip, err := authoring.LoadRecording(path, ebitengine.Decode)
	if err != nil {
		t.Fatal(err)
	}
	value, err := clip.SuggestCalibration(nil, authoring.DefaultCalibrationOptions())
	if err != nil || !value.Limited || math.Abs(value.PeakDB+value.GainDB+3) > 1e-8 {
		t.Fatalf("sample peak ceiling not applied: %+v %v", value, err)
	}
}

func TestGatedCalibrationReferenceToneAndRelativeGate(t *testing.T) {
	// A -20 dBFS peak, 1 kHz sine is approximately -23 LUFS in one channel;
	// two equal channels add 10*log10(2). This signal-level reference does not
	// derive expected values from the implementation's filter coefficients.
	// The tolerance allows PCM quantization, filter startup and partial gate
	// windows at the loud/quiet transition; this is not meter certification.
	for _, test := range []struct {
		name     string
		channels int
		quiet    bool
	}{
		{"one channel", 1, false},
		{"two channels", 2, false},
		{"relative gate", 2, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, data := sourceFixture(t, 10000)
			clear(data[44:])
			frames := (len(data) - 44) / 4
			for frame := range frames {
				amplitude := 0.1
				if test.quiet && frame >= frames/2 {
					// -60 dBFS peak: above the absolute gate but far below the
					// relative gate. Including it would lower the result ~3 dB.
					amplitude = 0.001
				}
				sample := int16(math.Round(32768 * amplitude * math.Sin(2*math.Pi*1000*float64(frame)/authoring.EditorSampleRate)))
				for channel := range test.channels {
					binary.LittleEndian.PutUint16(data[44+frame*4+channel*2:], uint16(sample))
				}
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			clip, err := authoring.LoadRecording(path, ebitengine.Decode)
			if err != nil {
				t.Fatal(err)
			}
			suggestion, err := clip.SuggestCalibration(nil, authoring.CalibrationOptions{
				Method: authoring.GatedLoudness, TargetDB: -23,
			})
			if err != nil {
				t.Fatal(err)
			}
			want := -23 + 10*math.Log10(float64(test.channels))
			if math.Abs(suggestion.MeasuredDB-want) > 0.2 {
				t.Fatalf("reference tone: measured %.4f, want %.4f ±0.2 LU", suggestion.MeasuredDB, want)
			}
			if suggestion.Limited || math.Abs(suggestion.GainDB-(-23-suggestion.MeasuredDB)) > 1e-8 {
				t.Fatal("unlimited correction did not reach target", suggestion)
			}
		})
	}
}

func TestAuthoringStreamGainPreservesSnapshotAndOddReadSeek(t *testing.T) {
	path, _ := sourceFixture(t, 100)
	clip, err := authoring.LoadRecording(path, ebitengine.Decode)
	if err != nil {
		t.Fatal(err)
	}
	peak := clip.PeakBetween(0, clip.DurationMs())
	stream, err := clip.Stream(&authoring.PlaybackWindow{StartMs: 20, EndMs: 60}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := stream.SetGain(20); err != nil {
		t.Fatal(err)
	}
	var odd []byte
	for {
		buffer := make([]byte, 3)
		count, err := stream.Read(buffer)
		odd = append(odd, buffer[:count]...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := stream.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	again, err := io.ReadAll(stream)
	if err != nil || !bytes.Equal(odd, again) {
		t.Fatal("gain changed with buffer size or seek", err)
	}
	for offset := 2; offset < len(odd); offset += 4 {
		sample := int16(binary.LittleEndian.Uint16(odd[offset:]))
		if sample != 32767 && sample != -32768 {
			t.Fatalf("unsaturated sample %d", sample)
		}
	}
	if err := stream.SetGain(math.NaN()); !errors.Is(err, authoring.ErrLibrary) {
		t.Fatal(err)
	}
	if peak != clip.PeakBetween(0, clip.DurationMs()) {
		t.Fatal("audition gain mutated source analysis")
	}
}
