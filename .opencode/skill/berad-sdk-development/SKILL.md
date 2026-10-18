---
name: berad-sdk-development
description: Use when designing, extracting, implementing, reviewing, or validating a Berad SDK module or its consumer adapter.
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
6. Follow [compatibility](../../../docs/COMPATIBILITY.md) and the
   [consumer template](../../../docs/templates/consumer-integration.md). Keep an
   unchanged supported-consumer fixture; publish an available dependency before
   pinning the game. Treat workspace convenience and actual installation separately.
7. Complete the [module README](../../../docs/templates/module-readme.md), public
   examples, relevant consumer docs, catalogs, and notices. Reconcile the final
   diff and every required delivery. Report native observations and limitations
   honestly. Humans merge.

## Foundation commands

From the SDK checkout, use `python3 -I -B scripts/check.py plan`, then `validate`
or `quality-gate`. These currently validate the foundation, not runtime modules.
The guide defines how actual module checks must be added.

Consumer-specific workflows may add their own repository coordination and evidence
requirements. This skill does not grant publication, filesystem, or remote-state
permissions and requires no private tooling. Restart OpenCode after installing or
changing skill configuration; verify that this skill is discovered once.
