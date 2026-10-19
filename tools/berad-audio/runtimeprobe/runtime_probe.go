package runtimeprobe

import (
	"errors"
	"fmt"
	"io"
	"time"

	berad "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"

	"github.com/hajimehoshi/ebiten/v2"
	eaudio "github.com/hajimehoshi/ebiten/v2/audio"
)

type runtimeProbeGame struct {
	output    io.Writer
	soak      *runtimeSoakState
	runtime   *berad.Runtime
	cue       string
	started   time.Time
	voice     berad.Voice
	advanced  bool
	releasing bool
	complete  bool
}

const (
	runtimeProbeWidth      = 480
	runtimeProbeHeight     = 270
	runtimeProbeFadeMs     = 200
	runtimeProbeSampleRate = 44100
)

// Run borrows an immutable playback library and owns its temporary runtime/loop.
// False selects the bounded progress/release check; true selects the one-minute
// muted lifecycle/heap soak. It does not certify human listening or other devices.
func Run(library *berad.Library, cue string, soak bool, output io.Writer) error {
	if library == nil || cue == "" {
		return fmt.Errorf("%w: library and cue required", berad.ErrInvalidRequest)
	}
	if output == nil {
		output = io.Discard
	}
	context := eaudio.CurrentContext()
	if context == nil {
		context = eaudio.NewContext(runtimeProbeSampleRate)
	}
	probeGain := 0.2
	if soak {
		probeGain = 0
	}
	options := berad.DefaultOptions(ebitengine.Backend{Context: context}, ebitengine.Decode)
	options.Volume = probeGain
	engine, err := berad.NewRuntime(library, options)
	if err != nil {
		return fmt.Errorf("initialize native runtime: %w", err)
	}
	probe := &runtimeProbeGame{output: output, soak: nil, runtime: engine, cue: cue,
		started: time.Time{}, voice: berad.Voice{},
		advanced: false, releasing: false, complete: false}
	if soak {
		probe.soak = new(runtimeSoakState)
	}
	ebiten.SetWindowSize(runtimeProbeWidth, runtimeProbeHeight)
	ebiten.SetWindowTitle("Audio runtime device probe")
	runErr := ebiten.RunGame(probe)
	closeErr := engine.Close()
	if err := errors.Join(runErr, closeErr); err != nil {
		return fmt.Errorf("native audio probe: %w", err)
	}
	if !probe.complete {
		return fmt.Errorf("%w: device probe did not complete", berad.ErrInvalidRequest)
	}
	_, err = fmt.Fprintln(output, "Ebitengine probe passed: position advanced; release retired; closed")
	if err != nil {
		return fmt.Errorf("report native probe: %w", err)
	}

	return nil
}

func (probe *runtimeProbeGame) Update() error {
	if probe.soak != nil {
		return probe.updateSoak()
	}
	if probe.started.IsZero() {
		options := berad.DefaultPlayOptions()
		options.Loop, options.FadeIn = true, runtimeProbeFadeMs*time.Millisecond
		result, err := probe.runtime.Play(berad.CueID(probe.cue), options)
		if err != nil {
			return fmt.Errorf("start native probe cue: %w", err)
		}
		probe.voice, probe.started = result.Voice, time.Now()
	}
	if err := probe.runtime.Update(); err != nil {
		return fmt.Errorf("service native probe: %w", err)
	}
	elapsed := time.Since(probe.started)
	snapshot := probe.runtime.Snapshot()
	for _, voice := range snapshot {
		probe.advanced = probe.advanced || voice.Position > 0
	}
	if elapsed >= time.Second && !probe.releasing {
		if !probe.advanced {
			return fmt.Errorf("%w: device position did not advance", berad.ErrInvalidRequest)
		}
		if _, err := fmt.Fprintf(probe.output, "Native module snapshot: %+v\n", snapshot); err != nil {
			return fmt.Errorf("report native snapshot: %w", err)
		}
		if err := probe.voice.Stop(runtimeProbeFadeMs * time.Millisecond); err != nil {
			return fmt.Errorf("release native probe: %w", err)
		}
		probe.releasing = true
	}
	const completionDelay = 1500 * time.Millisecond
	if elapsed >= completionDelay {
		if len(snapshot) != 0 {
			return fmt.Errorf("%w: owned release did not retire", berad.ErrInvalidRequest)
		}
		probe.complete = true

		return ebiten.Termination
	}

	return nil
}

func (*runtimeProbeGame) Draw(_ *ebiten.Image)       {}
func (*runtimeProbeGame) Layout(_, _ int) (int, int) { return runtimeProbeWidth, runtimeProbeHeight }
