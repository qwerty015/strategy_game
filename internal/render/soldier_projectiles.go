package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/soldier"
)

// DrawSoldierProjectiles draws only the short arrow flight tied to an actual
// Archer hit. The soldier package owns its transient attack state; this layer
// reads it without altering combat damage or target selection.
func DrawSoldierProjectiles(screen *ebiten.Image, soldiers []*soldier.Soldier, cam *Camera) {
	tilePixels := cam.TilePixels()
	visible := cam.VisibleTileBounds(2)
	for _, sd := range soldiers {
		if sd == nil || !sd.Alive() || sd.Profession != soldier.Archer {
			continue
		}
		targetX, targetY, progress, ok := sd.AttackVisual()
		if !ok || (!visible.Intersects(sd.X, sd.Y, 1) && !visible.Intersects(targetX, targetY, 1)) {
			continue
		}
		drawArrowShot(screen, cam, sd.X, sd.Y, targetX, targetY, progress, tilePixels)
	}
}

// drawArrowShot turns the release half of a three-step Archer attack into one
// thin, fast visual arrow. It purposely begins after the pose's wind-up and
// ends before recovery, so there is never an arrow floating beside an idle bow.
func drawArrowShot(screen *ebiten.Image, cam *Camera, fromX, fromY, targetX, targetY int, progress, tilePixels float64) {
	const releaseStart = 0.36
	if progress < releaseStart {
		return
	}
	flight := (progress - releaseStart) / (1 - releaseStart)
	if flight > 1 {
		flight = 1
	}
	fromSX, fromSY := cam.TileToScreen(fromX, fromY)
	toSX, toSY := cam.TileToScreen(targetX, targetY)
	startX, startY := fromSX+tilePixels*0.59, fromSY+tilePixels*0.45
	endX, endY := toSX+tilePixels*0.5, toSY+tilePixels*0.5
	arc := tilePixels * 0.12 * 4 * flight * (1 - flight)
	x := startX + (endX-startX)*flight
	y := startY + (endY-startY)*flight - arc

	dx, dy := endX-startX, endY-startY
	length := math.Hypot(dx, dy)
	if length < 0.001 {
		return
	}
	dx, dy = dx/length, dy/length
	shaft := tilePixels * 0.22
	width := float32(math.Max(1, tilePixels*0.025))
	vector.StrokeLine(screen, float32(x-dx*shaft), float32(y-dy*shaft), float32(x+dx*shaft*0.15), float32(y+dy*shaft*0.15), width, color.RGBA{R: 120, G: 76, B: 35, A: 255}, false)
	vector.FillCircle(screen, float32(x+dx*shaft*0.2), float32(y+dy*shaft*0.2), width*0.75, color.RGBA{R: 206, G: 213, B: 207, A: 255}, false)
}
