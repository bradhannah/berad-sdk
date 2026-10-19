# Contributing

Discuss a proposed module, API change, or significant extraction in an issue
before broad implementation. An approved maintainer request also establishes
scope. Bug reports should name the module/revision, platform, reproduction, and
expected behavior. Use public-safe examples and assets you may redistribute.

Start with [AGENTS.md](AGENTS.md) and the
[extraction guide](docs/EXTRACTION_GUIDE.md). Work on a dedicated branch and submit
a PR for human review. Describe affected contracts, consumers, licensing, selected
validation, reused results, and any explicit limitations. Private orchestration
is not a contributor prerequisite.

Validation requires Python 3.10+, the registered Go toolchain/dependencies and
the pinned formatter/linter. Provision tools explicitly with `make tools` and
dependencies with `make provision`, then use the offline Make targets:

```sh
make plan
make format-check
make lint
make quality-gate
```

See [strict linting](docs/LINTING.md) and [validation](docs/VALIDATION.md) before
adding a module or check. Every registered runtime/tool must have zero full-source
findings; no changed-line baseline hides inherited debt. Begin cleanup with a
reviewed `make lint-fix MODULE=...` pass, then address remaining findings manually.
Checks run locally without downloads; provision dependencies separately.
No hosted CI workflow is configured by this foundation.

Contributed project-authored code and documentation use the repository's
[MIT license](LICENSE). Preserve existing notices and identify third-party terms.
Do not assume a source-code license also covers accompanying media.
