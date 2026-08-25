package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
	"strategy_game/internal/villagers"
)

// DrawVillagers renders every Farmer/Baker at its current tile. Idle
// (working at home) villagers hold a single pose; ones out walking to
// or from the Tavern use the profession's frame-array hook. A starving villager
// (see villagers.Villager.Starving) is tinted red so the player can see
// at a glance which building's worker needs a Tavern reachable.
func DrawVillagers(screen *ebiten.Image, vills []*villagers.Villager, cam *Camera) {
	tilePixels := cam.TilePixels()
	for _, v := range vills {
		// A villager at its workplace is represented by the building's
		// small green/red worker marker. Draw the person only while walking
		// to the Tavern or back, so workers do not visually sit on roofs.
		if v.Working() {
			continue
		}
		frames := assets.Farmer
		if v.Profession == villagers.Baker {
			frames = assets.Baker
		}

		sx, sy := cam.TileToScreen(v.X, v.Y)

		frame := 0
		bob := 0.0
		tint := color.Color(color.White)
		switch {
		case v.Starving:
			tint = color.RGBA{R: 255, G: 90, B: 90, A: 255}
		case !v.Working():
			frame = (animFrame / 10) % len(frames)
			bob = unitBob()
		}
		drawStandingTintedAtScale(screen, frames[frame], sx, sy+bob*tilePixels/TileSize, serfHeight, tilePixels, tint)
	}
}
