package pathfind_test

import (
	"container/heap"
	"errors"
	"image"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	pathfind "github.com/bradhannah/berad-sdk/modules/berad-path"
)

// grid is a test map: '#' blocks, '.' costs 1 and a digit costs its value.
// Its top-left character is the cell origin.
type grid struct {
	origin image.Point
	rows   []string
}

func newGrid(origin image.Point, rows ...string) grid {
	return grid{origin: origin, rows: rows}
}

func (g grid) bounds() image.Rectangle {
	return image.Rect(0, 0, len(g.rows[0]), len(g.rows)).Add(g.origin)
}

func (g grid) at(cell image.Point) byte {
	local := cell.Sub(g.origin)

	return g.rows[local.Y][local.X]
}

// cost charges the destination cell's price for any step onto it.
func (g grid) cost(_, to image.Point) (int, bool) {
	switch mark := g.at(to); mark {
	case '#':
		return 0, false
	case '.':
		return 1, true
	default:
		return int(mark - '0'), true
	}
}

func (g grid) query(start, goal image.Point, moves pathfind.Moves) pathfind.Query {
	return pathfind.Query{
		Start: start, Goal: goal, Bounds: g.bounds(), Moves: moves, Cost: g.cost, MinStep: 0, MaxVisited: 0,
	}
}

func find(t *testing.T, query pathfind.Query) pathfind.Result {
	t.Helper()
	result, err := pathfind.Find(query)
	if err != nil {
		t.Fatal(err)
	}

	return result
}

// The route-around test from the original implementation's suite.
func TestRouteAroundBlockedCells(t *testing.T) {
	t.Parallel()
	g := newGrid(image.Pt(0, 0), ".#.", ".#.", "...")
	result := find(t, g.query(image.Pt(0, 0), image.Pt(2, 0), pathfind.Four))
	if result.Outcome != pathfind.Found || len(result.Route) != 7 || result.Cost != 6 {
		t.Fatalf("result %+v", result)
	}
	if result.Route[0] != image.Pt(0, 0) || result.Route[6] != image.Pt(2, 0) {
		t.Fatalf("route must include both ends: %v", result.Route)
	}
	for index, cell := range result.Route {
		if g.at(cell) == '#' {
			t.Fatalf("route crosses a blocked cell: %v", result.Route)
		}
		if index > 0 && manhattan(cell, result.Route[index-1]) != 1 {
			t.Fatalf("route jumps: %v", result.Route)
		}
	}
}

// The no-route test from the original implementation's suite.
func TestNoRouteIsAnOutcome(t *testing.T) {
	t.Parallel()
	g := newGrid(image.Pt(0, 0), ".#.", ".#.", ".#.")
	result := find(t, g.query(image.Pt(0, 0), image.Pt(2, 0), pathfind.Eight))
	if result.Outcome != pathfind.NoRoute || result.Route != nil || result.Visited != 3 {
		t.Fatalf("result %+v", result)
	}
}

func TestNegativeCoordinates(t *testing.T) {
	t.Parallel()
	g := newGrid(image.Pt(-5, -5), strings.Split(strings.Repeat("..........,", 10), ",")[:10]...)
	result := find(t, g.query(image.Pt(-4, -4), image.Pt(4, 4), pathfind.Eight))
	if result.Outcome != pathfind.Found || result.Cost != 8 || len(result.Route) != 9 {
		t.Fatalf("result %+v", result)
	}
	for index, cell := range result.Route {
		if cell != image.Pt(index-4, index-4) {
			t.Fatalf("an open grid's diagonal is straight: %v", result.Route)
		}
	}
}

// A host whose map wraps searches a window in unwrapped coordinates and wraps
// each cell in its step cost, so a route can cross the map's edge.
func TestHostWrapsAWindowAcrossTheEdge(t *testing.T) {
	t.Parallel()
	const width = 8
	world := newGrid(image.Pt(0, 0), "...#....", "...#....", "...#....")
	wrap := func(cell image.Point) image.Point { return image.Pt((cell.X%width+width)%width, cell.Y) }
	query := pathfind.Query{
		Start: image.Pt(1, 1), Goal: image.Pt(6-width, 1), Bounds: image.Rect(-3, 0, 5, 3), Moves: pathfind.Four,
		Cost:    func(from, to image.Point) (int, bool) { return world.cost(wrap(from), wrap(to)) },
		MinStep: 0, MaxVisited: 0,
	}
	across := []image.Point{image.Pt(1, 1), image.Pt(0, 1), image.Pt(-1, 1), image.Pt(-2, 1)}
	if result := find(t, query); result.Outcome != pathfind.Found || !slices.Equal(result.Route, across) {
		t.Fatalf("the wall leaves only the way across the edge: %+v", result)
	}
	inside := world.query(image.Pt(1, 1), image.Pt(6, 1), pathfind.Four)
	if result := find(t, inside); result.Outcome != pathfind.NoRoute {
		t.Fatalf("without wrapping the wall blocks every route: %+v", result)
	}
}

// A caller's step cost carries rules such as charging more for diagonals and
// refusing to squeeze between two blocked cells.
func TestStepCostCarriesTheRules(t *testing.T) {
	t.Parallel()
	g := newGrid(image.Pt(0, 0),
		"...",
		".#.",
		"#..",
	)
	noSqueeze := func(from, to image.Point) (int, bool) {
		if _, open := g.cost(from, to); !open {
			return 0, false
		}
		delta := to.Sub(from)
		if delta.X == 0 || delta.Y == 0 {
			return 10, true
		}
		_, sideA := g.cost(from, image.Pt(to.X, from.Y))
		_, sideB := g.cost(from, image.Pt(from.X, to.Y))
		if !sideA && !sideB {
			return 0, false
		}

		return 14, true
	}
	query := g.query(image.Pt(0, 1), image.Pt(1, 2), pathfind.Eight)
	query.Cost, query.MinStep = noSqueeze, 10
	result := find(t, query)
	want := []image.Point{{X: 0, Y: 1}, {X: 1, Y: 0}, {X: 2, Y: 1}, {X: 1, Y: 2}}
	if result.Outcome != pathfind.Found || result.Cost != 42 || !slices.Equal(result.Route, want) {
		t.Fatalf("the squeeze from 0,1 to 1,2 is refused, so the route goes round: %+v", result)
	}
}

func TestCheapestRouteMatchesAReferenceSearch(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewPCG(20261023, 7)) //nolint:gosec // Repeatable test grids, not security.
	for trial := range 300 {
		g := randomGrid(random)
		start, goal := randomOpenCell(random, g), randomOpenCell(random, g)
		for _, moves := range []pathfind.Moves{pathfind.Four, pathfind.Eight} {
			query := g.query(start, goal, moves)
			result := find(t, query)
			want, reachable := reference(query)
			switch {
			case reachable && (result.Outcome != pathfind.Found || result.Cost != want):
				t.Fatalf("trial %d, %d moves: cost %d (%v), reference %d\n%s", trial, moves, result.Cost,
					result.Outcome, want, strings.Join(g.rows, "\n"))
			case !reachable && result.Outcome != pathfind.NoRoute:
				t.Fatalf("trial %d: the reference finds no route, the search says %v", trial, result.Outcome)
			case reachable && routeCost(t, g, result.Route) != result.Cost:
				t.Fatalf("trial %d: the route does not cost what the result says", trial)
			}
		}
	}
}

func TestTiesAreRepeatable(t *testing.T) {
	t.Parallel()
	g := newGrid(image.Pt(3, -2), "......", "......", "......", "......")
	first := find(t, g.query(image.Pt(3, -2), image.Pt(8, 1), pathfind.Four))
	second := find(t, g.query(image.Pt(3, -2), image.Pt(8, 1), pathfind.Four))
	if !slices.Equal(first.Route, second.Route) || first.Visited != second.Visited {
		t.Fatalf("equal queries chose different routes: %v and %v", first.Route, second.Route)
	}
}

func TestQueriesSeeCurrentCosts(t *testing.T) {
	t.Parallel()
	open := newGrid(image.Pt(0, 0), "...", "...")
	walled := newGrid(image.Pt(0, 0), ".#.", "...")
	before := find(t, open.query(image.Pt(0, 0), image.Pt(2, 0), pathfind.Four))
	after := find(t, walled.query(image.Pt(0, 0), image.Pt(2, 0), pathfind.Four))
	if before.Cost != 2 || after.Cost != 4 {
		t.Fatalf("no state carries between searches: %d then %d", before.Cost, after.Cost)
	}
}

func TestVisitLimit(t *testing.T) {
	t.Parallel()
	g := newGrid(image.Pt(0, 0), strings.Split(strings.Repeat("....................,", 20), ",")[:20]...)
	query := g.query(image.Pt(0, 0), image.Pt(19, 19), pathfind.Four)
	query.MaxVisited = 5
	result := find(t, query)
	if result.Outcome != pathfind.Limited || result.Visited != 5 || result.Route != nil {
		t.Fatalf("result %+v", result)
	}
}

func TestBoundsLimitTheSearch(t *testing.T) {
	t.Parallel()
	g := newGrid(image.Pt(0, 0), ".#..", ".#..", "....")
	query := g.query(image.Pt(0, 0), image.Pt(2, 0), pathfind.Four)
	query.Bounds = image.Rect(0, 0, 4, 2)
	query.Cost = func(from, to image.Point) (int, bool) {
		if !to.In(query.Bounds) {
			t.Errorf("cost asked for %v, outside the bounds", to)
		}

		return g.cost(from, to)
	}
	if result := find(t, query); result.Outcome != pathfind.NoRoute {
		t.Fatalf("the only way round lies outside the bounds: %+v", result)
	}
}

func TestStartIsGoal(t *testing.T) {
	t.Parallel()
	g := newGrid(image.Pt(0, 0), "..")
	result := find(t, g.query(image.Pt(1, 0), image.Pt(1, 0), pathfind.Eight))
	if result.Outcome != pathfind.Found || !slices.Equal(result.Route, []image.Point{{X: 1, Y: 0}}) || result.Cost != 0 {
		t.Fatalf("result %+v", result)
	}
}

func TestInvalidQueries(t *testing.T) {
	t.Parallel()
	g := newGrid(image.Pt(0, 0), "...", "...")
	valid := g.query(image.Pt(0, 0), image.Pt(2, 1), pathfind.Four)
	for name, change := range map[string]func(*pathfind.Query){
		"no cost":           func(q *pathfind.Query) { q.Cost = nil },
		"moves":             func(q *pathfind.Query) { q.Moves = 6 },
		"empty bounds":      func(q *pathfind.Query) { q.Bounds = image.Rectangle{} },
		"start outside":     func(q *pathfind.Query) { q.Start = image.Pt(-1, 0) },
		"goal outside":      func(q *pathfind.Query) { q.Goal = image.Pt(3, 1) },
		"negative limit":    func(q *pathfind.Query) { q.MaxVisited = -1 },
		"negative min step": func(q *pathfind.Query) { q.MinStep = -1 },
	} {
		query := valid
		change(&query)
		if _, err := pathfind.Find(query); !errors.Is(err, pathfind.ErrQuery) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestStepCheaperThanTheMinimum(t *testing.T) {
	t.Parallel()
	g := newGrid(image.Pt(0, 0), "...")
	query := g.query(image.Pt(0, 0), image.Pt(2, 0), pathfind.Four)
	query.MinStep = 2
	if _, err := pathfind.Find(query); !errors.Is(err, pathfind.ErrStepCost) {
		t.Fatalf("a step costing 1 breaks a minimum of 2: %v", err)
	}
}

func manhattan(a, b image.Point) int {
	return abs(a.X-b.X) + abs(a.Y-b.Y)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}

	return value
}

func randomGrid(random *rand.Rand) grid {
	width, height := 2+random.IntN(9), 2+random.IntN(9)
	rows := make([]string, height)
	for row := range rows {
		var line strings.Builder
		for range width {
			switch roll := random.IntN(10); {
			case roll < 2:
				line.WriteByte('#')
			case roll < 5:
				line.WriteByte(byte('1' + random.IntN(9)))
			default:
				line.WriteByte('.')
			}
		}
		rows[row] = line.String()
	}

	return newGrid(image.Pt(random.IntN(7)-3, random.IntN(7)-3), rows...)
}

func randomOpenCell(random *rand.Rand, g grid) image.Point {
	bounds := g.bounds()
	for {
		cell := image.Pt(bounds.Min.X+random.IntN(bounds.Dx()), bounds.Min.Y+random.IntN(bounds.Dy()))
		if g.at(cell) != '#' {
			return cell
		}
	}
}

func routeCost(t *testing.T, g grid, route []image.Point) int {
	t.Helper()
	total := 0
	for index := 1; index < len(route); index++ {
		cost, allowed := g.cost(route[index-1], route[index])
		if !allowed {
			t.Fatalf("route takes a refused step: %v", route)
		}
		total += cost
	}

	return total
}

// reference is a plain Dijkstra search over the same steps and costs.
func reference(query pathfind.Query) (int, bool) {
	offsets := []image.Point{{0, -1}, {1, 0}, {0, 1}, {-1, 0}, {1, -1}, {1, 1}, {-1, 1}, {-1, -1}}
	best := map[image.Point]int{query.Start: 0}
	done := map[image.Point]bool{}
	frontier := &costQueue{{cell: query.Start, cost: 0}}
	for frontier.Len() > 0 {
		current, _ := heap.Pop(frontier).(costed) //nolint:errcheck,forcetypeassert // The queue holds costed values.
		if done[current.cell] {
			continue
		}
		if current.cell == query.Goal {
			return current.cost, true
		}
		done[current.cell] = true
		for _, offset := range offsets[:query.Moves] {
			next := current.cell.Add(offset)
			if !next.In(query.Bounds) {
				continue
			}
			cost, allowed := query.Cost(current.cell, next)
			if known, seen := best[next]; allowed && (!seen || current.cost+cost < known) {
				best[next] = current.cost + cost
				heap.Push(frontier, costed{cell: next, cost: current.cost + cost})
			}
		}
	}

	return 0, false
}

type costed struct {
	cell image.Point
	cost int
}

type costQueue []costed

func (q *costQueue) Len() int           { return len(*q) }
func (q *costQueue) Less(i, j int) bool { return (*q)[i].cost < (*q)[j].cost }
func (q *costQueue) Swap(i, j int)      { (*q)[i], (*q)[j] = (*q)[j], (*q)[i] }
func (q *costQueue) Push(value any)     { *q = append(*q, value.(costed)) } //nolint:forcetypeassert // Test queue.
func (q *costQueue) Pop() any {
	old := *q
	last := old[len(old)-1]
	*q = old[:len(old)-1]

	return last
}
