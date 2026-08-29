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
	drawTextColor(screen, s, x, y, color.White)
}

var textOutlineColor = color.RGBA{R: 20, G: 15, B: 15, A: 255}

// DrawMenuText gives the three left-panel tabs a stronger pixel type treatment
// without affecting map labels or the information-dense inspector. Bitmap
// glyphs are never fractionally scaled: that was blurring their edges in both
// side panels. A one-pixel dark outline supplies weight and contrast while
// keeping every glyph aligned to the pixel grid.
// DrawTitleText is a nearest-neighbour scaled, outlined heading for the
// title screen and Markdown help. It shares the UI font, so Cyrillic remains
// crisp rather than depending on a separate logo image.
func DrawTitleText(screen *ebiten.Image, s string, x, y, scale float64) {
	if scale <= 0 {
		return
	}
	for _, offset := range [][2]float64{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		drawScaledTextColor(screen, s, x+offset[0]*scale, y+offset[1]*scale, scale, textOutlineColor)
	}
	drawScaledTextColor(screen, s, x, y, scale, color.White)
}

func drawScaledTextColor(screen *ebiten.Image, s string, x, y, scale float64, tint color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(tint)
	op.Blend = ebiten.BlendSourceOver
	text.Draw(screen, s, textFace, op)
}
func DrawMenuText(screen *ebiten.Image, s string, x, y float64) {
	drawOutlinedText(screen, s, x, y)
}

// DrawCompactMenuText is the same crisp left-menu style for controls split
// into narrow segments, such as the six speed buttons in the Options tab.
func DrawCompactMenuText(screen *ebiten.Image, s string, x, y float64) {
	drawOutlinedText(screen, s, x, y)
}

// DrawInspectorText uses the same crisp, high-contrast treatment as the left
// menu, so long resource rows stay readable without blurred scaling.
func DrawInspectorText(screen *ebiten.Image, s string, x, y float64) {
	drawOutlinedText(screen, s, x, y)
}

func drawOutlinedText(screen *ebiten.Image, s string, x, y float64) {
	for _, offset := range [][2]float64{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		drawTextColor(screen, s, x+offset[0], y+offset[1], textOutlineColor)
	}
	drawTextColor(screen, s, x, y, color.White)
}

func drawTextColor(screen *ebiten.Image, s string, x, y float64, tint color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(tint)
	op.Blend = ebiten.BlendSourceOver // see the comment on the same field in render/sprite.go
	text.Draw(screen, s, textFace, op)
}
