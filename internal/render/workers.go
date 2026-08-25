package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/building"
	"strategy_game/internal/villagers"
)

// DrawWorkerMarkers shows whether a staffed production building currently
// has its worker inside. A green plus means the Farmer/Baker is at the post;
// a red minus means the worker is absent (usually walking to eat), so the
// player can understand why production is temporarily paused.
func DrawWorkerMarkers(screen *ebiten.Image, buildings []*building.Building, vills []*villagers.Villager, cam *Camera) {
	tilePixels := cam.TilePixels()
	for _, b := range buildings {
		if building.Types[b.Kind].Recipe.TicksToProduce <= 0 {
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
		centerX := sx + tilePixels*0.5
		centerY := sy + tilePixels*0.22
		arm := float32(4 * tilePixels / TileSize)
		thickness := float32(2 * tilePixels / TileSize)
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
