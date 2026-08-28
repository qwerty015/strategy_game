package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/building"
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

// drawVineyardGrowth renders one of the eight permanent grape plots. The
// vines exist immediately after construction; the growth value only changes
// their height, leaf density and purple fruit. The last stage gently shimmers
// so a ripe vineyard does not look like a static checkerboard.
func drawVineyardGrowth(screen *ebiten.Image, sx, sy float64, growth float32, tileX, tileY int, tilePixels float64) {
	if growth < 0 {
		growth = 0
	}
	if growth > 1 {
		growth = 1
	}
	scale := tilePixels / TileSize
	seed := tileX*13 + tileY*19
	rowY := sy + 43*scale
	postColor := color.RGBA{R: 91, G: 63, B: 33, A: 255}
	vineColor := color.RGBA{R: 49, G: 112, B: 49, A: 235}
	leafColor := color.RGBA{R: 75, G: 143, B: 57, A: 245}
	fruitColor := color.RGBA{R: 111, G: 47, B: 101, A: 250}

	// Three low trellis rows make the crop readable even when zoomed out.
	for row := 0; row < 3; row++ {
		y := rowY - float64(row*11)*scale
		vector.FillRect(screen, float32(sx+5*scale), float32(y), float32(18*scale), float32(maxPixel(scale)), postColor, false)
		if growth < 0.08 {
			continue
		}
		vineHeight := float64(2+int(growth*8)) * scale
		for col := 0; col < 3; col++ {
			x := sx + float64(7+col*6+(seed+row+col)%2)*scale
			vector.FillRect(screen, float32(x), float32(y-vineHeight), float32(maxPixel(scale)), float32(vineHeight), vineColor, false)
			if growth >= 0.32 {
				vector.FillRect(screen, float32(x-1*scale), float32(y-vineHeight+2*scale), float32(3*scale), float32(maxPixel(scale)), leafColor, false)
			}
			if growth >= 0.68 && (row+col+seed)%2 == 0 {
				vector.FillCircle(screen, float32(x+2*scale), float32(y-vineHeight+4*scale), float32(maxPixel(scale)), fruitColor, false)
			}
		}
	}

	if growth >= 0.68 {
		wave := float64((animFrame/16+seed)%16) * scale
		vector.FillRect(screen, float32(sx+4*scale+wave), float32(sy+7*scale), float32(maxPixel(scale)), float32(maxPixel(scale)), color.RGBA{R: 167, G: 207, B: 106, A: 120}, false)
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

// drawConstructionSiteEffect keeps a site visibly alive without adding more
// animation sheets. Foundation work throws brown dust, the finishing stage
// throws pale sawdust and a paused site pulses a compact materials warning.
func drawConstructionSiteEffect(screen *ebiten.Image, stage building.ConstructionStage, sx, sy, footprint, tilePixels float64) {
	scale := tilePixels / TileSize
	size := footprint * tilePixels
	phase := (animFrame / 7) % 4

	switch stage {
	case building.ConstructionFoundation:
		for i := 0; i < 3; i++ {
			x := sx + size*(0.22+float64((i*3+phase)%5)*0.13)
			y := sy + size*(0.67-float64((i+phase)%3)*0.07)
			vector.FillCircle(screen, float32(x), float32(y), float32(maxPixel(scale)), color.RGBA{R: 181, G: 135, B: 73, A: 145}, false)
		}
	case building.ConstructionFinishing:
		for i := 0; i < 3; i++ {
			x := sx + size*(0.24+float64((i*5+phase)%6)*0.1)
			y := sy + size*(0.23+float64((i+phase)%4)*0.11)
			vector.FillRect(screen, float32(x), float32(y), float32(maxPixel(scale)), float32(maxPixel(scale)), color.RGBA{R: 237, G: 205, B: 126, A: 190}, false)
		}
	case building.ConstructionWaitingMaterials:
		pulse := float64((animFrame/12)%2) * scale
		x := sx + size - 7*scale
		y := sy + 5*scale
		vector.FillCircle(screen, float32(x), float32(y), float32(4*scale+pulse), color.RGBA{R: 129, G: 38, B: 34, A: 220}, false)
		vector.FillRect(screen, float32(x-scale/2), float32(y-2*scale), float32(maxPixel(scale)), float32(3*scale), color.RGBA{R: 255, G: 221, B: 108, A: 255}, false)
		vector.FillRect(screen, float32(x-scale/2), float32(y+2*scale), float32(maxPixel(scale)), float32(maxPixel(scale)), color.RGBA{R: 255, G: 221, B: 108, A: 255}, false)
	}
}

// drawBuilderWorkCue adds a small hammer strike and a matching dust cloud to
// the dedicated builder sprite. It deliberately differs from drawChopCue:
// lumberjacks and quarrymen swing tools at a natural resource, while a
// builder alternates between digging soil and fitting prepared materials.
func drawBuilderWorkCue(screen *ebiten.Image, sx, sy, tilePixels float64, finishing bool) {
	scale := tilePixels / TileSize
	phase := (animFrame / 5) % 3
	cueColor := color.RGBA{R: 166, G: 120, B: 64, A: 205}
	if finishing {
		cueColor = color.RGBA{R: 224, G: 193, B: 119, A: 220}
	}

	x := sx + float64(17+phase*2)*scale
	y := sy + float64(19-phase)*scale
	vector.FillRect(screen, float32(x), float32(y), float32(5*scale), float32(maxPixel(scale)), cueColor, false)
	vector.FillCircle(screen, float32(x+4*scale), float32(y+2*scale), float32(maxPixel(scale)), cueColor, false)
}
