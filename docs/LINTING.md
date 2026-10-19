# Strict Go formatting and lint

## SDK-STYLE-001 — Complete enforcement

All registered runtime and tool modules use the repository's `.golangci.yml` and
the pinned tools provisioned by `make tools`: golangci-lint **v2.12.2** and
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
