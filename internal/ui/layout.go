package ui

import (
	"image"

	"strategy_game/internal/economy"
)

// Layout defines the screen regions shared by drawing and mouse hit-testing.
// Keeping these rectangles in one place prevents the UI from drifting away
// from the input coordinates as the panels evolve.
type Layout struct {
	Width, Height int

	LeftWidth    int
	RightWidth   int
	BottomHeight int
}

func NewLayout(width, height int) Layout {
	return Layout{
		Width:        width,
		Height:       height,
		LeftWidth:    220,
		RightWidth:   260,
		BottomHeight: 64,
	}
}

func (l Layout) LeftPanel() image.Rectangle {
	return image.Rect(0, 0, l.LeftWidth, l.Height)
}

func (l Layout) RightPanel() image.Rectangle {
	return image.Rect(l.Width-l.RightWidth, 0, l.Width, l.Height-l.BottomHeight)
}

func (l Layout) BottomPanel() image.Rectangle {
	return image.Rect(0, l.Height-l.BottomHeight, l.Width, l.Height)
}

func (l Layout) MapRect() image.Rectangle {
	return image.Rect(l.LeftWidth, 0, l.Width-l.RightWidth, l.Height-l.BottomHeight)
}

// BuildIndexAt returns the building palette card under the cursor.
func (l Layout) BuildIndexAt(x, y int, count int) (int, bool) {
	card := image.Rect(12, 56, l.LeftWidth-12, 108)
	for i := 0; i < count; i++ {
		r := card.Add(image.Pt(0, i*58))
		if image.Pt(x, y).In(r) {
			return i, true
		}
	}
	return 0, false
}

// SpeedAt returns the speed button under the cursor. Buttons occupy the
// lower strip, leaving the left side for the help text.
func (l Layout) SpeedAt(x, y int) (economy.Speed, bool) {
	if !image.Pt(x, y).In(l.BottomPanel()) {
		return economy.Normal, false
	}

	buttonY := l.Height - l.BottomHeight + 12
	buttonW := 62
	startX := l.speedStartX()
	if y < buttonY || y >= buttonY+36 || x < startX {
		return economy.Normal, false
	}
	index := (x - startX) / buttonW
	if index < 0 || index > int(economy.Quadruple) {
		return economy.Normal, false
	}
	if x >= startX+(index+1)*buttonW {
		return economy.Normal, false
	}
	return economy.Speed(index), true
}

func (l Layout) speedStartX() int {
	return l.Width - 5*62 - 12
}
