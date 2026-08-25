package pathfind

import (
	"testing"

	"strategy_game/internal/building"
)

func TestFindPath_ConnectedViaRoad(t *testing.T) {
	from := &building.Building{Kind: building.Farm, X: 0, Y: 0} // footprint (0,0)-(1,1)
	to := &building.Building{Kind: building.Mill, X: 5, Y: 0}   // footprint (5,0)-(6,1)

	buildings := []*building.Building{from, to}
	for x := 2; x <= 4; x++ {
		buildings = append(buildings, &building.Building{Kind: building.Road, X: x, Y: 0})
	}

	path, ok := FindPath(buildings, from, to)
	if !ok {
		t.Fatal("FindPath() = not found, want a path along the road")
	}
	if last := path[len(path)-1]; !footprintSet(to)[last] {
		t.Errorf("path ends at %v, want a tile in to's footprint", last)
	}
	if first := path[0]; !footprintSet(from)[first] {
		t.Errorf("path starts at %v, want a tile in from's footprint", first)
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
