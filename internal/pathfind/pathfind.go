// Package pathfind finds routes for serfs to walk across the road
// network laid out with building.Road tiles. It depends only on
// building (plain data), not on ebiten or any other logic package.
package pathfind

import (
	"strategy_game/internal/building"
	"strategy_game/internal/world"
)

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
	return findPathBetween(buildings, accessPoint(from), accessPoint(to), true)
}

// FindPathFromPoint returns a route from an arbitrary saved unit position to
// a building's access point. It is used when a unit was saved halfway through
// a road trip: the unit keeps its screen position, then receives a fresh job
// from that point instead of visually jumping back to its old building.
func FindPathFromPoint(buildings []*building.Building, from Point, to *building.Building) ([]Point, bool) {
	goal := accessPoint(to)
	if from == goal {
		return []Point{from}, true
	}
	return findPathBetween(buildings, from, goal, true)
}

// FindLandPath returns a shortest route across every non-water tile. Roads
// are walkable as ordinary land, while building and tree footprints are
// obstacles; the start and goal tiles are allowed so a worker can leave a
// workplace and reach a tree occupying its goal tile. Unlike FindPath, this
// route does not require a road anywhere in the path.
func FindLandPath(grid *world.Grid, buildings []*building.Building, from, to Point) ([]Point, bool) {
	if grid == nil || !grid.InBounds(from.X, from.Y) || !grid.InBounds(to.X, to.Y) {
		return nil, false
	}
	if from == to {
		return []Point{from}, true
	}

	visited := map[Point]Point{from: from}
	queue := []Point{from}
	found := false
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		if p == to {
			found = true
			break
		}
		for _, n := range neighbors(p) {
			if _, seen := visited[n]; seen || !landWalkable(grid, buildings, n, from, to) {
				continue
			}
			visited[n] = p
			queue = append(queue, n)
		}
	}
	if !found {
		return nil, false
	}

	path := []Point{to}
	for cur := to; visited[cur] != cur; {
		parent := visited[cur]
		path = append(path, parent)
		cur = parent
	}
	reverse(path)
	return path, true
}

// FindWaterPath returns a shortest route that stays entirely on Water tiles.
// It deliberately ignores placed Fish objects: fish occupy a water cell for
// population/rendering, not as a solid obstacle to a boat.
func FindWaterPath(grid *world.Grid, from, to Point) ([]Point, bool) {
	if grid == nil || !grid.InBounds(from.X, from.Y) || !grid.InBounds(to.X, to.Y) ||
		grid.At(from.X, from.Y).Terrain != world.Water || grid.At(to.X, to.Y).Terrain != world.Water {
		return nil, false
	}
	if from == to {
		return []Point{from}, true
	}

	visited := map[Point]Point{from: from}
	queue := []Point{from}
	found := false
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		if p == to {
			found = true
			break
		}
		for _, n := range neighbors(p) {
			if _, seen := visited[n]; seen || !grid.InBounds(n.X, n.Y) || grid.At(n.X, n.Y).Terrain != world.Water {
				continue
			}
			visited[n] = p
			queue = append(queue, n)
		}
	}
	if !found {
		return nil, false
	}

	path := []Point{to}
	for cur := to; visited[cur] != cur; {
		parent := visited[cur]
		path = append(path, parent)
		cur = parent
	}
	reverse(path)
	return path, true
}

func landWalkable(grid *world.Grid, buildings []*building.Building, p, start, goal Point) bool {
	if !grid.InBounds(p.X, p.Y) || !grid.At(p.X, p.Y).Buildable() {
		return false
	}
	if p == start || p == goal {
		return true
	}
	for _, b := range buildings {
		if b == nil || b.Kind == building.Road {
			continue
		}
		footprint := building.Types[b.Kind].Footprint
		if p.X >= b.X && p.X < b.X+footprint && p.Y >= b.Y && p.Y < b.Y+footprint {
			return false
		}
	}
	return true
}

func findPathBetween(buildings []*building.Building, start, goal Point, requireRoad bool) ([]Point, bool) {
	walkable := roadSet(buildings)
	walkable[start] = true
	walkable[goal] = true

	roads := roadSet(buildings)

	// Breadth-first search over the walkable tile graph. visited maps
	// each reached tile to the tile it was reached from; the root maps to
	// itself.
	visited := map[Point]Point{}
	queue := []Point{start}
	for _, p := range queue {
		visited[p] = p
	}

	found := false
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		if p == goal {
			found = true
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
	if requireRoad && !containsRoad(path, roads) {
		return nil, false
	}
	return path, true
}

func accessPoint(b *building.Building) Point {
	p := b.AccessPoint()
	return Point{X: p.X, Y: p.Y}
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
