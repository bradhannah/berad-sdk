# Neutral audio quickstart

Use a reviewed available SDK revision; the API is experimental. See the
[module contract](../README.md) for ownership and the
[tools build instructions](../../../tools/berad-audio/README.md#build-and-checks).
No game checkout, production media or private orchestration is required.

## Playback in an Ebitengine host

The independently built [consumer fixture](../testdata/consumer/consumer_test.go)
demonstrates public playback construction/cleanup with a controlled backend.
For a native game, supply its single Ebitengine context through
`backend/ebitengine.Backend` and `backend/ebitengine.Decode` instead. The game loop
must run for native progress. Call `Update` on presentation frames including modal
frames, handle returned failures, then `Close` every owned runtime at host shutdown.
Closing a runtime does not close the borrowed context.

`audio.NewLibrary` accepts an in-memory `Definition`, `fs.FS`, and positive encoded
byte budget. It does not require an authoring manifest. Use arbitrary cue IDs and
explicit assets; start with `DefaultOptions` and `DefaultPlayOptions`, then set
category/request gains and loop/owner policy deliberately. Use an `Owner` for
workflow-local replacement and release; the consumer owns sequencing and duration.

## An explicit terminal project

Create `library.json` containing this complete intentionally silent schema-3 project:

```json
{
  "schemaVersion": 3,
  "recordings": {
    "panel_signal": {
      "file": null,
      "provenance": {
        "originalFilename": "unknown", "sourceType": "unknown",
        "sourceUrl": "unknown", "sourceCreator": "unknown",
        "license": "unknown", "importedFrom": "unknown", "processing": []
      }
    }
  },
  "cues": {
    "panel/accept": {
      "name": "Panel accepted", "recordingId": "panel_signal",
      "volumeModifier": 0.5, "howToHear": "Accept a panel in the host application."
    }
  },
  "taxonomy": {
    "recordingFamilies": {"signal": ["panel_signal"]},
    "cueProfiles": {"interface": ["panel/accept"]},
    "profiles": {
      "interface": {
        "targetAcousticFamily": "signal", "phase": "select", "outcome": "accepted",
        "mixRole": "interface", "emitters": ["panel"], "contexts": ["menu"]
      }
    }
  },
  "routing": {}
}
```

Beside it, create `project.json`:

```json
{"schemaVersion": 1, "library": "library.json", "mixLabel": "PROJECT MIX"}
```

The descriptor version is **1**, independently of manifest schema **3**. Its
`library` is descriptor-relative; `assetRoot` is manifest-relative. No target/world
registry is invented. Omitted tool policies use neutral linear sliders and active-RMS
calibration, not another application's preset.

From the tools module, after building:

```sh
./bin/audio-editor -project /absolute/path/to/project.json -check
./bin/audio-editor -project /absolute/path/to/project.json
```

`-check` opens no UI/device and writes nothing. In Recordings, select `panel_signal`
and use `f` to import your own generated or suitably licensed WAV/MP3/OGG. Record
truthful provenance, audition/calibrate it, then Ctrl+S. A null slot is initially
silent; replacing it is explicit. Use `?` for contextual Help and the
[editor guide](../../../tools/berad-audio/docs/EDITOR.md) for drafts/history/quit.
Undo restores assignments but leaves copied imports available for review.

The [independent authoring fixture](../testdata/consumer/authoring_test.go) uses
generated audio for a public-API edit/save/playback round trip. It is the executable
example for a host integrating authoring without terminal presentation. Native
listening and actual host event routing are additional consumer acceptance.
