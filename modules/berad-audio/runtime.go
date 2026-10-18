package audio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"
)

var (
	// ErrInvalidRequest identifies invalid options, identity or backend wiring.
	ErrInvalidRequest = errors.New("invalid audio request")
	// ErrClosed rejects new work after terminal runtime closure.
	ErrClosed = errors.New("audio runtime is closed")
	// ErrVoiceClosed rejects control operations on a retired or zero voice.
	ErrVoiceClosed = errors.New("audio voice is closed")
	// ErrOwnerClosed rejects playback through a released owner.
	ErrOwnerClosed = errors.New("audio owner is closed")
	// ErrVoiceLimit reports that a loop cannot displace protected loops.
	ErrVoiceLimit = errors.New("audio voice limit occupied by loops")
	// ErrDiagnosticOverflow reports bounded failures omitted before servicing.
	ErrDiagnosticOverflow = errors.New("audio diagnostic queue overflow")
)

// Options are explicit: zero Volume mutes; Now and Decode are required. Prefer
// DefaultOptions and override fields to avoid accidental zero-value preferences.
type Options struct {
	Backend    Backend
	Decode     Decoder
	Now        func() time.Time
	Volume     float64
	VoiceLimit int
}

// DefaultOptions supplies presentation time, unity volume and eight voices.
func DefaultOptions(backend Backend, decode Decoder) Options {
	const defaultVoiceLimit = 8

	return Options{Backend: backend, Decode: decode, Now: time.Now, Volume: 1, VoiceLimit: defaultVoiceLimit}
}

// PlayOptions control one instance. Slot is allowed only through Owner.Play;
// a successfully prepared instance replaces that owner's same slot, even if its
// cue changed. Empty Slot permits independent overlap. Gain zero is deliberate.
type PlayOptions struct {
	Loop   bool
	Gain   float64
	FadeIn time.Duration
	Slot   string
	// ReleaseTail fades the final portion of a finite recording using presentation
	// time. Zero leaves natural EOF unchanged; loops cannot have a release tail.
	ReleaseTail time.Duration
}

// DefaultPlayOptions selects a finite, independent, unity-gain instance.
func DefaultPlayOptions() PlayOptions {
	return PlayOptions{Loop: false, Gain: 1, FadeIn: 0, Slot: "", ReleaseTail: 0}
}

// Outcome explains successful admission or deliberate absence of a new voice.
type Outcome string

// Playback outcomes do not turn routine silence/capacity drops into errors.
const (
	Admitted  Outcome = "admitted"
	Empty     Outcome = "empty"
	Inaudible Outcome = "inaudible"
	Dropped   Outcome = "dropped"
)

// PlayResult contains a cleanup capability only when a voice was admitted.
type PlayResult struct {
	Outcome Outcome
	Voice   Voice
}

// Runtime serializes control operations. Backend reads never take this mutex.
// Decode/backend/clock calls may block and must not re-enter the runtime.
// Regular Update calls service fades and failure retirement; no goroutine ticks
// audio automatically. The host device remains borrowed throughout closure.
type Runtime struct {
	mu             sync.Mutex
	library        *Library
	options        Options
	rate           int
	latest, origin time.Time
	closed         bool
	sequence       uint64
	next           map[CueID]int
	voices         map[uint64]*voice
	errors         []error
	lostErrors     int
}

// NewRuntime validates required dependencies without opening a native device.
func NewRuntime(library *Library, options Options) (*Runtime, error) {
	if library == nil || options.Backend == nil || options.Decode == nil || options.Now == nil ||
		!unitGain(options.Volume) || options.VoiceLimit < 1 {
		return nil, fmt.Errorf("%w: library, backend, decoder, clock, volume and voice limit required", ErrInvalidRequest)
	}
	rate := options.Backend.SampleRate()
	// Bound arithmetic for source-relative Duration conversion as well as reject
	// unusable host rates. This is far above ordinary game sample rates.
	if rate <= 0 || rate > 768000 {
		return nil, fmt.Errorf("%w: unsupported sample rate %d", ErrInvalidRequest, rate)
	}
	now := options.Now()
	runtime := new(Runtime)
	runtime.library, runtime.options, runtime.rate = library, options, rate
	runtime.latest, runtime.origin = now, now
	runtime.next, runtime.voices = make(map[CueID]int), make(map[uint64]*voice)

	return runtime, nil
}

func (runtime *Runtime) now() time.Duration {
	if sample := runtime.options.Now(); sample.After(runtime.latest) {
		runtime.latest = sample
	}

	return runtime.latest.Sub(runtime.origin)
}

// Play admits an independent instance; use Owner.Play for replacement slots.
func (runtime *Runtime) Play(cue CueID, options PlayOptions) (PlayResult, error) {
	if options.Slot != "" {
		return PlayResult{}, fmt.Errorf("%w: replacement slot requires Owner.Play", ErrInvalidRequest)
	}

	return runtime.play(cue, options, nil)
}

func (runtime *Runtime) play(cueID CueID, options PlayOptions, owner *Owner) (PlayResult, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return PlayResult{}, ErrClosed
	}
	if owner != nil && owner.closed {
		return PlayResult{}, ErrOwnerClosed
	}
	if err := options.validate(); err != nil {
		return PlayResult{}, err
	}
	cue, exists := runtime.library.definition.Cues[cueID]
	if !exists {
		return PlayResult{}, fmt.Errorf("%w: unknown cue %s", ErrInvalidRequest, cueID)
	}
	index := runtime.next[cueID] % len(cue.Recordings)
	recordingID := cue.Recordings[index]
	recording := runtime.library.definition.Recordings[recordingID]
	if recording.File == "" {
		return PlayResult{Outcome: Empty, Voice: Voice{runtime: nil, id: 0, result: nil}}, nil
	}
	const decibelBase, amplitudeFactor = 10, 20
	base := options.Gain * cue.Gain * math.Pow(decibelBase, recording.CalibrationDB/amplitudeFactor)
	if !options.Loop && base*runtime.options.Volume == 0 {
		return PlayResult{Outcome: Inaudible, Voice: Voice{runtime: nil, id: 0, result: nil}}, nil
	}
	prepared, err := runtime.prepare(recording, options.Loop)
	if err != nil {
		return PlayResult{}, fmt.Errorf("prepare cue %s recording %s: %w", cueID, recordingID, err)
	}
	if !runtime.admit(prepared, owner, options.Slot) {
		if options.Loop {
			return PlayResult{}, ErrVoiceLimit
		}

		return PlayResult{Outcome: Dropped, Voice: Voice{runtime: nil, id: 0, result: nil}}, nil
	}
	handle := runtime.startPrepared(prepared, cueID, recordingID, options, owner, 1, runtime.now())
	runtime.next[cueID] = (index + 1) % len(cue.Recordings)

	return PlayResult{Outcome: Admitted, Voice: handle}, nil
}

func (options PlayOptions) validate() error {
	if !unitGain(options.Gain) || options.FadeIn < 0 || options.ReleaseTail < 0 ||
		(options.Loop && options.ReleaseTail > 0) {
		return fmt.Errorf("%w: invalid gain/fade/release tail", ErrInvalidRequest)
	}

	return nil
}

// startPrepared shares playback machinery; admission belongs to the controller.
func (runtime *Runtime) startPrepared(prepared *voice, cueID CueID, recordingID RecordingID,
	options PlayOptions, owner *Owner, sourceGain float64, now time.Duration,
) Voice {
	runtime.sequence++
	prepared.cue, prepared.recording = cueID, recordingID
	prepared.authoredGain = runtime.library.authoredGain(cueID, recordingID)
	prepared.requestGain, prepared.sourceGain = options.Gain, sourceGain
	prepared.owner, prepared.slot, prepared.loop = owner, options.Slot, options.Loop
	prepared.started, prepared.fadeIn = now, options.FadeIn
	prepared.envelope = 1
	if options.FadeIn > 0 {
		prepared.envelope = 0
	}
	if options.ReleaseTail > 0 {
		duration := voiceDuration(prepared, runtime.rate)
		prepared.releaseDuration = min(options.ReleaseTail, duration)
		prepared.releaseStart = now + duration - prepared.releaseDuration
		prepared.releaseGain = 1
		if options.FadeIn > 0 {
			prepared.releaseGain = min(1, float64(duration-prepared.releaseDuration)/float64(options.FadeIn))
		}
	}
	prepared.result = new(VoiceStatus)
	runtime.applyGain(prepared)
	runtime.voices[runtime.sequence] = prepared
	prepared.device.Play()

	return Voice{runtime: runtime, id: runtime.sequence, result: prepared.result}
}

func (library *Library) authoredGain(cue CueID, recording RecordingID) float64 {
	const decibelBase, amplitudeFactor = 10, 20

	return library.definition.Cues[cue].Gain *
		math.Pow(decibelBase, library.definition.Recordings[recording].CalibrationDB/amplitudeFactor)
}

func (runtime *Runtime) terminalReason(playing *voice) (VoiceState, error) {
	if err := playing.stream.Failure(); err != nil {
		return VoiceFailed, err
	}
	if !playing.paused && !playing.device.IsPlaying() {
		if playing.loop {
			return VoiceFailed, io.ErrUnexpectedEOF
		}

		return VoiceCompleted, nil
	}

	return "", nil
}

func (runtime *Runtime) prepare(recording Recording, loop bool) (*voice, error) {
	data := bytes.Clone(runtime.library.assets[recording.File])
	source, err := runtime.options.Decode(data, recording.File, runtime.rate)
	if err != nil {
		if source != nil {
			err = errors.Join(err, closeSource(source))
		}

		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("%w: decoder returned no source", ErrInvalidLibrary)
	}
	selected, err := selectSource(source, runtime.rate, recording.Window, loop)
	if err != nil {
		return nil, errors.Join(err, closeSource(source))
	}
	stream := new(pcmBoundary)
	stream.source = selected
	stream.gain.Store(math.Float64bits(1))
	device, err := runtime.options.Backend.NewPlayer(stream)
	if err != nil || device == nil {
		if device != nil {
			device.PauseAndStopReading()
		}
		if err == nil {
			err = fmt.Errorf("%w: backend returned no transport", ErrInvalidRequest)
		}
		failure := fmt.Errorf("create audio transport: %w", err)

		return nil, errors.Join(failure, closeSource(source))
	}
	if err := stream.Failure(); err != nil {
		device.PauseAndStopReading()

		return nil, errors.Join(err, closeSource(source))
	}
	prepared := new(voice)
	prepared.device, prepared.stream = device, stream

	return prepared, nil
}

func (runtime *Runtime) admit(prepared *voice, owner *Owner, slot string) bool {
	runtime.service(runtime.now())
	if slot != "" {
		for id, playing := range runtime.voices {
			if playing.owner == owner && playing.slot == slot {
				runtime.retire(id)
			}
		}
	}
	if len(runtime.voices) < runtime.options.VoiceLimit {
		return true
	}
	var oldest uint64
	for id, playing := range runtime.voices {
		if !playing.loop && (oldest == 0 || id < oldest) {
			oldest = id
		}
	}
	if oldest != 0 {
		runtime.finish(oldest, VoiceEvicted, nil)

		return true
	}
	prepared.device.PauseAndStopReading()
	runtime.remember(closeSource(prepared.stream.source.source))

	return false
}

// SetVolume updates current voices without restarting decoders or envelopes.
func (runtime *Runtime) SetVolume(gain float64) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	if !unitGain(gain) {
		return fmt.Errorf("%w: volume must be in [0,1]", ErrInvalidRequest)
	}
	runtime.options.Volume = gain
	runtime.service(runtime.now())

	return nil
}

// Update returns pending decoder/retirement errors once, including failures of
// voices already retired by admission/Stop. The bounded queue reports overflow.
func (runtime *Runtime) Update() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	runtime.service(runtime.now())

	return runtime.drainErrors()
}

// Close terminally retires owned voices and reports pending failures once.
func (runtime *Runtime) Close() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return nil
	}
	runtime.closed = true
	for id := range runtime.voices {
		runtime.retire(id)
	}
	runtime.library = nil
	clear(runtime.next)

	return runtime.drainErrors()
}

func (runtime *Runtime) remember(err error) {
	if err == nil {
		return
	}
	const errorLimit = 32
	if len(runtime.errors) < errorLimit {
		runtime.errors = append(runtime.errors, err)
	} else {
		runtime.lostErrors++
	}
}

func (runtime *Runtime) drainErrors() error {
	if runtime.lostErrors > 0 {
		failure := fmt.Errorf("%w: %d additional audio failures omitted", ErrDiagnosticOverflow, runtime.lostErrors)
		runtime.errors = append(runtime.errors, failure)
	}
	err := errors.Join(runtime.errors...)
	runtime.errors, runtime.lostErrors = nil, 0

	return err
}
