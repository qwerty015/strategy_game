// Package pathfind finds routes for serfs to walk across the road
// network laid out with building.Road tiles. It depends only on
// building (plain data), not on ebiten or any other logic package.
package pathfind

import "strategy_game/internal/building"

// Point is a tile coordinate.
type Point struct{ X, Y int }

// FindPath returns a shortest tile-by-tile path connecting the access points
// of from and to. The path may use Road tiles and the two endpoint access
// tiles, but not the rest of either footprint or any third building. At
// least one Road tile must be used, so two buildings touching edge-to-edge
// are not considered connected without an actual road.
func FindPath(buildings []*building.Building, from, to *building.Building) ([]Point, bool) {
	if from == to {
		return []Point{accessPoint(from)}, true
	}

	walkable := walkableSet(buildings, from, to)
	goals := map[Point]bool{accessPoint(to): true}
	roads := roadSet(buildings)

	// Breadth-first search over the walkable tile graph. visited maps
	// each reached tile to the tile it was reached from; the root maps to
	// itself.
	visited := map[Point]Point{}
	queue := []Point{accessPoint(from)}
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
	if !containsRoad(path, roads) {
		return nil, false
	}
	return path, true
}

func accessPoint(b *building.Building) Point {
	p := b.AccessPoint()
	return Point{X: p.X, Y: p.Y}
}

// walkableSet is every tile a serf may step on for a trip between from and
// to: all Road tiles, plus the access points of the two endpoint buildings.
func walkableSet(buildings []*building.Building, from, to *building.Building) map[Point]bool {
	set := make(map[Point]bool)
	for _, b := range buildings {
		if b.Kind == building.Road {
			set[Point{b.X, b.Y}] = true
		}
	}
	set[accessPoint(from)] = true
	set[accessPoint(to)] = true
	return set
}

func roadSet(buildings []*building.Building) map[Point]bool {
	set := make(map[Point]bool)
	for _, b := range buildings {
		if b.Kind == building.Road {
			set[Point{b.X, b.Y}] = true
		}
	}
	return set
}

func containsRoad(path []Point, roads map[Point]bool) bool {
	for _, p := range path {
		if roads[p] {
			return true
		}
	}
	return false
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
