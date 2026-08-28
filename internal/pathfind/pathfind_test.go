package pathfind

import (
	"testing"
	"time"

	"strategy_game/internal/building"
	"strategy_game/internal/world"
)

func TestFindPath_ConnectedViaRoad(t *testing.T) {
	from := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	to := &building.Building{Kind: building.Mill, X: 5, Y: 0}

	buildings := []*building.Building{from, to}
	for x := 1; x <= 4; x++ {
		buildings = append(buildings, &building.Building{Kind: building.Road, X: x, Y: 0})
	}

	path, ok := FindPath(buildings, from, to)
	if !ok {
		t.Fatal("FindPath() = not found, want a path along the road")
	}
	if last := path[len(path)-1]; last != (Point{X: to.X, Y: to.Y}) {
		t.Errorf("path ends at %v, want target access point (%d,%d)", last, to.X, to.Y)
	}
	if first := path[0]; first != (Point{X: from.X, Y: from.Y}) {
		t.Errorf("path starts at %v, want source access point (%d,%d)", first, from.X, from.Y)
	}
}

func TestFindLandPath_WalksWithoutRoadButNotThroughWater(t *testing.T) {
	grid := world.NewGrid(8, 3)
	from, to := Point{X: 0, Y: 1}, Point{X: 7, Y: 1}

	path, ok := FindLandPath(grid, nil, from, to)
	if !ok {
		t.Fatal("FindLandPath() without roads = not found, want a land route")
	}
	if path[0] != from || path[len(path)-1] != to {
		t.Fatalf("path endpoints = %v -> %v, want %v -> %v", path[0], path[len(path)-1], from, to)
	}

	for y := 0; y < grid.Height; y++ {
		grid.Set(3, y, world.Tile{Terrain: world.Water})
	}
	if _, ok := FindLandPath(grid, nil, from, to); ok {
		t.Fatal("FindLandPath() across a full water barrier = found, want not found")
	}
}

// TestFindLandPathStaysFastWithManyBuildings is a regression guard for a
// real "high CPU, game hangs" bug: landWalkable used to rescan the whole
// buildings slice for every single tile the BFS visited, turning
// FindLandPath into O(visited tiles * building count). That was invisible
// on a small map with a handful of buildings, but on a generated map sized
// like a real town (100x75, with hundreds of individual stone/ore deposit
// Buildings -- each deposit cell is its own Building, not one region
// object) it made every lumberjack/quarryman/miner/builder/serf route
// request visibly stall the game. buildingOccupancy fixed it to O(visited
// tiles + building count); this test's time budget catches the O(n*m)
// blowup coming back.
func TestFindLandPathStaysFastWithManyBuildings(t *testing.T) {
	grid := world.NewGrid(100, 75)
	const clearRow = 37 // guaranteed clear lane so a path always exists
	var buildings []*building.Building
	for y := 0; y < grid.Height; y++ {
		if y == clearRow {
			continue
		}
		for x := 0; x < grid.Width; x += 2 {
			buildings = append(buildings, &building.Building{Kind: building.StoneDeposit, X: x, Y: y, Reserve: 1})
		}
	}

	start := time.Now()
	path, ok := FindLandPath(grid, buildings, Point{X: 0, Y: clearRow}, Point{X: 99, Y: clearRow})
	elapsed := time.Since(start)

	if !ok {
		t.Fatal("FindLandPath() with many scattered buildings = not found, want the clear row to connect")
	}
	if path[0] != (Point{X: 0, Y: clearRow}) || path[len(path)-1] != (Point{X: 99, Y: clearRow}) {
		t.Fatalf("path endpoints = %v -> %v, want (0,%d) -> (99,%d)", path[0], path[len(path)-1], clearRow, clearRow)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("FindLandPath with %d buildings took %v, want well under 200ms", len(buildings), elapsed)
	}
}

func TestFindWaterPath_StaysInOneWaterBody(t *testing.T) {
	grid := world.NewGrid(7, 3)
	for x := 0; x < grid.Width; x++ {
		grid.Set(x, 1, world.Tile{Terrain: world.Water})
	}
	from, to := Point{X: 0, Y: 1}, Point{X: 6, Y: 1}
	path, ok := FindWaterPath(grid, from, to)
	if !ok {
		t.Fatal("FindWaterPath() through connected water = not found")
	}
	for _, point := range path {
		if grid.At(point.X, point.Y).Terrain != world.Water {
			t.Fatalf("water path contains non-water tile %v", point)
		}
	}

	grid.Set(3, 1, world.Tile{Terrain: world.Grass})
	if _, ok := FindWaterPath(grid, from, to); ok {
		t.Fatal("FindWaterPath() across a land break = found, want not found")
	}
}

func TestFindPath_NoRoad(t *testing.T) {
	from := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	to := &building.Building{Kind: building.Mill, X: 5, Y: 0}

	if _, ok := FindPath([]*building.Building{from, to}, from, to); ok {
		t.Fatal("FindPath() with no road between buildings = found, want not found")
	}
}

func TestFindPath_BrokenRoadIsNotFound(t *testing.T) {
	from := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	to := &building.Building{Kind: building.Mill, X: 5, Y: 0}

	// Gap at x=3 breaks the chain.
	buildings := []*building.Building{
		from, to,
		{Kind: building.Road, X: 2, Y: 0},
		{Kind: building.Road, X: 4, Y: 0},
	}

	if _, ok := FindPath(buildings, from, to); ok {
		t.Fatal("FindPath() across a broken road = found, want not found")
	}
}

func TestFindPath_RoadMustMeetBuildingAccessPoint(t *testing.T) {
	from := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	to := &building.Building{Kind: building.Mill, X: 5, Y: 0}
	buildings := []*building.Building{from, to}
	for x := 1; x <= 5; x++ {
		// This road reaches the Farm's field, but not its farmhouse/access tile.
		buildings = append(buildings, &building.Building{Kind: building.Road, X: x, Y: 1})
	}

	if _, ok := FindPath(buildings, from, to); ok {
		t.Fatal("FindPath() through a road touching only the farm field = found, want not found")
	}
}

func TestFindPath_AdjacentBuildingsNeedRoad(t *testing.T) {
	from := &building.Building{Kind: building.Mill, X: 0, Y: 0}
	to := &building.Building{Kind: building.Bakery, X: 1, Y: 0}

	if _, ok := FindPath([]*building.Building{from, to}, from, to); ok {
		t.Fatal("FindPath() between adjacent buildings without Road = found, want not found")
	}
}

func TestFindPathFromPoint_StartsAtSavedRoadTile(t *testing.T) {
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0}
	buildings := []*building.Building{tavern}
	for x := 1; x <= 4; x++ {
		buildings = append(buildings, &building.Building{Kind: building.Road, X: x, Y: 0})
	}

	path, ok := FindPathFromPoint(buildings, Point{X: 2, Y: 0}, tavern)
	if !ok {
		t.Fatal("FindPathFromPoint() = not found, want a path from the saved road tile")
	}
	if first := path[0]; first != (Point{X: 2, Y: 0}) {
		t.Fatalf("path starts at %v, want saved point (2,0)", first)
	}
}
