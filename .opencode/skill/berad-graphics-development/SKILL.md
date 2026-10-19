---
name: berad-graphics-development
description: Use when implementing, using, reviewing, or validating Berad Graphics image/channel operations, procedural kernels, masks, dissolve, cycle sampling, or Ebitengine resource ownership.
---

# Berad Graphics development

Load [berad-sdk-development](../berad-sdk-development/SKILL.md) for family policy.
Read the [module guide](../../../modules/berad-graphics/README.md) and
[API contract](../../../modules/berad-graphics/docs/API.md). Root graphics mechanisms
are CPU-only; `backend/ebitengine` owns the deliberately coupled render boundary.

Hosts supply images, colours, destination selections, phase/progress and random
ordering. Keep tile IDs, palette classification, asset catalogs, camera/lighting,
input and scene/workflow policy with the consumer. No automatic file watching.

Preserve alpha conventions: ordinary opacity scales premultiplied channels;
independent alpha scaling and additive alpha pulse intentionally preserve RGB.
Changing those visual semantics requires consumer review, not just renaming.
Selections restrict destination writes, not source samples. RGB gain includes
alpha-zero emissive RGB. Hosts own frame arrays and placement; `CycleIndex` samples
explicit ticks. Use native `CopyRegion` with explicit `Insets` for owned padding;
do not infer padding or atlas geometry from familiar dimensions. See the API's
v0 removal notes before migrating older examples: HSV, Frame/Animation, CPU
tint/opacity/crop/pad and the redundant import/dissolve conveniences were retired.

Processors are host-serialized. Imported GPU images are borrowed immutable inputs;
explicit mutation requires invalidation. Cache eviction drops retained borrows,
not their GPU contents. Mutable surfaces have distinct, explicitly closed owners.
Account retained snapshots and operands as well as derived images; no total-VRAM claim.

Use `make provision MODULE=modules/berad-graphics`, `make graphics-check`,
`make graphics-demo`, and `make graphics-demo-smoke`. Start lint cleanup with a
reviewed scoped automatic-fix pass. Tests stay in `*_test.go`, use independent
pixel oracles and generated media, and separate core maths from native blend proof.
