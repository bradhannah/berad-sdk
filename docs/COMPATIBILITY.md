# Compatibility and consumer coordination

## SDK-COMP-001 — Supported behavior evolves compatibly

Within a stable major version, supported consumer source and documented behavior
MUST remain compatible. Signature compatibility alone is insufficient: defaults,
ownership, wrapping, timing, failure outcomes, and persistence can also break users.
Prefer additive capabilities and opt-in behavior. Deprecation MUST explain the
replacement and migration; removal requires a reviewed version boundary.

Experimental `v0` APIs still require explicit change notes and consumer review.
Before v1, exercise the API through a real game adapter and an unrelated consumer;
seek representative use by the second game. Establish ownership, lifecycle, and
resource contracts before making stability promises.

## SDK-COMP-002 — Independent versions, coordinated acceptance

Modules have independent versions. Games pin reviewed versions in their own
`go.mod`; those files remain dependency authority. Record the resolved build list
when accepting a compatibility combination, since a direct requirement alone does
not describe all selected transitive versions.

Coordinate supported consumers around tested combinations of SDK versions,
consumer revisions, Go/Ebitengine versions, and platform observations. Prefer
advancing them together. Before freezing a consumer on an old SDK line, consider
compatible additions, a small game adapter, or a coherent optional advanced package.
An enduring freeze or major-version split MUST have an explicit rationale,
maintenance/support decision, and migration or reconsideration condition.

A shared repository does not automatically prove consumer compatibility. Preserve
an unchanged representative consumer/API fixture across compatible SDK changes;
updating every caller at the same time can conceal a breaking change. API comparison
and consumer compilation supplement behavioral regressions, not replace them.

## SDK-DIST-001 — Resolvable distribution

The module's permanent import path SHOULD be settled before v1. Moving from another
repository is a deliberate import identity migration, even with identical APIs.
For a module at `modules/berad-audio`, a v1.2.0 tag would be
`modules/berad-audio/v1.2.0`. Go major versions v2+ require the corresponding module
path suffix. No tag/version is promised by a directory or catalog entry.

A local workspace MAY assist development. Published consumers MUST resolve actual
available module versions independently, without private credentials or local
checkout substitutions. Test with `GOWORK=off` and ordinary consumer resolution.
Nested modules are excluded from parent module archives.

Safe cutover order:

1. Establish independent module/public-API proof.
2. Exercise the game adapter in an explicitly identified staging fixture if needed.
3. Publish an authorized, resolvable module revision/version.
4. Pin that revision in the consumer and switch its adapter.
5. Verify installation/build outside the development workspace.
6. Retire the duplicate reusable implementation after accepted parity.

Separate minimum supported Go/engine versions from tested toolchain versions.
Dependency upgrades invalidate applicable downstream validation and need consumer
review. Go API versions, asset/persistence schemas, engine compatibility, and
import-path migration are separate contracts.

## Tool modules

Developer tools use independent modules at `tools/berad-<domain>` and tags such as
`tools/berad-audio/v0.1.0`. A tools version pins an already available runtime/authoring
version. Publish runtime capabilities first, then tools that require them, then
update consuming games. Do not introduce unavailable future requirements.

Runtime and tools module archives do not include each other or repository-root
scripts/assets. Include their own notices and required resources. Version-qualified
tool installation must work outside the SDK checkout without replacements, a
workspace, private data or another command installed implicitly. Cross-module
documentation references should use stable public URLs when needed from an archive.

Tools' published-pin checks and candidate-runtime integration are separate evidence:
candidate integration uses only explicitly registered temporary substitutions and
does not establish distribution. Games must check the actual selected runtime and
tools versions together, since tools' requirements participate in Go module selection.
