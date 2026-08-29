package render

import (
	"image"
	"testing"

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
