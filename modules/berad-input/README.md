# Berad Input

Experimental keyboard mechanisms for a supplied key-state source. The module is
`github.com/bradhannah/berad-sdk/modules/berad-input`; its root package is `input`.
The API is experimental, with no stable release tag. Consumers must pin a
reviewed, remotely available revision before ordinary installation.

## Boundary

The module has no engine or third-party dependencies. A host can supply an
Ebitengine key as the comparable identity without making the library import
Ebitengine. There is no backend, device discovery, mouse API, command router,
text/IME service, or settings format here. A future separately scoped pointer
package/module can coexist without changing these keyboard contracts. Generic
keys serve the existing injected keyboard-source boundary; they do not promise
support for another platform or input device.

Go 1.27 is the declared minimum; development uses Go 1.27.1. Engine integration
and physical keyboard acceptance are consumer-owned. No native-platform input
acceptance is established by these ordinary algorithm tests.

## Use

```go
// The host supplies a complete finite universe, including relevant modifiers.
keys := input.NewKeyboard([]string{"down", "enter", "shift"}, hostHeld)
bindings := input.NewBindings("down", "enter")
var cadence input.Cadence

// Once at the beginning of each host update, including reader-only frames:
keys.ObserveReleases()
if key, found := bindings.FirstHeld(keys.Held, nil); found && keys.Ready(key, true) {
    if cadence.Accept(elapsedMilliseconds, 165, false) {
        keys.Consume(key) // Commit before invoking callbacks or another owner.
        handleKey(key)
    }
}
```

`FirstHeld` is observational, preserves caller priority and does not automatically
skip consumed keys. Supply an explicit filter when a different selection policy
is required. `Capture` replaces `ObserveReleases` when fresh edges are also needed;
capture once before any owner advances. It does not freeze subsequent live queries.

## API and lifecycle

| Capability | Contract |
| --- | --- |
| `Keyboard[K]` / `NewKeyboard` | Copies a finite key universe, borrows a synchronous physical-state callback. Shared by ordered host owners. |
| `Held`, `Consumed`, `Suppressed`, `Suspended`, `WaitingForRelease` | Non-consuming observations. Only `Held` calls the source; status reads do not service release. |
| `ObserveReleases`, `Ready` | Explicit servicing. `Ready(key, once)` also checks the transition barrier; it need not poll the candidate as held because selection may already have done so. |
| `Consume`, `SuppressUntilRelease` | Ordinary ownership versus stronger hold protection. Repeat admission can bypass ordinary consumption, never suppression. |
| `PrepareTransition`, `ResetTransition` | Capture all current holds, even before the first Update; reset only this barrier. Captured keys must be up together. |
| `Suspend` | Masks input; returned closure restores once, is idempotent, and supports nested lifetimes. Final restore suppresses all current holds, including new keys pressed while suspended. |
| `Sample[K]`, `Capture`, `ConsumeEdge` | Immutable held/edge snapshot. Consumption accepts only a current same-keyboard sample, once, without cadence. New captures, transitions and suspensions retire old samples. |
| `Bindings[K]` | Copied ordered key registration, `FirstHeld` with optional filter, `SelectSingle` with explicit conflict result. No action callbacks or modifier interpretation. |
| `ReleaseBarrier[K]` | Owner-local captured-key barrier for modal cover/return, independent of the shared keyboard barrier. |
| `Cadence` | Explicit millisecond throttle with **strict `elapsed > interval`** comparison. Hosts choose immediate admission; all-key deadlines still apply. |
| `Repeat[K]`, `RepeatTiming`, `Result[K]` | Owner-local focused-update countdown. Fresh taps trigger immediately; throttled and conflicting holds remain owned. Timing/tolerance are supplied, not global defaults. |

`Cadence.PostponeUntil` changes the acceptance timestamp, not the all-key deadline.
`BlockUntil` replaces an all-key deadline and allows equality. `ResetClock` clears
timestamps. `Accept`'s immediate flag bypasses only the interval. The host retains
last-key identity, different-key immediate admission, one-use force flags and any
post-accept delay policy. Signed intervals preserve
their arithmetic meaning, including zero and negative compatibility intervals.

For `Repeat`, pass a focused-update duration rather than wall-time gaps if that is
the desired policy. Changing modifier meaning does not require changing the held
physical key. Resolve the action from each triggered result. Each Poll emits at
most one action and retains residual countdown; this is **not** a no-catch-up
wall-clock scheduler. Call once per focused owner update. Reset on close/mode
change; use a release barrier/suppression when old holds must not cross owners.

### Ownership, execution, errors and limits

- All mutation, source/filter callbacks and suspension closures run synchronously
  on one host input lane. No goroutines, locks or concurrent/reentrant calls are
  supported. Sources and filters must not mutate the service they are inspecting.
- Construct `Keyboard` with `NewKeyboard`; its zero value is not initialized.
  Do not copy it. Other state types have usable zero values; do not copy live
  `Repeat`/`Cadence`/barrier state to create another input owner. Bindings and
  detached Samples/results may be copied. There is no device resource to close.
- Closing an owner means stop polling it and reset its repeat/barrier state;
  shared keyboard consumption/suppression lives until observed release. End every
  suspension using its closure. Do not dispose a host-owned input source.
- Nil source callbacks, duplicate/non-reflexive key identities, and invalid repeat
  timing panic as programming errors. Keys must have stable reflexive equality
  (no NaNs or interface values with non-comparable dynamic contents). Unknown
  keyboard keys cannot accumulate state; `Consume` ignores them and `Ready` rejects
  them. Empty bindings/universes are meaningful. Binding duplicates remain ordered
  entries and can produce a conflict in `SelectSingle`.
- Repeat requires nonnegative step/initial/tolerance, positive interval, and
  tolerance below interval. The host supplies times/durations whose arithmetic
  fits the signed Go types; do not mix clock origins or change cadence mid-hold
  without a reset. The SDK does not read wall time or silently clamp host time.
- Retained keyboard maps and capture work are bounded by the supplied universe;
  bindings/barriers by their supplied lists. Each retained snapshot owns up to
  two maps of that universe and retains its originating keyboard for identity.
  The host bounds input sizes and snapshot lifetime;
  there is no process-global cache or implied allocation-free polling guarantee.
- There is no I/O or recoverable device error channel. False admission and
  owned-but-untriggered results are ordinary outcomes, not errors.

## Extraction and validation

Extracted from Redux's supplied keyboard seam, reviewed against SDK guidance
`151d28eacdb5f8d4d824901c05c465088912fca1` and game source
`96509b38014b7b32318998647c0d68e43dbb2ed0`. The game retains its concrete facade,
ordered command routing, key priority/mapping, Caps Lock exclusion, modifier
meaning, simulation compatibility, native text and focus orchestration. SDK
mechanisms replace its consumption/suppression, release barriers, edge tracking,
shared millisecond cadence and list hold countdown; they are not duplicated.

Public external-package tests cover binding/filter ownership, strict cadence,
stale/foreign samples, preactivation barriers, suspension, repeats and release.
`testdata/consumer` is a neutral menu with arbitrary string keys and synthetic
input, testing callback reentry, focus cover/return, modifier changes and retained
throttled ownership. Its `../library` replacement is solely the checker's isolated
fixture layout, not consumer distribution metadata.

From the SDK repository use `make input-check` (or
`make quality-gate MODULE=modules/berad-input`) for
registered ordinary unit/race/consumer, closure, formatting and full-source lint
lanes; `make plan MODULE=modules/berad-input` explains selection/reuse. Direct core
tests use `GOWORK=off go test ./...` in this module. Game adapter/widget/scene tests
remain separate Ebitengine-linked evidence; SDK tests need no original game data.
Publication, a resolvable game pin, and ordinary source installation follow
independent candidate proof. Local workspaces do not establish those steps.

Project code is MIT; see [LICENSE](LICENSE). Transferred source attribution is
preserved. Tests use synthetic inputs and require no media or fixtures from a game.
