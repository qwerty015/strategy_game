package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/quarry"
)

// quarrymanHeight matches lumberjackHeight: both are workers who roam free
// land, drawn at the same readable scale.
const quarrymanHeight = 0.88

// DrawQuarrymen renders workers while they are outside the hut. Idle and
// unloading workers are represented by the hut's compact +/- marker instead
// of being drawn on top of the roof, same as DrawLumberjacks.
func DrawQuarrymen(screen *ebiten.Image, quarrymen []*quarry.Quarryman, cam *Camera) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, q := range quarrymen {
		if !q.VisibleOnMap() {
			continue
		}
		if !visible.Intersects(q.X, q.Y, 1) {
			continue
		}

		sx, sy := cam.TileToScreen(q.X, q.Y)
		frame := (animFrame / 10) % len(assets.Quarryman)
		bob := unitBob()
		tint := color.Color(color.White)
		if q.Starving {
			tint = color.RGBA{R: 255, G: 105, B: 90, A: 255}
		}
		drawStandingTintedAtScale(screen, assets.Quarryman[frame], sx, sy+bob*tilePixels/TileSize, quarrymanHeight, tilePixels, tint)

		cargo, amount := q.Cargo()
		if amount > 0 {
			badge := 5 * tilePixels / TileSize
			vector.FillRect(screen, float32(sx+17*tilePixels/TileSize), float32(sy+(3+bob)*tilePixels/TileSize), float32(badge), float32(badge), cargoColor(cargo), false)
		}

		if q.State() == quarry.StateMining {
			drawChopCue(screen, sx, sy+bob*tilePixels/TileSize, tilePixels)
		}
	}
}
