package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/building"
)

// buildingHeight is how tall (in tiles) a standing building sprite is
// drawn -- deliberately more than 1, so it rises above its single-tile
// footprint the way the source art (see AGENTS.md) is meant to be used,
// rather than being squashed to fit exactly on its tile.
const buildingHeight = 1.7

// animFrame drives simple pseudo-animation (the mill's rotating sails,
// serfs' walk poses): a slowly-advancing counter shared by everything
// drawn this frame. Good enough for a handful of small looping
// animations; a per-entity clock would only matter if they needed to be
// out of sync with each other, which nothing here does.
var animFrame int

// Tick advances the shared animation clock by one render frame. Call
// once per Draw.
func Tick() {
	animFrame++
}

var (
	soilColor      = color.RGBA{R: 92, G: 66, B: 38, A: 255}   // freshly tilled earth (tints assets.Fertile)
	ripeWheatColor = color.RGBA{R: 231, G: 196, B: 84, A: 255} // golden, ready to harvest
)

// lerpColor blends from a to b as t goes from 0 to 1, clamped.
func lerpColor(a, b color.RGBA, t float32) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	lerp := func(a, b uint8) uint8 {
		return uint8(float32(a) + (float32(b)-float32(a))*t)
	}
	return color.RGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: 255}
}

// DrawBuildings renders every placed building.
//
// A Farm is its house sprite standing on one corner of its footprint,
// with tilled field on the rest of it -- the field's color sweeps from
// bare earth to golden wheat as the crop matures (see AGENTS.md), so the
// field itself shows the growth the player asked to be able to see. A
// Road is one path tile. Mill/Bakery/Warehouse/Tavern each stand on
// their single tile taller than the tile itself (see buildingHeight),
// with a production-progress bar underneath. The Mill's sails rotate
// through three frames.
func DrawBuildings(screen *ebiten.Image, buildings []*building.Building, cam *Camera) {
	for _, b := range buildings {
		bt := building.Types[b.Kind]
		sx, sy := cam.TileToScreen(b.X, b.Y)

		switch b.Kind {
		case building.Road:
			drawStanding(screen, assets.Road, sx, sy, 1)
			continue

		case building.Farm:
			growth := float32(1)
			if bt.Recipe.TicksToProduce > 0 {
				growth = float32(b.ProgressTicks) / float32(bt.Recipe.TicksToProduce)
			}
			tint := lerpColor(soilColor, ripeWheatColor, growth)
			for dy := range bt.Footprint {
				for dx := range bt.Footprint {
					if dx == 0 && dy == 0 {
						continue // this corner is the farmhouse, drawn below
					}
					drawStandingTinted(screen, assets.Fertile, sx+float64(dx*TileSize), sy+float64(dy*TileSize), 1, tint)
				}
			}
			drawStanding(screen, assets.FarmHouse, sx, sy, buildingHeight)

		case building.Mill:
			drawStanding(screen, assets.MillFrames[(animFrame/12)%len(assets.MillFrames)], sx, sy, buildingHeight)

		case building.Bakery:
			drawStanding(screen, assets.Bakery, sx, sy, buildingHeight)

		case building.Warehouse:
			drawStanding(screen, assets.Warehouse, sx, sy, buildingHeight)

		case building.Tavern:
			drawStanding(screen, assets.Tavern, sx, sy, buildingHeight)
		}

		if bt.Recipe.TicksToProduce > 0 {
			progress := float32(b.ProgressTicks) / float32(bt.Recipe.TicksToProduce)
			if progress > 1 {
				progress = 1
			}
			barWidth := float32(bt.Footprint) * TileSize
			barY := float32(sy) + float32(bt.Footprint)*TileSize - 3
			vector.FillRect(screen, float32(sx), barY, barWidth*progress, 3, color.RGBA{R: 255, G: 255, B: 0, A: 220}, false)
		}
	}
}
