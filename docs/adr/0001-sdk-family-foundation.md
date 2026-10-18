# ADR 0001: One SDK family, independently versioned modules

Status: Accepted for the repository foundation.

## Context

Reusable audio, text, and graphics services are being extracted for more than one
Go/Ebitengine game. They need common public-boundary, ownership, documentation,
licensing, and validation conventions before separate conversions diverge.

## Decision

Use one public repository, with independent modules under `modules/berad-*`.
Project-authored code/docs are MIT-licensed for commercial and non-commercial use.
Canonical rules and templates are public; a public OpenCode skill routes to them.
Consumers own game policy and may add private orchestration externally.

Expose only required capabilities. Permit deliberate Ebitengine types where they
simplify integration; introduce abstractions for concrete contracts, not imagined
backends or future tile systems. Prefer compatible additions and coordinated
consumer upgrades while preserving independent version selection.

Validation must be affected-input-aware and reusable. Begin with a small executable
foundation gate; register actual module/consumer check graphs as modules arrive.
An empty active inventory must not silently validate unregistered runtime code.

## Consequences

Family-wide decisions have one home and cross-module changes are easier to review.
Independent versions and Go subdirectory tags still require deliberate release
coordination. Incubating modules can follow these contracts before moving; a later
module-path change remains a real consumer migration.

Rules are supported by semantic review and meaningful tests. Mechanical checks
alone do not prove compatibility, asset rights, native acceptance, or completed
consumer delivery. A future repository split or tile library requires a new decision.
