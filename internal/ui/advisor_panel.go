package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/i18n"
)

const (
	advisorToastWidth  = 460
	advisorToastHeight = 64
	advisorToastMargin = 16

	advisorButtonWidth  = 130
	advisorButtonHeight = 26
	advisorButtonMargin = 10
)

// AdvisorToastRect is a small banner docked top-center of the *map* area
// (not the side panels) -- the advisor (see internal/advisor) is meant to
// stay visible without blocking the rest of the game, unlike the modal
// DialogKind confirmations, so it lives over the map rather than in
// either panel.
func (l Layout) AdvisorToastRect() image.Rectangle {
	r := l.MapRect()
	w := advisorToastWidth
	if usable := r.Dx() - 2*advisorToastMargin; w > usable {
		w = usable
	}
	if w < 0 {
		w = 0
	}
	x := r.Min.X + (r.Dx()-w)/2
	y := r.Min.Y + advisorToastMargin
	return image.Rect(x, y, x+w, y+advisorToastHeight)
}

// AdvisorAcknowledgeButtonRect is the "Ознакомлен" button inside the toast.
func (l Layout) AdvisorAcknowledgeButtonRect() image.Rectangle {
	r := l.AdvisorToastRect()
	x := r.Max.X - advisorButtonWidth - advisorButtonMargin
	y := r.Max.Y - advisorButtonHeight - advisorButtonMargin
	return image.Rect(x, y, x+advisorButtonWidth, y+advisorButtonHeight)
}

// AdvisorAcknowledgeAt reports whether (x, y) is over the toast's
// acknowledge button.
func (l Layout) AdvisorAcknowledgeAt(x, y int) bool {
	return image.Pt(x, y).In(l.AdvisorAcknowledgeButtonRect())
}

// DrawAdvisorToast renders one already-localized tip line with its
// acknowledge button. It knows nothing about advisor.Kind -- cmd/game
// turns a Tip into text (see its own advisorTipText) so this package
// stays as unaware of that package as every other logic package is of
// ebiten.
func DrawAdvisorToast(screen *ebiten.Image, layout Layout, text string) {
	r := layout.AdvisorToastRect()
	if r.Dx() <= 0 {
		return
	}
	vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), panelColor, false)
	vector.StrokeRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 2, panelEdgeColor, false)
	DrawInspectorText(screen, text, float64(r.Min.X+14), float64(r.Min.Y+10))

	btn := layout.AdvisorAcknowledgeButtonRect()
	vector.FillRect(screen, float32(btn.Min.X), float32(btn.Min.Y), float32(btn.Dx()), float32(btn.Dy()), selectedColor, false)
	vector.StrokeRect(screen, float32(btn.Min.X), float32(btn.Min.Y), float32(btn.Dx()), float32(btn.Dy()), 2, panelEdgeColor, false)
	DrawMenuText(screen, i18n.T().AdvisorAcknowledgeButton, float64(btn.Min.X+10), float64(btn.Min.Y+7))
}
