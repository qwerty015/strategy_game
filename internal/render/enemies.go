package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/enemy"
)

const enemyHeight = 0.94

// DrawEnemies visualizes the deliberately simple debug opponents introduced
// with the first WatchTower pass. Real hostile AI can later reuse this render
// path without placing Ebitengine dependencies in package enemy.
func DrawEnemies(screen *ebiten.Image, enemies []*enemy.Enemy, cam *Camera) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, foe := range enemies {
		if !foe.Alive() || !visible.Intersects(foe.X, foe.Y, 1) {
			continue
		}
		sx, sy := cam.TileToScreen(foe.X, foe.Y)
		frames := assets.Enemy
		frame, flip := 0, false
		if path := foe.RemainingPath(); len(path) > 0 {
			frames = assets.EnemyWalkFrames
			frame = walkingFrame(path, foe.X+foe.Y)
			flip = facingLeft(foe.X, path)
		}
		drawStandingFacingTintedAtScale(screen, frames[frame], sx, sy, enemyHeight, tilePixels, color.White, flip)
		drawEnemyHealth(screen, sx, sy, tilePixels, foe.HP)
	}
}

// drawEnemyHealth stays hidden at full health so test targets do not add HUD
// noise; the bar appears immediately after a sentry's first successful shot.
func drawEnemyHealth(screen *ebiten.Image, sx, sy, tilePixels float64, hp int) {
	if hp >= enemy.MaxHP {
		return
	}
	if hp < 0 {
		hp = 0
	}
	width := tilePixels * 0.68
	height := tilePixels * 0.08
	if height < 2 {
		height = 2
	}
	x := sx + (tilePixels-width)/2
	y := sy + tilePixels*0.16
	vector.FillRect(screen, float32(x), float32(y), float32(width), float32(height), color.RGBA{R: 55, G: 25, B: 20, A: 230}, false)
	fill := width * float64(hp) / float64(enemy.MaxHP)
	vector.FillRect(screen, float32(x), float32(y), float32(fill), float32(height), color.RGBA{R: 220, G: 72, B: 54, A: 255}, false)
}
