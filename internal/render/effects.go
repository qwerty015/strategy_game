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
func drawCropGrowth(screen *ebiten.Image, sx, sy float64, growth float32, tileX, tileY int, tilePixels float64) {
	if growth < 0.12 {
		return
	}

	seed := tileX*11 + tileY*17
	scale := tilePixels / TileSize
	phase := (animFrame/8 + seed) & 1
	count := 1
	if growth >= 0.38 {
		count = 2
	}
	if growth >= 0.7 {
		count = 4
	}

	for i := 0; i < count; i++ {
		px := sx + float64(5+(seed+i*7)%13)*scale
		baseY := sy + float64(20-(seed+i*3)%4)*scale
		height := float64(3+int(growth*5)) * scale
		sway := float64(0)
		if growth >= 0.7 {
			sway = float64(phase)
		}

		stalk := color.RGBA{R: 73, G: 139, B: 49, A: 210}
		if growth >= 0.7 {
			stalk = color.RGBA{R: 227, G: 182, B: 54, A: 235}
		}
		stalkWidth := math.Max(1, scale)
		vector.FillRect(screen, float32(px), float32(baseY-height), float32(stalkWidth), float32(height), stalk, false)
		if growth >= 0.45 {
			vector.FillRect(screen, float32(px+sway*scale), float32(baseY-height), float32(2*scale), float32(scale), stalk, false)
		}
	}

	// A barely visible highlight travels across mature crops, giving the
	// whole field a living shimmer without changing the underlying texture.
	if growth >= 0.7 {
		wave := math.Mod(float64(animFrame)/18+float64(seed), 18) * scale
		vector.FillRect(screen, float32(sx+wave), float32(sy+4*scale), float32(scale), float32(scale), color.RGBA{R: 255, G: 231, B: 112, A: 130}, false)
	}
}

// drawFire is a three-frame procedural hearth effect. The building sprites
// provide the oven/chimney; these small layered shapes add a readable glow
// without another atlas or a texture-loading path.
func drawFire(screen *ebiten.Image, x, y, tilePixels float64) {
	scale := float32(tilePixels / TileSize)
	phase := (animFrame / 6) % 3
	heights := [...]float32{6, 8, 5}
	widths := [...]float32{4, 3, 5}
	height := heights[phase] * scale
	width := widths[phase] * scale

	vector.FillCircle(screen, float32(x), float32(y+2*float64(scale)), 5*scale, color.RGBA{R: 255, G: 115, B: 35, A: 42}, false)
	vector.FillRect(screen, float32(x)-width/2, float32(y+2*float64(scale))-height, width, height, color.RGBA{R: 229, G: 74, B: 25, A: 230}, false)
	vector.FillRect(screen, float32(x)-scale, float32(y+float64(scale))-height/2, 2*scale, height/2, color.RGBA{R: 255, G: 219, B: 77, A: 255}, false)
	if phase == 1 {
		vector.FillRect(screen, float32(x+2*float64(scale)), float32(y-5*float64(scale)), scale, 2*scale, color.RGBA{R: 255, G: 173, B: 49, A: 220}, false)
	}
}
