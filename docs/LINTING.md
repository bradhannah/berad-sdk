# Strict Go and Python formatting and lint

## SDK-STYLE-001 — Complete enforcement

All registered runtime and tool modules use the repository's `.golangci.yml` and
the pinned tools provisioned by `make tools`: golangci-lint **v2.15.0** and
goimports from **golang.org/x/tools v0.37.0**. Tool build metadata and the linter's
reported version must match; missing or substituted versions fail explicitly.
The SDK owns these tools under ignored `.cache/tools`, independently of consumers.

The starting rule set and cyclomatic threshold match Ultima V Redux's strict Go
policy. The executable configuration remains the rule inventory. SDK enforcement
has **no revision cutoff, inherited-debt baseline, diagnostic caps or broad
package exemptions**. Every finding in the selected production inventory fails.
Default linter exclusion presets are disabled. Ebitengine's optional draw-option
fields retain the narrow documented literal exception from the source policy.

Strict production lint includes runtimes, authoring services, tools, commands,
examples and non-test support packages. Each registered nested module is enumerated
explicitly; root `./...` is not assumed to traverse them. Consumer fixtures are
checked in isolation with their reviewed substitution. Their non-test packages
receive the same lint policy. Published modules cannot use those substitutions.

As in Redux, `_test.go` files do not receive the production style-linter suite.
They **do** receive goimports checking, `go vet`, ordinary and race tests through
the registered module/consumer lanes. Test-only consumer packages are reported
explicitly rather than counted as production lint coverage. No blanket `testdata/`
exemption hides authored formatting or non-test consumer source. Native-render
tests still require their real backend. This policy describes Go source; it does
not claim that golangci-lint checks the Python orchestration.

## Commands

| Command | Purpose |
| --- | --- |
| `make tools` | Explicit network-enabled provisioning of pinned style tools |
| `make format` | Format all authored Go source, including tests and nested consumers |
| `make format-check` | Fail on formatting/import differences without editing |
| `make lint-config-check` | Verify tool pins/schema and exercise ignored-error/type-assertion diagnostics |
| `make lint` / `make lint-all` | Full-source lint; both are unfiltered |
| `make lint-fix MODULE=modules/berad-text` | Apply supported automatic fixes to the selected module; remaining findings still fail |
| `make tooling-test` | Checker regressions, including real formatter/linter failure probes |
| `make validate` / `make quality-gate` | Mandatory style gates plus registered checker/runtime/consumer lanes |

Use `MODULE=modules/berad-audio`, `MODULE=modules/berad-text`, or
`MODULE=tools/berad-audio` for focused work. Omit it for complete SDK acceptance.
Normal checks are offline; dependency provisioning remains `make provision`.
`make clean` preserves provisioned style tools and validation evidence.

## SDK-STYLE-002 — Fixes and exceptions

Start with the automatic-fix mode on clean, reviewed source scope and inspect its
diff separately. Then fix the remaining diagnostics in every manually changed
file. Preserve API identity, source-format names, callback ordering and numerical
behavior. Do not reshape a cohesive algorithm merely to satisfy a metric; extract
real validation, shaping, publication or rendering responsibilities where helpful.

Use only narrow named `//nolint:<linter> // rationale` exceptions where the
contract genuinely requires them. Examples include exact `io.EOF` forwarding,
signed PCM wire-bit reinterpretation, process-wide device ownership mandated by
the backend, and compatibility-preserved public names. Do not suppress ignored
errors, unchecked assertions or fixable correctness/security findings. Optional
zero-valued configuration is made explicit; caller-selected filesystem paths retain
their documented trust boundary rather than claiming an invented sandbox.

The checker verifies effective `errcheck` behavior with intentionally broken and
checked-control code. Regression tests prove bad formatting fails without edits,
lint failures block acceptance, and all registered modules are selected.

## Reuse and delivery

Style proof binds source and consumer bytes/modes, lint policy, checker policy,
Go/toolchain identity, pinned executable contents and material environment.
Changed policy or tools cannot reuse an old pass. Failed, incomplete or
changing-input work cannot mint success. These style lanes are mandatory through
the same SDK checker used by commit and PR preflight; test success alone is not
style acceptance. See [validation](VALIDATION.md).

Lint-driven corrections retain the existing source/layout/shutdown and audio
stream/publication contracts. Errors gain contextual wrapping while preserving
causes; IO adapters retain required exact sentinels. The catalog exporter now
reports unexpected protected-input inspection failures before writing either
report. The standalone probe releases its signal context before process exit.

## SDK-PYTHON-001 — Repository-wide Python policy

All authored Python, including tests and nonignored new `.py` files, requires
**Python 3.12+**, meaningful argument/return and collection annotations, explicit
JSON/schema shapes and typed callbacks where relevant. Dynamic module loaders
validate the spec/loader. Do not replace useful types with widespread `Any`.
Runtime utilities use only the standard library; development tooling lives in
the ignored repository-local `.cache/python-quality` virtual environment.

The only Python quality tools are **Black 26.10.1** and **Pylint 4.0.4**.
`pyproject.toml` selects Black's stable style, 88 columns and Python 3.12 syntax;
preview/unstable style is off. Pylint loads `pylint.extensions.typing`, retains
its default diagnostics, uses `score = false` and effective `exit-zero = false`, and must
report **zero enabled findings**. No Mypy gate or pytest migration is involved.
`mypy-extensions` is only a transitive Black dependency. Suppress a named diagnostic
only at the smallest useful scope with an adjacent explanation. Tests may omit
duplicative method docstrings when their names state the behavior; they still
receive formatting, annotations and all other enabled lint checks.
Pylint 4.0.4 implements `exit-zero` as a flag: placing even `false` in TOML
enables it. The repository therefore deliberately omits that option and verifies
its false default with a real failing fixture. Do not add `exit-zero` to the config.
Pylint's three ignore lists are explicitly empty, so hidden Python filenames in
the Git inventory are checked too. The helper requires an empty valid JSON
diagnostic list and a zero process status: configuration diagnostics such as
E0015 may otherwise accompany a misleading zero exit status.

### Provisioning and independent checks

```sh
make python-tools PYTHON_BOOTSTRAP=python3.12
make python-format
make python-format-check
make python-lint
make python-check
```

`requirements-quality.in` owns the direct exact pins;
`requirements-quality.lock` pins the complete universal dependency closure with
hashes, including Windows markers. The explicit `python-tools` operation creates
the venv and runs `pip install --require-hashes -r requirements-quality.lock`.
It is the only Python operation allowed to install packages or use the network.
Provisioning rejects symlinked cache/environment directories and any existing
environment rather than overwriting it. Remove an old environment deliberately
before reprovisioning. `PYTHON_BOOTSTRAP` selects the setup interpreter separately
from checks; it defaults to `PYTHON` for compatibility with existing callers.
Refresh the lock deliberately using
`uv pip compile --python-version 3.12 --universal --generate-hashes --output-file requirements-quality.lock requirements-quality.in`;
review every version/hash change. Ordinary checks never refresh or install tools.

Make prefers the provisioned interpreter and otherwise uses `python3`; select a
compatible bootstrap with `PYTHON_BOOTSTRAP=python3.12` when ambient Python is older. The
checker fails clearly below 3.12. Standalone use is
`.cache/python-quality/bin/python -I -B scripts/python_quality.py python-check`
(Windows uses `.cache/python-quality/Scripts/python.exe`). The same `python-*`
operations are available through `scripts/check.py`; `python-identity` prints the
material-input digest. These routes do not probe Go, native devices or private
repositories. `python-format` is the only formatting operation that edits source.

The helper inventories tracked and nonignored new paths through Git, includes
tests anywhere in the repository, prunes `.cache`/`__pycache__` directories and
rejects source symlinks. `MODULE=...` never narrows this inventory. Both final gates
include the Python formatting and lint lanes exactly once. Existing
`.cache/validation` lane records bind source paths/bytes/modes, configuration,
lock files, hook, interpreter/stdlib implementation and actual installed bytes of
every pinned tool dependency and installed library files (including added imports
and bytecode), plus material environment. Matching versions alone
are insufficient. Failed probes/runs and changing inputs cannot leave reusable
success. `plan` reads identities/results without creating caches or running tools.

### Optional native pre-commit hook

`.githooks/pre-commit` is a thin, check-only invocation of the public helper. It
never formats, stages, installs dependencies or changes Git configuration. The
hook requires the complete index to match the worktree (including nonignored new
files), checks repository-wide Python, then verifies the index/worktree again.
Partially staged candidates fail with a clear request to stage or set aside the
remaining changes. It is a Python check, not a replacement for final SDK gates.

Installation is opt-in. First inspect `git config --get core.hooksPath` and any
existing `.git/hooks/pre-commit`. If a hook manager or existing hook is present,
preserve it and compose an invocation of this repository's hook into that flow.
Only when no existing hook configuration or hook would be displaced, install with
`git config --local core.hooksPath .githooks`. Provision tools first. Replacing an
existing hooks path without reviewing its hooks is not supported. No checks or
provisioning commands automatically install this hook.
