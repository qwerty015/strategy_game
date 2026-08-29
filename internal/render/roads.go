package render

import (
	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
)

// drawOrganicRoad keeps normal roads on the original square cobblestone tile.
// Only the one road cell at a building entrance uses the same texture with
// softly rounded grass corners; no bright seam correction is drawn anywhere.
func drawOrganicRoad(screen *ebiten.Image, roads map[roadTile]bool, entranceRoads map[roadTile]bool, x, y int, sx, sy, tilePixels float64) {
	image := assets.Road
	if entranceRoads[roadTile{x: x, y: y}] {
		image = assets.RoadEntrance
	}
	drawGround(screen, image, sx, sy, tilePixels, false)
}
