# API and ownership design

## SDK-API-001 — Minimum useful public surface

Expose only what supported consumers require. Every public package and significant
capability MUST have a concrete consumer rationale. Prefer public concrete service
types with private fields and detached value results. Use small interfaces for
dependencies supplied by callers or genuine substitution boundaries.

Adding a required method to a consumer-implemented interface breaks existing
implementers. Avoid large universal service interfaces and blanket aliases of an
existing game subsystem. Consumer-specific interfaces can remain in the consumer.
Optional capabilities and explicit configuration MAY support compatible growth.

## SDK-API-002 — Deliberate Ebitengine exposure

This is an Ebitengine-first SDK family. Use the simplest useful integration.
Direct third-party types MAY cross a public boundary when their supported behavior,
ownership, mutation, and version coupling are explicitly reviewed.

For example, a caller-owned `*ebiten.Image` drawing target can be an appropriate
boundary. Exposing mutable font faces or cached images requires a separate decision
because mutation/disposal can violate retained layout and cache identity.

Each module MUST maintain a small exposure matrix: surface/type, consumer benefit,
owned/borrowed status, allowed mutation, lifetime, supported engine versions, and
compatibility cost. `backend/ebitengine` is the shared adapter convention where
package separation is useful. Audio's engine-free core is a useful precedent,
not a requirement to invent an equivalent backend abstraction for every service.
Prefer headless data/algorithm boundaries where they represent useful real seams.

## SDK-OWN-001 — Resource and data ownership

Each resource-bearing API MUST define creation, mutation, retention, transfer,
borrowing, and shutdown. Freeze authoritative input or document exclusive ownership;
return detached snapshots when exposing diagnostic/state values. Library identity
must keep independently loaded projects and same-named assets separate.

Specify zero/stale/foreign handle outcomes, idempotent versus terminal operations,
and whether retained results survive closure. Cached resources, user-retained
results, and caller-owned destinations have distinct lifetimes. Do not dispose a
borrowed host device or destination. Escaping cache images need an explicit lease
or ownership contract before export.

## SDK-CFG-001 — Defaults, units, and failure

Public contracts MUST distinguish missing/default values from meaningful zero:
mute, transparent paint, an empty collection, and zero duration can be deliberate.
Use named configuration and explicit constructors/default helpers where useful.
Validate external inputs, non-finite numbers, dimensions, and nonprogressing work.

Specify units, ranges, coordinate spaces, transforms, and rounding. Return
contextual/wrapped errors for recoverable input, resource, configuration, and I/O
failures. Preserve causes and distinguish routine outcomes from errors. Document
programming invariants separately. SDK code must not terminate the application or
require its logger. Asynchronous failures need bounded, observable delivery.

## SDK-RUN-001 — Execution and observation

Document which calls can run concurrently, which may block, required execution
lanes, reentrancy restrictions, and who services updates. Choose domain-appropriate
concurrency; do not impose audio's mutex/device model on graphical text.

Explicitly distinguish presentation time, device time, and authoritative game
time. Accept a clock/elapsed time or RNG when the service needs it and repeatability
matters. Cosmetic SDK work must not advance game state or consume its RNG implicitly.
Diagnostics MUST be observational: no playback servicing, content edits, input
consumption, or hidden game actions while inspecting state.

## SDK-LIMIT-001 — Bounded work and truthful accounting

Specify limits for input size, preparation/work, retained caches, active resources,
and old versions where applicable. Reject unsupported work before excessive
allocation. Cache byte accounting is not a promise about total heap/GPU/device
memory. Measure representative workloads before changing performance architecture.

## SDK-DOM-001 — Mechanisms and consumer policy

The host owns semantic meaning, assets, game rules, settings persistence, scene
orchestration, input/focus, and world interpretation. SDKs own their documented
reusable service. Text can return geometry and content identities while a host
control owns interaction. Audio can reconcile sources while the host discovers
them. Graphics boundaries require concrete use cases; future tile abstractions
are not a prerequisite for today's APIs.
