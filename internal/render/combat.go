package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/sentry"
)

// towerRangeTileColor matches ui.DrawPlacementPreview's "valid" green
// exactly, so a highlighted tile reads with the same established meaning
// ("this is fine/available") instead of introducing a new color language
// just for towers.
var towerRangeTileColor = color.RGBA{R: 60, G: 200, B: 90, A: 90}

// DrawTowerRange highlights every tile a WatchTower at world tile (tileX,
// tileY) can actually hit -- per the user's explicit request, individual
// green tiles rather than a circle, since sentry.WatchTowerRange is a
// plain square (Chebyshev) radius, not a circle, and the tiles are the
// literal truth of what's in range. Shown "при строительстве" (during
// placement) and "вокруг действующих башен" (around a selected finished
// one) -- never unconditionally for every tower at once, see this
// function's call sites in cmd/game.
func DrawTowerRange(screen *ebiten.Image, cam *Camera, tileX, tileY int) {
	tilePixels := float32(cam.TilePixels())
	for dy := -sentry.WatchTowerRange; dy <= sentry.WatchTowerRange; dy++ {
		for dx := -sentry.WatchTowerRange; dx <= sentry.WatchTowerRange; dx++ {
			sx, sy := cam.TileToScreen(tileX+dx, tileY+dy)
			vector.FillRect(screen, float32(sx), float32(sy), tilePixels, tilePixels, towerRangeTileColor, false)
		}
	}
}

// attackMarkerColor is a clear "hostile target" red, distinct from every
// other highlight color this package uses.
var attackMarkerColor = color.RGBA{R: 220, G: 40, B: 30, A: 255}

// DrawAttackMarker draws the red square outline over a selected soldier
// group's current attack target tile -- per the user's explicit "отмечает
// противника красной рамкой (квадрат на котором он находится)".
func DrawAttackMarker(screen *ebiten.Image, cam *Camera, tileX, tileY int) {
	tilePixels := float32(cam.TilePixels())
	sx, sy := cam.TileToScreen(tileX, tileY)
	vector.StrokeRect(screen, float32(sx), float32(sy), tilePixels, tilePixels, 3, attackMarkerColor, false)
}
