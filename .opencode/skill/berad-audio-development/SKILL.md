---
name: berad-audio-development
description: Use when implementing, using, reviewing, or validating Berad Audio playback, music or ambience transport, source analysis/calibration, authoring transactions, audio editor/session, catalog, project descriptors, or native probes.
---

# Berad Audio development

Load [berad-sdk-development](../berad-sdk-development/SKILL.md) for family rules.
Resolve these links from this SDK-owned skill, including in consumer sessions.

| Task | Canonical guidance |
| --- | --- |
| Construct/use playback, owners/voices, finite music, ambient reconciliation, backend | [Runtime contract](../../../modules/berad-audio/README.md) and [neutral quickstart](../../../modules/berad-audio/docs/QUICKSTART.md) |
| Schema-3 documents, source analysis, calibration, regions, history, Save/import/recovery | [Authoring contract](../../../modules/berad-audio/docs/AUTHORING.md) |
| Operate or change editor input, waveform, drafts, variants, Soundscape and safe quit | [Editor guide](../../../tools/berad-audio/docs/EDITOR.md) |
| Integrate explicit projects, session completion, standalone audition and probes | [Tools contract](../../../tools/berad-audio/README.md) |
| Generate/extend inventories, measurements and reports | [Catalog contract](../../../tools/berad-audio/docs/CATALOG.md) |

Inspect actual exported APIs and their tests before changing ownership or failure
behavior. Runtime code must not import tools/TUI. Reuse the shared PCM and
source-bound transactions; a frontend must not independently calculate the edit
it commits. Keep file publication facts even when a detached result is stale.
Native devices, terminal rendering and human listening require separate evidence.

Consumers supply media, IDs, labels, routing eligibility, calibration-family
targets, slider mapping, world discovery and score scheduling. Do not infer policy
from one game's cue prefixes or copy its integration rules into SDK. The retained
schema-3 group wire names are compatibility fields, not a new game vocabulary.

Use the SDK checker with `--module modules/berad-audio` or
`--module tools/berad-audio` for affected lanes; the checker selects downstream
candidate integration and valid reuse. Final checks follow the family validation
contract. Preserve source/fixture notices and keep neutral tests independent of
consumer checkouts, production media and private orchestration.
