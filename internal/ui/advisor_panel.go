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

// AdvisorGoToButtonRect is the optional action beside acknowledgement. It is
// absent on an unusually narrow map area, where an empty rectangle keeps the
// hit-test harmless while the acknowledgement button remains usable.
func (l Layout) AdvisorGoToButtonRect() image.Rectangle {
	r := l.AdvisorToastRect()
	ack := l.AdvisorAcknowledgeButtonRect()
	x := ack.Min.X - advisorButtonMargin - advisorButtonWidth
	if x < r.Min.X+advisorButtonMargin {
		return image.Rectangle{}
	}
	return image.Rect(x, ack.Min.Y, x+advisorButtonWidth, ack.Max.Y)
}

// AdvisorGoToAt reports whether the optional focus action is under the cursor.
func (l Layout) AdvisorGoToAt(x, y int) bool {
	return image.Pt(x, y).In(l.AdvisorGoToButtonRect())
}

// DrawAdvisorToast renders one already-localized tip line with its action
// buttons. showGoTo is true only when the tip has a concrete building for the
// game layer to select and center; this package remains unaware of advisor.
func DrawAdvisorToast(screen *ebiten.Image, layout Layout, text string, showGoTo bool) {
	r := layout.AdvisorToastRect()
	if r.Dx() <= 0 {
		return
	}
	vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), panelColor, false)
	vector.StrokeRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 2, panelEdgeColor, false)
	DrawInspectorText(screen, text, float64(r.Min.X+14), float64(r.Min.Y+10))

	if showGoTo {
		if goTo := layout.AdvisorGoToButtonRect(); !goTo.Empty() {
			vector.FillRect(screen, float32(goTo.Min.X), float32(goTo.Min.Y), float32(goTo.Dx()), float32(goTo.Dy()), panelInnerColor, false)
			vector.StrokeRect(screen, float32(goTo.Min.X), float32(goTo.Min.Y), float32(goTo.Dx()), float32(goTo.Dy()), 2, panelEdgeColor, false)
			DrawMenuText(screen, i18n.T().AdvisorGoToButton, float64(goTo.Min.X+10), float64(goTo.Min.Y+7))
		}
	}
	btn := layout.AdvisorAcknowledgeButtonRect()
	vector.FillRect(screen, float32(btn.Min.X), float32(btn.Min.Y), float32(btn.Dx()), float32(btn.Dy()), selectedColor, false)
	vector.StrokeRect(screen, float32(btn.Min.X), float32(btn.Min.Y), float32(btn.Dx()), float32(btn.Dy()), 2, panelEdgeColor, false)
	DrawMenuText(screen, i18n.T().AdvisorAcknowledgeButton, float64(btn.Min.X+10), float64(btn.Min.Y+7))
}
