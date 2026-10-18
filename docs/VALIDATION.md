# Validation and avoiding repeated work

## Actual foundation coverage

Requires Python 3.10+. No Go toolchain, original game data, graphics/audio device,
private service, Node, or network access is needed. From the repository:

```sh
python3 -I -B scripts/check.py plan
python3 -I -B scripts/check.py validate
python3 -I -B scripts/check.py quality-gate
```

Make offers the same three targets. `validate` and `quality-gate` currently cover
the same two foundation checks:

| Check | Inputs and execution |
| --- | --- |
| contracts | Always runs the cheap manifest/module-inventory, required-file, Markdown link/catalog, skill, and source-path checks |
| checker-tests | Runs stdlib regression tests for the checker; reuses success when scripts/tests, Makefile, manifest, Python runtime identity, and relevant environment match |

`plan` reports intended work and does not execute tests or create cache files.
Each final run reports executed/reused checks and elapsed time. Documentation edits
do not invalidate checker regression evidence. Checker/test/configuration changes do.
An unknown active module or Go source fails until its real validation is registered.

Local regression results live under ignored `.cache/validation/`. They are an
optimization, not signed evidence or a publication authorization. A process lock
prevents concurrent duplicate regression runs. A failed/interrupted attempt is not
reusable; inputs are compared before and after execution. Missing or malformed
records rerun the check. Removing this cache requests a clean run. Source inputs
must be regular files; external symlinks are not implicit validation dependencies.

The cache identifies the selected interpreter, its binary, standard-library Python
sources, runtime version/platform, and environment affecting execution. Validation
uses isolated Python subprocesses. No freshness claim for native device observations
or future modules follows from this cache. Cross-repository receipt integration is
external orchestration and is not required to run these public checks.

## SDK-TEST-001 — Select affected checks

Register each module and check explicitly when onboarding it. A module check graph
MUST account for its production/test imports, fixtures/assets, schemas/generated
inputs, backend, tools, flags, and relevant consumer contracts. Root `go test ./...`
does not traverse nested modules. Package/module directory changes alone are not
a sufficient dependency model.

| Change | Default affected scope |
| --- | --- |
| Prose-only documentation | Links/catalog/contracts; changed executable examples if any |
| Game adapter | Adapter and relevant consumer integration; unchanged SDK evidence remains reusable |
| SDK implementation | Affected module tests and behavioral consumer scenarios |
| Public API | Compatibility comparison, supported-consumer compilation, affected integration |
| Shared toolchain/engine/dependency | Applicable downstream modules and consumers |
| Check policy or unknown dependency impact | Explicitly broadened scope, with explanation |

Meaningful tests prove contracts, regressions, resource/failure boundaries, and
independent use. Avoid tests that mirror implementation or duplicate another
layer's complete suite. Retain the lowest useful test level plus necessary boundary
coverage. Native listening/rendering/input observations and automated tests prove
different properties.

## SDK-TEST-002 — Reuse valid results

Run each required check once for matching material inputs. A reusable result MUST
bind its source/test/assets/dependency closure, checker policy, execution flags,
tools, platform, and material environment. A different consumer revision does not
invalidate an independent SDK unit result unless it is actually an input.
However, the consumer integration check has its own input set and may need rerunning.

Use Go's build/test caches where correct. Separate ordinary, race, independent
consumer, and native checks. Reuse identical vet/build work across those lanes.
Do not indiscriminately attach uncached all-module suites to every development,
commit, and PR step. Later gates may consume valid earlier results; they must not
silently treat stale, failed, interrupted, or incomplete proof as success.

Coordinate expensive work across agents. Explain run/reuse decisions and durations.
Preserve isolation correctness when reusing caches: absolute temporary paths,
generated artifacts, and borrowed assets can change their validity. Native/device
checks need their own explicit environment/freshness policy.

## SDK-TEST-003 — Module-onboarding acceptance

Before a runtime module is accepted here, demonstrate:

1. A repeated unchanged gate reuses eligible expensive checks.
2. Documentation-only changes do not rerun unrelated runtime tests.
3. Adapter-only changes retain unchanged SDK unit evidence.
4. Shared dependency/tool changes invalidate affected downstream results.
5. Concurrent requests do not run the same expensive check twice.
6. Missing/failed/stale proof cannot satisfy required acceptance.
7. The plan explains every selection and reports actual execution/reuse.
8. Independent consumer builds work without a workspace masking dependencies.

These are onboarding requirements. The foundation implements and tests reuse for
its checker regressions only; a generalized Go-module/consumer scheduler will be
registered with the first real modules and their actual dependency graphs.
