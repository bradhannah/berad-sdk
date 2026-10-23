---
name: berad-sdk-development
description: Use when designing, extracting, implementing, using, reviewing, or validating a Berad SDK runtime, authoring service, developer tool, or consumer adapter.
---

# Berad SDK development

Read the repository's [AGENTS.md](../../../AGENTS.md) and
[extraction guide](../../../docs/EXTRACTION_GUIDE.md). These public contracts apply
to conversions incubating elsewhere as well as modules already hosted here.
Resolve links from this skill directory, not the current consumer working directory.

## Workflow

1. Establish the approved scope, source baseline, concrete consumers, current
   module path, concurrent work, and reviewed guidance commit. Fill in the
   [extraction checkpoint](../../../docs/templates/extraction-plan.md) in the
   issue/PR before broad movement.
2. Follow [module conventions](../../../docs/SDK_CONVENTIONS.md). Expose only
   required capabilities; retain game policy and assets in consumer adapters.
   Document exceptions by stable SDK rule ID and review guidance changes against
   already-active conversions rather than silently expanding their scope.
3. Review [API and ownership](../../../docs/API_DESIGN.md): concrete public
   services, small injected interfaces, deliberate Ebitengine types, defaults,
   units, mutation, closure, concurrency, resource limits, and observable failures.
   Use the [API template](../../../docs/templates/api-contract.md).
4. Follow [licensing](../../../docs/LICENSING.md): preserve source notices and
   establish commercial redistribution suitability of fixtures/dependencies.
   Use generated or suitably licensed assets for independent proof.
5. Select checks using [validation](../../../docs/VALIDATION.md). Start with the
   lowest meaningful regression, add necessary public/consumer boundary proof,
   and reuse matching results. Explain invalidation and avoid repeated all-module
   suites. Register new modules/check graphs before claiming full gate coverage.
   Enforce [strict linting](../../../docs/LINTING.md) for every runtime and tool:
   pinned tools, full-source zero findings, reviewed automatic fixes first, then
   complete manual cleanup. Prefer the documented Make targets. No debt cutoff or
   broad exemption can substitute for compliance.
6. Follow [compatibility](../../../docs/COMPATIBILITY.md) and the
   [consumer template](../../../docs/templates/consumer-integration.md). Keep an
   unchanged supported-consumer fixture; publish an available dependency before
   pinning the game. Treat workspace convenience and actual installation separately.
7. Complete the [module README](../../../docs/templates/module-readme.md), public
   examples, relevant consumer docs, catalogs, and notices. Account for skills,
   contributor instructions, Help, and source-side duplicate retirement in the
   extraction inventory. Reconcile the final diff and every required delivery.
   Report native observations and limitations honestly. Humans merge.

## Implemented system guidance

Load [berad-audio-development](../berad-audio-development/SKILL.md) for audio
runtime/backend, analysis/calibration, document/publication, terminal editor,
catalog, project and probe work. It routes to the audio-owned contracts and
neutral quickstart. Consumer policy stays in that consumer's integration guide.

For Berad Text, read the [module guide](../../../modules/berad-text/README.md) and
[API contracts](../../../modules/berad-text/docs/API.md) for supplied fonts,
styled layout, local geometry, renderer ownership and native-render checks.
Input, editing, focus, stacking and application text policy remain host-owned.
Use the module guide's Make targets for repeatable checks and demonstrations.
Load [berad-graphics-development](../berad-graphics-development/SKILL.md) for
supplied pixels, procedural kernels, mask operations and Ebitengine resources.

For Berad RNG, read the [module guide](../../../modules/berad-rng/README.md) and
[stream contract](../../../modules/berad-rng/docs/API.md). Preserve exact draw
counts, sequence and checkpoint compatibility; hosts own seed/stream policy.
For Berad Input, read the [keyboard guide](../../../modules/berad-input/README.md).
Hosts supply key observations, clock values and binding priorities. Consumption,
suppression, transition barriers and repeat owners are distinct; do not normalize
consumer timing or move command, native-text, widget or pointer routing into it.
For Berad Path, read the [module guide](../../../modules/berad-path/README.md) and
[search contract](../../../modules/berad-path/docs/API.md). Hosts supply bounds,
steps and step costs, including diagonal and occupancy rules; keep maps, units,
wrapping and no-route behaviour in the consumer.

## Registered validation commands

From the SDK checkout, use `make plan`, then `make validate` or `make quality-gate`.
Read `sdk.json` and the validation contract for actual registered
coverage and reuse. Add required module checks before claiming onboarding acceptance.
The interpreter must be Python 3.12+. Use `make plan` after explicit
`make python-tools PYTHON_BOOTSTRAP=python3.12` provisioning: Make selects the local tool
environment. Follow the [Python policy](../../../docs/LINTING.md#sdk-python-001--repository-wide-python-policy)
for repository-wide Black/Pylint, meaningful annotations including tests, offline
Python-only checks and opt-in hook composition. Both final gates include Python
once even with a module selector.

Consumer-specific workflows may add their own repository coordination and evidence
requirements. This skill does not grant publication, filesystem, or remote-state
permissions and requires no private tooling. Restart OpenCode after installing or
changing skill configuration; verify that this skill is discovered once.
