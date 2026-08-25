package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

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
			terrain := g.At(tx, ty).Terrain
			tilePixels := cam.TilePixels()
			drawStandingAtScale(screen, terrainImage(terrain), sx, sy, 1, tilePixels)
			if terrain == world.Grass {
				drawGrassSway(screen, sx, sy, tx, ty, tilePixels)
			}
		}
	}
}

// drawGrassSway is a tiny two-frame ambient animation. Only a deterministic
// subset of grass tiles gets a few one-pixel blades, so the whole map does not
// flicker and the effect remains cheap at the prototype's scale.
func drawGrassSway(screen *ebiten.Image, sx, sy float64, tx, ty int, tilePixels float64) {
	seed := (tx*92821 + ty*68917) & 7
	if seed > 1 {
		return
	}
	phase := (animFrame/10 + tx + ty) & 1
	xOffset := float32(phase) * float32(tilePixels/TileSize)
	baseX := float32(sx) + float32(5+seed*7)*float32(tilePixels/TileSize)
	baseY := float32(sy) + float32(19)*float32(tilePixels/TileSize)
	blade := color.RGBA{R: 91, G: 157, B: 61, A: 190}
	width := float32(tilePixels / TileSize)
	if width < 1 {
		width = 1
	}
	vector.FillRect(screen, baseX+xOffset, baseY-4*width, width, 4*width, blade, false)
	vector.FillRect(screen, baseX+width-xOffset, baseY-3*width, width, 3*width, blade, false)
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
