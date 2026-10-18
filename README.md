# Berad SDK

Focused Go/Ebitengine libraries for reusable game services, maintained as one
SDK family with independently versioned modules.

**MIT-licensed for commercial and non-commercial use**, including closed-source
games. Retain the required copyright and license notices. Third-party code and
assets retain their own terms; see [licensing](docs/LICENSING.md).

## Current availability

This repository is at the **foundation stage**: engineering contracts, conversion
guidance, templates, an OpenCode skill, and lightweight validation are available.
There are **no installable runtime modules or SDK version releases here yet**.
Audio, text, and graphics are planned module identities, not available packages.
See the [module catalog](modules/README.md).

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

## Validate this foundation

Requires Python 3.10 or newer. Make is a convenience, not a requirement:

```sh
python3 -I -B scripts/check.py plan
python3 -I -B scripts/check.py validate
```

Equivalent Make targets are `make plan`, `make validate`, and `make quality-gate`.
Checks are local and offline. Foundation regression tests reuse a successful local
result when their inputs match; documentation checks remain inexpensive and run
every time. See [validation](docs/VALIDATION.md) for exact coverage and limitations.

Read the [documentation catalog](docs/README.md) and
[contribution policy](CONTRIBUTING.md) for the remaining contracts.
