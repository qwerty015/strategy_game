package pathfind

import (
	"testing"

	"strategy_game/internal/building"
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
