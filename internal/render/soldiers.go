package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/combat"
	"strategy_game/internal/soldier"
)

// soldierHeight matches sentryHeight: a person on the ground, not a second
// building.
const soldierHeight = 0.90

// archerTint/swordsmanTint distinguish the two professions with a color
// tint on the shared Serf sprite -- neither has its own art yet, the same
// "never block on a missing sprite" convention already used for the
// WatchTower/Barracks/Sentry before their real art arrived.
var (
	archerTint    = color.RGBA{R: 130, G: 205, B: 120, A: 255}
	swordsmanTint = color.RGBA{R: 150, G: 165, B: 225, A: 255}
)

// DrawSoldiers renders every living Archer/Swordsman on the map.
func DrawSoldiers(screen *ebiten.Image, soldiers []*soldier.Soldier, cam *Camera) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, sd := range soldiers {
		if sd == nil || !sd.Alive() || !visible.Intersects(sd.X, sd.Y, 1) {
			continue
		}
		sx, sy := cam.TileToScreen(sd.X, sd.Y)
		path := sd.RemainingPath()
		frame := walkingFrame(path, sd.X+sd.Y)
		tint := archerTint
		if sd.Profession == soldier.Swordsman {
			tint = swordsmanTint
		}
		flip := facingLeft(sd.X, path)
		drawStandingFacingTintedAtScale(screen, assets.Serf[frame], sx, sy, soldierHeight, tilePixels, tint, flip)
		drawSoldierHealth(screen, sx, sy, tilePixels, sd.HP)
	}
}

// drawSoldierHealth mirrors drawEnemyHealth: hidden at full health so an
// untouched soldier adds no HUD noise.
func drawSoldierHealth(screen *ebiten.Image, sx, sy, tilePixels float64, hp int) {
	if hp >= combat.MaxHP {
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
	fill := width * float64(hp) / float64(combat.MaxHP)
	vector.FillRect(screen, float32(x), float32(y), float32(fill), float32(height), color.RGBA{R: 220, G: 72, B: 54, A: 255}, false)
}
