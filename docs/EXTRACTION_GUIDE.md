# Converting a subsystem to a Berad SDK

This is the entry point for an extraction agent, including conversions incubating
in another repository. Read [AGENTS.md](../AGENTS.md),
[conventions](SDK_CONVENTIONS.md), [API design](API_DESIGN.md),
[compatibility](COMPATIBILITY.md), [validation](VALIDATION.md), and
[licensing](LICENSING.md). The public `berad-sdk-development` skill routes here.

## 1. Establish scope and a standards checkpoint

Use the [extraction plan template](templates/extraction-plan.md) in the approved
issue/PR. Record source revision, reviewed SDK-guidance commit, concrete consumers,
approved requirements, and active parallel work. Inventory implementation, callers,
tests, assets, tools, persisted formats, maintained docs, skills, contributor
instructions, Help and command guides before choosing cuts.
Repository placement and the issue's identity are separate decisions.

Preserve existing approved decisions. Identify differences against these rules and
obtain focused decisions; do not silently redesign another ongoing conversion.
Review graphics/audio/text responsibilities separately. Shared conventions do not
imply identical APIs or a universal manager.

## 2. Review the useful public boundary before broad moves

Create a small export/exposure map with concrete consumer needs and the
[API contract template](templates/api-contract.md). Resolve:

- Which mechanisms move and which game semantics remain in the adapter.
- Actual module/import identities during incubation and after migration.
- Supplied assets/configuration, third-party types, and engine execution boundaries.
- Ownership, immutability, lifetime, concurrency, defaults, units, and errors.
- Resource/work limits and required diagnostics.
- Stable, experimental, unsupported, and explicitly deferred behavior.

Exit: another developer can explain how a neutral consumer constructs, uses,
inspects, and closes the service using only its intended public API.

## 3. Establish meaningful proof and extract incrementally

Retain relevant existing regression knowledge. Reproduce suspected defects before
claiming fixes. Create a small unrelated consumer with arbitrary identities and
synthetic/licensed inputs. Use external-package tests for actual public contracts.
Keep game-owned bridge tests distinct from independent SDK tests.

Move proven mechanisms behind the reviewed public boundary; preserve game-owned
facades where they make adaptation small. Document behavioral changes explicitly.
Avoid two independently evolving implementations after cutover.

For every increment, select affected checks and reuse valid evidence. A new method
does not automatically need a separate test if meaningful existing coverage proves
the contract. A new ownership/failure boundary does need appropriate evidence.

## 4. Register validation before onboarding code here

This repository registers module lanes explicitly. Adding a `go.mod` or Go
implementation requires extending `sdk.json` and its validator together with an
explicit check graph, dependencies, tools, and consumer fixtures. The initial gate
deliberately rejects unsupported code rather than reporting a false full pass.

Required module proof includes import/asset closure, independent build/consumer,
relevant unit/integration/race/native lanes, and declared support coverage. Verify
that unrelated module changes reuse earlier results and that material dependency
changes select downstream checks. See `SDK-TEST-001` through `SDK-TEST-003` in
[validation](VALIDATION.md).

## 5. Publish and integrate through a real dependency

Use the [consumer integration template](templates/consumer-integration.md).
Follow the availability-before-consumer-pin order in `SDK-DIST-001`. A workspace
or test-only replacement is development evidence, not publication/install proof.
An unchanged old-consumer fixture should still pass when compatibility is promised.

Verify game-visible parity or approved differences, shutdown/scene ownership,
configuration/persistence behavior, and actual source installation as applicable.
Record native observations separately from mocked, headless, or cross-build proof.

## 6. Reconcile completion

Complete the [module README](templates/module-readme.md), API contracts, examples,
applicable consumer docs, license notices, and catalogs. Explain selected/reused
validation and any unsupported acceptance. Review the final full diff against the
checkpoint, including later fixes and companion repositories.

Transfer reusable system knowledge with its implementation. Split mixed documents
and skills: SDK owns mechanism/API/tool guidance; consumers retain integration,
policy, media and adoption rationale; private orchestration and evidence remain
with their owners. Preserve unique failure/lifecycle/compatibility rules before
removing source-side duplicates. Update inbound links, catalogs, skill triggers
and current source maps. Historical ADR context is not a competing live manual.

Map old tests to SDK mechanism coverage, consumer adapter/policy coverage, or a
specifically justified temporary oracle. Move/port missing assertions, then retire
the old implementation and its last tests together. Test-only copies are not an
indefinite second implementation. Do not delete assets/notices as code cleanup.

Verify representative tasks from skill to contract to public API in an SDK-only
session and an independent consumer. Coordinated launchers load SDK skills from
this checkout rather than copying them. Restart loaded sessions after skill/config
changes and verify actual discovery. Link checks do not prove semantic coverage.

Completion means an independently usable module and the agreed consumer cutover,
not merely a new directory. A local checkpoint is not a published revision or a
submitted PR. Humans review/merge; record each repository's actual delivery.
