package consumer_test

import (
	"image"
	"testing"

	pathfind "github.com/bradhannah/berad-sdk/modules/berad-path"
)

// A host-owned terrain map: the host decides what blocks and what each step
// costs, including a rule against cutting between two rocks.
var terrain = []string{
	"~~~~~~~",
	"~.^^^.~",
	"~.^R^.~",
	"~.....~",
	"~~~~~~~",
}

func tile(cell image.Point) byte { return terrain[cell.Y+2][cell.X+3] }

func roverCost(from, to image.Point) (int, bool) {
	price := map[byte]int{'.': 10, '^': 30}
	cost, open := price[tile(to)]
	if !open {
		return 0, false
	}
	if from.X != to.X && from.Y != to.Y {
		_, sideA := price[tile(image.Pt(to.X, from.Y))]
		_, sideB := price[tile(image.Pt(from.X, to.Y))]
		if !sideA && !sideB {
			return 0, false
		}
		cost += cost / 2
	}

	return cost, true
}

func TestRoverAvoidsHillsAndWater(t *testing.T) {
	query := pathfind.Query{
		Start: image.Pt(-2, -1), Goal: image.Pt(2, -1), Bounds: image.Rect(-3, -2, 4, 3),
		Moves: pathfind.Eight, Cost: roverCost, MinStep: 10, MaxVisited: 0,
	}
	result, err := pathfind.Find(query)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != pathfind.Found || result.Cost != 70 {
		t.Fatalf("the cheapest way skirts the hills along the bottom: %+v", result)
	}
	for _, cell := range result.Route {
		if tile(cell) != '.' {
			t.Fatalf("route crosses %q at %v", tile(cell), cell)
		}
	}
	query.Goal = image.Pt(0, 0)
	if result, err := pathfind.Find(query); err != nil || result.Outcome != pathfind.NoRoute {
		t.Fatalf("the rock cannot be reached: %+v %v", result, err)
	}
}
