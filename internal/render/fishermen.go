package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/fishing"
	"strategy_game/internal/resource"
)

const fishermanHeight = 0.9

// DrawFishermen renders the worker as a road walker on land and as a boat
// unit on water. Idle/unloading fishermen remain represented by their hut's
// compact worker marker rather than sitting on top of the roof.
func DrawFishermen(screen *ebiten.Image, fishermen []*fishing.Fisherman, cam *Camera) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, f := range fishermen {
		if !f.VisibleOnMap() {
			continue
		}
		if !visible.Intersects(f.X, f.Y, 1) {
			continue
		}
		sx, sy := cam.TileToScreen(f.X, f.Y)
		bob := unitBob()
		tint := color.Color(color.White)
		if f.Starving {
			tint = color.RGBA{R: 255, G: 105, B: 90, A: 255}
		}
		if f.InBoat() {
			drawStandingTintedAtScale(screen, assets.FishingBoat, sx, sy+bob*tilePixels/TileSize, 1.08, tilePixels, tint)
			if f.State() == fishing.StateFishing {
				drawFishingCue(screen, sx, sy+bob*tilePixels/TileSize, tilePixels)
			}
		} else {
			frame := (animFrame / 10) % len(assets.Fisherman)
			drawStandingTintedAtScale(screen, assets.Fisherman[frame], sx, sy+bob*tilePixels/TileSize, fishermanHeight, tilePixels, tint)
		}

		_, amount := f.Cargo()
		if amount > 0 {
			badge := 5 * tilePixels / TileSize
			vector.FillRect(screen, float32(sx+17*tilePixels/TileSize), float32(sy+(3+bob)*tilePixels/TileSize), float32(badge), float32(badge), cargoColor(resource.Fish), false)
		}
	}
}

func drawFishingCue(screen *ebiten.Image, sx, sy, tilePixels float64) {
	if (animFrame/8)%2 == 0 {
		return
	}
	scale := tilePixels / TileSize
	line := color.RGBA{R: 224, G: 213, B: 165, A: 220}
	vector.FillRect(screen, float32(sx+19*scale), float32(sy+9*scale), float32(maxPixel(scale)), float32(7*scale), line, false)
	vector.FillRect(screen, float32(sx+17*scale), float32(sy+15*scale), float32(5*scale), float32(maxPixel(scale)), line, false)
}
