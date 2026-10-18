# MODULE_DISPLAY_NAME

Describe the reusable service and concrete intended consumers.

## Availability and installation

State experimental/stable status and an actually resolvable module version/path.
Separate minimum Go/engine requirements from tested toolchain/platforms.

## Minimal use

Provide a complete, compiling public-API example covering construction, meaningful
work, error handling, and cleanup. Name who owns supplied assets/backend/targets.

## Supported packages and responsibilities

Explain supported entry points, engine adapter exposure, optional authoring/tools,
and consumer-owned policy. Link substantial contracts instead of duplicating them.

## Contracts

- Inputs, defaults, explicit zero/absence, units and valid ranges.
- Owned/borrowed state, snapshot/copy rules, and independent project identity.
- Update/execution lane, concurrent calls, blocking and reentrancy.
- Closure, stale/foreign handles, retained results and asynchronous errors.
- Resource limits, cache accounting and transient work.
- Capabilities/limitations and documented exceptions.

## Validation and compatibility

Document exact module and consumer commands, check selection/reuse, and distinct
native/race observations. Link version/migration notes and maintained examples.

## License and fixtures

Include applicable MIT/transferred notices and third-party fixture/asset terms.

Template guidance: [SDK conventions](../SDK_CONVENTIONS.md).
