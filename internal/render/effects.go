package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/building"
)

// drawCropGrowth turns the eight Farm plots from a quiet texture cue into a
// readable crop. Young plots retain sparse green sprouts; the final third
// becomes a dense set of tall, two-tone golden ears that can be recognized at
// ordinary zoom as well as when the player zooms in.
func drawCropGrowth(screen *ebiten.Image, sx, sy float64, growth float32, tileX, tileY int, tilePixels float64) {
	if growth < 0.12 {
		return
	}

	seed := tileX*11 + tileY*17
	scale := tilePixels / TileSize
	phase := (animFrame/8 + seed) & 1
	count := 2
	if growth >= 0.38 {
		count = 4
	}
	if growth >= 0.7 {
		count = 7
	}

	for i := 0; i < count; i++ {
		px := sx + float64(3+(seed+i*7)%17)*scale
		baseY := sy + float64(21-(seed+i*3)%5)*scale
		height := float64(3+int(growth*7)) * scale
		sway := float64(0)
		if growth >= 0.7 {
			sway = float64((phase+i)&1) * scale
		}

		stalk := color.RGBA{R: 73, G: 139, B: 49, A: 220}
		if growth >= 0.7 {
			stalk = color.RGBA{R: 195, G: 139, B: 40, A: 255}
		}
		stalkWidth := math.Max(1, scale)
		vector.FillRect(screen, float32(px+sway), float32(baseY-height), float32(stalkWidth), float32(height), stalk, false)
		if growth >= 0.42 {
			leafColor := color.RGBA{R: 88, G: 156, B: 51, A: 220}
			if growth >= 0.7 {
				leafColor = color.RGBA{R: 222, G: 171, B: 51, A: 255}
			}
			vector.FillRect(screen, float32(px+sway-2*scale), float32(baseY-height/2), float32(3*scale), float32(maxPixel(scale)), leafColor, false)
		}
		if growth >= 0.7 {
			// The ear is deliberately wider and brighter than the stalk: the
			// field reads as harvest-ready instead of merely yellow grass.
			earX := px + sway - scale
			earY := baseY - height - scale
			vector.FillRect(screen, float32(earX), float32(earY), float32(3*scale), float32(3*scale), color.RGBA{R: 239, G: 190, B: 57, A: 255}, false)
			vector.FillRect(screen, float32(earX+scale), float32(earY), float32(maxPixel(scale)), float32(3*scale), color.RGBA{R: 255, G: 227, B: 107, A: 255}, false)
		}
	}

	if growth >= 0.7 {
		wave := math.Mod(float64(animFrame)/16+float64(seed), 16) * scale
		vector.FillRect(screen, float32(sx+3*scale+wave), float32(sy+5*scale), float32(2*scale), float32(maxPixel(scale)), color.RGBA{R: 255, G: 237, B: 137, A: 175}, false)
	}
}

// drawVineyardGrowth renders one of the eight permanent grape plots. The
// trellis stays visible from the start, but mature plots become dense with
// leafy canopies and layered purple bunches rather than tiny isolated dots.
func drawVineyardGrowth(screen *ebiten.Image, sx, sy float64, growth float32, tileX, tileY int, tilePixels float64) {
	growth = min(max(growth, 0), 1)
	scale := tilePixels / TileSize
	seed := tileX*13 + tileY*19
	postColor := color.RGBA{R: 91, G: 63, B: 33, A: 255}
	vineColor := color.RGBA{R: 43, G: 105, B: 45, A: 255}
	leafDark := color.RGBA{R: 42, G: 102, B: 43, A: 245}
	leafLight := color.RGBA{R: 90, G: 157, B: 59, A: 255}
	grapeDark := color.RGBA{R: 71, G: 30, B: 74, A: 255}
	grapeLight := color.RGBA{R: 148, G: 63, B: 137, A: 255}

	// Every plot has two complete trellis rows inside its own tile. Keeping
	// them inside the cell avoids the faint grid-like pattern made by the old
	// rows spilling into neighbouring plots.
	for row := 0; row < 2; row++ {
		y := sy + float64(9+row*11)*scale
		vector.FillRect(screen, float32(sx+3*scale), float32(y), float32(18*scale), float32(maxPixel(scale)), postColor, false)
		for col := 0; col < 3; col++ {
			x := sx + float64(5+col*7+(seed+row+col)%2)*scale
			vector.FillRect(screen, float32(x), float32(y-5*scale), float32(maxPixel(scale)), float32(6*scale), postColor, false)
			if growth < 0.08 {
				continue
			}

			vineHeight := float64(2+int(growth*5)) * scale
			vector.FillRect(screen, float32(x), float32(y-vineHeight), float32(maxPixel(scale)), float32(vineHeight), vineColor, false)
			if growth >= 0.28 {
				leafW := float64(3) * scale
				leafH := float64(2) * scale
				vector.FillRect(screen, float32(x-leafW/2), float32(y-vineHeight+scale), float32(leafW), float32(leafH), leafDark, false)
				vector.FillRect(screen, float32(x+scale/2), float32(y-vineHeight+2*scale), float32(leafW), float32(leafH), leafLight, false)
			}
			if growth >= 0.68 && (row+col+seed)%2 == 0 {
				// Three overlapping berries make a recognisable bunch even at
				// normal zoom, with a lighter berry as a tiny highlight.
				bunchX := x + 2*scale
				bunchY := y - vineHeight + 4*scale
				radius := float32(math.Max(1, scale))
				vector.FillCircle(screen, float32(bunchX), float32(bunchY), radius, grapeDark, false)
				vector.FillCircle(screen, float32(bunchX+2*scale), float32(bunchY), radius, grapeDark, false)
				vector.FillCircle(screen, float32(bunchX+scale), float32(bunchY+2*scale), radius, grapeLight, false)
			}
		}
	}

	if growth >= 0.68 {
		wave := float64((animFrame/14+seed)%14) * scale
		vector.FillRect(screen, float32(sx+3*scale+wave), float32(sy+4*scale), float32(2*scale), float32(maxPixel(scale)), color.RGBA{R: 186, G: 223, B: 114, A: 150}, false)
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

// drawProductionWorkEffect adds sparse dust, sawdust or sparks only while a
// compatible building has actual production progress. It is visual feedback,
// not a worker, and draws at most three tiny particles per visible building.
func drawProductionWorkEffect(screen *ebiten.Image, kind building.Kind, progress int, sx, sy, tilePixels float64) {
	if progress <= 0 {
		return
	}
	var particle color.RGBA
	switch kind {
	case building.CarpentryWorkshop:
		particle = color.RGBA{R: 231, G: 195, B: 113, A: 188}
	case building.MeatWorkshop:
		particle = color.RGBA{R: 219, G: 155, B: 88, A: 150}
	case building.QuarryHut, building.MinerHut:
		particle = color.RGBA{R: 182, G: 177, B: 160, A: 176}
	case building.Smeltery:
		particle = color.RGBA{R: 255, G: 170, B: 64, A: 205}
	default:
		return
	}
	scale := tilePixels / TileSize
	phase := (animFrame / 5) % 4
	for index := 0; index < 4; index++ {
		x := sx + float64(9+(index*6+phase*4)%19)*scale
		y := sy + float64(15-(index+phase)%4*3)*scale
		size := maxPixel(scale) * 1.65
		vector.FillRect(screen, float32(x)-size/2, float32(y)-size/2, size, size, particle, false)
	}
}
