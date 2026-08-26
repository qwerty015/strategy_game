package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/building"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/villagers"
)

// DrawWorkerMarkers shows whether a worker is assigned to a Farm, Bakery,
// Winery or Lumberjack Hut. A green plus means the worker is at work/assigned; a red
// minus means the resident worker is away or missing.
func DrawWorkerMarkers(screen *ebiten.Image, buildings []*building.Building, vills []*villagers.Villager, jacks []*lumberjack.Lumberjack, cam *Camera) {
	tilePixels := cam.TilePixels()
	for _, b := range buildings {
		if b.Kind != building.Farm && b.Kind != building.Bakery && b.Kind != building.Winery && b.Kind != building.LumberjackHut {
			continue
		}
		present := false
		for _, v := range vills {
			if v.Home == b && v.Working() {
				present = true
				break
			}
		}
		for _, j := range jacks {
			if j.HomeBuilding() == b && j.AtPost() {
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
