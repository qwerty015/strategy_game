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
// tiles, but not the rest of either footprint or any third building. It uses
// all eight neighbouring tiles, so a diagonal road can be walked in one
// movement step. A road must still meet a building's door by a side, rather
// than merely touching its corner; this keeps AccessPoint meaningful for
// multi-tile buildings. At least one Road tile must be used, so two buildings
// touching edge-to-edge are not considered connected without an actual road.
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
//
// Most callers pass two different lists to the two pathfinding-related
// parameters here and in FindLandPathForFaction: buildings/candidates
// stays scoped to one faction (which tree/deposit/site is mine to work),
// while the actual obstacle list passed as this function's own buildings
// parameter is the WHOLE map -- every faction's buildings, so a rival's
// wall genuinely blocks the route instead of being invisible to it (see
// logistics/lumberjack/quarry/miner/builder's own Controller.Tick doc
// comments, each with the specific playtest bug this fixes). A plain
// building.GatePassable check inside that obstacle map still only ever
// evaluates gates at face value (owner-blind) -- see FindLandPathForFaction
// for the one caller (soldier movement) that additionally needs a foreign
// Gate to block like a solid wall regardless of its Open/Auto state.
func FindLandPath(grid *world.Grid, buildings []*building.Building, from, to Point) ([]Point, bool) {
	return findLandPath(grid, buildingOccupancy(buildings), buildings, from, to)
}

// FindLandPathForFaction is FindLandPath, but a Gate belonging to any owner
// other than owner blocks like a solid wall regardless of its Open/Auto
// state -- see building.GatePassableTo's doc comment for the playtest bug
// this fixes. Used only by soldier movement (package soldier): an attacking
// faction's soldiers must actually be stopped by an opponent's walls/gates,
// which means their obstacle list has to include the opponent's buildings
// at all (see cmd/game's Update/tickAIFaction passing g.buildings, not the
// usual per-faction ownedBuildingsWithRoads, to soldiers.Tick) -- the only
// unit kind for which "another faction's gate" is ever actually in the list.
func FindLandPathForFaction(grid *world.Grid, buildings []*building.Building, from, to Point, owner int) ([]Point, bool) {
	return findLandPath(grid, buildingOccupancyForFaction(buildings, owner), buildings, from, to)
}

func findLandPath(grid *world.Grid, blocked map[Point]bool, buildings []*building.Building, from, to Point) ([]Point, bool) {
	if grid == nil || !grid.InBounds(from.X, from.Y) || !grid.InBounds(to.X, to.Y) {
		return nil, false
	}
	if from == to {
		return []Point{from}, true
	}

	// Occupancy is computed once per call, not once per visited tile --
	// see landWalkable's doc comment for why this matters on a large,
	// densely-decorated map (hundreds of stone/ore deposits, each its own
	// Building).
	barriers := wallBarrierOccupancy(buildings)

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
			if _, seen := visited[n]; seen || (diagonal(p, n) && crossesWallBarrier(barriers, p, n)) || !landWalkable(grid, blocked, n, from, to) {
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

// buildingOccupancy indexes every tile covered by a building's footprint
// (Road excluded -- it's walkable as ordinary land, same as landWalkable
// always treated it) into an O(1)-lookup set. Building this once per
// FindLandPath call, instead of having landWalkable rescan the whole
// buildings slice for every single tile the BFS visits, is what turns
// FindLandPath from O(visited tiles × building count) into O(visited tiles
// + building count) -- on a map with hundreds of individual stone/ore
// deposit Buildings this was the difference between an imperceptible
// lookup and the game visibly stalling under high CPU load every time a
// lumberjack/quarryman/miner/builder/serf needed a new route.
func buildingOccupancy(buildings []*building.Building) map[Point]bool {
	blocked := make(map[Point]bool, len(buildings))
	for _, b := range buildings {
		if b == nil || b.Kind == building.Road || building.GatePassable(b) {
			continue
		}
		footprint := building.Types[b.Kind].Footprint
		for dy := 0; dy < footprint; dy++ {
			for dx := 0; dx < footprint; dx++ {
				blocked[Point{b.X + dx, b.Y + dy}] = true
			}
		}
	}
	return blocked
}

// buildingOccupancyForFaction is buildingOccupancy, but a Gate not owned by
// owner never exempts its tile -- see building.GatePassableTo and
// FindLandPathForFaction.
func buildingOccupancyForFaction(buildings []*building.Building, owner int) map[Point]bool {
	blocked := make(map[Point]bool, len(buildings))
	for _, b := range buildings {
		if b == nil || b.Kind == building.Road || building.GatePassableTo(b, owner) {
			continue
		}
		footprint := building.Types[b.Kind].Footprint
		for dy := 0; dy < footprint; dy++ {
			for dx := 0; dx < footprint; dx++ {
				blocked[Point{b.X + dx, b.Y + dy}] = true
			}
		}
	}
	return blocked
}

// wallBarrierOccupancy is narrower than buildingOccupancy: it contains only
// solid wall pieces and manual closed gates. It exists solely to prevent a
// diagonal path from clipping through the corner between two wall tiles. Auto
// gates stay routeable, so a unit can approach one and make it open.
func wallBarrierOccupancy(buildings []*building.Building) map[Point]bool {
	barriers := make(map[Point]bool)
	for _, b := range buildings {
		if b == nil {
			continue
		}
		if b.Kind == building.StoneWall || (b.Kind == building.Gate && !building.GatePassable(b)) {
			barriers[Point{X: b.X, Y: b.Y}] = true
		}
	}
	return barriers
}

// crossesWallBarrier rejects the two possible corner cuts for a diagonal step.
// Ordinary objects retain the game's existing diagonal movement behaviour;
// only an intentional defensive wall gets this stricter rule.
func crossesWallBarrier(barriers map[Point]bool, from, to Point) bool {
	if !diagonal(from, to) {
		return false
	}
	return barriers[Point{X: to.X, Y: from.Y}] || barriers[Point{X: from.X, Y: to.Y}]
}

// ClosedWallAreas returns all passable land cells inside a completed wall loop
// with no usable gate. It uses the very same wall and diagonal rules as
// FindLandPath, so a resource worker cannot choose a target across a sealed
// enclosure. Automatic and open gates connect the regions; a manual closed
// gate keeps them split. The result is calculated on demand by the advisor,
// not every render frame.
func ClosedWallAreas(grid *world.Grid, buildings []*building.Building) map[Point]bool {
	enclosed := make(map[Point]bool)
	if grid == nil {
		return enclosed
	}
	barriers := wallBarrierOccupancy(buildings)
	seen := make(map[Point]bool)
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			start := Point{X: x, Y: y}
			if seen[start] || barriers[start] || !grid.At(x, y).Buildable() {
				continue
			}
			component := []Point{start}
			seen[start] = true
			reachesEdge := x == 0 || y == 0 || x == grid.Width-1 || y == grid.Height-1
			touchesWall := false
			for head := 0; head < len(component); head++ {
				current := component[head]
				for _, next := range neighbors(current) {
					if !grid.InBounds(next.X, next.Y) {
						continue
					}
					if barriers[next] {
						touchesWall = true
						continue
					}
					if diagonal(current, next) && crossesWallBarrier(barriers, current, next) {
						touchesWall = true
						continue
					}
					if seen[next] || !grid.At(next.X, next.Y).Buildable() {
						continue
					}
					if next.X == 0 || next.Y == 0 || next.X == grid.Width-1 || next.Y == grid.Height-1 {
						reachesEdge = true
					}
					seen[next] = true
					component = append(component, next)
				}
			}
			if reachesEdge || !touchesWall {
				continue
			}
			for _, point := range component {
				enclosed[point] = true
			}
		}
	}
	return enclosed
}

func landWalkable(grid *world.Grid, blocked map[Point]bool, p, start, goal Point) bool {
	if !grid.InBounds(p.X, p.Y) || !grid.At(p.X, p.Y).Buildable() {
		return false
	}
	if p == start || p == goal {
		return true
	}
	return !blocked[p]
}

func findPathBetween(buildings []*building.Building, start, goal Point, requireRoad bool) ([]Point, bool) {
	walkable := roadSet(buildings)
	walkable[start] = true
	walkable[goal] = true

	roads := roadSet(buildings)
	barriers := wallBarrierOccupancy(buildings)
	// A saved unit can start on a Road, where leaving diagonally is normal.
	// A building access tile is not a Road, however: its first/last step has
	// to be cardinal so a road cannot activate a building by just touching the
	// corner of its footprint.
	restrictStartDiagonal := !roads[start]
	restrictGoalDiagonal := !roads[goal]

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
			if diagonal(p, n) && ((p == start && restrictStartDiagonal) || (n == goal && restrictGoalDiagonal) || crossesWallBarrier(barriers, p, n)) {
				continue
			}
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

// roadSet only counts a finished Road tile -- one still under construction
// (see building.ConstructionStage) isn't walkable as a road yet, the same
// way an unfinished building isn't a working producer yet.
func roadSet(buildings []*building.Building) map[Point]bool {
	set := make(map[Point]bool)
	for _, b := range buildings {
		if b == nil {
			continue
		}
		if (b.Kind == building.Road && b.ConstructionStage == building.ConstructionNone) || building.GatePassable(b) {
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

// neighbors returns the eight adjacent tiles. Moving diagonally costs the
// same simulation step as moving straight: units occupy discrete map cells
// and their animation interpolates only between successive path points.
func neighbors(p Point) [8]Point {
	return [8]Point{
		{p.X - 1, p.Y},
		{p.X + 1, p.Y},
		{p.X, p.Y - 1},
		{p.X, p.Y + 1},
		{p.X - 1, p.Y - 1},
		{p.X + 1, p.Y - 1},
		{p.X - 1, p.Y + 1},
		{p.X + 1, p.Y + 1},
	}
}

func diagonal(a, b Point) bool {
	return a.X != b.X && a.Y != b.Y
}

func reverse(pts []Point) {
	for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
		pts[i], pts[j] = pts[j], pts[i]
	}
}
