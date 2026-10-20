package input_test

import (
	"testing"
	"time"

	input "github.com/bradhannah/berad-sdk/modules/berad-input"
)

func TestBindingOrderFilteringAndSharedConsumption(t *testing.T) {
	held := map[string]bool{"north": true, "confirm": true}
	keys := []string{"north", "confirm", "shift"}
	keyboard := input.NewKeyboard(keys, func(key string) bool { return held[key] })
	bindings := input.NewBindings(keys...)
	keys[0] = "mutated"
	key, found := bindings.FirstHeld(keyboard.Held, nil)
	if !found || key != "north" || !keyboard.Ready(key, true) {
		t.Fatal("registration must copy priority and key universe")
	}
	keyboard.Consume(key)
	key, _ = bindings.FirstHeld(keyboard.Held, nil)
	if key != "north" || keyboard.Ready(key, true) || !keyboard.Ready(key, false) {
		t.Fatal("first held is not first unconsumed; repeat bypasses ordinary consumption")
	}
	key, found = bindings.FirstHeld(keyboard.Held, func(key string) bool { return !keyboard.Consumed(key) })
	if !found || key != "confirm" {
		t.Fatal("explicit eligibility filter must be separate from default priority")
	}
	keyboard.SuppressUntilRelease("north")
	keyboard.ResetTransition()
	if keyboard.Ready("north", false) || !keyboard.Held("north") {
		t.Fatal("suppression survives transition reset but does not hide physical holds")
	}
	delete(held, "north")
	keyboard.ObserveReleases()
	held["north"] = true
	if !keyboard.Ready("north", true) {
		t.Fatal("physical release rearms ownership")
	}
	if keyboard.Ready("unknown", false) || keyboard.Held("unknown") {
		t.Fatal("unknown keys cannot enter the bounded key universe")
	}
}

func TestTransitionBeforeFirstCaptureAndModifierRelease(t *testing.T) {
	held := map[int]bool{1: true, 2: true}
	keyboard := input.NewKeyboard([]int{1, 2, 3}, func(key int) bool { return held[key] })
	keyboard.PrepareTransition() // Activation before any Update/capture.
	delete(held, 1)
	keyboard.ObserveReleases()
	if !keyboard.WaitingForRelease() || keyboard.Ready(3, true) {
		t.Fatal("unbound modifier still owns activation barrier")
	}
	delete(held, 2)
	if !keyboard.WaitingForRelease() {
		t.Fatal("status queries must not service release")
	}
	if !keyboard.Ready(3, true) || keyboard.WaitingForRelease() {
		t.Fatal("admission services release, even for an already selected non-held key")
	}
}

func TestSnapshotsRejectStaleForeignAndDuplicatePresses(t *testing.T) {
	held := map[int]bool{1: true}
	keyboard := input.NewKeyboard([]int{1, 2}, func(key int) bool { return held[key] })
	first := keyboard.Capture()
	if !first.Held(1) || !first.Pressed(1) || keyboard.Consumed(1) {
		t.Fatal("capture must be detached and nonconsuming")
	}
	if !keyboard.ConsumeEdge(first, 1) || keyboard.ConsumeEdge(first, 1) {
		t.Fatal("same frame edge must be committed before another handler runs")
	}
	held[2] = true
	second := keyboard.Capture()
	if second.Pressed(1) || !keyboard.ConsumeEdge(second, 2) || first.Held(2) {
		t.Fatal("held older key must not starve a replacement edge or mutate an old snapshot")
	}
	delete(held, 1)
	keyboard.ObserveReleases()
	if keyboard.ConsumeEdge(first, 1) {
		t.Fatal("servicing a release cannot revive a stale sample")
	}
	other := input.NewKeyboard([]int{1, 2}, func(int) bool { return true })
	if other.ConsumeEdge(second, 2) || keyboard.ConsumeEdge(input.Sample[int]{}, 1) {
		t.Fatal("foreign and zero samples cannot consume")
	}
	held[1] = true
	keyboard.Capture() // release was not captured, so this is not a new edge.
	clear(held)
	keyboard.Capture()
	held[1] = true
	third := keyboard.Capture()
	keyboard.PrepareTransition()
	clear(held)
	keyboard.ObserveReleases()
	if keyboard.ConsumeEdge(third, 1) {
		t.Fatal("transition retires the prior owner's sample")
	}
}

func TestFocusSuspensionDoesNotManufactureRelease(t *testing.T) {
	held := map[int]bool{1: true}
	keyboard := input.NewKeyboard([]int{1, 2}, func(key int) bool { return held[key] })
	before := keyboard.Capture()
	keyboard.Consume(1)
	outer := keyboard.Suspend()
	inner := keyboard.Suspend()
	keyboard.ObserveReleases()
	masked := keyboard.Capture()
	if keyboard.Held(1) || masked.Held(1) || !keyboard.Consumed(1) {
		t.Fatal("masking is not release")
	}
	delete(held, 1)
	held[2] = true // New hold while no input owner receives Updates.
	outer()
	outer() // Idempotent, including out-of-order nested restoration.
	if !keyboard.Suspended() {
		t.Fatal("remaining suspension owns input")
	}
	inner()
	if !keyboard.Suppressed(2) || keyboard.Ready(2, false) || keyboard.ConsumeEdge(before, 1) {
		t.Fatal("return must suppress all current holds and retire old samples")
	}
	clear(held)
	keyboard.Capture()
	held[2] = true
	if !keyboard.ConsumeEdge(keyboard.Capture(), 2) {
		t.Fatal("actual release then fresh press rearms focus input")
	}
}

func TestSampleIdentityCannotConsumeAnotherEligiblePress(t *testing.T) {
	held := map[int]bool{1: true}
	keyboard := input.NewKeyboard([]int{1}, func(key int) bool { return held[key] })
	old := keyboard.Capture()
	delete(held, 1)
	keyboard.Capture()
	held[1] = true
	current := keyboard.Capture()
	if !old.Pressed(1) || !current.Pressed(1) || keyboard.Consumed(1) || !keyboard.Held(1) {
		t.Fatal("fixture must have a current unconsumed edge and an older edge for the same held key")
	}
	if keyboard.ConsumeEdge(old, 1) || !keyboard.ConsumeEdge(current, 1) {
		t.Fatal("old sample consumed a later eligible physical press")
	}
	first := input.NewKeyboard([]int{1}, func(int) bool { return true })
	second := input.NewKeyboard([]int{1}, func(int) bool { return true })
	firstSample, secondSample := first.Capture(), second.Capture()
	if first.ConsumeEdge(secondSample, 1) || first.ConsumeEdge(input.Sample[int]{}, 1) {
		t.Fatal("foreign or fabricated sample consumed an otherwise eligible current edge")
	}
	if !first.ConsumeEdge(firstSample, 1) || !second.ConsumeEdge(secondSample, 1) ||
		first.ConsumeEdge(firstSample, 1) {
		t.Fatal("rejected samples must leave each rightful edge available exactly once")
	}
}

func TestCurrentCapturedEdgeHonorsIndependentPhysicalGates(t *testing.T) {
	t.Run("transition", func(t *testing.T) {
		keyboard := input.NewKeyboard([]int{1}, func(int) bool { return true })
		keyboard.PrepareTransition()
		sample := keyboard.Capture() // Current generation and fresh edge, after preparation.
		if !sample.Pressed(1) || keyboard.Consumed(1) || keyboard.Suppressed(1) || keyboard.Suspended() {
			t.Fatal("fixture must isolate the transition barrier")
		}
		if keyboard.ConsumeEdge(sample, 1) {
			t.Fatal("transition barrier admitted a current held edge")
		}
		keyboard.ResetTransition()
		if !keyboard.ConsumeEdge(sample, 1) {
			t.Fatal("transition rejection must not consume the edge")
		}
	})
	t.Run("suppression", func(t *testing.T) {
		keyboard := input.NewKeyboard([]int{1}, func(int) bool { return true })
		sample := keyboard.Capture()
		keyboard.SuppressUntilRelease(1)
		if !sample.Pressed(1) || keyboard.Consumed(1) || keyboard.WaitingForRelease() || keyboard.Suspended() {
			t.Fatal("fixture must isolate strong suppression")
		}
		if keyboard.ConsumeEdge(sample, 1) || keyboard.Consumed(1) {
			t.Fatal("suppression must reject the otherwise current edge without accepting it")
		}
	})
}

func TestObservedReleaseRetiresCurrentEdgeWithoutAnotherCapture(t *testing.T) {
	held := true
	keyboard := input.NewKeyboard([]int{1}, func(int) bool { return held })
	sample := keyboard.Capture()
	held = false
	keyboard.ObserveReleases()
	held = true
	if keyboard.ConsumeEdge(sample, 1) {
		t.Fatal("an old edge cannot become a fresh press after observed release")
	}
	held = false
	keyboard.Capture()
	held = true
	fresh := keyboard.Capture()
	if !keyboard.ConsumeEdge(fresh, 1) {
		t.Fatal("new physical edge must work")
	}
	held = false
	keyboard.ObserveReleases()
	held = true
	if keyboard.ConsumeEdge(fresh, 1) {
		t.Fatal("clearing ordinary consumption cannot restore a consumed edge")
	}
}

func TestCadenceStrictBoundaryAndExplicitTime(t *testing.T) {
	for _, interval := range []int64{0, 165, 250} {
		t.Run(time.Duration(interval).String(), func(t *testing.T) {
			var cadence input.Cadence
			if !cadence.Accept(1000, interval, false) || cadence.Accept(1000+interval, interval, false) {
				t.Fatal("cadence requires strictly greater elapsed time")
			}
			if !cadence.Accept(1001+interval, interval, false) || !cadence.Accept(1001+interval, interval, true) {
				t.Fatal("due and explicitly immediate admission must pass")
			}
			cadence.BlockUntil(2000)
			if cadence.Accept(1999, interval, true) || !cadence.Accept(2000, interval, true) {
				t.Fatal("force bypasses interval, not the all-key deadline")
			}
			cadence.PostponeUntil(3000)
			if cadence.Accept(2001, interval, false) || !cadence.Accept(2001, interval, true) {
				t.Fatal("postponing interval is not an all-key block")
			}
			if cadence.Elapsed(2010) != 9 || cadence.Accept(2001, interval, false) {
				t.Fatal("observing elapsed time must not alter acceptance")
			}
			cadence.ResetClock()
			if !cadence.Ready(0) || cadence.Elapsed(0) != 0 {
				t.Fatal("clock reset clears timing without inventing wall time")
			}
		})
	}
}

func TestRepeatOwnerThrottleModifierRemapAndClose(t *testing.T) {
	var repeat input.Repeat[string]
	timing := input.RepeatTiming{Initial: 300 * time.Millisecond, Interval: 120 * time.Millisecond, Tolerance: time.Microsecond}
	step := time.Second / 60
	selection := input.Selection[string]{Key: "down", Held: true}
	if result := repeat.Poll(selection, true, step, timing); !result.Owned || !result.Triggered {
		t.Fatal("fresh hold must trigger")
	}
	for range 17 {
		if result := repeat.Poll(selection, false, step, timing); !result.Owned || result.Triggered {
			t.Fatal("throttled hold still owns input")
		}
	}
	if !repeat.Poll(selection, false, step, timing).Triggered {
		t.Fatal("eighteenth tick is the initial repeat")
	}
	// Modifiers are resolved by the host after each result; physical key identity
	// stays down, so changing a modifier must not restart the physical countdown.
	for range 7 {
		if repeat.Poll(selection, false, step, timing).Triggered {
			t.Fatal("repeat cannot become immediate when a host modifier changes")
		}
	}
	if !repeat.Poll(selection, false, step, timing).Triggered {
		t.Fatal("next quantized repeat remains due")
	}
	repeat.Reset() // Owner removed without another input Update.
	if !repeat.Poll(selection, false, step, timing).Triggered {
		t.Fatal("new owner starts a new countdown; host release barrier is separate")
	}
	selection.Conflicting = true
	if result := repeat.Poll(selection, false, step, timing); !result.Owned || result.Triggered {
		t.Fatal("conflict remains owned without triggering")
	}
	selection.Conflicting = false
	if !repeat.Poll(selection, false, step, timing).Triggered {
		t.Fatal("conflict resolution restarts the list hold")
	}
	if !repeat.Poll(selection, true, step, timing).Triggered {
		t.Fatal("release observed while owner dormant must rearm")
	}
}

func TestReleaseBarrierTracksBlockedKeysOutsideBindings(t *testing.T) {
	held := map[string]bool{"left": true, "shift": true}
	pressed := func(key string) bool { return held[key] }
	var barrier input.ReleaseBarrier[string]
	barrier.Capture([]string{"left", "shift"}, pressed)
	delete(held, "left")
	if barrier.Observe(pressed) {
		t.Fatal("modifier holds barrier even if not in the next owner's bindings")
	}
	held["left"] = true
	delete(held, "shift")
	if barrier.Observe(pressed) {
		t.Fatal("barrier requires captured set to be up together")
	}
	clear(held)
	if !barrier.Observe(pressed) || barrier.Waiting() {
		t.Fatal("release rearms")
	}
}

func TestInvalidConstructionAndRepeatTiming(t *testing.T) {
	for _, operation := range []func(){
		func() { input.NewKeyboard([]int{1}, nil) },
		func() { input.NewKeyboard([]int{1, 1}, func(int) bool { return false }) },
		func() {
			var repeat input.Repeat[int]
			repeat.Poll(input.Selection[int]{}, false, 0, input.RepeatTiming{})
		},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("programming invariant must panic")
				}
			}()
			operation()
		}()
	}
}
