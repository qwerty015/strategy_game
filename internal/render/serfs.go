package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/logistics"
	"strategy_game/internal/resource"
)

// serfHeight: less than a full tile, since these are small figures, not
// buildings -- see buildingHeight in buildings.go for the same idea
// applied to buildings.
const serfHeight = 0.85

// DrawSerfs renders every serf as a small figure at its current tile, so
// the player can actually see goods being hauled along the road network
// instead of resources just teleporting between buildings. A busy serf uses
// the frame-array hook in assets.Serf (the current generated pass has one
// consistent pose); an idle one is dimmed.
func DrawSerfs(screen *ebiten.Image, serfs []*logistics.Serf, cam *Camera) {
	for _, s := range serfs {
		sx, sy := cam.TileToScreen(s.X, s.Y)

		frame := 0
		bob := 0.0
		tint := color.Color(color.RGBA{R: 200, G: 200, B: 200, A: 180})
		if s.Starving {
			tint = color.RGBA{R: 255, G: 115, B: 95, A: 255}
		}
		if s.Busy() {
			frame = (animFrame / 10) % len(assets.Serf)
			bob = unitBob()
			if !s.Starving {
				tint = color.White
			}
		}
		drawStandingTinted(screen, assets.Serf[frame], sx, sy+bob, serfHeight, tint)

		// A tiny resource badge makes the logistics simulation readable on the
		// map itself: the player can see that this is a loaded serf before
		// opening the inspector. The badge uses a stable color per resource,
		// not text, so it remains legible at the game's small tile scale.
		cargo, amount := s.Cargo()
		if amount > 0 {
			vector.FillRect(screen, float32(sx+17), float32(sy+3+bob), 5, 5, cargoColor(cargo), false)
		}
	}
}

func cargoColor(t resource.Type) color.RGBA {
	switch t {
	case resource.Wheat:
		return color.RGBA{R: 245, G: 207, B: 74, A: 255}
	case resource.Flour:
		return color.RGBA{R: 239, G: 226, B: 183, A: 255}
	case resource.Bread:
		return color.RGBA{R: 177, G: 103, B: 46, A: 255}
	default:
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
}
