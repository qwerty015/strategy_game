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
// a dedicated three-frame walking atlas; an idle one is dimmed.
func DrawSerfs(screen *ebiten.Image, serfs []*logistics.Serf, cam *Camera) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, s := range serfs {
		if !visible.Intersects(s.X, s.Y, 1) {
			continue
		}
		sx, sy := cam.TileToScreen(s.X, s.Y)

		frame := walkingFrame(s.RemainingPath(), s.X+s.Y)
		bob := 0.0
		tint := color.Color(color.RGBA{R: 200, G: 200, B: 200, A: 180})
		if s.Starving {
			tint = color.RGBA{R: 255, G: 115, B: 95, A: 255}
		}
		if s.Busy() {
			bob = unitBob()
			if !s.Starving {
				tint = color.White
			}
		}
		flip := facingLeft(s.X, s.RemainingPath())
		drawStandingFacingTintedAtScale(screen, assets.SerfWalkFrames[frame], sx, sy+bob*tilePixels/TileSize, serfHeight, tilePixels, tint, flip)

		// A tiny resource badge makes the logistics simulation readable on the
		// map itself: the player can see that this is a loaded serf before
		// opening the inspector. The badge uses a stable color per resource,
		// not text, so it remains legible at the game's small tile scale.
		cargo, amount := s.Cargo()
		if amount > 0 {
			badge := 5 * tilePixels / TileSize
			vector.FillRect(screen, float32(sx+17*tilePixels/TileSize), float32(sy+(3+bob)*tilePixels/TileSize), float32(badge), float32(badge), cargoColor(cargo), false)
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
	case resource.Fish:
		return color.RGBA{R: 72, G: 156, B: 207, A: 255}
	case resource.Wine:
		return color.RGBA{R: 146, G: 67, B: 147, A: 255}
	case resource.Sausage:
		return color.RGBA{R: 186, G: 86, B: 58, A: 255}
	case resource.Log:
		return color.RGBA{R: 139, G: 89, B: 43, A: 255}
	case resource.StoneBlock:
		return color.RGBA{R: 150, G: 150, B: 150, A: 255}
	case resource.Coal:
		return color.RGBA{R: 45, G: 45, B: 45, A: 255}
	case resource.GoldOre, resource.Gold:
		return color.RGBA{R: 212, G: 175, B: 55, A: 255}
	case resource.IronOre, resource.Iron:
		return color.RGBA{R: 165, G: 124, B: 96, A: 255}
	default:
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
}
