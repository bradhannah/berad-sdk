# Berad RNG

An explicitly seeded, reproducible PCG stream with exact source-draw accounting
and logarithmic-time restore. Package `rng` has one concrete `Source` and only
`New`, `Restore`, `Reset`, `Seed`, `Calls`, `Intn`, and `Shuffle` operations.

Canonical import:

```go
import "github.com/bradhannah/berad-sdk/modules/berad-rng"
```

## Availability and requirements

This is an experimental v0 API. Pin an actually published, reviewed module
revision; a directory in a checkout does not establish remote availability.
The minimum Go version is **1.27.0**; the selected development toolchain is
**1.27.1**. No engine, native graphics/audio device, game installation, or media
is required. The implementation preserves `golang.org/x/exp/rand` at
`v0.0.0-20251023183803-a4bb9ffd2546`, including its bounded-draw and shuffle order.
It intentionally does not migrate to `math/rand/v2`, whose stream would differ.

## Use

```go
source := rng.New(8675309) // Explicit seed; zero is also meaningful.
roll := source.Intn(1000) // 604; range is [0, 1000).
seed, calls := source.Seed(), source.Calls()

// Store these two uint64 values in the host's own checkpoint format.
resumed := rng.Restore(seed, calls)
next := resumed.Intn(1000)
_, _ = roll, next
```

The [executable example](example_test.go) and independent
[synthetic deck consumer](testdata/consumer/consumer_test.go) demonstrate the
public API. The SDK checker copies the consumer and module into an isolated
layout; its fixture-only `../library` substitution is not an installation pin.

## Ownership and guarantees

- Hosts own seeds, operation order, serialization, and any gameplay policies.
  There is no global RNG, implicit seed, clock, save schema, or hidden draw.
- Retain `*Source`, rather than copying its value. `Reset` preserves that pointer
  so existing borrowers see the restarted stream. Use `New`/`Restore` or initialize
  a zero value with `Reset` before drawing. No shutdown or close is needed.
- Hosts serialize all access, including observations. There is no internal lock.
  `Shuffle` invokes swaps synchronously; callbacks must not reenter the source.
- `Calls` counts **underlying 64-bit source draws**, including rejected samples.
  It is not the number of API calls. Seed and count observations consume nothing.
- `Restore` needs at most 64 jump iterations and constant space for a uint64
  count. `Intn` uses rejection sampling; `Shuffle` makes `n-1` swaps for `n > 1`
  and does not retain the collection. Hosts choose collection/work limits.
- Invalid `Intn` bounds and negative shuffle sizes panic before consuming any
  draw. Callback failures propagate with already-consumed draws/swaps intact.
- This is deterministic simulation randomness, not cryptographic randomness.

See [API and compatibility contracts](docs/API.md) for boundary behavior,
counter overflow, dependency exposure, and supported persistence semantics.

## Validation

With provisioned dependencies, from this module:

```sh
GOWORK=off GOPROXY=off GOSUMDB=off go test -mod=readonly ./...
GOWORK=off GOPROXY=off GOSUMDB=off go test -mod=readonly -race ./...
GOWORK=off GOPROXY=off GOSUMDB=off go vet -mod=readonly ./...
GOWORK=off GOPROXY=off GOSUMDB=off go build -mod=readonly ./...
```

From an SDK checkout, `make lint-fix MODULE=modules/berad-rng` starts the scoped
autofix/review pass. `make rng-check` (or `make quality-gate MODULE=modules/berad-rng`) covers strict
format/lint, unit/race, and isolated-consumer checks under the SDK's registered
validation policy. Tests include fixed PCG outputs from the published dependency,
multi-seed iterative restoration, mixed operations and rejection draw counts,
pointer-stable reset, invalid bounds, callback panic behavior, and huge jumps.
These CPU-only checks make no native-device or untested-platform claim.

## License and documentation

Project source/docs are [MIT licensed](LICENSE), including preserved transferred
source attribution. Dependencies retain their [third-party terms](THIRD_PARTY_NOTICES.md).
The [module documentation catalog](docs/README.md) owns maintained RNG contracts.
