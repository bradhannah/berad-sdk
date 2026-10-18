package audio_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

type ambientFixture struct {
	controller *audio.AmbientController
	library    *audio.Library
	host       *backend
	now        time.Time
	decoded    []*pcm
	broken     map[string]error
	attempts   map[string]int
}

func newAmbientFixture(t *testing.T, limit int) *ambientFixture {
	t.Helper()
	definition := audio.Definition{
		Recordings: map[audio.RecordingID]audio.Recording{
			"motor": {File: "motor.pcm", CalibrationDB: 6}, "wind": {File: "wind.pcm"}, "empty": {}},
		Cues: map[audio.CueID]audio.Cue{
			"machine": {Recordings: []audio.RecordingID{"motor"}, Gain: 0.5},
			"weather": {Recordings: []audio.RecordingID{"wind"}, Gain: 1},
			"silent":  {Recordings: []audio.RecordingID{"empty"}, Gain: 1},
			"cycle":   {Recordings: []audio.RecordingID{"motor", "wind"}, Gain: 1}},
	}
	library, err := audio.NewLibrary(definition, fstest.MapFS{
		"motor.pcm": {Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}}, "wind.pcm": {Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}}}, 16)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &ambientFixture{library: library, host: new(backend), now: time.Unix(100, 0),
		broken: make(map[string]error), attempts: make(map[string]int)}
	options := audio.DefaultAmbientOptions(fixture.host, func(data []byte, file string, _ int) (audio.PCMStream, error) {
		fixture.attempts[file]++
		if err := fixture.broken[file]; err != nil {
			return nil, err
		}
		decoded := &pcm{Reader: bytes.NewReader(data), length: int64(len(data))}
		fixture.decoded = append(fixture.decoded, decoded)
		return decoded, nil
	})
	options.Playback.Now = func() time.Time { return fixture.now }
	options.Playback.VoiceLimit = limit
	options.MaxSources = 64
	fixture.controller, err = audio.NewAmbientController(library, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.controller.Close(); err != nil {
			t.Error(err)
		}
	})
	return fixture
}

func ambientSource(id string) audio.AmbientSource {
	return audio.AmbientSource{ID: audio.SourceID(id), Cue: "weather", Gain: 1, Loop: true, FadeIn: time.Second, FadeOut: time.Second}
}

func sourceStatus(t *testing.T, controller *audio.AmbientController, id string) audio.AmbientSourceStatus {
	t.Helper()
	for _, source := range controller.Snapshot().Sources {
		if source.ID == audio.SourceID(id) {
			return source
		}
	}
	t.Fatalf("missing source %s", id)
	return audio.AmbientSourceStatus{}
}

func TestAmbientGainUpdatesAndReentryRetainDecoder(t *testing.T) {
	fixture := newAmbientFixture(t, 2)
	source := ambientSource("moving-machine")
	source.Cue = "machine"
	source.Gain = 2
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	initial := sourceStatus(t, fixture.controller, string(source.ID))
	if initial.Voice.Gain != 0 || math.Abs(initial.Voice.TargetGain-math.Pow(10, 6.0/20)) > 1e-9 {
		t.Fatal(initial)
	}
	fixture.now = fixture.now.Add(time.Second)
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.controller.Reconcile(nil); err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(500 * time.Millisecond)
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	before := sourceStatus(t, fixture.controller, string(source.ID)).Voice.Gain
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	after := sourceStatus(t, fixture.controller, string(source.ID)).Voice.Gain
	if before != after || len(fixture.host.players) != 1 {
		t.Fatalf("discontinuous reentry: %v -> %v", before, after)
	}
	source.Gain = 0.5
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	if len(fixture.host.players) != 1 || fixture.decoded[0].closed != 0 {
		t.Fatal("gain restarted decoder")
	}
	fixture.now = fixture.now.Add(2 * time.Second)
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	status := sourceStatus(t, fixture.controller, string(source.ID))
	if math.Abs(status.Voice.Gain-0.25*math.Pow(10, 6.0/20)) > 1e-9 {
		t.Fatal(status)
	}
}

func TestAmbientBudgetRetainsTailsAndIgnoresSnapshotOrder(t *testing.T) {
	fixture := newAmbientFixture(t, 2)
	sources := []audio.AmbientSource{ambientSource("b"), ambientSource("a"), ambientSource("c")}
	if err := fixture.controller.Reconcile(sources); err != nil {
		t.Fatal(err)
	}
	if sourceStatus(t, fixture.controller, "c").Voice.State != audio.VoiceStopped {
		t.Fatal("wrong deterministic selection")
	}
	fixture.now = fixture.now.Add(time.Second)
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	slices.Reverse(sources)
	if err := fixture.controller.Reconcile(sources); err != nil {
		t.Fatal(err)
	}
	if len(fixture.host.players) != 2 {
		t.Fatal("snapshot order restarted voice")
	}
	for index := range sources {
		if sources[index].ID == "c" {
			sources[index].Gain = 2
		}
	}
	if err := fixture.controller.Reconcile(sources); err != nil {
		t.Fatal(err)
	}
	if len(fixture.host.players) != 2 || sourceStatus(t, fixture.controller, "b").Voice.State != audio.VoiceReleasing {
		t.Fatal("tail bypassed budget")
	}
	fixture.now = fixture.now.Add(time.Second)
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	if len(fixture.host.players) != 3 || sourceStatus(t, fixture.controller, "c").Voice.State != audio.VoicePlaying {
		t.Fatal("waiting source not admitted")
	}
}

func TestAmbientMuteDoesNotChangeAdmissionOrPresence(t *testing.T) {
	fixture := newAmbientFixture(t, 1)
	source := ambientSource("room")
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.controller.SetVolume(0); err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(time.Second)
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	status := sourceStatus(t, fixture.controller, "room")
	if status.Voice.Gain != 0 || status.Voice.TargetGain != 0 {
		t.Fatal(status)
	}
	if err := fixture.controller.SetVolume(1); err != nil {
		t.Fatal(err)
	}
	if len(fixture.host.players) != 1 || sourceStatus(t, fixture.controller, "room").Voice.Gain != 1 {
		t.Fatal("mute lost continuity")
	}
}

func TestAmbientFailedReplacementRetainsOldVoiceAndBackoff(t *testing.T) {
	fixture := newAmbientFixture(t, 1)
	source := ambientSource("room")
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("missing decoder")
	fixture.broken["motor.pcm"] = failure
	source.Cue = "machine"
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	for range 10 {
		if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
			t.Fatal(err)
		}
	}
	status := sourceStatus(t, fixture.controller, "room")
	if fixture.attempts["motor.pcm"] != 1 || fixture.decoded[0].closed != 0 || status.Voice.Cue != "weather" || status.Cue != "machine" {
		t.Fatal(status)
	}
	fixture.now = fixture.now.Add(time.Second)
	delete(fixture.broken, "motor.pcm")
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	if fixture.decoded[0].closed != 1 || sourceStatus(t, fixture.controller, "room").Voice.Cue != "machine" {
		t.Fatal("replacement did not commit")
	}
}

func TestAmbientRetryScheduleSaturatesAndDeparturesForgetFailures(t *testing.T) {
	fixture := newAmbientFixture(t, 1)
	failure := errors.New("decoder failed")
	fixture.broken["wind.pcm"] = failure
	source := ambientSource("storm")
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	for index, seconds := range []int{1, 2, 4, 8, 16, 30, 30} {
		status := sourceStatus(t, fixture.controller, "storm")
		if status.RetryAfter != time.Duration(seconds)*time.Second || status.Attempts != min(index+1, 6) {
			t.Fatal(status)
		}
		fixture.now = fixture.now.Add(status.RetryAfter)
		if err := fixture.controller.Update(); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	before := fixture.attempts["wind.pcm"]
	fixture.now = fixture.now.Add(time.Hour)
	if err := fixture.controller.Update(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if fixture.attempts["wind.pcm"] != before+1 {
		t.Fatal("long-gap retry burst")
	}
	if err := fixture.controller.Reconcile(nil); err != nil {
		t.Fatal(err)
	}
	if len(fixture.controller.Snapshot().Sources) != 0 {
		t.Fatal("departed source retained failure")
	}
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if sourceStatus(t, fixture.controller, "storm").Attempts != 1 {
		t.Fatal("new presence retained backoff")
	}
}

func TestAmbientFiniteCompletionAndUnexpectedLoopEOF(t *testing.T) {
	fixture := newAmbientFixture(t, 1)
	source := ambientSource("event")
	source.Loop = false
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	fixture.host.players[0].playing = false
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	if !sourceStatus(t, fixture.controller, "event").Completed {
		t.Fatal("finite completion not remembered")
	}
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	if len(fixture.host.players) != 1 {
		t.Fatal("finite source restarted")
	}
	source.Loop = true
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	fixture.host.players[1].playing = false
	if err := fixture.controller.Update(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if sourceStatus(t, fixture.controller, "event").Attempts != 1 {
		t.Fatal("loop EOF was treated as completion")
	}
}

func TestAmbientDisableRequiresFreshSnapshotAndCloseIsTerminal(t *testing.T) {
	fixture := newAmbientFixture(t, 1)
	source := ambientSource("engine")
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.controller.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.controller.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	if len(fixture.host.players) != 1 || fixture.decoded[0].closed != 1 || fixture.controller.Snapshot().Desired != 0 {
		t.Fatal("enable revived old sources")
	}
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.controller.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.controller.SetEnabled(true); !errors.Is(err, audio.ErrClosed) {
		t.Fatal(err)
	}
	if err := fixture.controller.Reset(); !errors.Is(err, audio.ErrClosed) {
		t.Fatal(err)
	}
}

func TestAmbientValidationIsAtomicAndSilenceRetiresWithoutRetry(t *testing.T) {
	fixture := newAmbientFixture(t, 1)
	source := ambientSource("engine")
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	for _, cue := range []audio.CueID{"unknown", "cycle"} {
		invalid := source
		invalid.Cue = cue
		if err := fixture.controller.Reconcile([]audio.AmbientSource{invalid}); !errors.Is(err, audio.ErrInvalidRequest) {
			t.Fatal(err)
		}
	}
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source, source}); !errors.Is(err, audio.ErrInvalidRequest) {
		t.Fatal(err)
	}
	invalid := source
	invalid.Gain = math.NaN()
	if err := fixture.controller.Reconcile([]audio.AmbientSource{invalid}); !errors.Is(err, audio.ErrInvalidRequest) {
		t.Fatal(err)
	}
	if sourceStatus(t, fixture.controller, "engine").Cue != "weather" || fixture.decoded[0].closed != 0 {
		t.Fatal("invalid snapshot changed desired state")
	}
	source.Cue = "silent"
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	status := sourceStatus(t, fixture.controller, "engine")
	if fixture.decoded[0].closed != 1 || status.Attempts != 0 || status.Voice.State != audio.VoiceStopped {
		t.Fatal(status)
	}
}

func TestAmbientSourcesSharingCueRemainIndependent(t *testing.T) {
	fixture := newAmbientFixture(t, 3)
	sources := []audio.AmbientSource{ambientSource("left"), ambientSource("right"), ambientSource("background")}
	if err := fixture.controller.Reconcile(sources); err != nil {
		t.Fatal(err)
	}
	if len(fixture.host.players) != 3 {
		t.Fatal("controller merged consumer-owned identities")
	}
	fixture.now = fixture.now.Add(time.Second)
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.controller.Reconcile(sources[1:]); err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(time.Second)
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	if len(fixture.controller.Snapshot().Sources) != 2 {
		t.Fatal("departure affected other identities")
	}
}

func TestAmbientBackoffDoesNotReserveCapacity(t *testing.T) {
	fixture := newAmbientFixture(t, 1)
	broken := ambientSource("a")
	broken.Cue = "machine"
	broken.Gain = 2
	working := ambientSource("b")
	failure := errors.New("cannot decode")
	fixture.broken["motor.pcm"] = failure
	if err := fixture.controller.Reconcile([]audio.AmbientSource{broken, working}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	if sourceStatus(t, fixture.controller, "b").Voice.State != audio.VoicePlaying {
		t.Fatal("failed source reserved slot")
	}
}

func TestQuadraticDistanceGainProfile(t *testing.T) {
	for _, item := range []struct{ distance, radius, want float64 }{{0, 8, 1}, {1, 8, 1}, {4, 8, 0.75}, {8, 8, 0}, {100, 0, 1}, {-1, 8, 0}, {2, -1, 0}, {math.NaN(), 8, 0}} {
		t.Run(fmt.Sprint(item), func(t *testing.T) {
			if got := audio.QuadraticDistanceGain(item.distance, item.radius); got != item.want {
				t.Fatal(got)
			}
		})
	}
}

func TestAmbientRecurringDecoderFaultKeepsBackoffAcrossSuccessfulStarts(t *testing.T) {
	fixture := newAmbientFixture(t, 1)
	source := ambientSource("unstable")
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	for index, delay := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second} {
		failure := fmt.Errorf("decoder fault %d", index)
		fixture.decoded[index].failure = failure
		_, err := fixture.host.players[index].read(4)
		if !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if err := fixture.controller.Update(); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		status := sourceStatus(t, fixture.controller, "unstable")
		if status.RetryAfter != delay {
			t.Fatal(status)
		}
		fixture.now = fixture.now.Add(delay)
		if err := fixture.controller.Update(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAmbientSnapshotsAreDetachedAndDoNotSampleClock(t *testing.T) {
	fixture := newAmbientFixture(t, 1)
	source := ambientSource("room")
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(time.Second)
	snapshot := fixture.controller.Snapshot()
	if snapshot.Sources[0].Voice.Gain != 0 || snapshot.Sources[0].Voice.TargetGain != 1 {
		t.Fatal(snapshot)
	}
	snapshot.Sources[0].Cue = "modified"
	if fixture.controller.Snapshot().Sources[0].Cue != "weather" {
		t.Fatal("snapshot aliases controller")
	}
	if err := fixture.controller.Update(); err != nil {
		t.Fatal(err)
	}
	if fixture.controller.Snapshot().Sources[0].Voice.Gain != 1 {
		t.Fatal("servicing did not advance fade")
	}
}

func TestAmbientZeroSourceGainRetainsLiveLoopForIntensityChanges(t *testing.T) {
	fixture := newAmbientFixture(t, 1)
	source := ambientSource("portal")
	source.Gain = 0
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(time.Second)
	source.Gain = 1
	if err := fixture.controller.Reconcile([]audio.AmbientSource{source}); err != nil {
		t.Fatal(err)
	}
	if len(fixture.host.players) != 1 || sourceStatus(t, fixture.controller, "portal").Voice.Gain != 1 {
		t.Fatal("intensity restarted loop")
	}
}

func TestAmbientConcurrentReconcileVolumeSnapshotAndClose(t *testing.T) {
	fixture := newAmbientFixture(t, 2)
	source := ambientSource("shared")
	var workers sync.WaitGroup
	failures := make(chan error, 120)
	for range 4 {
		workers.Go(func() {
			for range 10 {
				failures <- fixture.controller.Reconcile([]audio.AmbientSource{source})
				failures <- fixture.controller.SetVolume(0.5)
				failures <- fixture.controller.Update()
				_ = fixture.controller.Snapshot()
			}
		})
	}
	workers.Go(func() {
		if err := fixture.controller.Close(); err != nil {
			t.Error(err)
		}
	})
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil && !errors.Is(err, audio.ErrClosed) {
			t.Fatal(err)
		}
	}
	for _, decoded := range fixture.decoded {
		if decoded.closed != 1 {
			t.Fatal("source not retired once")
		}
	}
}
