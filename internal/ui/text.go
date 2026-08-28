package ui

import (
	"image/color"

	"github.com/hajimehoshi/bitmapfont/v4"
	"github.com/hajimehoshi/ebiten/v2"
	text "github.com/hajimehoshi/ebiten/v2/text/v2"
)

// textFace is the font used for every bit of text the game draws.
// ebitenutil.DebugPrint's built-in font only covers Latin-1 (U+0000 to
// U+00FF per its own doc comment) so it silently drops Cyrillic --
// bitmapfont's Face covers a much wider Unicode range (Cyrillic
// included) and works with ebiten's text/v2 renderer via NewGoXFace.
var textFace = text.NewGoXFace(bitmapfont.Face)

// DrawText draws s with its top-left corner at (x, y) in white. Shared
// by every package that draws HUD/status text so there's exactly one
// font to swap out later if real UI art replaces this placeholder text.
func DrawText(screen *ebiten.Image, s string, x, y float64) {
	drawText(screen, s, x, y, 1)
}

const (
	menuTextScale        = 1.17
	compactMenuTextScale = 1.05
	inspectorTextScale   = 1.12
)

// DrawMenuText gives the three left-panel tabs a larger, stronger pixel type
// treatment without affecting map labels or the information-dense inspector.
// The one-pixel second pass is intentional: bitmapfont has no bold face, so
// it creates a crisp pixel-art weight instead of blurry raster scaling.
func DrawMenuText(screen *ebiten.Image, s string, x, y float64) {
	drawText(screen, s, x, y, menuTextScale)
	drawText(screen, s, x+1, y, menuTextScale)
}

// DrawCompactMenuText is the same heavier left-menu style for controls split
// into narrow segments, such as the six speed buttons in the Options tab.
func DrawCompactMenuText(screen *ebiten.Image, s string, x, y float64) {
	drawText(screen, s, x, y, compactMenuTextScale)
	drawText(screen, s, x+1, y, compactMenuTextScale)
}

// DrawInspectorText raises the right panel's information density without
// making its many resource rows hard to read. It uses the same crisp bold
// treatment as the menu, at a slightly calmer scale for long labels.
func DrawInspectorText(screen *ebiten.Image, s string, x, y float64) {
	drawText(screen, s, x, y, inspectorTextScale)
	drawText(screen, s, x+1, y, inspectorTextScale)
}

func drawText(screen *ebiten.Image, s string, x, y, scale float64) {
	op := &text.DrawOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(color.White)
	op.Blend = ebiten.BlendSourceOver // see the comment on the same field in render/sprite.go
	text.Draw(screen, s, textFace, op)
}
