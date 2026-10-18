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

The foundation requires Python 3.10+. Run:

```sh
python3 -I -B scripts/check.py plan
python3 -I -B scripts/check.py quality-gate
```

See [validation](docs/VALIDATION.md) before adding a module or check. Checks run
locally without downloads; provision any future module dependencies separately.
No hosted CI workflow is configured by this foundation.

Contributed project-authored code and documentation use the repository's
[MIT license](LICENSE). Preserve existing notices and identify third-party terms.
Do not assume a source-code license also covers accompanying media.
