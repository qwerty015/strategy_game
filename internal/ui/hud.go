package ui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/render"
	"strategy_game/internal/resource"
)

// DrawResourceBar prints current stockpile counts and population along
// the top-left of the screen. Placeholder text HUD -- swappable for real
// art later without touching game logic.
func DrawResourceBar(screen *ebiten.Image, stock *resource.Stockpile, pop *economy.Population) {
	line := fmt.Sprintf(
		"Population: %d | Wheat: %d  Flour: %d  Bread: %d",
		pop.Count, stock.Amount(resource.Wheat), stock.Amount(resource.Flour), stock.Amount(resource.Bread),
	)
	ebitenutil.DebugPrintAt(screen, line, 8, 8)
}

// DrawPalette prints the building selection hotkeys, highlighting the
// currently selected one.
func DrawPalette(screen *ebiten.Image, p *Palette) {
	for i, kind := range p.Kinds {
		marker := "  "
		if i == p.Selected {
			marker = "> "
		}
		line := fmt.Sprintf("%s[%d] %s", marker, i+1, building.Types[kind].Name)
		ebitenutil.DebugPrintAt(screen, line, 8, 28+i*16)
	}
}

// DrawPlacementPreview highlights the footprint the currently selected
// building would occupy at tile (tx, ty): green if CanPlace would allow
// it, red otherwise.
func DrawPlacementPreview(screen *ebiten.Image, cam *render.Camera, kind building.Kind, tx, ty int, valid bool) {
	c := color.RGBA{R: 220, G: 60, B: 60, A: 140}
	if valid {
		c = color.RGBA{R: 60, G: 200, B: 90, A: 140}
	}

	footprint := building.Types[kind].Footprint
	sx, sy := cam.TileToScreen(tx, ty)
	size := float32(footprint * render.TileSize)
	vector.FillRect(screen, float32(sx), float32(sy), size, size, c, false)
}
