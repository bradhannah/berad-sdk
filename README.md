# Berad SDK

Focused Go/Ebitengine libraries for reusable game services, maintained as one
SDK family with independently versioned modules.

**MIT-licensed for commercial and non-commercial use**, including closed-source
games. Retain the required copyright and license notices. Third-party code and
assets retain their own terms; see [licensing](docs/LICENSING.md).

## Current availability

Every module is experimental (v0). Berad Path serves berad-engine; the others
are used by Ultima V Redux. There are no stable
releases: APIs can change between revisions, so pin a reviewed revision.

| Module | Provides | Checks |
| --- | --- | --- |
| [Berad Audio](modules/berad-audio/README.md) and [its tools](tools/README.md) | Cues, voices, music, ambience, authoring; terminal editor, catalog and probe | `make audio-runtime-check`, `make audio-tools-check` |
| [Berad Text](modules/berad-text/README.md) | Supplied fonts, styled layout and caret geometry, bounded rendering | `make text-check` |
| [Berad Graphics](modules/berad-graphics/README.md) | Supplied pixels, procedural effects, masks, Ebitengine resource ownership | `make graphics-check` |
| [Berad RNG](modules/berad-rng/README.md) | Deterministic random streams with exact checkpoint restoration | `make rng-check` |
| [Berad Input](modules/berad-input/README.md) | Supplied keyboard state, bindings, repeat and release protection | `make input-check` |
| [Berad Path](modules/berad-path/README.md) | Cheapest-route grid search with caller-supplied steps and costs | `make path-check` |

Consumers own game policy: seeds, clocks, device sampling and media catalogs. Native
demo and smoke targets need a desktop backend. See the
[module catalog](modules/README.md) and the [tool catalog](tools/README.md).

## Converting an existing subsystem?

Start with [AGENTS.md](AGENTS.md) and the
[extraction guide](docs/EXTRACTION_GUIDE.md). Use the
[extraction checkpoint template](docs/templates/extraction-plan.md) before broad
source movement. It records the public boundary, ownership, consumer requirements,
licensing, validation selection, and the guidance revision being followed.

Conversions may adopt these rules while their code still incubates elsewhere.
Record existing decisions and review deviations explicitly; this repository does
not silently change an active conversion's approved scope.

OpenCode users can load the public
[`berad-sdk-development` skill](.opencode/skill/berad-sdk-development/SKILL.md).
Audio work also uses the SDK-owned
[`berad-audio-development` skill](.opencode/skill/berad-audio-development/SKILL.md).
Graphics work uses
[`berad-graphics-development`](.opencode/skill/berad-graphics-development/SKILL.md).
Other agents and human contributors can follow the same linked documents directly.
No private checkout, service, or original game installation is required.

## Principles

- Expose only capabilities needed by supported consumers.
- Count real SDK tool workflows as consumers; demos and tests alone do not justify
  new runtime features. Add future-game capabilities when their concrete need exists.
- Prefer straightforward Ebitengine integration over speculative abstraction.
- Keep game policy in game-owned adapters; accept explicitly supplied assets.
- Specify resource ownership, defaults, failure behavior, and compatibility.
- Validate affected work and reuse valid results instead of rerunning everything.
- Coordinate consumer upgrades; keep long-term version freezes an explicit last resort.

## Validate registered work

Requires Python 3.12 or newer; runtime lanes also require their declared Go
toolchain and provisioned dependencies. Provision Python tools separately:

```sh
make python-tools PYTHON_BOOTSTRAP=python3.12
make python-check
make plan
make validate
```

Make prefers `.cache/python-quality` after provisioning. The equivalent checker
entry point is `.cache/python-quality/bin/python -I -B scripts/check.py` with
`plan`, `validate` or `quality-gate` (Windows uses `Scripts/python.exe`). An explicit
`PYTHON` selection must meet the 3.12 floor; unsupported ambient Python fails clearly.
Provision pinned formatting/lint tools with `make tools`. `make format-check` and
`make lint` enforce the [strict Go style policy](docs/LINTING.md) across all registered
runtimes and tools, with zero unfiltered findings required by both final gates.
Use `make lint-fix MODULE=...` for a reviewed automatic-fix pass before manual cleanup.
Checks are local and offline. Checker and registered module lanes reuse successful
results when their inputs match; documentation checks remain inexpensive and run
every time. See [validation](docs/VALIDATION.md) for exact coverage and limitations.
Python source and tests use pinned Black 26.10.1 (stable, 88 columns) and Pylint
4.0.4 with meaningful annotations and zero enabled findings. Use
`make python-format`, `make python-format-check`, `make python-lint` or the combined
`make python-check`; these need no Go compiler or native backend. Both final gates
include repository-wide Python once, even when a module is selected. The
[Python policy and opt-in hook](docs/LINTING.md#sdk-python-001--repository-wide-python-policy)
describe provisioning, standalone helper use and existing-hook preservation.

Use `make plan MODULE=modules/berad-audio` for focused selection. `make provision`
is explicit network-enabled setup, separate from offline validation. `make clean`
removes only known generated tool binaries under `tools/berad-audio/bin` (and old
`.cache/bin` outputs); it keeps validation
results and authored projects. `MODULE=tools/berad-audio` selects the registered
audio-tool lanes; empty/planned module directories do not establish acceptance.

Read the [documentation catalog](docs/README.md) and
[contribution policy](CONTRIBUTING.md) for the remaining contracts.
