# Berad Path

Cheapest-route search on a grid with A*. Package `pathfind` has one function,
`Find`, which takes a `Query` and returns a `Result`. The caller supplies the
cells a search may visit, four or eight directions, and a step cost that also
decides whether each step is allowed, so terrain, occupancy and movement rules
stay with the game. The first consumer is berad-engine, whose NPCs walk tile
floors; the search began as Ultima V Redux's A*.

Canonical import:

```go
import pathfind "github.com/bradhannah/berad-sdk/modules/berad-path"
```

The package is named `pathfind` rather than `path`, so it does not shadow the
standard library's `path`.

## Availability and requirements

This is an experimental v0 API. Pin an actually published, reviewed module
revision; a directory in a checkout does not establish remote availability.
The minimum Go version is **1.27.0**; the selected development toolchain is
**1.27.1**. There are no dependencies beyond the standard library, and no engine,
native device, game installation or media is required.

## Use

```go
wall := map[image.Point]bool{{X: 0, Y: -2}: true, {X: 0, Y: -1}: true, {X: 0, Y: 0}: true, {X: 0, Y: 1}: true}
result, err := pathfind.Find(pathfind.Query{
	Start: image.Pt(-1, 0), Goal: image.Pt(1, 0), Bounds: image.Rect(-2, -2, 3, 3),
	Moves: pathfind.Four, MinStep: 1, MaxVisited: 0,
	Cost: func(_, to image.Point) (int, bool) { return 1, !wall[to] },
})
if err != nil {
	return err // An invalid query, or a step cheaper than MinStep.
}
if result.Outcome == pathfind.Found {
	walk(result.Route) // Start to goal, both included; cost 6 here.
}
```

The [executable example](example_test.go) and the independent
[synthetic rover consumer](testdata/consumer/consumer_test.go) use only the
public API. The SDK checker copies the consumer and module into an isolated
layout; its fixture-only `../library` substitution is not an installation pin.

## Ownership and guarantees

- The host owns the map, what blocks, each step's price, and what an NPC does
  when no route exists. `Find` keeps no state, cache or global configuration.
- Cells are `image.Point` values in any coordinate range, negative included;
  the query's `Bounds` limit them. Steps are one cell: north, east, south and
  west, and with `Eight` also the diagonals. A diagonal rule, such as refusing
  to squeeze between two blocked cells, belongs in the step cost. So does map
  wrapping: search a window in unwrapped coordinates, past the map's edge if
  needed, and wrap each cell before reading the map.
- `MinStep` is a lower bound on every allowed step's cost (zero means 1). The
  distance estimate is the least number of steps times `MinStep`, so routes are
  always cheapest; a step cheaper than `MinStep` is an error, not a wrong route.
- No route is an outcome (`NoRoute`), as is giving up at `MaxVisited`
  (`Limited`), whose zero value means the number of cells in `Bounds`. Errors
  wrap `ErrQuery` or `ErrStepCost`.
- Equal queries whose step costs agree return the same route, including the
  choice between routes of equal cost.
- `Find` runs synchronously on the caller's goroutine and calls the step cost
  only for cells inside `Bounds`. Concurrent searches are safe when their step
  costs are. A panic in a step cost propagates.
- Work and memory grow with the cells expanded, at most `MaxVisited`; nothing
  is allocated for cells the search never reaches.

See the [API contract](docs/API.md) for exact outcomes, limits and what was
left out of the original implementation.

## Validation

With provisioned dependencies, from this module:

```sh
GOWORK=off GOPROXY=off GOSUMDB=off go test -mod=readonly ./...
GOWORK=off GOPROXY=off GOSUMDB=off go test -mod=readonly -race ./...
GOWORK=off GOPROXY=off GOSUMDB=off go vet -mod=readonly ./...
```

From an SDK checkout, `make path-check` (or
`make quality-gate MODULE=modules/berad-path`) covers strict format and lint,
unit and race tests, and the isolated consumer. Tests include the original
implementation's route-around and no-route cases, agreement with a plain
Dijkstra reference on 300 random weighted grids in four and eight directions,
negative coordinates, a host-wrapped window crossing a map's edge, a step cost
carrying diagonal rules, repeatable ties,
bounds, the visit limit, and invalid queries. These are CPU-only checks.

## License and documentation

Project source and docs are [MIT licensed](LICENSE), keeping the 2024 notice of
the Ultima V Redux code the search came from. There are no third-party
dependencies or fixtures. The [module documentation catalog](docs/README.md)
owns the maintained contract.
