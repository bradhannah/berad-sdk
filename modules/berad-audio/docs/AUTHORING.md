# Audio authoring contracts

Package `authoring` owns schema-3 documents, history, immutable source analysis and
file publication. Frontends consume these services; they do not implement another
calibration/region/save engine. See the [runtime contract](../README.md) for
playback and the [editor guide](../../../tools/berad-audio/docs/EDITOR.md) for keys.

## Document and policy boundaries

`ParseDocument` performs structural validation without opening source files or a
device. `OpenLibraryProject` adds explicit decoder, filesystem and consumer policy.
The optional validator receives detached metadata. The editor owns serialized
mutations and is not concurrency-safe; `Clone` is a detached working copy.

Schema 3 retains recordings, cues, taxonomy, provenance and routing. It rejects
duplicate JSON keys, unknown fields, malformed numbers, broken references and
unsupported schema versions. Recording IDs are lowercase snake_case; paths are
exact relative filenames, not guessed extensions. A present `file: null` is an
intentional silent slot; omitted/empty/invalid non-null files are errors. Empty
slots cannot carry an excerpt or accepted calibration. A recording can be shared
by many cues. A single assignment uses `recordingId`; an ordered cycle uses
distinct nonempty `recordingIds` plus `selection: "cycle"`, never both forms.

Taxonomy separates recording acoustic families from cue semantic profiles.
Classification is not playback scheduling or listening approval. Search terms
are trimmed, nonempty and case-insensitively unique; profile synonyms supplement
individual terms. Provenance preserves original filename, source type/URL/creator,
license, import lineage and processing independently. Unknown text is literal
`unknown`; `processing: []` means no documented steps, not proof of no processing.
Editing a license records supplied terms rather than verifying their validity.

`Routing.Groups` retains schema-3 wire keys `music`, `dungeon`, `naval`. Their labels,
target identities, categories, inheritance defaults and eligibility are consumer
descriptors. Missing assignment/inheritance, explicit null/silence and a concrete
cue are distinct. New sections require an explicit future schema revision.
Consumers also supply tile families/labels and emitter targets; SDK does not discover
a world, infer target policy from prefixes, or decide which source is active.

`ReadProjectDescriptor` reads version 1 of the project descriptor (`project.json`)
that tells an editor which library to open and with what consumer policy. It is
editor configuration, not a second manifest; playback reads only the manifest. Its
`library` resolves against the descriptor (`LibraryPath`); `assetRoot` and
`importDirectory` stay library-relative. Nested targets and policies keep the Go
field names version 1 published. It rejects other versions, duplicate keys, unknown
fields, trailing data, unknown categories and calibration targets outside
[-60,-6] dB, and opens neither the library nor a device. `CueCategory` gives a
cue's category from the consumer's `CueCategories`, else ambient for a cue with an
ambient routing profile, else SFX, so every frontend groups cues alike.
`ParseProjectDescriptor` validates bytes the caller read itself, and
`EditorOptions` turns a descriptor into `OpenLibraryProject` options, calibrating by
family as the descriptor names. `CreateProjectDescriptor` writes a descriptor that
names only its library. `SetDescriptorCueCategory` changes one cue's entry in
`targets.CueCategories`, keeping every other byte; it refuses with
`ErrProjectChangedOnDisk` unless the file still holds the bytes the caller read.

`CreateLibrary` writes an empty library and never replaces a file. `CreateCue`
refuses an existing ID, because cue IDs are references; without a named profile
it files the cue under `unclassified`, which it adds with every field `unknown`.
`DeleteCue` takes the cue's profile membership and ambient settings with it and
refuses while tiles, surfaces, emitters or groups name the cue (`RoutingUses`).
`DeleteRecording` removes only a recording no cue plays, never its file.
`SetRecordingProvenance` changes provenance without decoding the source.

Asset roots resolve against the manifest directory. Imports default to `imported/`;
the consumer may supply destinations and eligibility. These are trusted local
filesystem capabilities, not a symlink-proof sandbox. The editor permits opening
missing/invalid sources for repair, while Save validates actual assigned sources.
`RuntimeLibrary` freezes a playback projection with an explicit byte budget; it
does not reinterpret routing as scheduling policy.

## Sources and measurements

`LoadRecordingContext` freezes a regular source file of at most 128 MiB. Its path
becomes absolute at load time. It retains encoded bytes and analysis buckets,
not the entire decoded stream. Each decoder gets its own encoded copy. This input
cap is not a total heap limit. Path/duration/fingerprint are scalar observations;
`PeakBetween` reads source-time buckets without exposing the peak storage.
Returned region suggestions are detached; changing them cannot change verification,
audition bytes or later suggestions.

Analysis uses 44.1 kHz stereo signed 16-bit PCM and 10 ms peak/energy buckets.
Peaks preserve either channel's transients; region boundary energy is weighted and
boundary peaks conservative. Short-effect calibration uses the 95th percentile
of active 50 ms stereo RMS windows, excluding windows below -70 dBFS or more than
40 dB below the strongest. Sustained analysis uses K weighting, 400 ms blocks,
100 ms hops, a -70 LUFS absolute gate and -10 LU relative gate. Selections shorter
than 400 ms fall back to the short-effect estimate. These are estimates, not a
certified LUFS/true-peak meter or listening approval.

`DefaultCalibrationOptions` chooses active RMS at -20 dB. Consumers explicitly
map families to methods/targets; no game-family heuristics live in core.
Targets are finite in [-60,-6] dB, suggested gain in [-48,+12] dB with a -3 dBFS
sample-peak cap. Limits may prevent reaching a target. Per-source headroom does
not guarantee an unclipped multi-voice mix. Silence has no suggested calibration.
`CalibrationOptionsFor` exposes the effective policy/override to frontends.

An unset gain remains unreviewed unity. `calibrationMode` is `auto` or `manual`
and requires `calibrationGainDb`; numeric legacy gains without a mode are manual.
Manual gain survives trim/replacement/duplication. AUTO recomputes as part of the
source/family/region edit. Loading, seeking, availability checks and history
hydration never author calibration. Neither a saved gain nor Manual mode implies
human acceptance. Cue gain, recording calibration and monitoring preferences are
separate coefficients; calibration does not rewrite source bytes.

Silence suggestions use a -40 dB threshold with 10 ms edge padding and change only
a draft. Windows are whole source milliseconds, start inclusive/end exclusive,
bounded by decoded duration. Cuts add no implicit fade or zero-crossing repair.
Audition quiet onsets/tails, clicks and loop seams before acceptance.

Cancellation is cooperative around reads of at most 64 KiB, every 10 ms analysis
bucket and EOF verification. An in-flight filesystem/codec operation must return
first. The decoder closes on success, cancellation and failure; no detached reader
continues after return. Cancellation/deadlines are not corrupt-source diagnoses.

`CheckSourceStamp` compares identity/size/mtime cheaply. `VerifySource` hashes actual
content, rejecting replacements with preserved size/timestamps. Neither locks an
external writer out afterward. Existing streams retain frozen bytes. A source
stream owns its decoder and must close after EOF too; Read/Seek/Close are caller
serialized, with native reads stopped before close. `SetGain` may run alongside
Read but is serialized with Close; queued bytes retain already-applied gain.
Window/loop/gain mechanisms are shared with runtime playback.

## Authored transactions and history

Edits, Undo/Redo and Save are serialized. No-op edits preserve Redo. A region draft
binds recording, document revision and immutable source. Preview and Apply use the
same region/calibration transformation. Apply verifies current source content and
publishes one undoable edit; duplicate creation plus optional cue membership is
likewise atomic. Invalid/crossed bounds cannot be applied and Cancel never authors.

`BeginCalibration`/`ApplyCalibration` provide the same source/revision-bound AUTO
approval. The frontend additionally owns selection/request generations: reloading
or invalidating its selected source must reject an older approval even if no
document edit occurred. Rejecting stale approvals preserves authored state/history.
`ApplyDocument` rejects stale composite document edits. Never recompute a different
commit from the candidate the user auditioned.

## Save, import and recovery

Save preserves untouched entries byte-for-byte, including when entries are deleted
and within the taxonomy's sections; new entries start their own line, indented like
their neighbours. No-op Save preserves the entire
manifest but still detects external edits. Publication flushes/closes a same-directory
temporary file before rename. An external-change conflict requires reconciliation,
not blind overwrite, and is `ErrLibraryChangedOnDisk`; `CheckDisk` reports it
without saving. Save never implicitly commits unfinished frontend drafts.

`SaveChanged` is Save for a frontend that saves after every edit: it decodes only
recordings whose file or excerpt changed since the last save, so a missing source
elsewhere does not block unrelated edits. Save still checks every assigned source.

File facts and authored `Dirty` state are independent. `LastPublication` distinguishes
`not_published`, `unchanged`, `published`, `published_sync_pending`, with affected
paths/import assignment facts. A successful rename becomes the conflict baseline
even if later cleanup or directory sync fails. `NeedsSync` retains retryable work;
retrying unchanged Save need not rewrite the file. Later errors cannot erase it.
`MergePublication` preserves irreversible facts from stale detached results without
replacing newer edits/history. Session code must merge these even on rejected approval.

Publishers take a nonblocking cooperative lock for canonical manifest directory/name.
A second editor reports busy; a stale writer still conflicts after the first finishes.
Stable `.audio-library-*.lock` files live one directory above the manifest directory;
do not remove their inodes while editors are active. Arbitrary external writers
can still race: this is not universal filesystem compare-and-swap.

Import prevalidates metadata and AUTO gain and verifies an auditioned snapshot.
External bytes are copied to a flushed/closed temporary file and installed using a
no-overwrite link. Content-qualified destinations are reused only after full hash
comparison and flush; conflicting bytes are never overwritten. Created directory
levels and file entries are flushed separately. A post-publication failure can
leave a retained file without a successful assignment. `RetainedImports` exposes
those files and derives current references from working metadata. Undo/Quit never
delete imported files; consumers review them independently in version control.

Windows directory sync is unsupported by this implementation and disclosed in
receipts, rather than claimed equivalent durability. Reopening does not prove an
earlier failed flush reached durable media. Native filesystem acceptance remains
separate from injected publication-failure regressions.

## Maintaining this contract

Authoring regressions live beside `authoring` and use synthetic fixtures; PCM
mechanism tests live in `internal/pcm`. Consumers test their own vocabulary,
eligibility, calibration policies and round trips through the public API. Do not
keep a second authoring implementation in a consumer's test package after cutover.

More than one frontend edits these files: the terminal audio editor, kept for its
existing consumers, and berad-editor, which is taking over audio editing. Games
that ship schema-3 libraries, Ultima V Redux among them, read the same files.
Every frontend reads and writes them only through `authoring`, never with its own
parser. Schema 3 and descriptor 1 stay readable and writable as they are. A change
to either is a new version with a step-by-step upgrade, never an edit to the
current one. `authoring/testdata/format` holds one library and one descriptor using
every field; its guard test fails when an encoded field is renamed, retyped, added
or dropped. Change those files only together with a new version.
