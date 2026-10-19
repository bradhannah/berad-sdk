# Berad SDK contributor instructions

Read [README.md](README.md), [the documentation catalog](docs/README.md), and
[the extraction guide](docs/EXTRACTION_GUIDE.md) before SDK conversion work.
Use the public `berad-sdk-development` skill when available; its linked documents
are canonical, so the skill is not a second copy of the rules.

## Boundaries

- The repository contains independently versioned runtimes under `modules/berad-*`
  and developer tools under `tools/berad-*`. Tools may consume runtimes; runtimes
  must not depend on tools/TUI. Keep executable packages beneath each tool's `cmd/`.
  `sdk.json` is the active-module inventory. Register checks before introducing
  module code; the original empty inventory supported foundation-only validation.
- Expose only required consumer capabilities. Prefer concrete service types with
  private state and small caller-supplied interfaces. Review every public package
  and third-party type against [API design](docs/API_DESIGN.md).
- Keep game semantics, media catalogs, settings, input policy, and world rules in
  consumer adapters. The SDK takes explicit resources and configuration.
- Document ownership, concurrency, time/coordinates/units, defaults, errors,
  shutdown, and resource limits. Observational diagnostics must not drive behavior.
- Ebitengine is the supported engine direction. Direct engine types are acceptable
  where they simplify a deliberate boundary. Do not invent a future renderer or
  tile abstraction without a concrete consumer requirement.

## Working practice

- Establish scope in an issue or an explicitly approved maintainer request. Use a
  dedicated branch; preserve concurrent work. Humans merge PRs.
- Before broad moves, complete the extraction checkpoint and record the reviewed
  standards commit. New guidance needs an impact review for ongoing conversions.
- Run focused checks while developing. Run the appropriate final validation once;
  reuse valid results at later handoffs. See [validation](docs/VALIDATION.md).
- Enforce [strict Go style](docs/LINTING.md) across every registered runtime/tool.
  Use `make tools`, `make format-check` and `make lint`; final gates require zero
  unfiltered findings. Start with scoped `make lint-fix` on reviewed source, inspect
  automatic edits separately, then resolve manual findings without debt baselines
  or blanket exclusions. Preserve public APIs and use only narrowly justified suppressions.
- Add tests for meaningful behavior, regressions, boundaries, and failure paths.
  Avoid implementation-mirroring tests and duplicate integration scenarios.
- Keep public contracts and examples current with implementation. Catalog retained
  `docs/**/*.md` exactly once. Document justified exceptions by stable rule ID.
- Project-authored source/docs are MIT. Preserve transferred source notices and
  review third-party/fixture terms under [licensing](docs/LICENSING.md).
- Public content must be self-contained: no private repository links, credentials,
  personal machine paths, private evidence, or private-tool prerequisites.
- Commit, push, publish, or alter remote state only with authorization. Do not
  commit to `main`, bypass validation, or merge PRs on the maintainer's behalf.

## Registered checks

Use `python3 -I -B scripts/check.py plan` to explain work, then `validate` or
`quality-gate`. Both final targets cover the checker and registered module lanes;
unchanged invocations reuse eligible evidence. Native-platform and semantic API
acceptance require their own scoped observations. See `docs/VALIDATION.md`.
