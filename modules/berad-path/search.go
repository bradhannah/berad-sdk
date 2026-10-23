package pathfind

import (
	"container/heap"
	"errors"
	"fmt"
	"image"
	"math"
)

// ErrQuery reports a query that cannot be searched.
var ErrQuery = errors.New("invalid path query")

// ErrStepCost reports a step cost below the query's MinStep.
var ErrStepCost = errors.New("step cost below the query's minimum")

// Moves is the set of steps a search may take from a cell.
type Moves int

// Step sets.
const (
	// Four allows the orthogonal steps: north, east, south and west.
	Four Moves = 4
	// Eight also allows the four diagonal steps.
	Eight Moves = 8
)

// StepCost returns the cost of stepping from a cell to an adjacent one, and
// false when that step is not allowed. A search calls it only for cells
// inside the query's bounds, synchronously, on the caller's goroutine.
type StepCost func(from, to image.Point) (int, bool)

// Query is one search.
type Query struct {
	// Start and Goal are cells inside Bounds. Coordinates may be negative.
	Start, Goal image.Point
	// Bounds are the cells the search may visit.
	Bounds image.Rectangle
	// Moves is Four or Eight.
	Moves Moves
	// Cost prices each step and decides whether it is allowed.
	Cost StepCost
	// MinStep is a lower bound on the cost of every allowed step; zero means
	// 1. It scales the distance estimate, so a higher true bound makes a
	// search visit fewer cells. A step cheaper than MinStep is an error.
	MinStep int
	// MaxVisited bounds the work: the search expands at most this many cells
	// before giving up. Zero means the number of cells in Bounds.
	MaxVisited int
}

// Outcome is how a search ended.
type Outcome int

// Outcomes of a search.
const (
	// Found means Route holds a cheapest route.
	Found Outcome = iota + 1
	// NoRoute means every cell reachable from Start was expanded without
	// reaching Goal.
	NoRoute
	// Limited means the search expanded MaxVisited cells without reaching
	// Goal; a route may still exist.
	Limited
)

// Result is the outcome of a search.
type Result struct {
	Outcome Outcome
	// Route lists the cells from Start to Goal, both included, when Found;
	// it is nil otherwise. The caller owns it.
	Route []image.Point
	// Cost is the sum of the route's step costs, or zero without a route.
	Cost int
	// Visited counts the cells the search expanded, never more than the
	// query's visit limit. Reaching the goal does not expand it.
	Visited int
}

// Find searches for a cheapest route. It returns an error only for a query
// that cannot be searched or a step cost below MinStep; finding no route is
// an outcome, not an error. Among routes of equal cost it returns the same
// one every time.
func Find(query Query) (Result, error) {
	minStep, maxVisited, err := check(query)
	if err != nil {
		return Result{Outcome: 0, Route: nil, Cost: 0, Visited: 0}, err
	}
	search := &search{
		query:   query,
		minStep: minStep,
		open:    &queue{},
		best:    map[image.Point]int{query.Start: 0},
		parent:  map[image.Point]image.Point{},
		closed:  map[image.Point]bool{},
		pushed:  0,
	}
	search.push(query.Start, 0)

	return search.run(maxVisited)
}

// steps lists the orthogonal steps, then the diagonal ones, in the fixed
// order that keeps ties repeatable.
func steps() []image.Point {
	return []image.Point{
		{X: 0, Y: -1}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: -1, Y: 0},
		{X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}, {X: -1, Y: -1},
	}
}

// check validates a query and resolves its defaults.
func check(query Query) (int, int, error) {
	switch {
	case query.Cost == nil:
		return 0, 0, fmt.Errorf("%w: no step cost", ErrQuery)
	case query.Moves != Four && query.Moves != Eight:
		return 0, 0, fmt.Errorf("%w: moves must be Four or Eight, not %d", ErrQuery, query.Moves)
	case query.Bounds.Empty() || !fits(query.Bounds):
		return 0, 0, fmt.Errorf("%w: bounds %v are empty or too large", ErrQuery, query.Bounds)
	case !query.Start.In(query.Bounds) || !query.Goal.In(query.Bounds):
		return 0, 0, fmt.Errorf("%w: start %v and goal %v must be inside %v", ErrQuery, query.Start, query.Goal,
			query.Bounds)
	case query.MinStep < 0 || query.MaxVisited < 0:
		return 0, 0, fmt.Errorf("%w: negative minimum step or visit limit", ErrQuery)
	}
	minStep := max(query.MinStep, 1)
	maxVisited := query.MaxVisited
	if maxVisited == 0 {
		maxVisited = multiply(query.Bounds.Dx(), query.Bounds.Dy())
	}

	return minStep, maxVisited, nil
}

// fits reports whether a rectangle's width and height are representable.
func fits(bounds image.Rectangle) bool {
	return (bounds.Min.X >= 0 || bounds.Max.X <= math.MaxInt+bounds.Min.X) &&
		(bounds.Min.Y >= 0 || bounds.Max.Y <= math.MaxInt+bounds.Min.Y)
}

// multiply multiplies non-negative numbers, saturating at math.MaxInt.
func multiply(a, b int) int {
	if a != 0 && b > math.MaxInt/a {
		return math.MaxInt
	}

	return a * b
}

type search struct {
	query   Query
	minStep int
	open    *queue
	best    map[image.Point]int
	parent  map[image.Point]image.Point
	closed  map[image.Point]bool
	pushed  int
}

func (s *search) run(maxVisited int) (Result, error) {
	visited := 0
	for s.open.Len() > 0 {
		current, ok := heap.Pop(s.open).(entry)
		if !ok {
			panic("pathfind: queue holds a value that is not an entry")
		}
		if s.closed[current.cell] {
			continue
		}
		if current.cell == s.query.Goal {
			return Result{Outcome: Found, Route: s.route(), Cost: current.cost, Visited: visited}, nil
		}
		if visited == maxVisited {
			return Result{Outcome: Limited, Route: nil, Cost: 0, Visited: visited}, nil
		}
		s.closed[current.cell] = true
		visited++
		if err := s.expand(current); err != nil {
			return Result{Outcome: 0, Route: nil, Cost: 0, Visited: visited}, err
		}
	}

	return Result{Outcome: NoRoute, Route: nil, Cost: 0, Visited: visited}, nil
}

// expand offers each allowed step from a cell.
func (s *search) expand(current entry) error {
	for _, step := range steps()[:s.query.Moves] {
		next := current.cell.Add(step)
		if !next.In(s.query.Bounds) || s.closed[next] {
			continue
		}
		cost, allowed := s.query.Cost(current.cell, next)
		if !allowed {
			continue
		}
		if cost < s.minStep {
			return fmt.Errorf("%w: %v to %v costs %d, below %d", ErrStepCost, current.cell, next, cost, s.minStep)
		}
		if cost > math.MaxInt-current.cost {
			return fmt.Errorf("%w: the route cost to %v overflows", ErrQuery, next)
		}
		total := current.cost + cost
		if known, seen := s.best[next]; seen && known <= total {
			continue
		}
		s.best[next] = total
		s.parent[next] = current.cell
		s.push(next, total)
	}

	return nil
}

func (s *search) push(cell image.Point, cost int) {
	estimate := s.estimate(cell)
	priority := math.MaxInt
	if estimate <= math.MaxInt-cost {
		priority = cost + estimate
	}
	heap.Push(s.open, entry{cell: cell, cost: cost, estimate: estimate, priority: priority, order: s.pushed})
	s.pushed++
}

// estimate is a lower bound on the cost from a cell to the goal: the least
// number of steps times MinStep.
func (s *search) estimate(cell image.Point) int {
	dx, dy := distance(cell.X, s.query.Goal.X), distance(cell.Y, s.query.Goal.Y)
	steps := max(dx, dy)
	if s.query.Moves == Four {
		steps = dx + dy
		if dx > math.MaxInt-dy {
			steps = math.MaxInt
		}
	}

	return multiply(steps, s.minStep)
}

// distance is |a-b|; both lie inside bounds that fits accepted.
func distance(a, b int) int {
	if a > b {
		return a - b
	}

	return b - a
}

// route walks back from the goal and returns the cells in order.
func (s *search) route() []image.Point {
	route := []image.Point{s.query.Goal}
	for cell := s.query.Goal; cell != s.query.Start; {
		cell = s.parent[cell]
		route = append(route, cell)
	}
	for left, right := 0, len(route)-1; left < right; left, right = left+1, right-1 {
		route[left], route[right] = route[right], route[left]
	}

	return route
}

// entry is a cell waiting to be expanded.
type entry struct {
	cell     image.Point
	cost     int
	estimate int
	priority int
	order    int
}

// queue orders entries by priority, then by the smaller estimate, so ties go
// towards the goal, then by the order they were pushed.
type queue []entry

func (q *queue) Len() int { return len(*q) }

func (q *queue) Less(i, j int) bool {
	first, second := (*q)[i], (*q)[j]
	if first.priority != second.priority {
		return first.priority < second.priority
	}
	if first.estimate != second.estimate {
		return first.estimate < second.estimate
	}

	return first.order < second.order
}

func (q *queue) Swap(i, j int) { (*q)[i], (*q)[j] = (*q)[j], (*q)[i] }

func (q *queue) Push(value any) {
	pushed, ok := value.(entry)
	if !ok {
		panic("pathfind: queue accepts only entries")
	}
	*q = append(*q, pushed)
}

func (q *queue) Pop() any {
	old := *q
	last := old[len(old)-1]
	*q = old[:len(old)-1]

	return last
}
