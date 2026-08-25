package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
	"strategy_game/internal/logistics"
)

// serfHeight: less than a full tile, since these are small figures, not
// buildings -- see buildingHeight in buildings.go for the same idea
// applied to buildings.
const serfHeight = 0.85

// DrawSerfs renders every serf as a small figure at its current tile, so
// the player can actually see goods being hauled along the road network
// instead of resources just teleporting between buildings. A busy serf
// cycles through the pose frames in assets.Serf (see animFrame in
// buildings.go) for a walking look; an idle one holds a single pose and
// is dimmed.
func DrawSerfs(screen *ebiten.Image, serfs []*logistics.Serf, cam *Camera) {
	for _, s := range serfs {
		sx, sy := cam.TileToScreen(s.X, s.Y)

		frame := 0
		tint := color.Color(color.RGBA{R: 200, G: 200, B: 200, A: 180})
		if s.Busy() {
			frame = (animFrame / 10) % len(assets.Serf)
			tint = color.White
		}
		drawStandingTinted(screen, assets.Serf[frame], sx, sy, serfHeight, tint)
	}
}
