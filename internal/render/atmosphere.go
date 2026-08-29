package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/world"
)

// Atmospheric timing deliberately follows render frames rather than simulation
// ticks. Pausing or changing the economy speed never makes rain or clouds
// jump, and this entire layer remains outside save data.
const (
	atmosphereWeatherCycleFrames = 7200
	atmosphereRainStartFrame     = 4700
	atmosphereRainEndFrame       = 5750
)

// DrawAtmosphericOverlay paints a small, bounded finishing layer over the map:
// occasional rain, rain ripples and a darkening wash while it rains. It never
// walks the full grid or draws over the fixed UI side panels.
//
// Per the user's explicit request ("убери смену освещения, пусть всегда
// будет статично и только при дожде - темнее"), lighting no longer cycles
// through the day at all -- the map stays at a constant, always-daylight
// tone, and rain is the *only* thing that darkens it (see drawRainShade).
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
}

// atmosphericTwilight is permanently frozen at 0 (constant daylight, no
// evening tint) per the user's request above. Kept as a function rather than
// deleted because internal/render/ambient.go still gates butterflies/
// fireflies on it: with no evening phase, butterflies (daytime-only) always
// show and fireflies (evening-only) never do, which is the correct outcome
// of "no more time-of-day" rather than a bug.
func atmosphericTwilight() float64 {
	return 0
}

func atmosphericRain() (raining bool, phase int) {
	phase = animFrame % atmosphereWeatherCycleFrames
	return phase >= atmosphereRainStartFrame && phase < atmosphereRainEndFrame, phase
}

func drawRain(screen *ebiten.Image, cam *Camera, viewX, viewY, viewWidth, viewHeight, phase int) {
	tilePixels := cam.TilePixels()
	cycle := animFrame / atmosphereWeatherCycleFrames
	lineColor := color.RGBA{R: 196, G: 197, B: 182, A: 156}
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
		vector.StrokeCircle(screen, cx, cy, radius, maxPixel(tilePixels*0.030), color.RGBA{R: 203, G: 204, B: 188, A: 150}, true)
	}
}

func drawRainShade(screen *ebiten.Image, viewX, viewY, viewWidth, viewHeight int) {
	// A neutral warm wash gives rain a cloudy, wet atmosphere without turning
	// the entire settlement blue.
	//
	// Real bug the user caught: at the old alpha (24, ~9% opacity) this
	// wash was too faint to read as darkening at all -- the 120 pale
	// raindrop streaks in drawRain (color ~196,197,182 at alpha 156) and
	// the pale ripple rings in drawRainRipples visually dominated instead,
	// so the net impression during rain was a *lighter* map, backwards
	// from wet ground actually getting darker. This wash is drawn last
	// (see DrawAtmosphericOverlay), on top of the streaks and ripples, so
	// raising its alpha is enough to make it the dominant, correctly
	// darkening effect without touching the streaks/ripples themselves.
	vector.FillRect(screen, float32(viewX), float32(viewY), float32(viewWidth), float32(viewHeight), color.RGBA{R: 66, G: 58, B: 45, A: 92}, false)
}
