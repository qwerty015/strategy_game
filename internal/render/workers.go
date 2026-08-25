package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/building"
	"strategy_game/internal/villagers"
)

// DrawWorkerMarkers shows whether a Farm or Bakery currently has its worker
// on duty. A green plus means the worker is present; a red minus means the
// worker is away eating. Mills have no resident worker, so they do not get
// this marker.
func DrawWorkerMarkers(screen *ebiten.Image, buildings []*building.Building, vills []*villagers.Villager, cam *Camera) {
	tilePixels := cam.TilePixels()
	for _, b := range buildings {
		if b.Kind != building.Farm && b.Kind != building.Bakery {
			continue
		}
		present := false
		for _, v := range vills {
			if v.Home == b && v.Working() {
				present = true
				break
			}
		}
		p := b.AccessPoint()
		sx, sy := cam.TileToScreen(p.X, p.Y)
		centerX := sx + tilePixels*0.80
		centerY := sy + tilePixels*0.18
		arm := float32(2.5 * tilePixels / TileSize)
		if arm > 4 {
			arm = 4
		}
		thickness := float32(1.5 * tilePixels / TileSize)
		if thickness < 1 {
			thickness = 1
		}
		marker := color.RGBA{R: 214, G: 63, B: 55, A: 240}
		if present {
			marker = color.RGBA{R: 76, G: 205, B: 112, A: 245}
		}
		if present {
			vector.FillRect(screen, float32(centerX)-thickness/2, float32(centerY)-arm, thickness, arm*2, marker, false)
			vector.FillRect(screen, float32(centerX)-arm, float32(centerY)-thickness/2, arm*2, thickness, marker, false)
		} else {
			vector.FillRect(screen, float32(centerX)-arm, float32(centerY)-thickness/2, arm*2, thickness, marker, false)
		}
	}
}
