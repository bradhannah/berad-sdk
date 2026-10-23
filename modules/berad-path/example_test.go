package pathfind_test

import (
	"fmt"
	"image"

	pathfind "github.com/bradhannah/berad-sdk/modules/berad-path"
)

func ExampleFind() {
	// A wall down column 0 from the top edge, open only at the bottom row.
	// Cells may be negative.
	wall := map[image.Point]bool{{X: 0, Y: -2}: true, {X: 0, Y: -1}: true, {X: 0, Y: 0}: true, {X: 0, Y: 1}: true}
	result, err := pathfind.Find(pathfind.Query{
		Start: image.Pt(-1, 0), Goal: image.Pt(1, 0), Bounds: image.Rect(-2, -2, 3, 3),
		Moves: pathfind.Four, MinStep: 1, MaxVisited: 0,
		Cost: func(_, to image.Point) (int, bool) { return 1, !wall[to] },
	})
	if err != nil {
		fmt.Println(err)

		return
	}
	fmt.Println(result.Outcome == pathfind.Found, result.Cost, result.Route)
	// Output:
	// true 6 [(-1,0) (-1,1) (-1,2) (0,2) (1,2) (1,1) (1,0)]
}
