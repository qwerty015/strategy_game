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
			drawStanding(screen, terrainImage(terrain), sx, sy, 1)
			if terrain == world.Grass {
				drawGrassSway(screen, sx, sy, tx, ty)
			}
		}
	}
}

// drawGrassSway is a tiny two-frame ambient animation. Only a deterministic
// subset of grass tiles gets a few one-pixel blades, so the whole map does not
// flicker and the effect remains cheap at the prototype's scale.
func drawGrassSway(screen *ebiten.Image, sx, sy float64, tx, ty int) {
	seed := (tx*92821 + ty*68917) & 7
	if seed > 1 {
		return
	}
	phase := (animFrame/10 + tx + ty) & 1
	xOffset := float32(phase)
	baseX := float32(sx) + float32(5+seed*7)
	baseY := float32(sy) + 19
	blade := color.RGBA{R: 91, G: 157, B: 61, A: 190}
	vector.FillRect(screen, baseX+xOffset, baseY-4, 1, 4, blade, false)
	vector.FillRect(screen, baseX+1-xOffset, baseY-3, 1, 3, blade, false)
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
