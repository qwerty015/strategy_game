package ui

import (
	"image"

	"strategy_game/internal/economy"
	"strategy_game/internal/i18n"
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

const (
	tabCount = 3 // Build, Hire, Settings
	tabGap   = 4
)

// tabRect returns the x position and width of tab button i, shared by
// drawMenuTabs and MenuTabAt so the drawn buttons and their hit-test
// regions can never drift apart.
func (l Layout) tabRect(i int) (x, w int) {
	total := l.LeftWidth - 24 - (tabCount-1)*tabGap
	width := total / tabCount
	return 12 + i*(width+tabGap), width
}

// MenuTabAt returns the left-panel category button under the cursor.
func (l Layout) MenuTabAt(x, y int) (LeftTab, bool) {
	if y < leftTabY || y >= leftTabY+leftTabHeight {
		return BuildTab, false
	}
	for i := 0; i < tabCount; i++ {
		tx, tw := l.tabRect(i)
		if x >= tx && x < tx+tw {
			return LeftTab(i), true
		}
	}
	return BuildTab, false
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

// Settings tab layout: a language row, a speed row (mirroring the bottom
// panel so the player need not leave the tab to change pace), a save-slot
// list, and -- in place of the slot list while a modal is open -- a naming
// or overwrite-confirmation dialog. Every offset here is shared between
// drawing (see drawSettingsContent/drawSettingsDialog) and hit-testing
// below, so the two can never disagree about where a button is.
const (
	settingsLangRowY = leftCardsStartY // 82
	settingsLangRowH = 26

	settingsSpeedRowY = settingsLangRowY + settingsLangRowH + 10
	settingsSpeedRowH = 30

	settingsSlotsLabelY = settingsSpeedRowY + settingsSpeedRowH + 24
	settingsSlotsStartY = settingsSlotsLabelY + 18
	settingsSlotNameH   = 20
	settingsSlotButtonH = 26
	settingsSlotStride  = 58
	settingsSlotCount   = 5

	settingsDialogFieldY  = settingsSlotsLabelY + 20
	settingsDialogFieldH  = 28
	settingsDialogButtonY = settingsDialogFieldY + settingsDialogFieldH + 14
	settingsDialogButtonH = 30
)

// SettingsLangAt returns the language button under the cursor, in the
// settings tab's own language row.
func (l Layout) SettingsLangAt(x, y int) (i18n.Lang, bool) {
	if y < settingsLangRowY || y >= settingsLangRowY+settingsLangRowH {
		return i18n.RU, false
	}
	startX := 12
	w := l.LeftWidth - 24
	segW := w / 2
	if segW <= 0 || x < startX || x >= startX+w {
		return i18n.RU, false
	}
	if x < startX+segW {
		return i18n.RU, true
	}
	return i18n.EN, true
}

// SettingsSpeedAt returns the speed button under the cursor within the
// settings tab's own speed row (distinct from SpeedAt, which reads the
// always-visible bottom panel).
func (l Layout) SettingsSpeedAt(x, y int) (economy.Speed, bool) {
	if y < settingsSpeedRowY || y >= settingsSpeedRowY+settingsSpeedRowH {
		return economy.Normal, false
	}
	startX := 12
	w := l.LeftWidth - 24
	segW := w / 5
	if segW <= 0 || x < startX {
		return economy.Normal, false
	}
	index := (x - startX) / segW
	if index < 0 || index > int(economy.Quadruple) || x >= startX+(index+1)*segW {
		return economy.Normal, false
	}
	return economy.Speed(index), true
}

// SettingsSlotActionAt returns which save-slot button (1-5) the cursor is
// over, and whether it's the Save or Load half of that slot's row. Slots
// are 1-indexed to match the on-screen numbering the player names against.
func (l Layout) SettingsSlotActionAt(x, y int) (int, SettingsSlotAction, bool) {
	startX := 12
	w := l.LeftWidth - 24
	for i := 0; i < settingsSlotCount; i++ {
		rowY := settingsSlotsStartY + i*settingsSlotStride
		btnY := rowY + settingsSlotNameH
		if y < btnY || y >= btnY+settingsSlotButtonH {
			continue
		}
		if x < startX || x >= startX+w {
			return 0, SettingsSlotNone, false
		}
		half := w / 2
		if x < startX+half {
			return i + 1, SettingsSlotSave, true
		}
		return i + 1, SettingsSlotLoad, true
	}
	return 0, SettingsSlotNone, false
}

// SettingsDialogButtonAt returns which half of the settings tab's modal
// button row the cursor is over: true for the left (confirm) button, false
// for the right (cancel) button. The row sits at a fixed offset shared by
// both the naming and overwrite-confirmation dialogs.
func (l Layout) SettingsDialogButtonAt(x, y int) (bool, bool) {
	if y < settingsDialogButtonY || y >= settingsDialogButtonY+settingsDialogButtonH {
		return false, false
	}
	startX := 12
	w := l.LeftWidth - 24
	if x < startX || x >= startX+w {
		return false, false
	}
	return x < startX+w/2, true
}
