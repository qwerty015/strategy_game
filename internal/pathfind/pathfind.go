// Package pathfind finds routes for serfs to walk across the road
// network laid out with building.Road tiles. It depends only on
// building (plain data), not on ebiten or any other logic package.
package pathfind

import "strategy_game/internal/building"

// Point is a tile coordinate.
type Point struct{ X, Y int }

// FindPath returns a shortest tile-by-tile path connecting any tile of
// from's footprint to any tile of to's footprint, moving only across
// Road buildings and the two endpoint buildings' own footprints (so a
// serf can stand "inside" either building to load/unload). It reports
// false if no such route exists, which is exactly what should happen
// when a building isn't connected to the road network yet.
func FindPath(buildings []*building.Building, from, to *building.Building) ([]Point, bool) {
	walkable := walkableSet(buildings, from, to)
	goals := footprintSet(to)

	// Breadth-first search over the walkable tile graph. visited maps
	// each reached tile to the tile it was reached from; a tile that
	// maps to itself is one of the search roots (from's footprint).
	visited := map[Point]Point{}
	queue := footprintPoints(from)
	for _, p := range queue {
		visited[p] = p
	}

	var goal Point
	found := false
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		if goals[p] {
			goal, found = p, true
			break
		}
		for _, n := range neighbors(p) {
			if _, seen := visited[n]; seen {
				continue
			}
			if !walkable[n] {
				continue
			}
			visited[n] = p
			queue = append(queue, n)
		}
	}
	if !found {
		return nil, false
	}

	path := []Point{goal}
	for cur := goal; visited[cur] != cur; {
		parent := visited[cur]
		path = append(path, parent)
		cur = parent
	}
	reverse(path)
	return path, true
}

func footprintPoints(b *building.Building) []Point {
	size := building.Types[b.Kind].Footprint
	pts := make([]Point, 0, size*size)
	for dy := range size {
		for dx := range size {
			pts = append(pts, Point{b.X + dx, b.Y + dy})
		}
	}
	return pts
}

func footprintSet(b *building.Building) map[Point]bool {
	set := make(map[Point]bool)
	for _, p := range footprintPoints(b) {
		set[p] = true
	}
	return set
}

// walkableSet is every tile a serf may step on for a trip between from
// and to: all Road tiles, plus both endpoint buildings' own footprints.
func walkableSet(buildings []*building.Building, from, to *building.Building) map[Point]bool {
	set := make(map[Point]bool)
	for _, b := range buildings {
		if b.Kind == building.Road {
			set[Point{b.X, b.Y}] = true
		}
	}
	for p := range footprintSet(from) {
		set[p] = true
	}
	for p := range footprintSet(to) {
		set[p] = true
	}
	return set
}

func neighbors(p Point) [4]Point {
	return [4]Point{
		{p.X - 1, p.Y},
		{p.X + 1, p.Y},
		{p.X, p.Y - 1},
		{p.X, p.Y + 1},
	}
}

func reverse(pts []Point) {
	for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
		pts[i], pts[j] = pts[j], pts[i]
	}
}
