package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
	"strategy_game/internal/miner"
)

// minerHeight matches quarrymanHeight: both are workers who roam free land,
// drawn at the same readable scale.
const minerHeight = 1.08

// DrawMiners renders workers while they are outside the hut. Idle and
// unloading workers are represented by the hut's compact +/- marker instead
// of being drawn on top of the roof, same as DrawQuarrymen.
func DrawMiners(screen *ebiten.Image, miners []*miner.Miner, cam *Camera) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, m := range miners {
		if !m.VisibleOnMap() {
			continue
		}
		if !visible.Intersects(m.X, m.Y, 1) {
			continue
		}

		sx, sy := cam.TileToScreen(m.X, m.Y)
		frame := walkingFrame(m.RemainingPath(), m.X+m.Y)
		bob := unitBob()
		tint := color.Color(color.White)
		if m.Starving {
			tint = color.RGBA{R: 255, G: 105, B: 90, A: 255}
		}
		flip := facingLeft(m.X, m.RemainingPath())
		drawStandingFacingTintedAtScale(screen, assets.MinerWalkFrames[frame], sx, sy+bob*tilePixels/TileSize, minerHeight, tilePixels, tint, flip)

		cargo, amount := m.Cargo()
		drawCarriedResource(screen, cargo, amount, sx, sy, tilePixels, bob)

		if m.State() == miner.StateMining {
			drawChopCue(screen, sx, sy+bob*tilePixels/TileSize, tilePixels)
		}
	}
}
