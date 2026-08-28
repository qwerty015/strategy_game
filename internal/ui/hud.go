package ui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/building"
	"strategy_game/internal/i18n"
	"strategy_game/internal/render"
)

// DrawPalette prints the building selection hotkeys, highlighting the
// currently selected one.
func DrawPalette(screen *ebiten.Image, p *Palette) {
	names := i18n.T().BuildingName
	for i, kind := range p.Kinds {
		marker := "  "
		if i == p.Selected {
			marker = "> "
		}
		line := fmt.Sprintf("%s[%d] %s", marker, i+1, names[kind])
		DrawText(screen, line, 8, float64(28+i*16))
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
	size := float32(float64(footprint) * cam.TilePixels())
	vector.FillRect(screen, float32(sx), float32(sy), size, size, c, false)
	accessX := tx + building.Types[kind].AccessX
	accessY := ty + building.Types[kind].AccessY
	DrawPlacementAccessMarker(screen, cam, accessX, accessY, valid)
}
