package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/sentry"
)

// DrawSentryProjectiles paints the short-lived visual trail created by a
// successful WatchTower shot. Projectile lifetime and target coordinates are
// owned by package sentry; this function converts that read-only state to a
// small arc in screen space and has no gameplay side effects.
func DrawSentryProjectiles(screen *ebiten.Image, sentries []*sentry.Sentry, cam *Camera) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(2)
	for _, guard := range sentries {
		fromX, fromY, targetX, targetY, progress, ok := guard.ShotVisual()
		if !ok || (!visible.Intersects(fromX, fromY, 1) && !visible.Intersects(targetX, targetY, 1)) {
			continue
		}
		drawSlingStone(screen, cam, fromX, fromY, targetX, targetY, progress, tilePixels)
	}
}

func drawSlingStone(screen *ebiten.Image, cam *Camera, fromX, fromY, targetX, targetY int, progress, tilePixels float64) {
	fromSX, fromSY := cam.TileToScreen(fromX, fromY)
	toSX, toSY := cam.TileToScreen(targetX, targetY)
	startX, startY := fromSX+tilePixels/2, fromSY-tilePixels*0.86
	endX, endY := toSX+tilePixels/2, toSY+tilePixels*0.47

	// A short parabola gives the sling stone a readable flight path at normal
	// zoom without looking like artillery across the tower's two-tile range.
	arc := tilePixels * 0.42 * 4 * progress * (1 - progress)
	x := startX + (endX-startX)*progress
	y := startY + (endY-startY)*progress - arc
	radius := float32(tilePixels * 0.065)
	if radius < 1.5 {
		radius = 1.5
	}

	// Two fading points make the motion legible without allocating particles.
	for step := 2; step >= 1; step-- {
		trailProgress := progress - float64(step)*0.09
		if trailProgress <= 0 {
			continue
		}
		trailArc := tilePixels * 0.42 * 4 * trailProgress * (1 - trailProgress)
		tx := startX + (endX-startX)*trailProgress
		ty := startY + (endY-startY)*trailProgress - trailArc
		alpha := uint8(70 + (2-step)*30)
		vector.FillCircle(screen, float32(tx), float32(ty), radius*0.56, color.RGBA{R: 167, G: 151, B: 127, A: alpha}, false)
	}
	vector.FillCircle(screen, float32(x), float32(y), radius, color.RGBA{R: 81, G: 76, B: 70, A: 255}, false)
	vector.FillCircle(screen, float32(x)-radius*0.24, float32(y)-radius*0.24, radius*0.38, color.RGBA{R: 201, G: 193, B: 169, A: 255}, false)
}
