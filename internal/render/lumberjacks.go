package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/lumberjack"
)

// lumberjackHeight keeps the worker readable while leaving enough of the
// tree and building silhouettes visible around him.
const lumberjackHeight = 1.08

// DrawLumberjacks renders workers while they are outside the hut. Idle and
// unloading workers are represented by the hut's compact +/- marker instead
// of being drawn on top of the roof.
// opponent -- see DrawSerfs's identical parameter doc comment.
func DrawLumberjacks(screen *ebiten.Image, jacks []*lumberjack.Lumberjack, cam *Camera, owner int) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, j := range jacks {
		if !j.VisibleOnMap() {
			continue
		}
		if !visible.Intersects(j.X, j.Y, 1) {
			continue
		}

		sx, sy := cam.TileToScreen(j.X, j.Y)
		DrawOpponentUnitMarker(screen, sx, sy, tilePixels, owner)
		frame := walkingFrame(j.RemainingPath(), j.X+j.Y)
		bob := unitBob()
		tint := color.Color(color.White)
		if j.Starving {
			tint = color.RGBA{R: 255, G: 105, B: 90, A: 255}
		}
		flip := facingLeft(j.X, j.RemainingPath())
		drawStandingFacingTintedAtScale(screen, assets.LumberjackWalkFrames[frame], sx, sy+bob*tilePixels/TileSize, lumberjackHeight, tilePixels, tint, flip)

		cargo, amount := j.Cargo()
		drawCarriedResource(screen, cargo, amount, sx, sy, tilePixels, bob)

		if j.State() == lumberjack.StateChopping {
			drawChopCue(screen, sx, sy+bob*tilePixels/TileSize, tilePixels)
		}
	}
}

func drawChopCue(screen *ebiten.Image, sx, sy, tilePixels float64) {
	scale := tilePixels / TileSize
	phase := (animFrame / 6) % 2
	if phase == 0 {
		return
	}
	// A few warm pixels and a short diagonal stroke give the small sprite a
	// readable axe/work gesture without requiring separate action frames.
	vector.FillRect(screen, float32(sx+20*scale), float32(sy+11*scale), float32(maxPixel(scale)), float32(4*scale), color.RGBA{R: 222, G: 180, B: 75, A: 230}, false)
	vector.FillRect(screen, float32(sx+23*scale), float32(sy+8*scale), float32(2*scale), float32(maxPixel(scale)), color.RGBA{R: 170, G: 185, B: 196, A: 235}, false)
}
