package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/logistics"
)

var (
	serfIdleColor = color.RGBA{R: 190, G: 190, B: 195, A: 255} // grey: waiting for a job
	serfBusyColor = color.RGBA{R: 240, G: 200, B: 60, A: 255}  // gold: carrying/travelling
)

// DrawSerfs renders every serf as a small dot at its current tile, so
// the player can actually see goods being hauled along the road network
// instead of resources just teleporting between buildings.
func DrawSerfs(screen *ebiten.Image, serfs []*logistics.Serf, cam *Camera) {
	for _, s := range serfs {
		sx, sy := cam.TileToScreen(s.X, s.Y)
		cx := float32(sx) + float32(TileSize)/2
		cy := float32(sy) + float32(TileSize)/2

		c := serfIdleColor
		if s.Busy() {
			c = serfBusyColor
		}
		vector.FillCircle(screen, cx, cy, float32(TileSize)/4, c, false)
	}
}
