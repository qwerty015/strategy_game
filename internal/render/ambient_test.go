package render

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/world"
)

func TestWalkingFrameAnimatesOnlyWithAnActiveRoute(t *testing.T) {
	originalFrame := animFrame
	t.Cleanup(func() { animFrame = originalFrame })

	if got := walkingFrame(nil, 0); got != 1 {
		t.Fatalf("stationary frame = %d, want neutral frame 1", got)
	}

	path := []pathfind.Point{{X: 1, Y: 1}}
	for frame, want := range []int{0, 1, 2, 0} {
		animFrame = frame * 7
		if got := walkingFrame(path, 0); got != want {
			t.Fatalf("walk frame at tick %d = %d, want %d", animFrame, got, want)
		}
	}
}

func TestFindHareRunUsesOnlyGrass(t *testing.T) {
	g := world.NewGrid(12, 3)
	g.Set(6, 1, world.Tile{Terrain: world.Water})

	for seed := uint32(0); seed < 96; seed++ {
		startX, y, direction, ok := findHareRun(g, seed)
		if !ok {
			continue
		}
		for step := 0; step <= hareRunTiles; step++ {
			x := startX + direction*step
			if !g.InBounds(x, y) || g.At(x, y).Terrain != world.Grass {
				t.Fatalf("hare lane for seed %d contains non-grass tile at (%d, %d)", seed, x, y)
			}
		}
	}
}

func TestFindHareRunRejectsNonGrassMap(t *testing.T) {
	g := world.NewGrid(10, 2)
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			g.Set(x, y, world.Tile{Terrain: world.Water})
		}
	}

	if _, _, _, ok := findHareRun(g, 1); ok {
		t.Fatal("findHareRun found a lane on a water-only map")
	}
}

func TestBuildingEntranceRoadPositionsUsesOnlyPreferredDoorway(t *testing.T) {
	house := &building.Building{Kind: building.Bakery, X: 4, Y: 4}
	roads := map[roadTile]bool{
		{4, 5}: true, // south: the same starter-road preference as FoundationRoad
		{5, 4}: true,
	}

	entrances := buildingEntranceRoadPositions([]*building.Building{house}, roads)
	if len(entrances) != 1 || !entrances[roadTile{4, 5}] {
		t.Fatalf("entrance roads = %#v, want south doorway only", entrances)
	}
}

func TestBuildingEntranceCornerMasksCutsOnlyOpenGrassCorner(t *testing.T) {
	entrance := roadTile{4, 5}
	entrances := map[roadTile]bool{entrance: true}
	roads := map[roadTile]bool{
		entrance: true,
		{3, 5}:   true, // the entrance's west edge continues into a road
	}
	occupied := map[roadTile]bool{
		{4, 4}: true, // the building directly above the entrance
	}

	masks := buildingEntranceCornerMasks(entrances, roads, occupied)
	if got, want := masks[entrance], roadCornerSouthEast; got != want {
		t.Fatalf("entrance corner mask = %04b, want only open south-east corner %04b", got, want)
	}
}
