package render

import (
	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
)

// drawOrganicRoad keeps normal roads on the original square cobblestone tile.
// At a building entrance, only corner bits whose surrounding tiles are open
// grass get a stronger round cut from that same texture.
func drawOrganicRoad(screen *ebiten.Image, entranceCorners map[roadTile]uint8, x, y int, sx, sy, tilePixels float64) {
	image := assets.Road
	if corners := entranceCorners[roadTile{x: x, y: y}]; corners != 0 {
		image = assets.RoadEntranceVariant(corners)
	}
	drawGround(screen, image, sx, sy, tilePixels, false)
}
