package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/building"
	"strategy_game/internal/render"
	"strategy_game/internal/world"
	"strategy_game/internal/worldclock"
)

// DrawMinimapPanel renders the always-visible minimap and clock docked at
// the bottom of the right panel (see Layout.MinimapRect/ClockRect). It's a
// separate call from DrawInspectorPanel, deliberately: it doesn't depend on
// anything selected, so it draws the same regardless of what the inspector
// above it is currently showing.
func DrawMinimapPanel(screen *ebiten.Image, layout Layout, grid *world.Grid, buildings []*building.Building, cam *render.Camera) {
	drawClockRow(screen, layout.ClockRect())
	render.DrawMinimap(screen, grid, buildings, cam, layout.MinimapRect())
}

func drawClockRow(screen *ebiten.Image, r image.Rectangle) {
	cy := float64(r.Min.Y) + float64(r.Dy())/2
	drawPhaseIcon(screen, float64(r.Min.X)+11, cy, 9)
	hour := render.CurrentHour()
	h := int(hour)
	m := int((hour - float64(h)) * 60)
	label := fmt.Sprintf("%02d:%02d", h, m)
	DrawInspectorText(screen, label, float64(r.Min.X)+26, float64(r.Min.Y)+4)
	if render.IsRaining() {
		drawRainIcon(screen, float64(r.Max.X)-16, cy, 8)
	}
}

// drawPhaseIcon draws a small hand-drawn sun/moon/sunrise/sunset glyph.
// Real emoji glyphs aren't used here: the game's only font (bitmapfont,
// see text.go) doesn't cover color emoji, so a Unicode sun/moon character
// would silently render as a blank/tofu box. A few vector shapes are
// reliable regardless of font coverage and match the rest of this file's
// existing icon-free, shape-drawn UI style.
func drawPhaseIcon(screen *ebiten.Image, cx, cy, radius float64) {
	x, y, r := float32(cx), float32(cy), float32(radius)
	sun := color.RGBA{R: 250, G: 200, B: 90, A: 255}
	moon := color.RGBA{R: 210, G: 214, B: 230, A: 255}
	horizon := color.RGBA{R: 214, G: 140, B: 70, A: 255}

	switch render.CurrentDayPhase() {
	case worldclock.Day:
		vector.FillCircle(screen, x, y, r, sun, true)
		for i := 0; i < 8; i++ {
			angle := float64(i) * (3.14159 / 4)
			dx, dy := float32(cosApprox(angle))*r*1.7, float32(sinApprox(angle))*r*1.7
			vector.StrokeLine(screen, x+float32(cosApprox(angle))*r*1.15, y+float32(sinApprox(angle))*r*1.15, x+dx, y+dy, 1.5, sun, true)
		}
	case worldclock.Night:
		vector.FillCircle(screen, x, y, r, moon, true)
		vector.FillCircle(screen, x+r*0.55, y-r*0.35, r*0.85, color.RGBA{R: 30, G: 25, B: 26, A: 255}, true)
	case worldclock.Sunrise:
		vector.StrokeLine(screen, x-r*1.3, y, x+r*1.3, y, 2, horizon, true)
		vector.FillCircle(screen, x, y, r*0.85, sun, true)
		vector.StrokeLine(screen, x, y+r*1.6, x, y+r*0.5, 1.5, sun, true)
		vector.StrokeLine(screen, x-r*0.5, y+r*1.1, x, y+r*0.5, 1.5, sun, true)
		vector.StrokeLine(screen, x+r*0.5, y+r*1.1, x, y+r*0.5, 1.5, sun, true)
	case worldclock.Sunset:
		vector.StrokeLine(screen, x-r*1.3, y, x+r*1.3, y, 2, horizon, true)
		vector.FillCircle(screen, x, y, r*0.85, sun, true)
		vector.StrokeLine(screen, x, y-r*0.5, x, y-r*1.6, 1.5, sun, true)
		vector.StrokeLine(screen, x-r*0.5, y-r*1.1, x, y-r*1.6, 1.5, sun, true)
		vector.StrokeLine(screen, x+r*0.5, y-r*1.1, x, y-r*1.6, 1.5, sun, true)
	}
}

func drawRainIcon(screen *ebiten.Image, cx, cy, size float64) {
	x, y, s := float32(cx), float32(cy), float32(size)
	cloud := color.RGBA{R: 196, G: 197, B: 182, A: 255}
	drop := color.RGBA{R: 120, G: 170, B: 214, A: 255}
	vector.FillCircle(screen, x-s*0.35, y-s*0.1, s*0.4, cloud, true)
	vector.FillCircle(screen, x+s*0.15, y-s*0.25, s*0.5, cloud, true)
	vector.FillRect(screen, x-s*0.7, y-s*0.1, s*1.3, s*0.35, cloud, false)
	for i, dx := range []float32{-0.35, 0.1, 0.5} {
		_ = i
		vector.StrokeLine(screen, x+dx*s, y+s*0.4, x+dx*s-s*0.12, y+s*0.85, 1.4, drop, true)
	}
}

// cosApprox/sinApprox avoid importing math just for eight fixed sun-ray
// angles -- a tiny lookup covers exactly the directions drawPhaseIcon uses.
var sunRayCos = [8]float64{1, 0.7071, 0, -0.7071, -1, -0.7071, 0, 0.7071}
var sunRaySin = [8]float64{0, 0.7071, 1, 0.7071, 0, -0.7071, -1, -0.7071}

func cosApprox(angle float64) float64 { return sunRayCos[angleIndex(angle)] }
func sinApprox(angle float64) float64 { return sunRaySin[angleIndex(angle)] }
func angleIndex(angle float64) int {
	const step = 3.14159 / 4
	i := int(angle/step + 0.5)
	if i < 0 {
		i = 0
	}
	if i > 7 {
		i = 7
	}
	return i
}
