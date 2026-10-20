# RNG API and stream contract

Package `rng` is the experimental reusable stream mechanism. The host owns its
seed selection, shared-stream ordering, synchronization, persistence schema,
game rules, and any mapping to dice or inclusive ranges. The API is concrete and
engine-independent; there is no pluggable RNG framework.

## Public surface and exposure

| Surface | Consumer need | Ownership / compatibility |
| --- | --- | --- |
| `Source`, `New(seed uint64) *Source` | Explicit deterministic stream | Host owns the pointer; independent instances never share state |
| `Restore(seed, calls uint64) *Source` | Exact checkpoint continuation | New independent pointer, no replay of host actions |
| `(*Source).Reset(seed uint64)` | Reseed a stream already shared with consumers | Mutates in place; preserves pointer identity; clears count |
| `(*Source).Seed() uint64`, `Calls() uint64` | Host-defined checkpoint and diagnostics | Detached observations, no consumption or mutation |
| `(*Source).Intn(n int) int` | Unbiased bounded choices | Half-open integer range; exact draw consumption is contractual |
| `(*Source).Shuffle(n int, swap func(i, j int))` | Shuffle caller-owned items | Synchronous callback; collection is neither owned nor retained |

No third-party types cross this API. The implementation privately uses
`golang.org/x/exp/rand` at `v0.0.0-20251023183803-a4bb9ffd2546`. PCG initialization,
state encoding, bounded sampling, swap ordering, and rejection consumption are
compatibility-sensitive. Upgrading that dependency needs vector and continuation
review; a change to `math/rand/v2` is not a compatible substitution.

There is deliberately no `Float64`, raw `Uint64`, generic source injection,
global stream, clock, implicit seed, serialization service, or close operation.

## State and lifecycle

All uint64 seeds are accepted, including zero and the maximum. `New` starts with
zero calls. The zero `Source` value has not been initialized: construct it or call
`Reset` before use. Nil receivers are programming errors. Do not copy `Source`
values; copies would share internal pointers, whereas `New` and `Restore` provide
independent streams. Borrowers retain the host's original `*Source`; `Reset`
replaces its internal state while keeping that pointer stable.

Hosts serialize **all** access to a source. Independent sources may run on
independent goroutines; one shared source has no mutex or concurrent-use promise.
`Shuffle` runs synchronously on the caller's goroutine, retains neither callback
nor items, and invokes no asynchronous work. A swap callback must not reenter or
reset the same source. Release ordinary Go references when finished; no external
resource or shutdown sequence exists.

## Draws, bounds, and failure

`Intn(n)` returns in `[0,n)` with the dependency's rejection sampling. Positive
power-of-two bounds use one raw draw, including `Intn(1) == 0`. Other bounds may
consume extra rejected draws, all included in `Calls`. `n <= 0` panics with
`invalid argument to Intn` before any draw. There is no returned-error path.

`Shuffle` preserves the dependency's descending Fisher-Yates order, including
self-swaps. For `n > 1`, it invokes exactly `n-1` swaps, with the first index
descending from `n-1` to `1`; the second index lies in `[0,i]`. Sizes zero and one
make no draws and invoke no callback, so nil is valid for those sizes. Negative
sizes panic with `invalid argument to Shuffle` before any draw. A nil callback for
`n > 1` panics when invoked, after the first bounded draw. Callback panics propagate
unchanged; previous draws and completed swaps remain committed. There is no
transactional rollback.

`Seed` and `Calls` are observations only. Hosts must read/store the pair at a
serialized checkpoint so another operation cannot advance it between reads.

## Restoration, limits, and persistence

`Calls` is the number of calls to the underlying PCG's 64-bit source, not the
number of `Intn` or `Shuffle` invocations. `Restore(seed, calls)` computes the
128-bit linear-congruential state after exactly that many draws using affine
exponentiation. It takes constant memory and at most 64 iterations, rather than
replaying `calls` draws. The PCG emits its next value after advancing that state,
matching construction followed by iterative consumption. Restore does not count
the jump itself as additional draws.

The package defines no wire format, schema version, JSON tags, seed policy, or
wall-clock checkpointing. Hosts persist two lossless uint64 values and preserve
future operation order. The `uint64` counter wraps after `math.MaxUint64`, just
like an ordinary unsigned increment. Since the generator period is larger than
that count range, **a wrapped seed/count pair cannot restore the same stream**.
Capture/restore within the representable draw range; overflow behavior is retained
for compatibility, not advertised as an extended checkpoint format.

`Intn` has expected constant work but rejection sampling has no fixed iteration
cap. `Shuffle` requires linear callback work in the collection size, plus any
rejected samples. The source retains constant-size state and no collection cache.
The host controls sizes and callback cost. No cryptographic guarantee is made.

## Validation scope

Internal tests preserve fixed raw PCG vectors from the published dependency,
compare restored state with independent iterative dependency draws across seeds,
and check huge jumps against fixed outputs obtained from independently computed
closed-form states. Public tests cover fixed bounded draws/swap sequences, mixed
restoration with rejection consumption, reset/borrowing, invalid bounds, and
callback failure. Large-bound rejection fixtures exercise 64-bit `int`; narrower
platforms use their own valid bounds in the dependency comparison.

The separate consumer module uses only public imports and synthetic labels,
round-tripping host-owned JSON checkpoint data before continuing its shuffled
deck. It has no game imports. Game-specific provider and save-schema checks belong
with each game's adapter rather than duplicating the SDK algorithm tests.
