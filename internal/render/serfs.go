package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
	"strategy_game/internal/logistics"
	"strategy_game/internal/resource"
)

// serfHeight: less than a full tile, since these are small figures, not
// buildings -- see buildingHeight in buildings.go for the same idea
// applied to buildings.
const serfHeight = 1.05

// DrawSerfs renders every serf as a small figure at its current tile, so
// the player can actually see goods being hauled along the road network
// instead of resources just teleporting between buildings. A busy serf uses
// a dedicated three-frame walking atlas; an idle one is dimmed.
//
// owner marks every serf in this call as belonging to that faction, not
// the player -- see DrawOpponentUnitMarker's doc comment. cmd/game's
// Draw calls this once per faction in a duel match: owner 0 for the
// player's own g.logi.Serfs (unchanged, no marker), then once per bot's
// own f.logi.Serfs (owner = f.owner).
func DrawSerfs(screen *ebiten.Image, serfs []*logistics.Serf, cam *Camera, owner int) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, s := range serfs {
		if !visible.Intersects(s.X, s.Y, 1) {
			continue
		}
		sx, sy := cam.TileToScreen(s.X, s.Y)
		DrawOpponentUnitMarker(screen, sx, sy, tilePixels, owner)

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

		// Cargo() is populated while a job is planned, too. Only the drop-off
		// leg means the serf has physically collected the item and should show
		// it on the map.
		cargo, amount := s.Cargo()
		if s.State() == logistics.SerfToDropoff {
			drawCarriedResource(screen, cargo, amount, sx, sy, tilePixels, bob)
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
