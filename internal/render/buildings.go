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
		return ripeWheatColor // overridden per-tick by growth color below; kept as a sane fallback
	case building.Mill:
		return color.RGBA{R: 200, G: 200, B: 205, A: 255} // stone grey
	case building.Bakery:
		return color.RGBA{R: 170, G: 96, B: 56, A: 255} // brick brown
	case building.Warehouse:
		return color.RGBA{R: 140, G: 60, B: 50, A: 255} // dark red, unmistakable hub
	case building.Road:
		return color.RGBA{R: 196, G: 172, B: 132, A: 255} // packed dirt path
	default:
		return color.RGBA{R: 200, G: 40, B: 200, A: 255} // unmistakable placeholder
	}
}

var (
	soilColor      = color.RGBA{R: 92, G: 66, B: 38, A: 255}   // freshly tilled earth
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

// DrawBuildings renders every placed building as a flat-colored square
// covering its footprint, plus a thin bar showing production progress.
// A Farm's color instead sweeps from bare soil to golden wheat as its
// crop matures, so the player can actually see it ripening.
func DrawBuildings(screen *ebiten.Image, buildings []*building.Building, cam *Camera) {
	for _, b := range buildings {
		bt := building.Types[b.Kind]
		sx, sy := cam.TileToScreen(b.X, b.Y)
		size := float32(bt.Footprint * TileSize)

		c := buildingColor(b.Kind)
		if b.Kind == building.Farm && bt.Recipe.TicksToProduce > 0 {
			growth := float32(b.ProgressTicks) / float32(bt.Recipe.TicksToProduce)
			c = lerpColor(soilColor, ripeWheatColor, growth)
		}

		vector.FillRect(screen, float32(sx), float32(sy), size-1, size-1, c, false)

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
