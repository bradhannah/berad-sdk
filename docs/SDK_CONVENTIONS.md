# SDK conventions

## SDK-MOD-001 — Independent modules and identities

Each active SDK MUST live at `modules/berad-<domain>/` with its own `go.mod` and
deliberate dependency graph. Its initial canonical path MUST be
`github.com/bradhannah/berad-sdk/modules/berad-<domain>`. Root packages SHOULD use
short domain names such as `audio`, `text`, or `graphics`.

An incubating conversion MAY temporarily retain its already-approved module path
in another repository. Record that identity, the future path, and an explicit
consumer migration. A GitHub-looking import is not proof that a module is available.
See [compatibility](COMPATIBILITY.md).

## SDK-MOD-002 — Layout and exposure

Use this structure, adding optional directories only for real content:

```text
modules/berad-<domain>/
  go.mod
  go.sum                     when dependencies require it
  LICENSE                    preserve all applicable source notices
  README.md
  doc.go
  <capability>.go
  <capability>_test.go
  backend/ebitengine/         engine adapter, where a separate package is useful
  internal/                  implementation subpackages, where useful
  examples/                  small public-API examples
  testdata/consumer/          independently built consumer module
  docs/                      substantial module-specific contracts
  cmd/                       supported tools, if any
```

Exported packages MUST represent supported consumer capabilities. Prefer
capability-based Go filenames, Go's normal `_test.go` convention, and private
implementation. A public root facade need not forward every internal method.
Avoid generic `common`, `utils`, `interfaces`, or `manager` packages without a
coherent consumer-facing responsibility. Game-owned bridges belong with the game.

## SDK-MOD-003 — Dependency and input boundaries

Reusable production code, independent tests, and examples MUST NOT depend on a
game's packages, private tooling, media catalog, original installation, settings,
or checkout layout. Accept explicit assets/roots and configuration. Use `fs.FS`
or another narrow supplied capability where useful; document filesystem trust
and path-resolution semantics rather than promising a sandbox accidentally.

A consumer bridge fixture MAY import its game. It MUST be identified separately
from independent SDK proof. Engine adapters MAY use Ebitengine directly according
to [API design](API_DESIGN.md). Ordinary runtime imports SHOULD avoid editor/TUI
packages and unrelated optional services.

## SDK-DOC-001 — Documentation with the contract

Every active module MUST document availability, supported public packages,
minimum versus tested Go/engine versions, construction/defaults, units, ownership,
errors, concurrency, resource limits, examples, and validation commands. Use the
[module README](templates/module-readme.md) and
[API contract](templates/api-contract.md) templates.

Public examples MUST compile against the declared public API. Document deliberate
limitations and separate automated evidence from native observations. Keep docs
self-contained and catalog `docs/**/*.md` exactly once under its repository or
module documentation owner. Root catalogs may route to module catalogs rather
than duplicating their inventories.

## SDK-GOV-001 — Shared standards and exceptions

An extraction MUST record the reviewed SDK-guidance commit and identify applicable
stable rule IDs. Capture the checkpoint in the approved issue/PR and maintain the
resulting contract in module docs. Guidance updates require an impact review for
active conversions; they do not silently expand another agent's scope.

Keep one canonical rule and link to it. The public skill routes to these contracts;
private consumer orchestration may add delivery steps without copying the rules.
