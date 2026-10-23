# Validation and avoiding repeated work

## Checker and registered-module coverage

The checker requires Python 3.12+. Provision Black 26.10.1 and Pylint 4.0.4 with
`make python-tools PYTHON_BOOTSTRAP=python3.12` before offline checks. Make prefers this
repository-local interpreter. Registered Go modules additionally need their
declared toolchain, cached dependencies, and a C compiler for race instrumentation.
Pinned goimports/golangci-lint executables are provisioned separately with
`make tools`; see [strict linting](LINTING.md) for the complete source/test policy.
Original game data, a native audio device, private service, Node and network are
not required. From the repository:

```sh
make plan
make validate
make quality-gate
```

The equivalent CLI is `.cache/python-quality/bin/python -I -B scripts/check.py`
with those operations (Windows uses `Scripts/python.exe`). `validate` and
`quality-gate` currently cover
the checker contracts and registered module lanes:

| Check | Inputs and execution |
| --- | --- |
| contracts | Always runs the cheap manifest/module-inventory, required-file, Markdown link/catalog, skill, and source-path checks |
| python-format-check / python-lint | Repository-wide Black and Pylint, including tests and nonignored new Python; runs once even with `--module`, reusing only matching content-bound results |
| style config | Checks tool pins and schema; real ignored-error/type-assertion probes establish effective policy |
| module format | Checks every authored Go file including tests and nested consumers; never formats silently |
| module lint | Full unfiltered production/example/support-package lint, plus non-test consumer packages in isolation |
| checker-tests | Runs checker regressions and real Go/Python style-tool failure probes; reuse binds all Python, scripts/tests, Makefile, manifest, Python policy/lock/hook, Python/Go toolchains, actual Python dependencies, pinned style binaries and relevant environment |
| module unit | Isolated dependency-closure check, zero-skip tests, vet and build; reusable per registered module |
| module race | Separate race-instrumented module tests; does not repeat vet/build |
| module consumer | Copied public-API consumer using only a validated temporary module substitution; vet, ordinary and race tests |

Contract checks enumerate public skills (valid identities/descriptions, no duplicate
names) and check their local document links. Documentation catalogs cover root and
registered module/tool `docs/` trees, each under its owner. New system skills and
module manuals must participate without copying private orchestration. These checks
do not prove semantic migration coverage or actual discovery in an already running
assistant; review ownership and verify fresh-session discovery separately.
Plain OpenCode startup may generate `.opencode/node_modules`, `package.json`,
`package-lock.json`, `bun.lock` and a local `.gitignore`. These exact bootstrap
outputs are ignored and excluded from source inventory; public skills, plugin
source, configuration and similarly named directories elsewhere remain checked.
The SDK's offline checks do not require those OpenCode dependencies.

`plan` reports intended work and does not execute tests or create cache files.
`make python-format-check`, `make python-lint` and `make python-check` are focused,
offline Python-only routes; they do not require Go or native tooling. The standalone
`scripts/python_quality.py` helper and `scripts/check.py` expose the same operations.
`python-format` is an explicit source-changing operation; `python-tools` is explicit
network-enabled provisioning, neither is implicit in a check. See
[Python policy and opt-in hook](LINTING.md#sdk-python-001--repository-wide-python-policy).
Each final run reports executed/reused checks and elapsed time. Documentation edits
do not invalidate checker regression evidence. Checker/test/configuration changes do.
An unknown active module or Go source fails until its real validation is registered.
The runtime inventory includes Berad Audio, Text, Graphics, RNG, Input and Path. Audio transfers its
consumer API scenarios with the canonical import path changed. Redux bridge checks remain
game-owned and are not dependencies of independent SDK validation.

Berad Text additionally declares `execution: native-render`. Its unit, race and
independent-consumer lanes start an Ebitengine harness and perform real offscreen
pixel assertions; an available desktop render backend is required. The backend
selection/display environment and platform are bound into its reuse identity.
Failure to start rendering is not a skipped success. Physical display, input/IME
and untested platforms still require separate observations. Font fixtures come
from the pinned, licensed x/image Go-font dependency; synthetic bitmap pixels are
created by tests. No new local go:embed exemption is introduced.

Berad Graphics uses the same declared `native-render` lane for backend and consumer
pixel assertions. Its root CPU image/mask/cycle package itself requires no display.
Synthetic media exercises non-square/nonzero-origin images, procedural sampling,
partial alpha, additive channels, clipped/fractional mask draws, immutable borrows,
cache eviction and explicit invalidation. `make graphics-demo-smoke` separately
starts the atelier example for eight seconds; it is bounded startup evidence, not
manual visual acceptance or validation of another game's assets.

Berad RNG, Input and Path use ordinary unit/race/independent-consumer lanes without
Ebitengine or original-data dependencies. RNG verifies exact stream/jump vectors
and mixed checkpoint continuation. Input verifies supplied observations, ordered
filtering, consumption, transition/focus protection and explicit cadence. Path
verifies cheapest costs against an independent reference search, repeatable ties,
bounds and visit limits. Their game save/input/movement and native interaction
checks remain consumer-owned.

Each module lane binds module files/modes, locks, checker policy, Python runtime,
Go environment/toolchain implementation, compiler identity and lane selection.
The consumer lane additionally binds its fixture; changes only to that fixture
retain SDK unit/race proof. Another module's source does not invalidate this
module; shared checker/manifest/tool changes do. Dependencies are provisioned
caches checked through `go.mod`/`go.sum`, not rehashed cache directories.
Checks disable workspaces/network and reject module replacements, game imports,
unregistered nested modules and GOFLAGS overrides. Embedded assets require
extending dependency selection before adding `go:embed`. Only the module root
README/LICENSE are excluded from expensive lane fingerprints; cheap contracts
still inspect them every time. Other module assets remain inputs.
Style lanes additionally bind `.golangci.yml` and actual pinned formatter/linter
executable contents and build metadata. Both `validate` and `quality-gate` require
style success; commit/PR orchestration cannot substitute unit-test evidence for it.
The source policy matches the game's strict rules without its inherited-debt cutoff
or game-specific exclusions. Production test exclusions are explicit in LINTING.md.
Each lane has a nonblocking exclusive lock, invalidates stale proof before execution,
and rechecks inputs before recording success. `plan` explains run/reuse without
creating results. Native listening is not part of controlled-transport lanes.

### Runtime/tools registration and focused selection

Inventory schema 2 names each module by its repository-relative identity (for
example `modules/berad-audio` or `tools/berad-audio`), its explicit check list, and
`requires` dependencies on other registered runtime modules. Schema 1 remains
readable. Paths, missing/duplicate identities, cycles and runtime-to-tools
dependencies are rejected. Registration does not imply publication or native proof.

`make quality-gate MODULE=modules/berad-audio` selects
that module's lanes and downstream **candidate** integration lanes. Select a tools
identity for its published-pin and candidate lanes without rerunning upstream unit
tests. Omit `--module` for aggregate acceptance. Make forwards `MODULE=...` to the
same checker; unknown selections fail rather than running zero checks.
Repository-wide Python checks still run once for every selected final gate.

Published-pin lanes resolve normal Go dependencies. A module with `requires` also
registers a `candidate` lane: isolated copies substitute the exact declared runtime
dependency closure, run the dependent module and its public consumer in ordinary
and race modes, and fingerprint those actual candidate inputs. Cache/lock names
include runtime/tools identity and lane. A candidate-runtime change invalidates
that integration evidence without invalidating tools' published-pin evidence.

Packages without test files are explicitly build-only, not missing or skipped
tests. Listed test-bearing packages must each produce passing assertions and a
single successful terminal event. Real test/subtest skips, failures, duplicate or
missing package completions remain errors.

`make provision` (or the checker's explicit `provision` operation) enables ordinary
network dependency provisioning for selected modules and their isolated consumers.
It resolves production and test import closures so lazy module downloads cannot
leave source-archive checksums absent from the next readonly/offline run.
It uses the same consumer layout and creates no validation proof. Normal validation
never provisions dependencies. Tool builds write to their own module-root `bin/`.
Git, source inventory, lane fingerprints and isolated copies exclude only that
generated directory; nested source directories such as `internal/bin` remain inputs.
Keep generated binaries there and
preserve `.cache/validation` during ordinary tool cleanup.

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
Python lanes use this same cache/lock machinery, binding their complete Git source
inventory, `pyproject.toml`, both requirements files, hook and actual installed
tool/dependency implementation bytes rather than version labels alone. Changed
source/configuration/tools, failed identity probes and failed/interrupted runs
invalidate relevant proof before execution. There is no separate Python receipt
store. Pylint caches are disabled. Black's own formatting cache cannot substitute
for the SDK's content identity or its check result.

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

These are onboarding requirements. Checker regressions cover module/consumer input
separation, unrelated-module/prose reuse, dependency/tool invalidation, concurrency,
failed/malformed/changing results and zero-skip package proof. Extend this graph for
new module capabilities (such as generated/embedded assets or native rendering)
instead of claiming audio lanes establish those properties.
