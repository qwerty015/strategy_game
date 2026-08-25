package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/building"
)

// buildingColor returns a flat placeholder color per building kind,
// distinct from the terrain colors in draw.go. Replaced by real sprites
// once art is added (see AGENTS.md); nothing else changes when it is.
func buildingColor(k building.Kind) color.RGBA {
	switch k {
	case building.Farm:
		return color.RGBA{R: 214, G: 178, B: 74, A: 255} // wheat gold
	case building.Mill:
		return color.RGBA{R: 200, G: 200, B: 205, A: 255} // stone grey
	case building.Bakery:
		return color.RGBA{R: 170, G: 96, B: 56, A: 255} // brick brown
	default:
		return color.RGBA{R: 200, G: 40, B: 200, A: 255} // unmistakable placeholder
	}
}

// DrawBuildings renders every placed building as a flat-colored square
// covering its footprint, plus a thin bar showing production progress.
func DrawBuildings(screen *ebiten.Image, buildings []*building.Building, cam *Camera) {
	for _, b := range buildings {
		bt := building.Types[b.Kind]
		sx, sy := cam.TileToScreen(b.X, b.Y)
		size := float32(bt.Footprint * TileSize)

		vector.FillRect(screen, float32(sx), float32(sy), size-1, size-1, buildingColor(b.Kind), false)

		if bt.Recipe.TicksToProduce > 0 {
			progress := float32(b.ProgressTicks) / float32(bt.Recipe.TicksToProduce)
			if progress > 1 {
				progress = 1
			}
			barY := float32(sy) + size - 3
			vector.FillRect(screen, float32(sx), barY, size*progress, 3, color.RGBA{R: 255, G: 255, B: 0, A: 220}, false)
		}
	}
}
