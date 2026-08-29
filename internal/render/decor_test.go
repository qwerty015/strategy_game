package render

import (
	"testing"

	"strategy_game/internal/world"
)

// TestFindShoreEdge_FindsTheGrassSide is a regression guard for the
// shoreline decoration (drawReeds/drawPebbles): a water tile bordering
// grass on exactly one side must report that side, and a tile with no
// differing neighbour at all must report none.
func TestFindShoreEdge_FindsTheGrassSide(t *testing.T) {
	g := world.NewGrid(3, 3)
	for y := range 3 {
		for x := range 3 {
			g.Set(x, y, world.Tile{Terrain: world.Water})
		}
	}
	g.Set(2, 1, world.Tile{Terrain: world.Grass}) // east of the middle tile

	edge, ok := findShoreEdge(g, 1, 1, world.Grass)
	if !ok {
		t.Fatal("water tile bordering grass to the east: findShoreEdge = not found, want found")
	}
	if edge != edgeE {
		t.Errorf("edge = %v, want edgeE", edge)
	}

	if _, ok := findShoreEdge(g, 0, 0, world.Grass); ok {
		t.Error("water tile with no grass neighbour: findShoreEdge = found, want not found")
	}
}

// TestTileHash_IsDeterministic guards the property blendTerrainEdges'
// sibling decoration functions rely on: the same tile must always hash to
// the same value (stable across frames/reloads with no stored state),
// and two different tiles should not always collide.
func TestTileHash_IsDeterministic(t *testing.T) {
	a := tileHash(5, 9, 2654435761, 40503)
	b := tileHash(5, 9, 2654435761, 40503)
	if a != b {
		t.Fatalf("tileHash(5,9) = %d then %d, want identical", a, b)
	}
	if tileHash(5, 9, 2654435761, 40503) == tileHash(6, 9, 2654435761, 40503) {
		t.Error("tileHash(5,9) == tileHash(6,9), want distinct tiles to usually differ")
	}
}
