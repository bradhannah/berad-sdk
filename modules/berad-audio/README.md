# Berad Audio

Experimental, media-free Go audio runtime for Ebitengine games. This module has
its own dependency graph and canonical SDK import path:

```go
import audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
import "github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
```

Development source is on `feat/berad-audio`; there is no stable or tagged release.
Use a reviewed published commit at the SDK path. Redux's dependency/import cutover
follows an available SDK revision; no application-local replacement is required.
Ambient and finite music transports are available; the existing Bubble Tea editor
is not yet a module API. The API is pre-stability; applications should
pin reviewed revisions once they are available remotely.

### SDK guidance and migration identity

This conversion has been reviewed against the [Berad SDK extraction guide](https://github.com/bradhannah/berad-sdk/blob/45169251e58951593f25cd2f323ea4dea47784ff/docs/EXTRACTION_GUIDE.md)
at guidance commit `45169251e58951593f25cd2f323ea4dea47784ff`.
The source was extracted from `UltimaVRedux` commit
`12092ea54c1898bed48951504c6038280b087287`, previously incubating at
`github.com/bradhannah/UltimaVRedux/modules/berad-audio` under `SDK-MOD-001`.
This move changes import identity, not runtime behavior. Availability-before-pin
and consumer install verification remain required (`SDK-DIST-001`).

The current module declares Go **1.27.0** minimum and selects **go1.27.1**; tests
used **Go 1.27.1** and **Ebitengine 2.10.4** on macOS arm64. Older engine versions
and native Windows/Linux audio are not established by that evidence.

| Public surface | Consumer need and ownership | Compatibility boundary |
| --- | --- | --- |
| `audio` library/runtime/owners/voices | Explicit assets and reusable playback lifetimes; immutable library inputs, detached observations, owned voices | Experimental Go API; no game enums, logger, settings or TUI dependency |
| `audio.MusicPlayer`, `audio.AmbientController` | Finite transport and stable-source reconciliation; host retains score/world policy | Independent lane lifetimes; source/cue IDs have no implicit game semantics |
| `backend/ebitengine.Backend.Context` (`*audio.Context` from Ebitengine) | Borrow the host's device and sample rate; host retains device lifetime | Reviewed against Ebitengine 2.10.4; do not replace the context while a lane uses it; lane closure never closes the context |
| `backend/ebitengine.Decode` | Host-rate WAV/MP3/OGG decoding through the neutral PCM contract | Codec/version behavior is part of downstream validation; callers do not mutate retained decoder state |

These boundaries apply `SDK-API-001/002`, `SDK-OWN-001`, `SDK-CFG-001`,
`SDK-RUN-001`, `SDK-LIMIT-001` and `SDK-DOM-001`. Production media remain outside
the module under their own terms (`SDK-LIC-002`). Authoring extraction must retain
those boundaries while separating its persistence/schema compatibility from the
experimental runtime API (`SDK-COMP-001/002`).

The SDK checker registers unit/vet/build, race, and isolated consumer lanes with
separate reusable results (`SDK-TEST-001` through `SDK-TEST-003`). The transferred
consumer retains the incubation API calls and assertions; only imports changed.
No stable-version compatibility promise or new native acceptance follows from
this path migration.

## Ownership and usage

Supply a playback `Definition`, an asset `fs.FS`, and an explicit encoded-byte
budget to `NewLibrary`. Metadata/windows/slices and unique asset bytes are frozen.
`Definition()` returns a detached view. Encoded bytes are eager-resident; decoded
streams are created per voice. The budget is not a total audio heap guarantee.

```go
host := ebitengine.Backend{Context: hostAudioContext}
options := audio.DefaultOptions(host, ebitengine.Decode)
runtime, err := audio.NewRuntime(library, options)
// Handle err; defer runtime.Close at the application's presentation boundary.
```

The host owns the Ebitengine context. The runtime never closes it. Device progress
requires the running Ebitengine game loop. The core package uses only the standard
library; Ebitengine and codec dependencies enter through the backend adapter.

`DefaultOptions` provides unity volume, eight voices and `time.Now`. A fully
explicit `Options` must supply its backend, decoder, clock and positive limit;
zero volume deliberately mutes. `DefaultPlayOptions` supplies unity request gain.

```go
request := audio.DefaultPlayOptions()
request.Loop = true
owner := runtime.NewOwner()
result, err := owner.Play("hangar/motor", request)
// Handle err/result; Release fades this lifetime, Close finishes it immediately.
```

- `PlayResult` distinguishes admitted, empty, inaudible and capacity-dropped
  requests. Unknown identities and invalid options remain errors, even while muted.
- Finite muted requests do not decode or advance selection. Loops retain their
  voice while category-muted so later volume changes preserve continuity.
- Owner-local named slots replace only after preparation succeeds. Independent
  `Runtime.Play` rejects slots. Empty slots allow overlap.
- `ReleaseTail` optionally fades the final portion of a finite recording using
  presentation time, clamped to the recording duration. It does not authorize
  gameplay timing and is invalid for loops. Explicit Stop can shorten the tail.
- `Voice.Stop` affects one instance; `Owner.Release` terminally rejects new play
  and releases its voices; `Owner.Close` finishes its tails. `StopCue` is explicitly
  cue-wide across owners. Zero/stale voice handles are safe.
- `Voice.SetGain`, `Pause`, `Resume`, `Seek` and `Position` provide presentation
  control without reopening retired voices. Pause retains the decoder and freezes
  its envelope; category changes still update a paused voice's mix. Positions are
  relative to the selected window. Invalid seeks do not change it.
- `Voice.Status` distinguishes playing, paused, releasing, completed, stopped,
  evicted and failed states. Retirement stores status without retaining its decoder.
  Unexpected loop completion is a failure, unlike healthy finite completion.
  Live status includes bounded position/duration, request/source coefficients and
  envelope, so consumer diagnostics need not reconstruct factors from a muted mix.
- Admission evicts the oldest finite voice; loops are protected. A new finite
  voice is dropped when all slots are loops; a new loop returns `ErrVoiceLimit`.
  Failed/dropped requests never advance the recording cycle.
- Call `Update` regularly, including paused/modal presentation frames. It services
  fades and retirement and drains contextual failures once. Pending errors are
  bounded to 32 with an overflow count. `Close` also reports pending failures.
- Snapshots report the last serviced gain without sampling clocks or advancing
  selection. Native device errors are limited by the host backend's capabilities.

Controls are mutex-serialized; decode/device/clock calls may block and must not
re-enter the runtime. Backend reads never acquire the runtime mutex. Read and seek
must be serialized by the backend. Retirement synchronously stops in-flight reads
before closing a decoder exactly once, including windows and loops.

Presentation time retains monotonic timestamps and clamps backward samples. It is
independent of game time and device consumption. Fades are linear amplitude, not
sample-accurate scheduling. Calibration above unity saturates signed 16-bit PCM;
this is not a mixer limiter or a claim of headroom across concurrent voices.

## Playback data and authoring

`Definition` is an in-memory projection, not a new JSON format. It contains only
recordings (exact file, calibration, optional source-relative window) and cues
(ordered recording IDs and explicit linear gain). Empty file means intentional
silence; cycle members cannot be empty. The caller supplies category/policy by
choosing runtimes/controllers and gains; score-selection policy remains game-owned.

Redux's schema-3 manifest, provenance, taxonomy and routing remain authoritative
for its TUI. Do not serialize `Definition` as a replacement authoring document.
All production recordings/notices remain game-owned. Examples/tests generate
their PCM/WAV; no bundled game media or original installation is required.

## Finite music transport

`NewMusicPlayer(library, options)` owns one finite transport, separate from SFX
and ambience. It uses the same decoding/gain/retirement machinery. The caller owns
score selection, scheduling, quiet periods, interruption owners and session history.
Only single-recording cues are accepted. There are no fixed track names/prefixes.

- `Play` prepares a fresh stream before replacing the current one. Unknown cues
  and preparation failures retain the working transport; a known empty recording
  intentionally stops it.
- `SetGain` controls the application's contextual/envelope coefficient and
  `SetVolume` controls the independent category preference. Zero-gain finite music
  remains admitted: applications can start silently and fade in without losing it.
- `Pause` retains the player and returns a bounded, window-relative position.
  `Resume(cue, position)` restores that position even after a same-cue preview;
  it reuses a healthy paused player or prepares/seeks a replacement transactionally.
- `Update` retires natural completion and failed decoders, including faults while
  paused. Healthy paused transport is not mistaken for completion. Errors remain
  observable once through the service/closure boundary.
- `Snapshot` reports actual cue/status/position and both gain coefficients without
  sampling presentation time. `Stop` clears playback; `Close` is terminal and leaves
  the host device and sibling lanes alive.

## Ambient sources

`NewAmbientController(library, DefaultAmbientOptions(backend, decode))` owns an
exclusive lane using the same decoder/voice/gain/envelope machinery as ordinary
playback. Defaults are 16 active-or-releasing voices, at most 4096 desired sources,
and retry delays of 1, 2, 4, 8, 16, then 30 seconds. Options are explicit/copied.

Submit a full `[]AmbientSource` to `Reconcile`, and call `Update` on presentation
frames independently of discovery. Each source supplies a stable ID, cue, loop
mode, fade durations and source gain in `[0,2]`. Source gain contains only intensity,
distance and caller overrides; the library applies cue gain/calibration exactly
once. Only single-recording cues are currently accepted. Finite ambient sources
have immediate onset and play once per presence or Cue/Loop change.

- Skipping Reconcile retains the preceding desired snapshot; an explicitly empty
  snapshot means departure and starts releases. All input is validated before
  desired state changes; a duplicate ID, unknown/cycle cue or invalid gain/fade
  rejects the entire snapshot. Playback failures after acceptance enter backoff,
  not rollback of the desired snapshot.
- Sources with different IDs remain independent even when sharing a cue. The
  consumer owns grouping, map queries/projection and source eligibility. The
  controller has no tile, lighthouse, bard, dungeon, vessel or JSON vocabulary.
- Live gain changes keep the decoder. Re-entry during release reverses its
  envelope from its current value, without restarting. This controller operation
  does not reopen a terminal `Owner.Release` or stopped voice.
- Admission ranks pre-category/pre-envelope gain, then retained voices, then ID.
  Category mute does not change rank or restart loops. Zero-intensity sources may
  retain loops for later intensity changes. Release tails occupy capacity; this
  lane never uses the SFX oldest-finite eviction rule.
- Failed replacement preparation keeps the old cue sounding. Retry attempts are
  presence/Cue/Loop-scoped and capped; repeated snapshots and temporarily successful
  reopenings do not reset recurring-failure backoff. Long gaps allow one attempt,
  not catch-up bursts. Backoff/completed sources do not reserve empty capacity.
- Empty recording slots retire prior voices without retries. `SetEnabled(false)`
  clears desired/active/retry state immediately; re-enable requires a fresh
  snapshot. `Reset` clears the lane without changing enabled preference. `Close`
  is terminal and does not close shared devices or sibling runtimes.
- `Snapshot` separates desired cue from actual voice (important after a failed
  replacement), includes completion/retry diagnostics, and never samples time.
  `Voice.TargetGain` excludes fade gain: a game can distinguish a muted source
  from the first zero-volume attack frame when implementing local-music priority.

`QuadraticDistanceGain` is an optional unit-agnostic helper preserving Redux's
ambient curve, including its one-unit inner range and radius-zero background
convention. The caller resolves world wrapping first. Positional SFX may use a
different profile; no stereo pan or occlusion is implied.

Redux owns the JSON-to-playback bridge and its `GameAudioManager` facade. The
game-owned `testdata/berad-audio-redux` fixture independently verifies that bridge,
alongside a source planner shared with the compatibility baseline. Neither the
game facade nor its JSON/media is a dependency or part of this module.

## Validation

From the SDK checkout, use `python3 -I -B scripts/check.py plan`, then
`python3 -I -B scripts/check.py quality-gate`. Provision the Go toolchain and
dependencies before these offline checks. The checker isolates the module from
other checkouts, rejects game imports/replacements and verifies package completion
and zero skipped tests. The consumer's one local replacement exists only inside
that isolated proof. See [validation policy](../../docs/VALIDATION.md).

Game-owned bridge tests remain in Redux and are distinct from independent SDK
acceptance. Controlled transports and generated PCM/WAV tests are not native
listening proof. The module's MIT source notice is in [LICENSE](LICENSE);
production game media are neither included nor relicensed by this extraction.
