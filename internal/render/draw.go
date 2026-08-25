package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/world"
)

// terrainColor returns a flat placeholder color for a terrain type. These
// get replaced by pixel-art tiles later; the game plays the same either
// way since rendering doesn't feed back into logic.
func terrainColor(t world.TerrainType) color.RGBA {
	switch t {
	case world.Fertile:
		return color.RGBA{R: 141, G: 110, B: 62, A: 255} // brown soil
	case world.Forest:
		return color.RGBA{R: 34, G: 89, B: 47, A: 255} // dark green
	case world.Water:
		return color.RGBA{R: 58, G: 105, B: 191, A: 255} // blue
	case world.Stone:
		return color.RGBA{R: 120, G: 120, B: 128, A: 255} // grey
	default: // world.Grass
		return color.RGBA{R: 86, G: 148, B: 74, A: 255} // green
	}
}

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
			c := terrainColor(g.At(tx, ty).Terrain)
			vector.FillRect(
				screen,
				float32(sx), float32(sy),
				float32(TileSize)-1, float32(TileSize)-1, // -1 leaves a thin grid line
				c, false,
			)
		}
	}
}
