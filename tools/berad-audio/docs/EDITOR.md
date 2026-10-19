# Audio editor guide

The Bubble Tea editor operates on an explicit library or project; see the
[neutral quickstart](../../../modules/berad-audio/docs/QUICKSTART.md). A consumer
launcher may supply its own labels/targets/calibration/sliders. The SDK owns this
interaction contract; consumers document their particular world assignments.

## Workspaces and input ownership

Recordings edits shared sources, calibration, notes and non-destructive windows.
Moments edits cues, assignments and contextual gain. Tiles and Locations expose
configured routing descriptors, not world discovery. Each workspace retains its
filter and selected item. Active tab/pane has text markers and a highlighted border;
availability and selection remain distinguishable without relying on color alone.

The editor uses terminal height for its browser and waveform, Details or Soundscape
pane and persistent status/shortcut strip. `?` opens full contextual Help. Long
Details scroll; dialogs scroll to the focused control on small terminals. Below
60×24 editing shortcuts pause so invisible controls cannot mutate data; Stop,
Save and Quit retain their guards. Footer status/hints are never text input.

One typed input owner drives dispatch, foreground rendering and hints. Assignment
pickers cannot open hidden gain/crop/history editors. Returning from a nested picker
restores its parent selection/filter. Search is a highlighted draft above the list;
numeric fields use a visible pane editor and paths/long text use dialogs.

Fields edit graphemes: arrows move, Shift+arrows select, Backspace/Delete remove
a grapheme/selection, Home/End move within a line, Alt+arrows or Alt+b/f move by word,
Ctrl+w deletes the previous word, Ctrl+u clears. Notes support multiline Enter and
retained-column vertical movement; Tab reaches Apply/Cancel. Bracketed paste inserts
literal payload, never executes pasted key names. Long fields scroll their caret.
Entering a field puts its caret at the end without selecting/replacing the value.
Enter/Tab finishes the field; Escape restores its entry value, then another Escape
cancels the form. Filter Enter applies and Escape restores the prior filter.

## Finding and repairing audio

| Badge | Technical meaning |
| --- | --- |
| READY | Decoding and selected window are valid |
| NO FILE | Intentional empty recording slot |
| MISSING | Assigned file is absent |
| INVALID | Unreadable/undecodable source, directory or invalid window |
| CHECKING | Background validation pending |

These are not listening approvals. Ctrl+f cycles All/Existing/Missing/Problems.
Existing includes files still checking or invalid; Missing includes empty slots.
Search combines substrings with facets such as `category:sfx`, `family:signal`,
`phase:select`, `outcome:accepted`, `role:interface`, `emitter:panel`, `context:menu`.
Words are ANDed across names, IDs, paths, provenance, notes, classification, custom
terms, linked recordings/cues and trigger descriptions. `category:unassigned` finds
unused recordings. Family searches include a recording's own family or cue uses.

F5 rechecks changed files without changing selection. Cancelled/deadline-expired
checks are not INVALID; stale/cancelled results cannot replace newer selections.
An asterisk marks changed entries, including cues sharing an edited recording.
Recording columns show family, calibration (`A`/`M`/unset) and distinct cue-use count;
Moments show intent family and assignment/cycle. A mismatch is a review prompt,
not invalid playback. Compact layouts omit columns rather than identity/context.

Use `f` to choose/import a source or assign a recording. File picker Enter opens a
folder/accepts a file, Backspace goes up, `/` enters a path, `p` previews. Recording
assignment picker Space previews/pauses and Enter assigns. Preview alone never
authors. Failed imports restore the assigned source while retaining the error;
candidate failures do not invalidate an unrelated assigned recording. License
editing (Ctrl+L) works even with missing audio, changes only the license and is
undoable; blank is rejected, literal `unknown` is permitted.

## Principal controls

| Key | Action outside text entry |
| --- | --- |
| Tab / Shift+Tab; 1–4 | Next/previous or direct workspace |
| Up/Down; Ctrl+P/N; PgUp/PgDown or Shift+Up/Down | Browse/page focused content |
| `/`; Ctrl+f; F5 | Filter draft; availability filter; source recheck |
| w / d / m | Waveform / Details / Soundscape focus |
| Enter or p; Space; l; Shift+S | Start/restart; pause/resume; loop; full-source/region |
| +/-; g | Recording calibration or Moment cue gain; exact value |
| v | Temporary category monitoring and Reference controls |
| i/o; [/]; s/e | Mark bounds; select marker; exact start/end |
| Left/Right; Shift+Left/Right; z | Seek/nudge 10/100 ms; zoom |
| t; r | Suggest bounds; reset region (Apply remains separate) |
| n; Shift+d; Shift+v | Empty recording; duplicate clip; ordered SFX variants |
| Shift+N/K/H/C | Note / custom search terms / typical trigger / classification |
| Shift+A; Shift+T | AUTO calibration approval; AUTO/unset target override |
| u; Ctrl+R; Ctrl+S | Undo; Redo; validate/save applied document |
| x / Ctrl+x | Stop voices and pending/repeat playback; retain mix assignments |
| q / Ctrl+C | Guarded quit; during publication Ctrl+C waits for completion |

Ctrl+x also stops while typing or busy. Source/request generations bind candidate
loads and AUTO approvals; opening another owner, changing selection or Stop rejects
late playback. Availability scanning never retargets the active editor.

## Region and duplicate drafts

Mark/nudge/exact bounds/reset/suggest opens one detached region draft, leaving the
waveform visible. Tab/Up/Down selects fields and visible Apply/Cancel; Enter edits
or activates. Duplicate uses Create/Cancel and a new ID. Enter on a bound finishes
only that field. Crossed bounds stay visible, stop audition and cannot apply.
`w` targets the source cursor; `[`/`]` retarget a marker. Help shows loaded path,
fingerprint and shared uses. Zoom retains a whole-source overview.

Valid changes update the draft immediately. Cached analysis and preview rebuilds
coalesce at 100 ms presentation ticks; repeat events do not clone/revalidate/read
the whole document. `p` resolves the latest candidate immediately. AUTO gain and
Apply/Create use the same source-bound transaction. Full-source comparison bypasses
the window but retains candidate calibration. Source-time positions stay bounded;
paused playback remains paused and completed previews do not restart on adjustment.

Apply/Create verifies revision and current source content in a background operation
and creates one Undo entry. No-op Apply preserves Redo. Ctrl+S is a separate disk
save and refuses unfinished drafts. Cancel restores authored playback with current
paused/playing intent and bounded position, never resurrecting Stop or natural EOF.
Matching Soundscape layers suspend to avoid doubling; Cancel restores them and Apply
updates their region while preserving position/pause. Unrelated layers keep playing.
Duplicate preserves original layers; its retained audition labels the original cue mix.

Stamp probes invalidate changed sources and stop affected voices. F5 in a draft
reloads/rebinds while retaining bounds for review. Content verification rejects
same-size/same-timestamp replacements. It cannot lock out arbitrary external writers.
The waveform uses stereo peaks, a -72..0 dB display and fractional bars; digital zero
stays flat. This scaling changes no audio, measurements or suggestion threshold.

Duplicate IDs use the next unused numeric suffix with zero padding. The new clip
shares bytes/provenance/calibration, gets a fresh note and independent window. If
possible its initial window begins after the selected clip. Optional append to one
unambiguous SFX cue is unchecked initially and atomic with creation. Variant lists
use arrows/Enter to select/preview, `a` to append, Delete to remove, Ctrl+Up/Down to
reorder. At least one clip remains; one member becomes a single assignment.
Outside the list, `n` advances preview and Enter restarts the current member.
Only successful preview advances its cursor; runtime and editor cursors are separate.

## Calibration and monitoring

AUTO is offline calibration, not a compressor. Unset gain displays a suggestion;
Shift+A plus confirmation accepts it. Manual +/-/g locks recording gain. Manual
survives source/window/family changes; only confirmed AUTO unlocks it. AUTO source,
region and family edits publish calibration atomically. Moment +/-/g changes cue
gain, not shared recording calibration. Seeking/loading never authors gain. See
the [analysis contract](../../../modules/berad-audio/docs/AUTHORING.md) for estimator
limits; consumer targets are provisional until contextual listening accepts them.

`v` opens temporary 0–10 Music/SFX/Ambience monitoring (initially 7 each), with a
consumer-supplied slider curve and mix label. Arrows or +/- adjust; Enter/Escape/v
closes while retaining the session values. `r` toggles Reference. Cue comparison
uses calibration × cue gain × category; it does not simulate host attenuation,
fades or suppression. Bare recordings have no cue gain; shared-use categories or
the consumer fallback select their bus, with an explicit override available.
Reference uses calibrated source at unity monitoring, bypassing cue/category gain;
layer gain/mute/solo still apply. File candidates bypass the old assigned source's
calibration. Details names the actual route/gain. Monitoring never changes authored
calibration, manifest or player preferences and resets on editor restart.

## Routing workspaces

Tiles/Locations use configured family/target labels and inheritance. `f` chooses a
compatible assignment, explicit silence or inheritance when supported. `o` jumps to
the Moment and `U` navigates usages; counts describe configured types/assignments,
not placed world objects or reachable gameplay. Ctrl+f cycles assignment scopes.
Tile `F` displays family details; Insert marks families for one bulk edit. `s` selects
a surface, `D` its default, `S` an ordered footstep-list draft. An empty list is silent.
These edits do not change host passability. Family members inherit canonical edits.

`Ctrl+A` creates ambient data when the consumer supplies a creation preset. `B` in
Tiles edits per-family range/level; `b` edits shared defaults (`B` in Moments too).
Effective values explicitly identify inheritance. Relative level is 0–200%; fades
display seconds and persist milliseconds. Zero-radius means background where the
target policy permits it. Removing an assignment removes obsolete overrides.
Visible Apply/Cancel controls publish one edit; previews never mutate routing.
`r` changes the assigned sound's recording; `i` opens that shared recording's picker.
The consumer decides which world situations use these assignments.

## Temporary Soundscape

`m` opens four slots: Ambient 1, Ambient 2, Music, SFX. The status strip identifies
playing/paused/finished/stopped/muted/empty layers and any standalone preview.
Select an ambient Moment and `a` fills an empty bed; when full, choose replacement
with 1/2 or arrows then Enter, or Escape. Existing beds continue during selection.
Music plays once unless explicitly set to audition-loop. Enter fires browsed SFX
over the beds; replacement avoids accumulating voices. Adding a layer closes the
standalone preview to avoid doubling. Failed replacement retains the working slot.

In Soundscape: arrows select, Enter restarts, Space pauses/resumes, `u` mutes,
`s` solos, +/- changes layer gain, Delete removes, `l` toggles loop/repeat. SFX repeat
is bounded to 500 ms and does not accumulate voices. Mute/solo/gain survive source
replacement. Stop All cancels pending/repeat playback but retains assignments.
Browsing preserves the mix; import, variant edits and Undo/Redo clear stale layers.
Quit closes every voice. None of these assignments/levels are persisted as production
mixes or saved sessions, and native host listening remains separate acceptance.

## Save, import and quit

Ctrl+S validates applied data. Finish/cancel fields/forms/regions first. Shared
recording edits affect every usage. File operations own one detached session job;
replayed/foreign/stale results cannot replace newer authored state. Publication facts
must still merge even if approval is rejected. Ctrl+C waits for publication to settle;
failure stays visible and successful completion still checks unsaved work.

The editor displays authored dirtiness separately from DURABILITY PENDING and retained
imports. Ctrl+S can retry pending sync without rewriting unchanged data. Quit guards
unfinished drafts and pending durability even when authored state is clean. Undo and
Quit never delete imported files. Review copied assets and metadata in version control.
See the [publication contract](../../../modules/berad-audio/docs/AUTHORING.md#save-import-and-recovery)
for locking/conflicts and platform limits. A host embedding assets must rebuild/restart
after Save; editor preview alone does not update an already built application.
