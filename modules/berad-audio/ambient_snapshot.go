package audio

import (
	"cmp"
	"maps"
	"slices"
	"time"
)

// AmbientSourceStatus includes desired state and actual playback separately: a
// failed replacement may leave an old cue sounding. Voice.TargetGain deliberately
// excludes the envelope so consumers can implement their own music precedence.
type AmbientSourceStatus struct {
	ID         SourceID
	Desired    bool
	Cue        CueID
	Voice      VoiceStatus
	Completed  bool
	Attempts   int
	RetryAfter time.Duration
	Failure    string
}

// AmbientSnapshot is bounded by desired-source and voice limits, not an event log.
type AmbientSnapshot struct {
	Enabled bool
	Closed  bool
	Desired int
	Sources []AmbientSourceStatus
	// At is the last controller clock sample, not a fresh clock read.
	At time.Time
}

// Snapshot neither samples time nor services playback. RetryAfter is relative to
// the last control/service clock sample. Results are sorted by source identity.
func (controller *AmbientController) Snapshot() AmbientSnapshot {
	runtime := controller.runtime
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	identities := make(map[SourceID]bool, len(controller.desired)+len(controller.active))
	for sourceID := range controller.desired {
		identities[sourceID] = true
	}
	for sourceID := range controller.active {
		identities[sourceID] = true
	}
	result := AmbientSnapshot{Enabled: controller.enabled, Closed: runtime.closed,
		Desired: len(controller.desired), Sources: make([]AmbientSourceStatus, 0, len(identities)), At: runtime.latest}
	for _, sourceID := range slices.SortedFunc(maps.Keys(identities), cmp.Compare) {
		source, desired := controller.desired[sourceID]
		attempt := controller.attempts[sourceID]
		failure := ""
		if attempt.failure != nil {
			failure = attempt.failure.Error()
		}
		result.Sources = append(result.Sources, AmbientSourceStatus{ID: sourceID, Desired: desired,
			Cue: source.Cue, Voice: runtime.voiceStatus(controller.active[sourceID].handle),
			Completed: attempt.completed, Attempts: attempt.failures,
			RetryAfter: max(0, attempt.next-runtime.latest.Sub(runtime.origin)), Failure: failure})
	}

	return result
}
