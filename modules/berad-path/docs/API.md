# Path API and search contract

Package `pathfind` is the experimental reusable route search. The host owns its
maps, what blocks, step prices, occupancy, movement rules and what to do when no
route exists. The API is one concrete, engine-independent function; there is no
pluggable graph framework.

## Public surface and exposure

| Surface | Consumer need | Ownership / compatibility |
| --- | --- | --- |
| `Find(Query) (Result, error)` | Cheapest route between two cells | Pure function; no retained state |
| `Query` | One search: start, goal, bounds, moves, step cost, minimum step, visit limit | Caller-built value; read during the call only |
| `Moves`, `Four`, `Eight` | Orthogonal-only or orthogonal and diagonal steps | Other values are rejected |
| `StepCost` | Host rules for each step's price and whether it is allowed | Called synchronously, never retained |
| `Result`, `Outcome`, `Found`, `NoRoute`, `Limited` | Route, its cost, work done, and why the search ended | Detached value; the caller owns `Route` |
| `ErrQuery`, `ErrStepCost` | Classify invalid queries and step costs below the minimum | Wrapped with the offending values |

`image.Point` and `image.Rectangle` from Go's standard library cross the API as
plain values. No other types are exposed.

Deliberately absent: map wrapping, cross-floor connections, arbitrary graphs,
per-unit terrain tables, path smoothing, asynchronous jobs and route caching.
The original implementation's wrapping served one game's world map; it returns
when a consumer needs it, as an additive option.

## Operation semantics

`Find` validates the query, then searches. It returns an error wrapping
`ErrQuery` when the step cost is nil, `Moves` is neither `Four` nor `Eight`,
`Bounds` is empty or wider than an `int` can measure, `Start` or `Goal` lies
outside `Bounds`, `MinStep` or `MaxVisited` is negative, or a route's total cost
would overflow `int`. It returns an error wrapping `ErrStepCost` when an allowed
step costs less than `MinStep`; the search stops there. `Result` is zero with an
error.

Without an error, `Outcome` is one of:

- `Found`: `Route` lists the cells from `Start` to `Goal`, both included, each
  one step from the last; `Cost` is the sum of its step costs and no allowed
  route inside `Bounds` costs less. `Start == Goal` gives a one-cell route
  costing zero.
- `NoRoute`: every cell reachable from `Start` inside `Bounds` was expanded.
- `Limited`: `MaxVisited` cells were expanded first. A route may exist.

`Visited` counts expanded cells, never more than the visit limit; reaching the
goal does not expand it. `Route` is nil and `Cost` zero unless `Found`.

Steps are tried in a fixed order: north, east, south, west, then north-east,
south-east, south-west and north-west. Cells wait in a priority queue ordered by
estimated total cost, then by the smaller remaining estimate, then by the order
they were queued. Equal queries whose step costs agree therefore return the same
route, including the choice between routes of equal cost; which of several
equal-cost routes that is, beyond being repeatable, is not part of the contract.

The estimate is the least number of steps to the goal (Manhattan distance with
`Four`, the larger axis distance with `Eight`) times `MinStep`, whose zero value
means 1. It never overestimates while every allowed step costs at least
`MinStep`, which `Find` enforces, so routes are cheapest. A truer, larger
`MinStep` only makes the search visit fewer cells.

`StepCost` receives two adjacent cells, both inside `Bounds`, and returns a cost
and whether the step is allowed. It runs synchronously on the caller's goroutine
and may call `Find` again. It is never called for a cell already expanded. A
panic in it propagates and ends the search.

## Limits and compatibility

A search allocates per cell it queues, not per cell of `Bounds`, so large or
mostly empty bounds cost nothing until searched. `MaxVisited` caps the cells
expanded; its zero value means the number of cells in `Bounds`, saturating at
`math.MaxInt`. Hosts with large floors or many searches per tick should choose a
limit. The queue may hold duplicate entries for a cell whose cost improved;
stale ones are skipped.

The v0 API may change between revisions with explicit notes. Additions such as
wrapping should arrive as new optional fields whose zero values keep today's
behaviour.

## Validation scope

External tests cover the original implementation's route-around and no-route
cases, cheapest cost and route validity against an independent Dijkstra search
on 300 seeded random grids with weights, walls and negative origins in both step
sets, a straight diagonal across negative coordinates, a step cost that charges
diagonals more and refuses squeezes, repeatable equal-cost choices, searches
seeing changed costs, the visit limit, bounds confining the step cost's calls,
start equal to goal, every invalid query, and a step below the minimum. The
separate consumer module plans a synthetic rover across host-priced terrain
using only public imports.
