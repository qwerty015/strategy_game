package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/world"
	"strategy_game/internal/worldclock"
)

// Rain timing deliberately follows render frames rather than simulation
// ticks. Pausing or changing the economy speed never makes rain or clouds
// jump, and this entire layer remains outside save data.
const (
	atmosphereWeatherCycleFrames = 7200
	atmosphereRainStartFrame     = 4700
	atmosphereRainEndFrame       = 5750
)

// worldTicks is the current simulation tick count (see economy.Simulator).
// Unlike animFrame (render frames, used for rain/clouds above), the
// day/night cycle and its night-time effects follow real game time: it
// pauses and speeds up/slows down with the simulation, exactly like
// worldclock.TicksPerDay's doc comment describes. SetWorldTicks is called
// once per Draw, the same way Tick() advances animFrame.
var worldTicks int

// SetWorldTicks updates the tick count DrawAtmosphericOverlay, the
// day/night-gated ambient wildlife (see ambient.go) and the night glow on
// buildings/units all read.
func SetWorldTicks(ticks int) {
	worldTicks = ticks
}

// currentDayState is the shared single source of truth for "what time is
// it" across every render function that cares (darkening, fireflies vs.
// butterflies, the night glow).
func currentDayState() worldclock.State {
	return worldclock.At(worldTicks)
}

// Per the user's explicit requests across three messages ("убери смену
// освещения, пусть всегда будет статично"; "давай оставим затемнение
// экрана на 15% ...при дожде экран не меняем, только дождь"; "на восходе и
// закате небольшое затемнение, на 7.5%"; "карту еще темнее"): the map stays
// at a constant darkening at all times, gets an extra 7.5% specifically
// during the sunrise/sunset slivers, and rain never changes the screen tint
// at all -- only its own particles (see drawRain/drawRainRipples) are
// visible. Raised from the original 15% (38) to 20% (51) per the user's
// last round of feedback, alongside toning down the night glow itself (see
// drawNightGlow) so the two changes read as "moodier overall, softer
// lanterns" rather than just "brighter lamps against a lighter map".
const (
	baseDarkenAlpha      = 51 // 20% of 255
	goldenHourExtraAlpha = 19 // +7.5% of 255, sunrise/sunset only
)

// DrawAtmosphericOverlay paints a small, bounded finishing layer over the
// map: a constant ambient darkening (stronger during sunrise/sunset), and
// occasional rain (particles only, no tint of its own). It never walks the
// full grid or draws over the fixed UI side panels.
func DrawAtmosphericOverlay(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	viewX, viewY, viewWidth, viewHeight := cam.Viewport()
	if viewWidth <= 0 || viewHeight <= 0 {
		return
	}

	if raining, phase := atmosphericRain(); raining {
		drawRain(screen, cam, viewX, viewY, viewWidth, viewHeight, phase)
		drawRainRipples(screen, g, cam, phase)
	}
	drawAmbientDarkening(screen, viewX, viewY, viewWidth, viewHeight)
}

func drawAmbientDarkening(screen *ebiten.Image, viewX, viewY, viewWidth, viewHeight int) {
	alpha := baseDarkenAlpha
	switch currentDayState().Phase {
	case worldclock.Sunrise, worldclock.Sunset:
		alpha += goldenHourExtraAlpha
	}
	// A neutral warm wash reads as ambient dimming without turning the
	// entire settlement blue.
	vector.FillRect(screen, float32(viewX), float32(viewY), float32(viewWidth), float32(viewHeight), color.RGBA{R: 66, G: 58, B: 45, A: uint8(alpha)}, false)
}

// drawNightGlow draws a small warm lamplight glow centered at the given
// screen coordinates. Used for staffed buildings (see buildings.go) and
// every mobile unit (see each profession's Draw* function) at night, per
// the user's request ("подумай над свечением зданий и юнитов ночью, тип
// они несут лампу"). A no-op outside the Night phase.
func drawNightGlow(screen *ebiten.Image, cx, cy, tilePixels float64) {
	if currentDayState().Phase != worldclock.Night {
		return
	}
	x, y := float32(cx), float32(cy)
	// Three soft rings falling off from a dim amber core, instead of one
	// bright, pale disc. Real bug the user caught ("слишком яркий, прям
	// белый шар"): the old core (255,224,160 @ alpha 85, radius 0.22) was
	// light and opaque enough to read as a solid near-white ball. The core
	// here is smaller, dimmer and warmer (more orange, less white); the
	// wider two rings replace that lost brightness as a softer spread
	// around the unit instead of concentrating it at the center.
	vector.FillCircle(screen, x, y, float32(tilePixels*0.85), color.RGBA{R: 255, G: 180, B: 90, A: 22}, true)
	vector.FillCircle(screen, x, y, float32(tilePixels*0.5), color.RGBA{R: 255, G: 190, B: 100, A: 34}, true)
	vector.FillCircle(screen, x, y, float32(tilePixels*0.16), color.RGBA{R: 255, G: 205, B: 130, A: 60}, true)
}

func atmosphericRain() (raining bool, phase int) {
	phase = animFrame % atmosphereWeatherCycleFrames
	return phase >= atmosphereRainStartFrame && phase < atmosphereRainEndFrame, phase
}

// IsRaining reports whether it's currently raining. Exposed for the UI
// clock panel (see internal/ui), which shows a rain icon alongside the
// sun/moon/sunrise/sunset phase icon while it's raining.
func IsRaining() bool {
	raining, _ := atmosphericRain()
	return raining
}

// CurrentDayPhase exposes the day/night phase (see internal/worldclock) for
// the UI clock panel.
func CurrentDayPhase() worldclock.Phase {
	return currentDayState().Phase
}

// CurrentHour exposes the in-game hour (0..24) for the UI clock panel.
func CurrentHour() float64 {
	return currentDayState().Hour
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
