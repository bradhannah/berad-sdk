package audio

import (
	"cmp"
	"fmt"
	"slices"
	"time"
)

type sourceRank struct {
	id       SourceID
	gain     float64
	retained bool
}

func (controller *AmbientController) rank(now time.Duration) []SourceID {
	ranks := make([]sourceRank, 0, len(controller.desired))
	for sourceID, source := range controller.desired {
		_, active := controller.active[sourceID]
		attempt := controller.attempts[sourceID]
		recordingID, recording := controller.recording(source)
		authored := controller.runtime.library.authoredGain(source.Cue, recordingID)
		if recording.File == "" || authored <= 0 || attempt.completed ||
			(!active && attempt.failure != nil && now < attempt.next) {
			continue
		}
		ranks = append(ranks, sourceRank{id: sourceID, gain: authored * source.Gain, retained: active})
	}
	slices.SortFunc(ranks, func(left, right sourceRank) int {
		if order := cmp.Compare(right.gain, left.gain); order != 0 {
			return order
		}
		if left.retained != right.retained {
			if left.retained {
				return -1
			}

			return 1
		}

		return cmp.Compare(left.id, right.id)
	})
	count := min(len(ranks), controller.runtime.options.VoiceLimit)
	admitted := make([]SourceID, 0, count)
	for _, rank := range ranks[:count] {
		admitted = append(admitted, rank.id)
	}

	return admitted
}

func (controller *AmbientController) recording(source AmbientSource) (RecordingID, Recording) {
	library := controller.runtime.library
	recordingID := library.definition.Cues[source.Cue].Recordings[0]

	return recordingID, library.definition.Recordings[recordingID]
}

func (controller *AmbientController) service(now time.Duration) {
	runtime := controller.runtime
	runtime.service(now)
	controller.harvest(now)
	admitted := controller.rank(now)
	controller.reconcileVoices(admitted, now)
	// Explicit silence and zero-duration releases may have retired more voices.
	controller.harvest(now)
	controller.startSources(admitted, now)
}

func (controller *AmbientController) harvest(now time.Duration) {
	for sourceID, active := range controller.active {
		if controller.runtime.voices[active.handle.id] != nil {
			continue
		}
		status := controller.runtime.voiceStatus(active.handle)
		if desired, exists := controller.desired[sourceID]; exists && sameAmbientPlayback(desired, active.source) {
			switch status.State {
			case VoiceCompleted:
				controller.attempts[sourceID] = ambientAttempt{completed: true,
					failures: 0, delay: 0, next: 0, failure: nil}
			case VoiceFailed:
				controller.fail(sourceID, status.Failure, now)
			case VoicePlaying, VoicePaused, VoiceReleasing, VoiceStopped, VoiceEvicted:
				// Explicit retirement does not count as finite completion/failure.
			}
		}
		delete(controller.active, sourceID)
	}
}

func (controller *AmbientController) reconcileVoices(admitted []SourceID, now time.Duration) {
	runtime := controller.runtime
	for sourceID, active := range controller.active {
		source, desired := controller.desired[sourceID]
		if desired {
			if _, recording := controller.recording(source); recording.File == "" {
				runtime.retire(active.handle.id)

				continue
			}
		}
		if !desired || !slices.Contains(admitted, sourceID) {
			runtime.stop(active.handle.id, active.source.FadeOut, now)

			continue
		}
		if !sameAmbientPlayback(source, active.source) {
			continue // The working voice survives failed replacement preparation.
		}
		playing := runtime.voices[active.handle.id]
		playing.sourceGain = source.Gain
		if playing.releaseDuration > 0 {
			runtime.attack(playing, now, source.FadeIn)
		}
		runtime.envelope(playing, now)
		active.source = source
		controller.active[sourceID] = active
	}
}

func (controller *AmbientController) startSources(admitted []SourceID, now time.Duration) {
	for _, sourceID := range admitted {
		source := controller.desired[sourceID]
		active, exists := controller.active[sourceID]
		if exists && sameAmbientPlayback(active.source, source) {
			continue
		}
		if !exists && len(controller.runtime.voices) >= controller.runtime.options.VoiceLimit {
			continue // Releasing voices still consume admission; never evict them.
		}
		attempt := controller.attempts[sourceID]
		if attempt.completed || now < attempt.next {
			continue
		}
		controller.start(source, active, exists, now)
	}
}

func (controller *AmbientController) start(source AmbientSource, prior ambientVoice, exists bool, now time.Duration) {
	runtime := controller.runtime
	recordingID, recording := controller.recording(source)
	prepared, err := runtime.prepare(recording, source.Loop)
	if err != nil {
		failure := fmt.Errorf("ambient source %s cue %s: %w", source.ID, source.Cue, err)
		controller.fail(source.ID, failure, now)
		runtime.remember(failure)

		return
	}
	if exists {
		runtime.retire(prior.handle.id)
	}
	options := DefaultPlayOptions()
	options.Loop = source.Loop
	if source.Loop {
		options.FadeIn = source.FadeIn // Retain Redux's immediate finite onset.
	}
	handle := runtime.startPrepared(prepared, source.Cue, recordingID, options, nil, source.Gain, now)
	controller.active[source.ID] = ambientVoice{source: source, handle: handle}
	attempt := controller.attempts[source.ID]
	attempt.failure, attempt.next = nil, 0
	controller.attempts[source.ID] = attempt // Success does not reset recurring-failure backoff.
}

func (controller *AmbientController) fail(sourceID SourceID, failure error, now time.Duration) {
	attempt := controller.attempts[sourceID]
	maximum := controller.options.RetryMaximum
	if attempt.delay == 0 {
		attempt.delay = controller.options.RetryInitial
		attempt.failures = 1
	} else if attempt.delay < maximum {
		attempt.delay += min(attempt.delay, maximum-attempt.delay)
		attempt.failures++
	}
	attempt.next = now + min(attempt.delay, time.Duration(1<<63-1)-now)
	attempt.failure = failure
	controller.attempts[sourceID] = attempt
}
