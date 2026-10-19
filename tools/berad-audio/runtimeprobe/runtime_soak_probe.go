package runtimeprobe

import (
	"fmt"
	"runtime"
	"time"

	berad "github.com/bradhannah/berad-sdk/modules/berad-audio"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	nativeSoakDuration      = time.Minute
	nativeSoakWarmup        = 5 * time.Second
	nativeSoakCadence       = 200 * time.Millisecond
	nativeSoakVoices        = 4
	nativeSoakHeapTolerance = 8 * 1024 * 1024
)

type runtimeSoakState struct {
	owner     *berad.Owner
	nextCycle time.Time
	cycles    int
	baseline  uint64
	warmed    bool
}

func (probe *runtimeProbeGame) updateSoak() error {
	now := time.Now()
	if probe.started.IsZero() {
		probe.started = now
	}
	if err := probe.runtime.Update(); err != nil {
		return fmt.Errorf("service native soak: %w", err)
	}
	snapshot := probe.runtime.Snapshot()
	for _, voice := range snapshot {
		probe.advanced = probe.advanced || voice.Position > 0
	}
	elapsed := now.Sub(probe.started)
	if elapsed >= nativeSoakDuration {
		return probe.finishSoak()
	}
	if !probe.soak.warmed && elapsed >= nativeSoakWarmup {
		runtime.GC()
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		probe.soak.baseline, probe.soak.warmed = memory.HeapAlloc, true
	}
	if now.Before(probe.soak.nextCycle) {
		return nil
	}
	if probe.soak.owner != nil {
		if err := probe.soak.owner.Close(); err != nil {
			return fmt.Errorf("retire native soak cycle: %w", err)
		}
	}
	probe.soak.owner = probe.runtime.NewOwner()
	for range nativeSoakVoices {
		request := berad.DefaultPlayOptions()
		request.Loop = true
		_, err := probe.soak.owner.Play(berad.CueID(probe.cue), request)
		if err != nil {
			return fmt.Errorf("start native soak cycle: %w", err)
		}
	}
	probe.soak.cycles++
	probe.soak.nextCycle = now.Add(nativeSoakCadence)

	return nil
}

func (probe *runtimeProbeGame) finishSoak() error {
	if !probe.advanced || probe.soak.owner == nil {
		return fmt.Errorf("%w: native soak device did not advance", berad.ErrInvalidRequest)
	}
	if err := probe.soak.owner.Close(); err != nil {
		return fmt.Errorf("finish native soak: %w", err)
	}
	if err := probe.runtime.Update(); err != nil {
		return fmt.Errorf("finish module soak: %w", err)
	}
	if len(probe.runtime.Snapshot()) != 0 {
		return fmt.Errorf("%w: native soak retained active voices", berad.ErrInvalidRequest)
	}
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	if memory.HeapAlloc > probe.soak.baseline+nativeSoakHeapTolerance {
		return fmt.Errorf("%w: native soak retained heap grew beyond %d bytes",
			berad.ErrInvalidRequest, nativeSoakHeapTolerance)
	}
	if _, err := fmt.Fprintf(probe.output, "Native soak: cycles=%d transports=%d baseline_heap=%d final_heap=%d active=0\n",
		probe.soak.cycles, probe.soak.cycles*nativeSoakVoices, probe.soak.baseline, memory.HeapAlloc); err != nil {
		return err
	}
	probe.complete = true

	return ebiten.Termination
}
