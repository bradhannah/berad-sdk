# Documentation catalog

Canonical contracts live here. The skill and contributor instructions route to
them; module documentation explains each actual implementation. Issues own work
status, and ADRs explain decisions. A template is a starting point, not evidence
that its requirements are implemented.

Every retained Markdown document beneath `docs/` is cataloged exactly once:

| Document | Owner | Authority / purpose |
| --- | --- | --- |
| [README.md](README.md) | documentation | Index and authority |
| [SDK_CONVENTIONS.md](SDK_CONVENTIONS.md) | SDK architecture | Mandatory module, package, and documentation conventions |
| [API_DESIGN.md](API_DESIGN.md) | SDK architecture | Public boundaries, ownership, and dependency exposure |
| [COMPATIBILITY.md](COMPATIBILITY.md) | SDK maintenance | Versioning and coordinated consumer evolution |
| [VALIDATION.md](VALIDATION.md) | SDK tooling | Python/foundation checks, content-bound reuse and module-onboarding requirements |
| [LINTING.md](LINTING.md) | SDK engineering | Pinned Go/Python formatting/lint, typed Python inventory, opt-in hook, exceptions and zero-findings acceptance |
| [LICENSING.md](LICENSING.md) | SDK maintenance | Commercial-use and transferred-asset/source policy |
| [EXTRACTION_GUIDE.md](EXTRACTION_GUIDE.md) | SDK architecture | Conversion workflow and review checkpoints |
| [adr/0001-sdk-family-foundation.md](adr/0001-sdk-family-foundation.md) | SDK architecture | Accepted family structure and trade-offs |
| [templates/module-readme.md](templates/module-readme.md) | documentation | Module consumer documentation template |
| [templates/api-contract.md](templates/api-contract.md) | SDK architecture | API and resource-contract template |
| [templates/consumer-integration.md](templates/consumer-integration.md) | SDK maintenance | Consumer adaptation and acceptance template |
| [templates/architecture-decision.md](templates/architecture-decision.md) | SDK architecture | Decision record template |
| [templates/extraction-plan.md](templates/extraction-plan.md) | SDK architecture | Conversion checkpoint and completion template |

## Requirement language

System documentation is maintained with its implementation:
[audio runtime/authoring catalog](../modules/berad-audio/docs/README.md),
[text API catalog](../modules/berad-text/docs/README.md),
[graphics API catalog](../modules/berad-graphics/docs/README.md), and
[audio tools catalog](../tools/berad-audio/docs/README.md). The SDK-owned public
skills route to these contracts; consumer integration guides own application policy.
The [RNG catalog](../modules/berad-rng/docs/README.md) owns deterministic stream
continuation; the [Input guide](../modules/berad-input/README.md) owns supplied-key
observations, binding filtering and release/repeat mechanics.

**MUST** is required. **SHOULD** is the default unless a reviewed reason justifies
an exception. **MAY** is optional. Stable `SDK-*` identifiers refer to the specific
rules below, not game mechanics. Keep an ID's meaning stable; introduce a new ID
when changing its meaning materially.

An exception identifies the rule, concrete consumer need, consequence, owner
approval, and reconsideration condition. Record durable exceptions with the
module's contract/ADR and link them from its extraction review. Mechanical
validation does not establish semantic conformance.
