package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/resource"
)

// drawCarriedResource places the actual resource sprite just above a worker's
// hands. The map-scale marker identifies a delivery while the profession sprite
// remains visible beneath it.
func drawCarriedResource(screen *ebiten.Image, kind resource.Type, amount int, sx, sy, tilePixels, bob float64) {
	if amount <= 0 {
		return
	}
	if image := assets.ResourceCarry(kind); image != nil {
		bounds := image.Bounds()
		size := tilePixels * 0.48
		scale := size / float64(bounds.Dx())
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(sx+tilePixels*0.49, sy+(1.5+bob)*tilePixels/TileSize)
		op.Blend = ebiten.BlendSourceOver
		screen.DrawImage(image, op)
	} else {
		// A newly added resource may precede its art file for one development
		// run. Retain a visible fallback instead of making cargo disappear.
		size := 5 * tilePixels / TileSize
		vector.FillRect(screen, float32(sx+17*tilePixels/TileSize), float32(sy+(3+bob)*tilePixels/TileSize), float32(size), float32(size), cargoColor(kind), false)
	}

	// Two tiny quantity ticks distinguish the occasional two-unit harvest from
	// a single item without turning moving characters into numerical labels.
	if amount > 1 {
		tickColor := color.RGBA{R: 255, G: 238, B: 155, A: 240}
		mark := float32(maxPixel(tilePixels / TileSize))
		vector.FillRect(screen, float32(sx+tilePixels*0.75), float32(sy+(4+bob)*tilePixels/TileSize), mark, mark, tickColor, false)
		if amount > 2 {
			vector.FillRect(screen, float32(sx+tilePixels*0.82), float32(sy+(4+bob)*tilePixels/TileSize), mark, mark, tickColor, false)
		}
	}
}
