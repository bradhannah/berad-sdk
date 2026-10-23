// Package pathfind finds the cheapest route between two cells of a grid with
// A* search. The caller supplies the cells a search may visit, whether it
// steps in four or eight directions, and the cost of each step, which also
// decides whether a step is allowed. The package holds no state between
// searches and knows nothing of maps, units or terrain.
//
// Find is a pure function of its Query: equal queries whose step costs agree
// return the same route, including the choice between routes of equal cost.
package pathfind
