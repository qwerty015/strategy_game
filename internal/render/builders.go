package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
	"strategy_game/internal/builder"
)

// builderHeight matches every other free-roaming worker's readable scale.
const builderHeight = 1.08

// DrawBuilders renders every builder. Unlike a hut-based profession, a
// builder has no interior to hide inside while idle (see
// builder.Builder.VisibleOnMap), so every one of them is always drawn.
// opponent -- see DrawSerfs's identical parameter doc comment.
func DrawBuilders(screen *ebiten.Image, builders []*builder.Builder, cam *Camera, opponent bool) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, b := range builders {
		if !visible.Intersects(b.X, b.Y, 1) {
			continue
		}
		sx, sy := cam.TileToScreen(b.X, b.Y)
		if opponent {
			DrawOpponentUnitMarker(screen, sx, sy, tilePixels)
		}
		frame := walkingFrame(b.RemainingPath(), b.X+b.Y)
		bob := unitBob()
		tint := color.Color(color.White)
		if b.Starving {
			tint = color.RGBA{R: 255, G: 105, B: 90, A: 255}
		}
		flip := facingLeft(b.X, b.RemainingPath())
		drawStandingFacingTintedAtScale(screen, assets.BuilderWalkFrames[frame], sx, sy+bob*tilePixels/TileSize, builderHeight, tilePixels, tint, flip)

		switch b.State() {
		case builder.StateFoundation:
			drawBuilderWorkCue(screen, sx, sy+bob*tilePixels/TileSize, tilePixels, false)
		case builder.StateFinishing:
			drawBuilderWorkCue(screen, sx, sy+bob*tilePixels/TileSize, tilePixels, true)
		}
	}
}
