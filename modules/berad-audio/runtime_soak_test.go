package audio_test

import (
	"bytes"
	"io"
	"runtime"
	"testing"
	"testing/fstest"
	"time"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

type soakBackend struct{ active, peak, created, retired int }

func (*soakBackend) SampleRate() int { return 1000 }
func (host *soakBackend) NewPlayer(source io.ReadSeeker) (audio.Transport, error) {
	host.active++
	host.peak = max(host.peak, host.active)
	host.created++
	return &soakTransport{transport: transport{source: source}, host: host}, nil
}

type soakTransport struct {
	transport
	host    *soakBackend
	retired bool
}

func (player *soakTransport) PauseAndStopReading() {
	player.transport.PauseAndStopReading()
	if !player.retired {
		player.retired = true
		player.host.active--
		player.host.retired++
	}
}

// Transferred from the consumer baseline; exercise the actual SDK public API,
// without retaining transports in the fake backend or depending on native audio.
func TestRuntimeLongSessionLifecycleAndRetainedHeapPlateau(t *testing.T) {
	definition := audio.Definition{Recordings: map[audio.RecordingID]audio.Recording{"signal": {File: "signal.pcm"}},
		Cues: map[audio.CueID]audio.Cue{"panel/accept": {Recordings: []audio.RecordingID{"signal"}, Gain: 1}}}
	library, err := audio.NewLibrary(definition, fstest.MapFS{"signal.pcm": {Data: make([]byte, 400)}}, 400)
	if err != nil {
		t.Fatal(err)
	}
	host := new(soakBackend)
	options := audio.DefaultOptions(host, func(data []byte, _ string, _ int) (audio.PCMStream, error) {
		return &pcm{Reader: bytes.NewReader(data), length: int64(len(data))}, nil
	})
	options.Now = func() time.Time { return time.Unix(0, 0) }
	engine, err := audio.NewRuntime(library, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Error(err)
		}
	})
	cycle := func(count int) {
		for range count {
			owner := engine.NewOwner()
			request := audio.DefaultPlayOptions()
			request.Loop = true
			for range 4 {
				if _, err := owner.Play("panel/accept", request); err != nil {
					t.Fatal(err)
				}
			}
			request.Loop = false
			for range 12 {
				if _, err := engine.Play("panel/accept", request); err != nil {
					t.Fatal(err)
				}
			}
			if err := owner.Close(); err != nil {
				t.Fatal(err)
			}
			if err := engine.StopCue("panel/accept", 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	cycle(50)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	cycle(1000)
	runtime.GC()
	runtime.ReadMemStats(&after)
	if host.created != host.retired || host.active != 0 || host.peak > 9 || len(engine.Snapshot()) != 0 {
		t.Fatalf("lifecycle imbalance: %+v", host)
	}
	delta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if delta > 2*1024*1024 {
		t.Fatalf("retained heap grew %d bytes", delta)
	}
	t.Logf("synthetic lifecycle: created=%d retired=%d peak=%d heap delta=%d", host.created, host.retired, host.peak, delta)
}
