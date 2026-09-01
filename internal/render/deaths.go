package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
)

const deathEffectLifetimeFrames = 96

// DeathEffect is a short-lived, universal visual record of a unit that has
// already been removed by simulation. It deliberately stores only a map tile
// and animation age: no profession, ownership or save-game data is required.
type DeathEffect struct {
	X, Y int
	Age  int
}

// NewDeathEffect creates the generic fall, soul and skeleton effect at a
// unit's final map position.
func NewDeathEffect(x, y int) DeathEffect {
	return DeathEffect{X: x, Y: y}
}

// AdvanceDeathEffects advances active screen-only effects by one rendered
// frame, discarding the final skeleton after it has been visible long enough.
// Effects intentionally freeze while the game is paused because Game.Update
// does not call this function in that state.
func AdvanceDeathEffects(effects []DeathEffect) []DeathEffect {
	alive := effects[:0]
	for _, effect := range effects {
		effect.Age++
		if effect.Age >= deathEffectLifetimeFrames {
			continue
		}
		alive = append(alive, effect)
	}
	return alive
}

// DrawDeathEffects draws a shared profession-free animation for every death:
// collapse, rising soul, then a skeleton that lingers briefly before fading.
func DrawDeathEffects(screen *ebiten.Image, effects []DeathEffect, cam *Camera) {
	visible := cam.VisibleTileBounds(1)
	tilePixels := cam.TilePixels()
	for _, effect := range effects {
		if !visible.Intersects(effect.X, effect.Y, 1) {
			continue
		}
		frame := deathFrame(effect.Age)
		tint := color.RGBA{R: 255, G: 255, B: 255, A: 255}
		if effect.Age > 78 {
			tint.A = uint8(255 * (deathEffectLifetimeFrames - effect.Age) / (deathEffectLifetimeFrames - 78))
		}
		sx, sy := cam.TileToScreen(effect.X, effect.Y)
		drawStandingTintedAtScale(screen, assets.DeathFrames[frame], sx, sy, 1, tilePixels, tint)
	}
}

func deathFrame(age int) int {
	switch {
	case age < 16:
		return 0
	case age < 50:
		return 1
	default:
		return 2
	}
}
