# Berad SDK

Focused Go/Ebitengine libraries for reusable game services, maintained as one
SDK family with independently versioned modules.

**MIT-licensed for commercial and non-commercial use**, including closed-source
games. Retain the required copyright and license notices. Third-party code and
assets retain their own terms; see [licensing](docs/LICENSING.md).

## Current availability

Engineering contracts, conversion guidance, templates and an OpenCode skill are
available. **Berad Audio** provides experimental runtime/authoring and separately
versioned terminal-editor/catalog/probe tools, with explicit module/consumer checks
on `main`. Consumers
must pin a remotely available reviewed revision; there are no stable SDK version
releases. **Berad Text** is also available on `main`,
with supplied fonts, styled layout/local geometry and native-render consumer
checks. **Berad Graphics** is available on `main`,
with supplied images, procedural effects, masks and explicit Ebitengine resource
ownership. Pin only actually available reviewed revisions.
**Berad RNG** and **Berad Input** are candidate modules for exact deterministic
stream continuation and supplied-state keyboard mechanisms. Their consumers own
seed/command policy, clocks, native device sampling and future pointer routing.
Standalone version-qualified tools installation awaits reviewed default-branch
availability or a separately authorized module tag; pinned Redux game/editor
installation is verified independently. Neither is native listening acceptance.
See the [module catalog](modules/README.md).
For the text demonstration, use `make build-text-demo`, `make text-demo`, or
`make text-demo-smoke`. The smoke target uses an isolated HOME/working directory,
an eight-second native startup window, and controlled process cleanup. It requires
a desktop backend and is separate from physical keyboard/mouse/IME acceptance.
The [tool catalog](tools/README.md) describes the separately versioned tools layout.
Use `make graphics-check`, `make graphics-demo`, and `make graphics-demo-smoke`
for graphics and its synthetic independent atelier example.
Use `make rng-check` and `make input-check` for the engine-free algorithm and
independent consumer checks; these modules have no desktop/device prerequisite.

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

Requires Python 3.10 or newer; runtime lanes also require their declared Go
toolchain and provisioned dependencies. Make is a convenience:

```sh
python3 -I -B scripts/check.py plan
python3 -I -B scripts/check.py validate
```

Equivalent Make targets are `make plan`, `make validate`, and `make quality-gate`.
Provision pinned formatting/lint tools with `make tools`. `make format-check` and
`make lint` enforce the [strict Go style policy](docs/LINTING.md) across all registered
runtimes and tools, with zero unfiltered findings required by both final gates.
Use `make lint-fix MODULE=...` for a reviewed automatic-fix pass before manual cleanup.
Checks are local and offline. Checker and registered module lanes reuse successful
results when their inputs match; documentation checks remain inexpensive and run
every time. See [validation](docs/VALIDATION.md) for exact coverage and limitations.

Use `make plan MODULE=modules/berad-audio` for focused selection. `make provision`
is explicit network-enabled setup, separate from offline validation. `make clean`
removes only known generated tool binaries under `tools/berad-audio/bin` (and old
`.cache/bin` outputs); it keeps validation
results and authored projects. `MODULE=tools/berad-audio` selects the registered
audio-tool lanes; empty/planned module directories do not establish acceptance.

Read the [documentation catalog](docs/README.md) and
[contribution policy](CONTRIBUTING.md) for the remaining contracts.
