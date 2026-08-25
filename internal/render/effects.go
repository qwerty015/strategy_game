package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// drawCropGrowth adds a few deliberately chunky crop marks on top of the
// fertile texture. Growth follows the Farm production cycle: early stages
// show sprouts, while mature wheat gets golden stalks that sway by one pixel.
func drawCropGrowth(screen *ebiten.Image, sx, sy float64, growth float32, tileX, tileY int) {
	if growth < 0.12 {
		return
	}

	seed := tileX*11 + tileY*17
	phase := (animFrame/8 + seed) & 1
	count := 1
	if growth >= 0.38 {
		count = 2
	}
	if growth >= 0.7 {
		count = 4
	}

	for i := 0; i < count; i++ {
		px := sx + float64(5+(seed+i*7)%13)
		baseY := sy + float64(20-(seed+i*3)%4)
		height := float64(3 + int(growth*5))
		sway := float64(0)
		if growth >= 0.7 {
			sway = float64(phase)
		}

		stalk := color.RGBA{R: 73, G: 139, B: 49, A: 210}
		if growth >= 0.7 {
			stalk = color.RGBA{R: 227, G: 182, B: 54, A: 235}
		}
		vector.FillRect(screen, float32(px), float32(baseY-height), 1, float32(height), stalk, false)
		if growth >= 0.45 {
			vector.FillRect(screen, float32(px+sway), float32(baseY-height), 2, 1, stalk, false)
		}
	}

	// A barely visible highlight travels across mature crops, giving the
	// whole field a living shimmer without changing the underlying texture.
	if growth >= 0.7 {
		wave := math.Mod(float64(animFrame)/18+float64(seed), 18)
		vector.FillRect(screen, float32(sx+wave), float32(sy+4), 1, 1, color.RGBA{R: 255, G: 231, B: 112, A: 130}, false)
	}
}

// drawFire is a three-frame procedural hearth effect. The building sprites
// provide the oven/chimney; these small layered shapes add a readable glow
// without another atlas or a texture-loading path.
func drawFire(screen *ebiten.Image, x, y float64) {
	phase := (animFrame / 6) % 3
	heights := [...]float32{6, 8, 5}
	widths := [...]float32{4, 3, 5}
	height := heights[phase]
	width := widths[phase]

	vector.FillCircle(screen, float32(x), float32(y+2), 5, color.RGBA{R: 255, G: 115, B: 35, A: 42}, false)
	vector.FillRect(screen, float32(x)-width/2, float32(y+2)-height, width, height, color.RGBA{R: 229, G: 74, B: 25, A: 230}, false)
	vector.FillRect(screen, float32(x)-1, float32(y+1)-height/2, 2, height/2, color.RGBA{R: 255, G: 219, B: 77, A: 255}, false)
	if phase == 1 {
		vector.FillRect(screen, float32(x+2), float32(y-5), 1, 2, color.RGBA{R: 255, G: 173, B: 49, A: 220}, false)
	}
}
