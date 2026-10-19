package audio

import (
	"fmt"
	"time"
)

// SourceID names a consumer-owned emitter/group, independently of its cue or
// coordinates. The controller never merges different IDs sharing a cue.
type SourceID string

// AmbientSource describes desired playback, not world discovery. Gain is a
// source/distance multiplier in [0,2]; cue gain and calibration apply separately.
// Only single-recording cues are supported. Finite sources play once per presence
// or Cue/Loop change. Gain/fade changes retain both transport and retry history.
type AmbientSource struct {
	ID      SourceID
	Cue     CueID
	Gain    float64
	Loop    bool
	FadeIn  time.Duration
	FadeOut time.Duration
}

// AmbientOptions configures an exclusive lane, sharing a Library and borrowed
// backend with other runtimes. VoiceLimit counts active and releasing voices.
type AmbientOptions struct {
	Playback     Options
	MaxSources   int
	RetryInitial time.Duration
	RetryMaximum time.Duration
}

// DefaultAmbientOptions uses sixteen voices, 4096 desired sources and retries
// after 1,2,4,8,16,30 seconds. Caller options are copied and never mutated.
func DefaultAmbientOptions(backend Backend, decode Decoder) AmbientOptions {
	const voices, sources = 16, 4096
	const retryMaximum = 30 * time.Second
	playback := DefaultOptions(backend, decode)
	playback.VoiceLimit = voices

	return AmbientOptions{Playback: playback, MaxSources: sources,
		RetryInitial: time.Second, RetryMaximum: retryMaximum}
}

// AmbientController owns a private runtime and its reconciliation state. All
// operations serialize on that runtime's mutex; no external Update or eviction
// can race its admission policy. It borrows, and never closes, the host device.
type AmbientController struct {
	runtime  *Runtime
	options  AmbientOptions
	enabled  bool
	desired  map[SourceID]AmbientSource
	active   map[SourceID]ambientVoice
	attempts map[SourceID]ambientAttempt
}

type ambientVoice struct {
	source AmbientSource
	handle Voice
}

type ambientAttempt struct {
	failures    int
	delay, next time.Duration
	failure     error
	completed   bool
}

// NewAmbientController constructs a dormant lane with no desired sources.
func NewAmbientController(library *Library, options AmbientOptions) (*AmbientController, error) {
	if options.MaxSources < 1 || options.RetryInitial <= 0 || options.RetryMaximum < options.RetryInitial {
		return nil, fmt.Errorf("%w: positive source limit and ordered retry durations required", ErrInvalidRequest)
	}
	runtime, err := NewRuntime(library, options.Playback)
	if err != nil {
		return nil, err
	}

	return &AmbientController{runtime: runtime, options: options, enabled: true,
		desired: make(map[SourceID]AmbientSource), active: make(map[SourceID]ambientVoice),
		attempts: make(map[SourceID]ambientAttempt)}, nil
}

// Reconcile atomically validates/copies a full desired snapshot, then services
// playback. Invalid snapshots leave the previous desired state intact. Playback
// errors after acceptance do not roll back the desired state; they enter backoff.
// Empty snapshots mean departure. Skipping Reconcile retains the snapshot. Disabled
// controllers validate but discard snapshots and require fresh input on enable.
func (controller *AmbientController) Reconcile(sources []AmbientSource) error {
	runtime := controller.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	desired, err := controller.validateSources(sources)
	if err != nil {
		return err
	}
	if !controller.enabled {
		return nil
	}
	for sourceID := range controller.attempts {
		source, exists := desired[sourceID]
		if !exists || !sameAmbientPlayback(source, controller.desired[sourceID]) {
			delete(controller.attempts, sourceID)
		}
	}
	controller.desired = desired
	controller.service(runtime.now())

	return runtime.drainErrors()
}

func (controller *AmbientController) validateSources(sources []AmbientSource) (map[SourceID]AmbientSource, error) {
	if len(sources) > controller.options.MaxSources {
		return nil, fmt.Errorf("%w: too many ambient sources", ErrInvalidRequest)
	}
	desired := make(map[SourceID]AmbientSource, len(sources))
	for _, source := range sources {
		if source.ID == "" || !finite(source.Gain) || source.Gain < 0 || source.Gain > 2 ||
			source.FadeIn < 0 || source.FadeOut < 0 {
			return nil, fmt.Errorf("%w: invalid ambient source %q", ErrInvalidRequest, source.ID)
		}
		if _, exists := desired[source.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate ambient source %s", ErrInvalidRequest, source.ID)
		}
		cue, exists := controller.runtime.library.definition.Cues[source.Cue]
		if !exists || len(cue.Recordings) != 1 {
			return nil, fmt.Errorf("%w: ambient cue %s requires one known recording", ErrInvalidRequest, source.Cue)
		}
		desired[source.ID] = source
	}

	return desired, nil
}

func sameAmbientPlayback(left, right AmbientSource) bool {
	return left.Cue == right.Cue && left.Loop == right.Loop
}

// Update services fades, completion and retries without changing desired sources.
func (controller *AmbientController) Update() error {
	runtime := controller.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	if controller.enabled {
		controller.service(runtime.now())
	}

	return runtime.drainErrors()
}

// SetVolume preserves source ranking/loops at zero category preference.
func (controller *AmbientController) SetVolume(volume float64) error {
	runtime := controller.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	if !unitGain(volume) {
		return fmt.Errorf("%w: volume must be in [0,1]", ErrInvalidRequest)
	}
	runtime.options.Volume = volume
	controller.service(runtime.now())

	return runtime.drainErrors()
}

// SetEnabled immediately clears voices, desired sources and retries when false.
// Enabling never revives an old snapshot. Close remains terminal.
func (controller *AmbientController) SetEnabled(enabled bool) error {
	runtime := controller.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	controller.enabled = enabled
	if !enabled {
		controller.reset()
	}

	return runtime.drainErrors()
}

// Reset immediately clears the lane, retaining its current enabled preference.
func (controller *AmbientController) Reset() error {
	runtime := controller.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return ErrClosed
	}
	controller.reset()

	return runtime.drainErrors()
}

func (controller *AmbientController) reset() {
	for _, active := range controller.active {
		controller.runtime.retire(active.handle.id)
	}
	clear(controller.active)
	clear(controller.desired)
	clear(controller.attempts)
}

// Close releases the lane without closing any other runtime or the host device.
func (controller *AmbientController) Close() error {
	runtime := controller.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return nil
	}
	controller.reset()
	controller.enabled, runtime.closed, runtime.library = false, true, nil

	return runtime.drainErrors()
}
