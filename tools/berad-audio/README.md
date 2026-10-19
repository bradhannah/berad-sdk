# Berad Audio tools

Experimental standalone tools and reusable tool services, versioned separately
from the audio runtime/authoring module. Go 1.27.0 is the declared minimum; the
selected/tested toolchain is Go 1.27.1. Runtime dependencies are pinned in `go.mod`.
Project-authored source is MIT; see [LICENSE](LICENSE). No game media is included.

## Current commands

See the [tool documentation catalog](docs/README.md), including the
[editor guide](docs/EDITOR.md), [catalog contract](docs/CATALOG.md) and
[neutral project quickstart](../../modules/berad-audio/docs/QUICKSTART.md).

`cmd/audio-probe` analyzes an explicitly selected WAV/MP3/OGG source with `-check`
or runs a bounded standalone Oto audition. `-file` is required; there is no game
checkout/default manifest lookup. `-check` opens no audio device and writes no
source data. Device probing is native acceptance only when actually run on the
named host; headless checks and compilation are separate evidence.
The standalone probe releases its interrupt context before process exit, including
failure paths; stream/native playback retirement completes within the run operation.

`cmd/audio-editor` opens `-library <manifest>` with neutral defaults or
`-project <descriptor>` with explicit targets/calibration/preview policy. `-check`
validates without opening audio/UI or writing files. There is no implicit game
manifest. The existing Bubble Tea workflows remain: recordings/moments, waveform,
crop/duplicate/calibration, routing, temporary Soundscape, Save and Undo/Redo.

`cmd/audio-catalog` takes `-library`, optional manifest-relative `-asset-root`, and
explicit `-output`/`-guide` destinations. `-measure` records sample peak and RMS,
not LUFS, true peak or listening approval. It hashes the exact supplied manifest;
game source scanning and gameplay review notes remain consumer enrichment. It
refuses output aliases of input files and unexpected input-inspection failures
before either write. Reports are derived outputs, not an atomic
multi-file transaction: a second-output I/O failure can leave the first report.

The optional `cmd/audio-runtime-probe` (`make audio-runtime-probe`) takes `-project`
and `-cue`, and owns an Ebitengine graphics loop to verify native progress/release.
`-soak` runs the existing muted one-minute lifecycle/heap check. It is deliberately
separate from the terminal editor and standalone Oto source probe. Its 512 MiB
encoded-library budget is not a total heap limit. Automated builds do not run these
native/device observations or constitute listening acceptance.

## Project integration

Applications can call `editor.Run(document, options)` (the current package name is
`editorui`; use an explicit import alias). The document comes from the SDK authoring
package. Options supply slider gain, unassigned-recording category, target eligibility
and help, ambient-creation defaults and save notices. Cue categories and group
labels/targets come from the document's detached AuthoringTargets. Prefixes and
special game target names do not select playback or picker eligibility.

`project.Open` reads a version-1 tool descriptor with a required `library` path,
resolved relative to the descriptor. `assetRoot` remains library-relative. Optional
`targets`, `calibration`, `calibrationFamilies`, `sliderGains` (11 values),
`recordingCategories`, `targetPolicies`, `ambientPreset`, `mixLabel` and `savedNotice`
provide standalone equivalents. This descriptor configures tools; it does not
replace or extend the schema-3 audio manifest. Duplicate/unknown fields and invalid
policies fail. The neutral default is linear sliders, active-RMS calibration, no
invented world targets or ambient-creation profile, and SFX for unclassified cues.

`session.Documents` owns admission and completion of detached document operations.
The frontend serializes Begin/Complete; a worker executes Job.Run exactly once.
There is one outstanding operation. Stale or approval-rejected completion merges
actual publication facts while retaining newer edits; foreign/duplicate results
cannot retire another job. Publication is allowed to finish before normal quit.
Source-load/request generations and input focus remain in the TUI. Source-bound
AUTO approval uses the shared authoring CalibrationDraft transaction.

All exported entry points are experimental. Runtime controls do not open terminal
UI; tools do not own game state, persisted player settings or production media.

`project.Config` remains public as the typed version-1 descriptor for integrations
that generate JSON project files. Its fields define the same schema consumed by
`project.Open`; retaining the type adds no second loader or configuration mechanism.
The production-surface cleanup makes the catalog's former `InspectAsset` helper
private: use `catalog.Build` for validated inventory construction. This is an
experimental Go API break, not a catalog JSON change. Adopt the accompanying
[runtime API changes](../../modules/berad-audio/README.md#experimental-api-compatibility)
with the matching runtime dependency; editor, catalog and probe workflows remain.

## Reusable audition

`audition.NewVoice` opens no device until Play. Controls are event-loop serialized;
native reading uses the shared SDK window/gain machinery. Play prepares before
replacement, Pause retains transport, and Seek/Position use source milliseconds.
Gain updates never restart or change authored metadata. Close stops native reads
before closing its stream, is idempotent, and leaves the process-owned device alive.
Failure inspection is observational. Use this in a standalone tool process, not
alongside an independently created Ebitengine/Oto context in a game process.

## Build and checks

From `tools/berad-audio`, `make audio-editor` creates `./bin/audio-editor`
(`.exe` on Windows). `make build` builds editor, catalog and source probe into
that same local `bin/` directory. Then select an explicit library or project:

```sh
make audio-editor
./bin/audio-editor -library /absolute/path/to/library.json
```

Use `-project /absolute/path/to/project.json` for project-specific routing labels,
slider mapping and other configured policies. Running with neither option reports
an argument error; this generic SDK tool does not automatically select Redux.
For consumer presets, use that application's maintained integration instructions
and launcher. A sibling game checkout is not part of the SDK tool contract.

From the SDK root, `make audio-tools` delegates to this module and uses the same
`tools/berad-audio/bin/` outputs.
`make clean` removes only known tool executables, preserving validation and authored
data. `make provision` explicitly provisions dependencies; it is network-enabled
and separate from offline checks. `make plan` and `make quality-gate` delegate to
the SDK checker with this module selected. The downloaded Go module does not need
repository Make/checker scripts to build its commands.

The checker tests published dependency pins and, separately, declared candidate
runtime integration. Both use isolated consumers and reject game dependencies.
All runtime/tool source also participates in the SDK's mandatory pinned formatting
and full-source lint gates. Provision tools with `make tools` from the SDK root;
`make lint MODULE=tools/berad-audio` selects this tool module explicitly.
See the [SDK validation contract](https://github.com/bradhannah/berad-sdk/blob/feat/berad-audio/docs/VALIDATION.md).
