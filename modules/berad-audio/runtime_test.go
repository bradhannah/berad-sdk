package audio_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

type pcm struct {
	*bytes.Reader
	length           int64
	closed           int
	failure          error
	entered, unblock chan struct{}
}

func (source *pcm) Length() int64 { return source.length }
func (source *pcm) Close() error  { source.closed++; return nil }
func (source *pcm) Read(buffer []byte) (int, error) {
	if source.entered != nil {
		close(source.entered)
		<-source.unblock
		source.entered = nil
	}
	n, err := source.Reader.Read(buffer)
	if source.failure != nil {
		return n, source.failure
	}
	return n, err
}

type transport struct {
	mu             sync.Mutex
	source         io.ReadSeeker
	playing        bool
	gain           float64
	barriers       int
	barrierEntered chan struct{}
	position       time.Duration
}

func (device *transport) Play()  { device.playing = true }
func (device *transport) Pause() { device.playing = false }
func (device *transport) PauseAndStopReading() {
	if device.barrierEntered != nil {
		close(device.barrierEntered)
		device.barrierEntered = nil
	}
	device.mu.Lock()
	defer device.mu.Unlock()
	device.playing = false
	device.barriers++
}
func (device *transport) IsPlaying() bool        { return device.playing }
func (device *transport) SetVolume(gain float64) { device.gain = gain }
func (device *transport) SetPosition(position time.Duration) error {
	_, err := device.source.Seek(int64(position/time.Millisecond)*4, io.SeekStart)
	if err == nil {
		device.position = position
	}
	return err
}
func (device *transport) Position() time.Duration { return device.position }
func (device *transport) read(count int) ([]byte, error) {
	device.mu.Lock()
	defer device.mu.Unlock()
	buffer := make([]byte, count)
	n, err := device.source.Read(buffer)
	return buffer[:n], err
}

type backend struct {
	players []*transport
	failure error
}

func (*backend) SampleRate() int { return 1000 }
func (host *backend) NewPlayer(source io.ReadSeeker) (audio.Transport, error) {
	if host.failure != nil {
		return nil, host.failure
	}
	player := &transport{source: source}
	host.players = append(host.players, player)
	return player, nil
}

type fixture struct {
	runtime   *audio.Runtime
	backend   *backend
	sources   []*pcm
	time      time.Time
	decodeErr error
}

func newFixture(t *testing.T, volume float64, limit int) *fixture {
	t.Helper()
	definition := audio.Definition{
		Recordings: map[audio.RecordingID]audio.Recording{"a": {File: "a.pcm"}, "b": {File: "b.pcm"}, "silent": {}},
		Cues: map[audio.CueID]audio.Cue{"panel/accept": {Recordings: []audio.RecordingID{"a", "b"}, Gain: 0.5},
			"empty": {Recordings: []audio.RecordingID{"silent"}, Gain: 1}},
	}
	files := fstest.MapFS{"a.pcm": {Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}}, "b.pcm": {Data: []byte{9, 10, 11, 12, 13, 14, 15, 16}}}
	library, err := audio.NewLibrary(definition, files, 1024)
	if err != nil {
		t.Fatal(err)
	}
	state := &fixture{backend: new(backend), time: time.Now()}
	options := audio.DefaultOptions(state.backend, func(data []byte, _ string, _ int) (audio.PCMStream, error) {
		if state.decodeErr != nil {
			return nil, state.decodeErr
		}
		source := &pcm{Reader: bytes.NewReader(data), length: int64(len(data))}
		state.sources = append(state.sources, source)
		return source, nil
	})
	options.Now = func() time.Time { return state.time }
	options.Volume, options.VoiceLimit = volume, limit
	state.runtime, err = audio.NewRuntime(library, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := state.runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	return state
}

func play(t *testing.T, runtime *audio.Runtime, options audio.PlayOptions) audio.Voice {
	t.Helper()
	result, err := runtime.Play("panel/accept", options)
	if err != nil || result.Outcome != audio.Admitted {
		t.Fatalf("play: %+v, %v", result, err)
	}
	return result.Voice
}

func TestLibrarySnapshotOwnsNestedMetadataAndAssets(t *testing.T) {
	window := &audio.Window{Start: time.Millisecond, End: 2 * time.Millisecond}
	definition := audio.Definition{Recordings: map[audio.RecordingID]audio.Recording{"a": {File: "a.pcm", Window: window}},
		Cues: map[audio.CueID]audio.Cue{"accept": {Recordings: []audio.RecordingID{"a"}, Gain: 1}}}
	files := fstest.MapFS{"a.pcm": {Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}}}
	library, err := audio.NewLibrary(definition, files, 8)
	if err != nil {
		t.Fatal(err)
	}
	window.End = 99 * time.Second
	definition.Cues["accept"].Recordings[0] = "wrong"
	clear(files["a.pcm"].Data)
	host := new(backend)
	options := audio.DefaultOptions(host, func(data []byte, _ string, _ int) (audio.PCMStream, error) {
		return &pcm{Reader: bytes.NewReader(data), length: int64(len(data))}, nil
	})
	runtime, err := audio.NewRuntime(library, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	result, err := runtime.Play("accept", audio.DefaultPlayOptions())
	if err != nil || result.Outcome != audio.Admitted {
		t.Fatal("input mutation retargeted playback", result, err)
	}
	if status := result.Voice.Status(); status.Recording != "a" || status.Duration != time.Millisecond || library.EncodedBytes() != 8 {
		t.Fatal("input mutation changed the frozen window", status)
	}
	played, err := host.players[0].read(4)
	if err != nil || !bytes.Equal(played, []byte{5, 6, 7, 8}) {
		t.Fatal("input mutation changed selected PCM", played, err)
	}
	_, err = audio.NewLibrary(definition, files, 7)
	if !errors.Is(err, audio.ErrInvalidLibrary) {
		t.Fatalf("invalid reference/budget accepted: %v", err)
	}
	definition.Cues["accept"].Recordings[0] = "a"
	window.End = 2 * time.Millisecond
	_, err = audio.NewLibrary(definition, files, 7)
	if !errors.Is(err, audio.ErrInvalidLibrary) || !bytes.Contains([]byte(err.Error()), []byte("budget")) {
		t.Fatalf("encoded budget not enforced: %v", err)
	}
}

func TestDecoderCannotMutateLibraryOrSiblingRuntime(t *testing.T) {
	definition := audio.Definition{Recordings: map[audio.RecordingID]audio.Recording{"a": {File: "a.pcm"}},
		Cues: map[audio.CueID]audio.Cue{"accept": {Recordings: []audio.RecordingID{"a"}, Gain: 1}}}
	files := fstest.MapFS{"a.pcm": {Data: []byte{1, 2, 3, 4}}}
	library, err := audio.NewLibrary(definition, files, 4)
	if err != nil {
		t.Fatal(err)
	}
	clear(files["a.pcm"].Data)
	decode := func(data []byte, _ string, _ int) (audio.PCMStream, error) {
		if !bytes.Equal(data, []byte{1, 2, 3, 4}) {
			t.Errorf("snapshot changed: %v", data)
		}
		clear(data)
		return &pcm{Reader: bytes.NewReader(data), length: 4}, nil
	}
	for range 2 {
		runtime, err := audio.NewRuntime(library, audio.DefaultOptions(new(backend), decode))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Play("accept", audio.DefaultPlayOptions()); err != nil {
			t.Fatal(err)
		}
		if err := runtime.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCueAndRequestGainsReachTwelveDecibels(t *testing.T) {
	files := fstest.MapFS{"a.pcm": {Data: []byte{1, 2, 3, 4}}}
	louder := func(gain float64) audio.Definition {
		return audio.Definition{Recordings: map[audio.RecordingID]audio.Recording{"a": {File: "a.pcm"}},
			Cues: map[audio.CueID]audio.Cue{"loud": {Recordings: []audio.RecordingID{"a"}, Gain: gain}}}
	}
	if _, err := audio.NewLibrary(louder(audio.MaxGain*1.001), files, 4); !errors.Is(err, audio.ErrInvalidLibrary) {
		t.Fatalf("a cue gain above +12 dB was accepted: %v", err)
	}
	library, err := audio.NewLibrary(louder(audio.MaxGain), files, 4)
	if err != nil {
		t.Fatal(err)
	}
	host := new(backend)
	runtime, err := audio.NewRuntime(library, audio.DefaultOptions(host, func(data []byte, _ string, _ int) (audio.PCMStream, error) {
		return &pcm{Reader: bytes.NewReader(data), length: int64(len(data))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	request := audio.DefaultPlayOptions()
	request.Gain = 2
	result, err := runtime.Play("loud", request)
	if err != nil || result.Outcome != audio.Admitted {
		t.Fatalf("a request louder than unity was refused: %+v, %v", result, err)
	}
	if snapshot := runtime.Snapshot()[0]; math.Abs(snapshot.TargetGain-2*audio.MaxGain) > 1e-9 {
		t.Fatalf("target gain %v, want cue gain times request gain", snapshot.TargetGain)
	}
	request.Gain = audio.MaxGain * 1.001
	if _, err := runtime.Play("loud", request); !errors.Is(err, audio.ErrInvalidRequest) {
		t.Fatalf("a request above +12 dB was accepted: %v", err)
	}
}

func TestInvalidRequestCannotEvictOrAdvanceSelection(t *testing.T) {
	state := newFixture(t, 1, 1)
	play(t, state.runtime, audio.DefaultPlayOptions())
	for _, gain := range []float64{-1, audio.MaxGain + 1, math.NaN(), math.Inf(1)} {
		request := audio.DefaultPlayOptions()
		request.Gain = gain
		if _, err := state.runtime.Play("panel/accept", request); !errors.Is(err, audio.ErrInvalidRequest) {
			t.Fatal(err)
		}
	}
	if state.sources[0].closed != 0 || len(state.sources) != 1 {
		t.Fatal("invalid request affected voices")
	}
	if err := state.runtime.StopCue("panel/accept", 0); err != nil {
		t.Fatal(err)
	}
	play(t, state.runtime, audio.DefaultPlayOptions())
	if state.runtime.Snapshot()[0].Recording != "b" {
		t.Fatal("invalid requests advanced cycle")
	}
}

func TestHealthyCompletionIsRetiredWithoutFailure(t *testing.T) {
	state := newFixture(t, 1, 1)
	play(t, state.runtime, audio.DefaultPlayOptions())
	_, err := state.backend.players[0].read(12)
	if !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	state.backend.players[0].playing = false
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	if len(state.runtime.Snapshot()) != 0 || state.sources[0].closed != 1 {
		t.Fatal("completion leaked voice")
	}
}

func TestErrorQueueIsBoundedAndReportsOverflow(t *testing.T) {
	state := newFixture(t, 1, 1)
	failure := errors.New("repeated decoder failure")
	for range 40 {
		voice := play(t, state.runtime, audio.DefaultPlayOptions())
		source := state.sources[len(state.sources)-1]
		source.failure = failure
		_, err := state.backend.players[len(state.backend.players)-1].read(4)
		if !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if err := voice.Stop(0); err != nil {
			t.Fatal(err)
		}
	}
	err := state.runtime.Update()
	if !errors.Is(err, failure) || !bytes.Contains([]byte(err.Error()), []byte("8 additional audio failures omitted")) {
		t.Fatalf("missing bounded failure evidence: %v", err)
	}
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
}

func TestMuteAndEmptyDoNotDecodeOrAdvanceSelection(t *testing.T) {
	state := newFixture(t, 0, 2)
	state.decodeErr = errors.New("bad encoded data")
	result, err := state.runtime.Play("panel/accept", audio.DefaultPlayOptions())
	if err != nil || result.Outcome != audio.Inaudible || len(state.sources) != 0 {
		t.Fatalf("muted: %+v %v", result, err)
	}
	result, err = state.runtime.Play("empty", audio.DefaultPlayOptions())
	if err != nil || result.Outcome != audio.Empty {
		t.Fatalf("empty: %+v %v", result, err)
	}
	_, err = state.runtime.Play("unknown", audio.PlayOptions{})
	if !errors.Is(err, audio.ErrInvalidRequest) {
		t.Fatalf("mute hid unknown identity: %v", err)
	}
	state.decodeErr = nil
	if err := state.runtime.SetVolume(1); err != nil {
		t.Fatal(err)
	}
	play(t, state.runtime, audio.DefaultPlayOptions())
	if state.runtime.Snapshot()[0].Recording != "a" {
		t.Fatal("skipped request advanced selection")
	}
}

func TestOwnerSlotsAreTransactionalAndIndependent(t *testing.T) {
	state := newFixture(t, 1, 4)
	first, second := state.runtime.NewOwner(), state.runtime.NewOwner()
	options := audio.DefaultPlayOptions()
	options.Loop, options.Slot = true, "motor"
	a, err := first.Play("panel/accept", options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Play("panel/accept", options); err != nil {
		t.Fatal(err)
	}
	state.backend.failure = errors.New("device preparation failed")
	if _, err := first.Play("panel/accept", options); !errors.Is(err, state.backend.failure) {
		t.Fatal(err)
	}
	if len(state.runtime.Snapshot()) != 2 || state.sources[0].closed != 0 || state.sources[2].closed != 1 {
		t.Fatal("failed replacement changed live owners or leaked source")
	}
	state.backend.failure = nil
	if _, err := first.Play("panel/accept", options); err != nil {
		t.Fatal(err)
	}
	if len(state.runtime.Snapshot()) != 2 || state.sources[0].closed != 1 {
		t.Fatal("replacement not owner local")
	}
	if err := a.Voice.Stop(0); err != nil {
		t.Fatal(err)
	}
	if len(state.runtime.Snapshot()) != 2 {
		t.Fatal("stale handle affected replacement")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if len(state.runtime.Snapshot()) != 1 {
		t.Fatal("owner closed unrelated voices")
	}
	if _, err := first.Play("panel/accept", options); !errors.Is(err, audio.ErrOwnerClosed) {
		t.Fatal(err)
	}
	if _, err := state.runtime.Play("panel/accept", options); !errors.Is(err, audio.ErrInvalidRequest) {
		t.Fatal("independent replacement accepted")
	}
	if err := state.runtime.StopCue("panel/accept", 0); err != nil {
		t.Fatal(err)
	}
}

func TestProtectedLoopsAndFiniteAdmission(t *testing.T) {
	state := newFixture(t, 1, 1)
	options := audio.DefaultPlayOptions()
	options.Loop = true
	voice := play(t, state.runtime, options)
	result, err := state.runtime.Play("panel/accept", audio.DefaultPlayOptions())
	if err != nil || result.Outcome != audio.Dropped {
		t.Fatalf("capacity: %+v %v", result, err)
	}
	if state.sources[1].closed != 1 {
		t.Fatal("dropped decoder leaked")
	}
	if _, err := state.runtime.Play("panel/accept", options); !errors.Is(err, audio.ErrVoiceLimit) {
		t.Fatal(err)
	}
	if err := voice.Stop(0); err != nil {
		t.Fatal(err)
	}
	play(t, state.runtime, audio.DefaultPlayOptions())
	if state.runtime.Snapshot()[0].Recording != "b" {
		t.Fatal("dropped requests advanced cycle")
	}
	play(t, state.runtime, audio.DefaultPlayOptions())
	if len(state.runtime.Snapshot()) != 1 || state.sources[3].closed != 1 {
		t.Fatal("oldest finite not retired")
	}
}

func TestFadesLiveVolumeAndBackwardClock(t *testing.T) {
	state := newFixture(t, 1, 2)
	options := audio.DefaultPlayOptions()
	options.Loop, options.FadeIn = true, time.Second
	voice := play(t, state.runtime, options)
	state.time = state.time.Add(500 * time.Millisecond)
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	if gain := state.runtime.Snapshot()[0].Gain; gain != 0.25 {
		t.Fatal(gain)
	}
	if err := voice.Stop(time.Second); err != nil {
		t.Fatal(err)
	}
	state.time = state.time.Add(500 * time.Millisecond)
	if err := state.runtime.SetVolume(0.5); err != nil {
		t.Fatal(err)
	}
	if gain := state.runtime.Snapshot()[0].Gain; gain != 0.0625 {
		t.Fatal(gain)
	}
	state.time = state.time.Add(-time.Second)
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	if gain := state.runtime.Snapshot()[0].Gain; gain != 0.0625 {
		t.Fatal("backward clock changed fade", gain)
	}
	state.time = state.time.Add(2 * time.Second)
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	if len(state.runtime.Snapshot()) != 0 {
		t.Fatal("release stranded voice")
	}
}

func TestDecoderFailureSurvivesRetirementAndCloseIsTerminal(t *testing.T) {
	state := newFixture(t, 1, 2)
	play(t, state.runtime, audio.DefaultPlayOptions())
	failure := errors.New("one-time decoder fault")
	state.sources[0].failure = failure
	data, err := state.backend.players[0].read(8)
	if len(data) != 8 || !errors.Is(err, io.EOF) {
		t.Fatalf("boundary: %v %v", data, err)
	}
	if err := state.runtime.StopCue("panel/accept", 0); err != nil {
		t.Fatal(err)
	}
	if err := state.runtime.Update(); !errors.Is(err, failure) {
		t.Fatalf("lost fault: %v", err)
	}
	if err := state.runtime.Update(); err != nil {
		t.Fatal("failure repeated", err)
	}
	if state.sources[0].closed != 1 || state.backend.players[0].barriers != 1 {
		t.Fatal("retirement not exactly once")
	}
	if err := state.runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if err := state.runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := state.runtime.Play("panel/accept", audio.DefaultPlayOptions()); !errors.Is(err, audio.ErrClosed) {
		t.Fatal(err)
	}
	if err := state.runtime.SetVolume(math.NaN()); !errors.Is(err, audio.ErrClosed) {
		t.Fatal(err)
	}
}

func TestRetirementWaitsForInflightRead(t *testing.T) {
	state := newFixture(t, 1, 1)
	voice := play(t, state.runtime, audio.DefaultPlayOptions())
	source := state.sources[0]
	source.entered, source.unblock = make(chan struct{}), make(chan struct{})
	readDone, stopDone := make(chan error, 1), make(chan error, 1)
	entered := source.entered
	barrierEntered := make(chan struct{})
	state.backend.players[0].barrierEntered = barrierEntered
	go func() { _, err := state.backend.players[0].read(4); readDone <- err }()
	<-entered
	go func() { stopDone <- voice.Stop(0) }()
	<-barrierEntered
	select {
	case err := <-stopDone:
		t.Fatalf("stop returned with a decoder read in flight: %v", err)
	default:
	}
	if source.closed != 0 {
		t.Fatal("source closed during read")
	}
	close(source.unblock)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := <-stopDone; err != nil {
		t.Fatal(err)
	}
	if source.closed != 1 {
		t.Fatal("source not closed")
	}
}

func TestWindowLoopAndBoostPreservePCM(t *testing.T) {
	definition := audio.Definition{Recordings: map[audio.RecordingID]audio.Recording{"a": {File: "a.pcm", CalibrationDB: 6,
		Window: &audio.Window{Start: time.Millisecond, End: 2 * time.Millisecond}}},
		Cues: map[audio.CueID]audio.Cue{"loop": {Recordings: []audio.RecordingID{"a"}, Gain: 1}}}
	encoded := make([]byte, 12)
	for i := 0; i < len(encoded); i += 2 {
		binary.LittleEndian.PutUint16(encoded[i:], 20000)
	}
	library, err := audio.NewLibrary(definition, fstest.MapFS{"a.pcm": {Data: encoded}}, 12)
	if err != nil {
		t.Fatal(err)
	}
	host := new(backend)
	options := audio.DefaultOptions(host, func(data []byte, _ string, _ int) (audio.PCMStream, error) {
		return &pcm{Reader: bytes.NewReader(data), length: int64(len(data))}, nil
	})
	runtime, err := audio.NewRuntime(library, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	request := audio.DefaultPlayOptions()
	request.Loop = true
	if _, err := runtime.Play("loop", request); err != nil {
		t.Fatal(err)
	}
	actual, err := host.players[0].read(12)
	if err != nil || len(actual) != 12 {
		t.Fatalf("loop: %v %v", actual, err)
	}
	for i := 0; i < len(actual); i += 2 {
		if binary.LittleEndian.Uint16(actual[i:]) != 32767 {
			t.Fatal("PCM boost did not saturate")
		}
	}
}

func TestFiniteReleaseTailBeginsAtRecordingEndAndCanBeShortened(t *testing.T) {
	state := newFixture(t, 1, 2)
	options := audio.DefaultPlayOptions()
	options.ReleaseTail = time.Millisecond
	handle := play(t, state.runtime, options) // Two milliseconds of synthetic PCM.
	if handle.Status().State != audio.VoicePlaying || handle.Status().Gain != 0.5 {
		t.Fatal(handle.Status())
	}
	state.time = state.time.Add(1500 * time.Microsecond)
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	if handle.Status().State != audio.VoiceReleasing || handle.Status().Gain != 0.25 {
		t.Fatal(handle.Status())
	}
	if err := handle.Stop(100 * time.Microsecond); err != nil {
		t.Fatal(err)
	}
	state.time = state.time.Add(100 * time.Microsecond)
	if err := state.runtime.Update(); err != nil {
		t.Fatal(err)
	}
	if handle.Status().State != audio.VoiceStopped {
		t.Fatal(handle.Status())
	}
	options.Loop = true
	if _, err := state.runtime.Play("panel/accept", options); !errors.Is(err, audio.ErrInvalidRequest) {
		t.Fatal(err)
	}
}
