package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
	"strategy_game/internal/builder"
)

// builderHeight matches every other free-roaming worker's readable scale.
const builderHeight = 0.88

// DrawBuilders renders every builder. Unlike a hut-based profession, a
// builder has no interior to hide inside while idle (see
// builder.Builder.VisibleOnMap), so every one of them is always drawn.
func DrawBuilders(screen *ebiten.Image, builders []*builder.Builder, cam *Camera) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, b := range builders {
		if !visible.Intersects(b.X, b.Y, 1) {
			continue
		}
		sx, sy := cam.TileToScreen(b.X, b.Y)
		frame := (animFrame / 10) % len(assets.Builder)
		bob := unitBob()
		tint := color.Color(color.White)
		if b.Starving {
			tint = color.RGBA{R: 255, G: 105, B: 90, A: 255}
		}
		drawStandingTintedAtScale(screen, assets.Builder[frame], sx, sy+bob*tilePixels/TileSize, builderHeight, tilePixels, tint)

		switch b.State() {
		case builder.StateFoundation:
			drawBuilderWorkCue(screen, sx, sy+bob*tilePixels/TileSize, tilePixels, false)
		case builder.StateFinishing:
			drawBuilderWorkCue(screen, sx, sy+bob*tilePixels/TileSize, tilePixels, true)
		}
	}
}
