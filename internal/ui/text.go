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
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(color.White)
	op.Blend = ebiten.BlendSourceOver // see the comment on the same field in render/sprite.go
	text.Draw(screen, s, textFace, op)
}
