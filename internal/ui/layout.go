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

	LeftWidth  int
	RightWidth int
}

const (
	leftTabY        = 44
	leftTabHeight   = 30
	leftCardsStartY = 82
	leftCardHeight  = 40
	leftCardStride  = 44

	// leftCardMinStride/leftCardMinHeight bound how far cardGeometry will
	// shrink a long list. A list that would need to go below these is a
	// sign the panel needs scrolling, not further compression.
	leftCardMinStride = 32
	leftCardMinHeight = 28
)

// cardGeometry returns the vertical stride and height to draw/hit-test count
// cards in the left panel's card list (the Build palette or the Hire tab).
// The list keeps its normal leftCardStride/leftCardHeight as long as it
// fits inside the left panel; a longer list -- the Build palette gains a
// new card with almost every new building kind -- is compressed just
// enough to keep every card clickable instead of running beyond the left
// panel. Shared by drawing and hit-testing so the two never disagree about
// where a card actually is.
func (l Layout) cardGeometry(count int) (stride, height int) {
	stride, height = leftCardStride, leftCardHeight
	if count <= 0 {
		return
	}
	available := l.Height - leftCardsStartY
	if available <= 0 || stride*count <= available {
		return
	}
	stride = available / count
	if stride < leftCardMinStride {
		stride = leftCardMinStride
	}
	height = stride - 4
	if height < leftCardMinHeight {
		height = leftCardMinHeight
	}
	return
}

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
	return Layout{
		Width:      width,
		Height:     height,
		LeftWidth:  left,
		RightWidth: right,
	}
}

func (l Layout) LeftPanel() image.Rectangle {
	return image.Rect(0, 0, l.LeftWidth, l.Height)
}

func (l Layout) RightPanel() image.Rectangle {
	return image.Rect(l.Width-l.RightWidth, 0, l.Width, l.Height)
}

func (l Layout) MapRect() image.Rectangle {
	return image.Rect(l.LeftWidth, 0, l.Width-l.RightWidth, l.Height)
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
	stride, height := l.cardGeometry(count)
	card := image.Rect(12, leftCardsStartY, l.LeftWidth-12, leftCardsStartY+height)
	for i := 0; i < count; i++ {
		r := card.Add(image.Pt(0, i*stride))
		if image.Pt(x, y).In(r) {
			return i, true
		}
	}
	return 0, false
}

const (
	priorityRowHeight = 30
	priorityMargin    = 18
	priorityBottomGap = 12

	inspectorRemoveHeight      = 30
	inspectorRemoveMargin      = 18
	inspectorRemoveBottomGap   = 12
	inspectorRemovePriorityGap = 26
)

const (
	inspectorConfirmRemoveTop     = 164
	inspectorConfirmRemoveHeight  = 94
	inspectorConfirmRemoveMargin  = 18
	inspectorConfirmRemoveButtonH = 30
	inspectorConfirmRemoveButtonY = 52
)

// InspectorConfirmRemoveRect is the modal that confirms a destructive
// inspector action. It deliberately lives in the inspector, beside the
// selected object, rather than returning the player to a separate UI area.
func (l Layout) InspectorConfirmRemoveRect() image.Rectangle {
	r := l.RightPanel()
	top := r.Min.Y + inspectorConfirmRemoveTop
	bottom := top + inspectorConfirmRemoveHeight
	if bottom > r.Max.Y-inspectorConfirmRemoveMargin {
		bottom = r.Max.Y - inspectorConfirmRemoveMargin
		top = bottom - inspectorConfirmRemoveHeight
	}
	return image.Rect(r.Min.X+inspectorConfirmRemoveMargin, top, r.Max.X-inspectorConfirmRemoveMargin, bottom)
}

// InspectorConfirmRemoveButtons returns the matching confirm/cancel button
// rectangles. Keeping them in Layout makes rendering and hit-testing share
// one geometry for every removable object.
func (l Layout) InspectorConfirmRemoveButtons() (confirm, cancel image.Rectangle) {
	r := l.InspectorConfirmRemoveRect()
	buttonY := r.Min.Y + inspectorConfirmRemoveButtonY
	half := r.Dx() / 2
	confirm = image.Rect(r.Min.X, buttonY, r.Min.X+half-2, buttonY+inspectorConfirmRemoveButtonH)
	cancel = image.Rect(r.Min.X+half, buttonY, r.Max.X, buttonY+inspectorConfirmRemoveButtonH)
	return confirm, cancel
}

// InspectorConfirmRemoveAt reports which confirmation button is under the
// cursor: true for confirm, false for cancel.
func (l Layout) InspectorConfirmRemoveAt(x, y int) (bool, bool) {
	point := image.Pt(x, y)
	confirm, cancel := l.InspectorConfirmRemoveButtons()
	if point.In(confirm) {
		return true, true
	}
	if point.In(cancel) {
		return false, true
	}
	return false, false
}

// InspectorRemoveRect returns the removal button bounds in the inspector.
// When the selected building exposes its supply-priority control, the button
// moves above that control and its label; otherwise it stays docked at the
// panel's bottom. Drawing and hit-testing use this one rectangle.
func (l Layout) InspectorRemoveRect(showPriority bool) image.Rectangle {
	r := l.RightPanel()
	y := r.Max.Y - inspectorRemoveBottomGap - inspectorRemoveHeight
	if showPriority {
		y -= priorityRowHeight + priorityBottomGap + inspectorRemovePriorityGap
	}
	return image.Rect(r.Min.X+inspectorRemoveMargin, y, r.Max.X-inspectorRemoveMargin, y+inspectorRemoveHeight)
}

// InspectorRemoveAt reports whether the cursor is over the selected object's
// removal button. Callers must first check CanRemoveSelection.
func (l Layout) InspectorRemoveAt(x, y int, showPriority bool) bool {
	return image.Pt(x, y).In(l.InspectorRemoveRect(showPriority))
}

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

// Settings tab layout: a language row, the game's only speed-control row, a
// save-slot list, and -- in place of the slot list while a modal is open -- a naming
// or overwrite-confirmation dialog. Every offset here is shared between
// drawing (see drawSettingsContent/drawSettingsDialog) and hit-testing
// below, so the two can never disagree about where a button is.
const (
	settingsLangRowY = leftCardsStartY // 82
	settingsLangRowH = 26

	settingsSpeedRowY = settingsLangRowY + settingsLangRowH + 10
	settingsSpeedRowH = 30

	// settingsNewGameRowY/H is the full-width "New Game" button, docked
	// between the speed row and the save-slot list -- always visible (like
	// the language/speed rows), unlike the slot list which the confirm
	// dialog below temporarily replaces.
	settingsNewGameRowY = settingsSpeedRowY + settingsSpeedRowH + 14
	settingsNewGameRowH = 28

	settingsSlotsLabelY = settingsNewGameRowY + settingsNewGameRowH + 20
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
// settings tab's speed row. Speed is intentionally configured only here.
func (l Layout) SettingsSpeedAt(x, y int) (economy.Speed, bool) {
	if y < settingsSpeedRowY || y >= settingsSpeedRowY+settingsSpeedRowH {
		return economy.Normal, false
	}
	startX := 12
	w := l.LeftWidth - 24
	segW := w / 6
	if segW <= 0 || x < startX {
		return economy.Normal, false
	}
	index := (x - startX) / segW
	if index < 0 || index > int(economy.Octuple) || x >= startX+(index+1)*segW {
		return economy.Normal, false
	}
	return economy.Speed(index), true
}

// SettingsNewGameAt reports whether the cursor is over the settings tab's
// "New Game" button. Callers must also check that no dialog is currently
// open (see Update's dialog branch), the same way every other settings-tab
// hit-test already implicitly relies on handleMouse not running then.
func (l Layout) SettingsNewGameAt(x, y int) bool {
	if y < settingsNewGameRowY || y >= settingsNewGameRowY+settingsNewGameRowH {
		return false
	}
	startX := 12
	w := l.LeftWidth - 24
	return x >= startX && x < startX+w
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
