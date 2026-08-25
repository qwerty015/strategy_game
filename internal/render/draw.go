package render

import (
	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
	"strategy_game/internal/world"
)

// DrawGrid renders every tile currently visible in the camera's viewport.
// Off-screen tiles are skipped so panning stays cheap on a big map.
func DrawGrid(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	screenW, screenH := screen.Bounds().Dx(), screen.Bounds().Dy()

	firstX, firstY := cam.ScreenToTile(0, 0)
	lastX, lastY := cam.ScreenToTile(screenW, screenH)

	for ty := firstY; ty <= lastY; ty++ {
		for tx := firstX; tx <= lastX; tx++ {
			if !g.InBounds(tx, ty) {
				continue
			}
			sx, sy := cam.TileToScreen(tx, ty)
			drawStanding(screen, terrainImage(g.At(tx, ty).Terrain), sx, sy, 1)
		}
	}
}

// terrainImage returns the ground sprite for a tile. Forest's sprite
// already combines grass and trees in one image (see assets.Forest), so
// there's no separate overlay draw needed.
func terrainImage(t world.TerrainType) *ebiten.Image {
	switch t {
	case world.Fertile:
		return assets.Fertile
	case world.Forest:
		return assets.Forest
	case world.Water:
		return assets.Water
	case world.Stone:
		return assets.Stone
	default: // world.Grass
		return assets.Grass
	}
}
