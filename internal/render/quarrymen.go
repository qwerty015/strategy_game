package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
	"strategy_game/internal/quarry"
)

// quarrymanHeight matches lumberjackHeight: both are workers who roam free
// land, drawn at the same readable scale.
const quarrymanHeight = 1.08

// DrawQuarrymen renders workers while they are outside the hut. Idle and
// unloading workers are represented by the hut's compact +/- marker instead
// of being drawn on top of the roof, same as DrawLumberjacks.
// opponent -- see DrawSerfs's identical parameter doc comment.
func DrawQuarrymen(screen *ebiten.Image, quarrymen []*quarry.Quarryman, cam *Camera, opponent bool) {
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
		if opponent {
			DrawOpponentUnitMarker(screen, sx, sy, tilePixels)
		}
		frame := walkingFrame(q.RemainingPath(), q.X+q.Y)
		bob := unitBob()
		tint := color.Color(color.White)
		if q.Starving {
			tint = color.RGBA{R: 255, G: 105, B: 90, A: 255}
		}
		flip := facingLeft(q.X, q.RemainingPath())
		drawStandingFacingTintedAtScale(screen, assets.QuarrymanWalkFrames[frame], sx, sy+bob*tilePixels/TileSize, quarrymanHeight, tilePixels, tint, flip)

		cargo, amount := q.Cargo()
		drawCarriedResource(screen, cargo, amount, sx, sy, tilePixels, bob)

		if q.State() == quarry.StateMining {
			drawChopCue(screen, sx, sy+bob*tilePixels/TileSize, tilePixels)
		}
	}
}
