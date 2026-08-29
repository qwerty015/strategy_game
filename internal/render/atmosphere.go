package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/world"
)

// Atmospheric timing deliberately follows render frames rather than simulation
// ticks. Pausing or changing the economy speed never makes rain, clouds or
// evening lighting jump, and this entire layer remains outside save data.
const (
	atmosphereWeatherCycleFrames = 7200
	atmosphereRainStartFrame     = 4700
	atmosphereRainEndFrame       = 5750
	atmosphereLightCycleFrames   = 18000
)

// DrawAtmosphericOverlay paints a small, bounded finishing layer over the map:
// slow cloud shadows, occasional rain, rain ripples and a restrained evening
// tint. It never walks the full grid or draws over the fixed UI side panels.
func DrawAtmosphericOverlay(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	viewX, viewY, viewWidth, viewHeight := cam.Viewport()
	if viewWidth <= 0 || viewHeight <= 0 {
		return
	}

	if raining, phase := atmosphericRain(); raining {
		drawRain(screen, cam, viewX, viewY, viewWidth, viewHeight, phase)
		drawRainRipples(screen, g, cam, phase)
		drawRainShade(screen, viewX, viewY, viewWidth, viewHeight)
	}
	drawEveningTint(screen, viewX, viewY, viewWidth, viewHeight)
}

// atmosphericTwilight varies smoothly only through a late-afternoon/evening
// segment of a long visual cycle. Daylight otherwise stays unfiltered so
// resource colours and production warnings remain easy to read.
func atmosphericTwilight() float64 {
	phase := float64(animFrame%atmosphereLightCycleFrames) / atmosphereLightCycleFrames
	if phase < 0.48 || phase > 0.90 {
		return 0
	}
	return math.Sin((phase - 0.48) / 0.42 * math.Pi)
}

func atmosphericRain() (raining bool, phase int) {
	phase = animFrame % atmosphereWeatherCycleFrames
	return phase >= atmosphereRainStartFrame && phase < atmosphereRainEndFrame, phase
}

func drawRain(screen *ebiten.Image, cam *Camera, viewX, viewY, viewWidth, viewHeight, phase int) {
	tilePixels := cam.TilePixels()
	cycle := animFrame / atmosphereWeatherCycleFrames
	lineColor := color.RGBA{R: 171, G: 205, B: 218, A: 156}
	for index := 0; index < 120; index++ {
		seed := ambientHash(uint32(cycle)*668265263 + uint32(index)*374761393)
		x := float64(viewX) + float64(seed%uint32(viewWidth))
		yProgress := int((seed>>12)%uint32(viewHeight)) + phase*3
		y := float64(viewY + yProgress%viewHeight)
		dx := tilePixels * 0.055
		dy := tilePixels * 0.36
		vector.StrokeLine(screen, float32(x), float32(y), float32(x-dx), float32(y+dy), maxPixel(tilePixels*0.045), lineColor, true)
	}
}

func drawRainRipples(screen *ebiten.Image, g *world.Grid, cam *Camera, phase int) {
	visible := cam.VisibleTileBounds(0)
	viewWidth := visible.MaxX - visible.MinX + 1
	viewHeight := visible.MaxY - visible.MinY + 1
	if viewWidth < 1 || viewHeight < 1 {
		return
	}
	tilePixels := cam.TilePixels()
	for index := 0; index < 28; index++ {
		seed := ambientHash(uint32(phase/18)*2246822519 + uint32(index)*40503)
		tileX := visible.MinX + int(seed%uint32(viewWidth))
		tileY := visible.MinY + int((seed>>13)%uint32(viewHeight))
		if !g.InBounds(tileX, tileY) || g.At(tileX, tileY).Terrain != world.Water {
			continue
		}
		sx, sy := cam.TileToScreen(tileX, tileY)
		cx := float32(sx + tilePixels*(0.24+0.54*float64((seed>>7)&0xff)/255))
		cy := float32(sy + tilePixels*(0.27+0.46*float64((seed>>18)&0xff)/255))
		radius := float32(tilePixels * (0.10 + 0.035*float64(index%3)))
		vector.StrokeCircle(screen, cx, cy, radius, maxPixel(tilePixels*0.030), color.RGBA{R: 184, G: 214, B: 223, A: 150}, true)
	}
}

func drawRainShade(screen *ebiten.Image, viewX, viewY, viewWidth, viewHeight int) {
	// A cool, low-alpha wash gives rain a cloudy, wet atmosphere while
	// preserving the clarity of roads, buildings and selection markers.
	vector.FillRect(screen, float32(viewX), float32(viewY), float32(viewWidth), float32(viewHeight), color.RGBA{R: 39, G: 61, B: 74, A: 31}, false)
}
func drawEveningTint(screen *ebiten.Image, viewX, viewY, viewWidth, viewHeight int) {
	twilight := atmosphericTwilight()
	if twilight <= 0 {
		return
	}
	// At its darkest the overlay is still translucent enough to keep road
	// connections and resource icons legible on the map.
	alpha := uint8(16 + int(twilight*38))
	vector.FillRect(screen, float32(viewX), float32(viewY), float32(viewWidth), float32(viewHeight), color.RGBA{R: 61, G: 43, B: 81, A: alpha}, false)
}
