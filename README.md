# Berad SDK

Focused Go/Ebitengine libraries for reusable game services, maintained as one
SDK family with independently versioned modules.

**MIT-licensed for commercial and non-commercial use**, including closed-source
games. Retain the required copyright and license notices. Third-party code and
assets retain their own terms; see [licensing](docs/LICENSING.md).

## Current availability

Engineering contracts, conversion guidance, templates and an OpenCode skill are
available. **Berad Audio** provides an experimental runtime with explicit
module/consumer checks on the `feat/berad-audio` development branch. Consumers
must pin a remotely available reviewed revision; there are no stable SDK version
releases. Text and graphics remain planned identities.
See the [module catalog](modules/README.md).
The [tool catalog](tools/README.md) describes the separately versioned tools layout.

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
Other agents and human contributors can follow the same linked documents directly.
No private checkout, service, or original game installation is required.

## Principles

- Expose only capabilities needed by supported consumers.
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
Checks are local and offline. Checker and registered module lanes reuse successful
results when their inputs match; documentation checks remain inexpensive and run
every time. See [validation](docs/VALIDATION.md) for exact coverage and limitations.

Use `make plan MODULE=modules/berad-audio` for focused selection. `make provision`
is explicit network-enabled setup, separate from offline validation. `make clean`
removes only known generated tool binaries under `.cache/bin`; it keeps validation
results and authored projects. Tool module selection becomes available when its
implementation is registered, rather than treating an empty directory as acceptance.

Read the [documentation catalog](docs/README.md) and
[contribution policy](CONTRIBUTING.md) for the remaining contracts.
