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

const (
	leftTabY        = 44
	leftTabHeight   = 30
	leftCardsStartY = 82
	leftCardHeight  = 40
	leftCardStride  = 44
)

func NewLayout(width, height int) Layout {
	left := width / 5
	if left < 180 {
		left = 180
	}
	if left > 240 {
		left = 240
	}
	right := width / 4
	if right < 220 {
		right = 220
	}
	if right > 300 {
		right = 300
	}
	// Keep a usable map strip even in a narrow resizable window. The two
	// side panels remain attached to the actual edges; only their widths
	// shrink when there is no room for the preferred proportions.
	if left+right > width-160 {
		right = width - left - 160
		if right < 160 {
			right = 160
			left = width - right - 160
		}
		if left < 150 {
			left = 150
		}
	}
	if left+right > width {
		left = width / 2
		right = width - left
	}
	bottom := height / 9
	if bottom < 64 {
		bottom = 64
	}
	if bottom > 84 {
		bottom = 84
	}
	if bottom > height {
		bottom = height
	}
	return Layout{
		Width:        width,
		Height:       height,
		LeftWidth:    left,
		RightWidth:   right,
		BottomHeight: bottom,
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

// MenuTabAt returns the left-panel category button under the cursor.
func (l Layout) MenuTabAt(x, y int) (LeftTab, bool) {
	if y < leftTabY || y >= leftTabY+leftTabHeight || x < 12 || x >= l.LeftWidth-12 {
		return BuildTab, false
	}
	mid := l.LeftWidth / 2
	if x < mid {
		return BuildTab, true
	}
	return HireTab, true
}

// BuildIndexAt returns the building palette card under the cursor.
func (l Layout) BuildIndexAt(x, y int, count int) (int, bool) {
	return l.menuIndexAt(x, y, count)
}

// HireIndexAt returns the hire-menu card under the cursor.
func (l Layout) HireIndexAt(x, y int, count int) (int, bool) {
	return l.menuIndexAt(x, y, count)
}

func (l Layout) menuIndexAt(x, y int, count int) (int, bool) {
	card := image.Rect(12, leftCardsStartY, l.LeftWidth-12, leftCardsStartY+leftCardHeight)
	for i := 0; i < count; i++ {
		r := card.Add(image.Pt(0, i*leftCardStride))
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

// HireAt reports whether the cursor is over the serf hiring button in the
// bottom panel.
func (l Layout) HireAt(x, y int) bool {
	r := image.Rect(l.LeftWidth+16, l.Height-l.BottomHeight+20, l.LeftWidth+196, l.Height-l.BottomHeight+54)
	return image.Pt(x, y).In(r)
}

func (l Layout) speedStartX() int {
	return l.Width - 5*62 - 12
}

const (
	priorityRowHeight = 30
	priorityMargin    = 18
	priorityBottomGap = 12
)

// PriorityLevelAt returns the supply-priority segment under the cursor,
// from the fixed five-segment control docked at the bottom of the
// inspector panel (see DrawPriorityControl). Segments map to the five
// levels PriorityLowest..PriorityHighest (-2..2) left to right. Docking
// the control at a fixed offset from the panel's bottom edge -- rather
// than after the building's own dynamic info text -- means drawing and
// hit-testing share the exact same geometry without needing to agree on
// how tall that text block happened to be this frame.
func (l Layout) PriorityLevelAt(x, y int) (int, bool) {
	r := l.RightPanel()
	rowY := r.Max.Y - priorityRowHeight - priorityBottomGap
	if y < rowY || y >= rowY+priorityRowHeight {
		return 0, false
	}
	startX := r.Min.X + priorityMargin
	segW := (r.Dx() - 2*priorityMargin) / 5
	if segW <= 0 || x < startX {
		return 0, false
	}
	index := (x - startX) / segW
	if index < 0 || index > 4 || x >= startX+(index+1)*segW {
		return 0, false
	}
	return index - 2, true
}
