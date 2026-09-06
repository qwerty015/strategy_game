package render

import (
	"image"
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/world"
)

func TestMinimapWorldPointRoundTripsCorners(t *testing.T) {
	grid := world.NewGrid(50, 38)
	rect := image.Rect(800, 20, 968, 188) // 168x168, matching minimapSize

	for _, tc := range []struct {
		name     string
		px, py   int
		wantTile image.Point
	}{
		{"top-left", rect.Min.X, rect.Min.Y, image.Pt(0, 0)},
		{"bottom-right just inside", rect.Max.X - 1, rect.Max.Y - 1, image.Pt(grid.Width-1, grid.Height-1)},
		{"center", rect.Min.X + rect.Dx()/2, rect.Min.Y + rect.Dy()/2, image.Pt(grid.Width/2, grid.Height/2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worldX, worldY, ok := MinimapWorldPoint(grid, rect, tc.px, tc.py)
			if !ok {
				t.Fatalf("MinimapWorldPoint(%d,%d) reported not ok", tc.px, tc.py)
			}
			gotTile := image.Pt(int(worldX/TileSize), int(worldY/TileSize))
			// Allow a one-tile slack from the scale-factor rounding baked
			// into minimapPixelToTile -- what matters is that a corner
			// click resolves near that corner, not to some other part of
			// the map entirely.
			if abs(gotTile.X-tc.wantTile.X) > 1 || abs(gotTile.Y-tc.wantTile.Y) > 1 {
				t.Fatalf("pixel (%d,%d) -> tile %v, want near %v", tc.px, tc.py, gotTile, tc.wantTile)
			}
		})
	}
}

func TestMinimapWorldPointRejectsOutsideRect(t *testing.T) {
	grid := world.NewGrid(50, 38)
	rect := image.Rect(800, 20, 968, 188)
	if _, _, ok := MinimapWorldPoint(grid, rect, rect.Min.X-5, rect.Min.Y); ok {
		t.Fatal("a click outside the minimap rect resolved to a world point")
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// TestMinimapBuildingColor_OpponentBuildingsUseTheirFactionColor is the
// regression for the user's explicit request ("раскрашивай постройки
// противников на миникарте согласно цвету игрока"): an opponent-owned
// building must show up on the minimap in that faction's own color (see
// colorForOwner, already used for the main map's drawOwnerOutline), not
// the generic kind-bucket color every building used to get regardless of
// owner.
func TestMinimapBuildingColor_OpponentBuildingsUseTheirFactionColor(t *testing.T) {
	for owner := 1; owner <= 3; owner++ {
		got := minimapBuildingColor(building.Barracks, owner)
		want := colorForOwner(owner)
		if got != want {
			t.Errorf("owner %d: minimapBuildingColor(Barracks, %d) = %v, want %v (colorForOwner)", owner, owner, got, want)
		}
	}
}

// TestMinimapBuildingColor_PlayerBuildingsUnchanged locks in that Owner 0
// (the player, and every building in an ordinary single-player game) keeps
// exactly its old kind-bucket color -- this change must not alter how a
// single-player minimap has always looked.
func TestMinimapBuildingColor_PlayerBuildingsUnchanged(t *testing.T) {
	warehouse := minimapBuildingColor(building.Warehouse, 0)
	if warehouse != minimapBuildingColor(building.Tavern, 0) {
		t.Fatalf("Warehouse and Tavern (owner 0) should share the same bucket color")
	}
	other := minimapBuildingColor(building.Farm, 0)
	if other == warehouse {
		t.Fatalf("Farm (owner 0) should not share the Warehouse/Tavern bucket color")
	}
}

// TestMinimapBuildingColor_NaturalResourcesAndRoadsIgnoreOwner keeps
// shared/neutral map objects reading as neutral regardless of which
// faction happens to be nearest -- these aren't territory markers (same
// convention as drawOwnerOutline/isNaturalResourceKind elsewhere).
func TestMinimapBuildingColor_NaturalResourcesAndRoadsIgnoreOwner(t *testing.T) {
	for _, kind := range []building.Kind{building.Road, building.Tree, building.Fish, building.StoneDeposit, building.CoalDeposit} {
		neutral := minimapBuildingColor(kind, 0)
		owned := minimapBuildingColor(kind, 1)
		if neutral != owned {
			t.Errorf("%v: owner 0 color %v != owner 1 color %v, want them equal (neutral)", kind, neutral, owned)
		}
	}
}
