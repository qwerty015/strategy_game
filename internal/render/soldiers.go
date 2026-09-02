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

// DrawSoldiers renders every living Archer/Swordsman on the map. The walking
// loop is replaced by a short profession-specific attack loop only after an
// actual hit lands (Soldier.AttackVisual), so aiming or waiting never looks
// like repeated combat.
// opponent -- see DrawSerfs's identical parameter doc comment. Most
// important for this Draw call of all nine: without a way to tell an
// opponent's soldier apart from the player's own, combat itself is
// unreadable, not just a minor cosmetic gap.
func DrawSoldiers(screen *ebiten.Image, soldiers []*soldier.Soldier, cam *Camera, opponent bool) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(1)
	for _, sd := range soldiers {
		if sd == nil || !sd.Alive() || !visible.Intersects(sd.X, sd.Y, 1) {
			continue
		}
		frames, attackFrames := assets.ArcherWalkFrames, assets.ArcherAttackFrames
		if sd.Profession == soldier.Swordsman {
			frames, attackFrames = assets.SwordsmanWalkFrames, assets.SwordsmanAttackFrames
		}

		sx, sy := cam.TileToScreen(sd.X, sd.Y)
		if opponent {
			DrawOpponentUnitMarker(screen, sx, sy, tilePixels)
		}
		path := sd.RemainingPath()
		frame := walkingFrame(path, sd.X+sd.Y)
		flip := facingLeft(sd.X, path)
		if targetX, _, progress, attacking := sd.AttackVisual(); attacking {
			frames = attackFrames
			frame = soldierAttackFrame(progress)
			flip = targetX < sd.X
		}
		drawStandingFacingScaled(screen, frames[frame], sx, sy, soldierHeight, tilePixels, color.White, flip)
		drawSoldierHealth(screen, sx, sy, tilePixels, sd.HP)
	}
}

// soldierAttackFrame maps the visual attack interval to wind-up, impact and
// recovery. Keeping the thresholds here makes the atlas ordering explicit and
// independent from the simulation tick rate.
func soldierAttackFrame(progress float64) int {
	switch {
	case progress < 0.34:
		return 0
	case progress < 0.68:
		return 1
	default:
		return 2
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
