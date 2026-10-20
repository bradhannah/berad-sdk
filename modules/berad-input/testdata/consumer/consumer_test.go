package consumer_test

import (
	"testing"
	"time"

	input "github.com/bradhannah/berad-sdk/modules/berad-input"
)

// This neutral menu uses arbitrary string key identities and a fake host source.
// It imports only the public module and no game/engine/testing framework.
func TestCoveredMenuCannotReplayConfirmationOrHeldNavigation(t *testing.T) {
	held := map[string]bool{}
	keyboard := input.NewKeyboard([]string{"previous", "next", "confirm", "shift"}, func(key string) bool { return held[key] })
	bindings := input.NewBindings("previous", "next")
	var repeat input.Repeat[string]
	var focus input.ReleaseBarrier[string]
	var cadence input.Cadence
	timing := input.RepeatTiming{Initial: 300 * time.Millisecond, Interval: 120 * time.Millisecond, Tolerance: time.Microsecond}
	index, confirmations := 0, 0
	now := int64(1000)
	poll := func() {
		keyboard.ObserveReleases()
		if !focus.Observe(keyboard.Held) {
			return
		}
		selection := bindings.SelectSingle(keyboard.Held)
		if selection.Held {
			keyboard.SuppressUntilRelease("confirm")
			if !keyboard.Ready(selection.Key, false) {
				return
			}
		}
		result := repeat.Poll(selection, !keyboard.Consumed(selection.Key), time.Second/60, timing)
		if result.Triggered {
			keyboard.Consume(result.Key)
			delta := 1
			if keyboard.Held("shift") {
				delta = 5
			}
			index += delta
		}
		if result.Owned {
			return // Throttled input cannot fall into confirmation.
		}
		if keyboard.Held("confirm") && keyboard.Ready("confirm", true) && cadence.Accept(now, 0, false) {
			keyboard.Consume("confirm") // Commit before callbacks can consult the same key.
			confirmations++
			if keyboard.Ready("confirm", true) {
				t.Fatal("callback reentry accepted the same physical press twice")
			}
		}
	}
	held["next"] = true
	poll()
	held["confirm"] = true
	poll()
	if index != 1 || confirmations != 0 {
		t.Fatal("throttled navigation must own its update")
	}
	// Cover and return without polling the old owner in between.
	repeat.Reset()
	focus.Capture([]string{"previous", "next", "confirm", "shift"}, keyboard.Held)
	for range 30 {
		poll()
	}
	if index != 1 || confirmations != 0 {
		t.Fatal("covered owner's held input replayed")
	}
	clear(held)
	poll()
	held["next"], held["shift"] = true, true
	poll()
	delete(held, "shift")
	for range 18 {
		poll()
	}
	if index != 7 {
		t.Fatalf("modifier resolves per emission without cadence reset: index=%d", index)
	}
	closeSuspension := keyboard.Suspend()
	closeSuspension()
	repeat.Reset()
	for range 30 {
		poll()
	}
	if index != 7 {
		t.Fatal("focus return must suppress held navigation")
	}
	clear(held)
	poll()
	held["confirm"] = true
	poll()
	now++
	poll()
	if confirmations != 1 {
		t.Fatal("confirmation must be one-shot until real release")
	}
}
