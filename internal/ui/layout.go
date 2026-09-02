package ui

import (
	"image"
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
	leftTabY             = 44
	leftTabHeight        = 30
	leftCardsStartY      = 82
	leftDemolitionY      = leftCardsStartY
	leftDemolitionHeight = 30
	leftBuildCardsStartY = leftDemolitionY + leftDemolitionHeight + 8
	leftCardHeight       = 40
	leftCardStride       = 44

	// leftCardMinStride/leftCardMinHeight bound how far cardGeometry will
	// shrink a long list. A list that would need to go below these is a
	// sign the panel needs scrolling, not further compression.
	leftCardMinStride = 32
	leftCardMinHeight = 28
)

// cardGeometry returns the vertical stride and height for a normal left-panel
// card list (Hire). Build has a compact demolition control above its cards,
// therefore it starts at a separate y-coordinate below.
func (l Layout) cardGeometry(count int) (stride, height int) {
	return l.cardGeometryFrom(leftCardsStartY, count)
}

func (l Layout) buildCardGeometry(count int) (stride, height int) {
	return l.cardGeometryFrom(leftBuildCardsStartY, count)
}

func (l Layout) cardGeometryFrom(startY, count int) (stride, height int) {
	stride, height = leftCardStride, leftCardHeight
	if count <= 0 {
		return
	}
	available := l.Height - startY
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
	tabCount = 2 // Build, Hire; game options live in the Esc pause menu.
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

// leftListWindow computes a scrollable left-panel card list's geometry and
// which slice of items is actually visible right now. cardGeometryFrom
// already shrinks stride/height to fit everything down to
// leftCardMinStride/leftCardMinHeight -- once a list is long enough that
// even that floor can't fit every item in the available height, further
// items stop shrinking and instead scroll: only as many rows as actually
// fit (visible) are shown, starting at index start. scroll is the desired
// first-visible index (e.g. from a mouse wheel accumulator); it is
// clamped here, so a caller never needs to know count in advance to keep
// it in range.
func (l Layout) leftListWindow(startY, count, scroll int) (stride, height, start, visible int) {
	stride, height = l.cardGeometryFrom(startY, count)
	if count <= 0 {
		return stride, height, 0, 0
	}
	available := l.Height - startY
	rows := count
	if available > 0 && stride > 0 {
		rows = available / stride
		if rows < 1 {
			rows = 1
		}
	}
	if rows >= count {
		return stride, height, 0, count
	}
	start = scroll
	if start < 0 {
		start = 0
	}
	if start > count-rows {
		start = count - rows
	}
	return stride, height, start, rows
}

// BuildIndexAt returns the building palette card under the cursor,
// accounting for the current scroll offset -- see Layout.leftListWindow.
func (l Layout) BuildIndexAt(x, y int, count, scroll int) (int, bool) {
	stride, height, start, visible := l.leftListWindow(leftBuildCardsStartY, count, scroll)
	local, ok := l.menuIndexAtFrom(x, y, visible, leftBuildCardsStartY, stride, height)
	if !ok {
		return 0, false
	}
	return start + local, true
}

// DemolitionModeRect is the Build-tab switch for continuous removal. Keeping
// its rectangle in Layout gives input and rendering exactly the same bounds.
func (l Layout) DemolitionModeRect() image.Rectangle {
	return image.Rect(12, leftDemolitionY, l.LeftWidth-12, leftDemolitionY+leftDemolitionHeight)
}

func (l Layout) DemolitionModeAt(x, y int) bool {
	return image.Pt(x, y).In(l.DemolitionModeRect())
}

// HireIndexAt returns the hire-menu card under the cursor, accounting for
// the current scroll offset -- see Layout.leftListWindow.
func (l Layout) HireIndexAt(x, y int, count, scroll int) (int, bool) {
	stride, height, start, visible := l.leftListWindow(leftCardsStartY, count, scroll)
	local, ok := l.menuIndexAtFrom(x, y, visible, leftCardsStartY, stride, height)
	if !ok {
		return 0, false
	}
	return start + local, true
}

func (l Layout) menuIndexAtFrom(x, y, count, startY, stride, height int) (int, bool) {
	card := image.Rect(12, startY, l.LeftWidth-12, startY+height)
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
	gateControlHeight          = 30
	gateControlGap             = 8
	gateControlTop             = 224
	barracksHireTop            = 316
	barracksHireHeight         = 38
	armoryQueueTop             = 420
)

const (
	inspectorConfirmRemoveTop     = 164
	inspectorConfirmRemoveHeight  = 94
	inspectorConfirmRemoveMargin  = 18
	inspectorConfirmRemoveButtonH = 30
	inspectorConfirmRemoveButtonY = 52
)

// The minimap and clock (see MinimapRect/ClockRect) are docked in a fixed
// band at the very bottom of the right panel, always visible regardless of
// what's selected. Every other bottom-anchored inspector element (the
// remove button, the priority control, the removal confirmation dialog)
// reads its "floor" from rightPanelUsableBottom instead of the panel's raw
// Max.Y, so nothing can ever be drawn underneath -- or hit-test behind --
// the minimap.
const (
	minimapSize      = 168
	minimapMargin    = 18
	minimapBottomGap = 14
	clockRowHeight   = 22
	clockMinimapGap  = 6
	minimapReserved  = clockRowHeight + clockMinimapGap + minimapSize + minimapBottomGap
)

// MinimapRect is the fixed square the minimap draws into, docked bottom
// center of the right panel.
func (l Layout) MinimapRect() image.Rectangle {
	r := l.RightPanel()
	size := minimapSize
	if usable := r.Dx() - 2*minimapMargin; size > usable {
		size = usable
	}
	if size < 0 {
		size = 0
	}
	x := r.Min.X + (r.Dx()-size)/2
	bottom := r.Max.Y - minimapBottomGap
	top := bottom - size
	return image.Rect(x, top, x+size, bottom)
}

// ClockRect is the thin row directly above the minimap where the phase icon
// and in-game hour are drawn.
func (l Layout) ClockRect() image.Rectangle {
	m := l.MinimapRect()
	return image.Rect(m.Min.X, m.Min.Y-clockMinimapGap-clockRowHeight, m.Max.X, m.Min.Y-clockMinimapGap)
}

// rightPanelUsableBottom is where bottom-anchored inspector content must
// stop so it never overlaps the minimap/clock band reserved above.
func (l Layout) rightPanelUsableBottom() int {
	return l.RightPanel().Max.Y - minimapReserved
}

// InspectorConfirmRemoveRect is the modal that confirms a destructive
// inspector action. It deliberately lives in the inspector, beside the
// selected object, rather than returning the player to a separate UI area.
func (l Layout) InspectorConfirmRemoveRect() image.Rectangle {
	r := l.RightPanel()
	floor := l.rightPanelUsableBottom()
	top := r.Min.Y + inspectorConfirmRemoveTop
	bottom := top + inspectorConfirmRemoveHeight
	if bottom > floor-inspectorConfirmRemoveMargin {
		bottom = floor - inspectorConfirmRemoveMargin
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
	y := l.rightPanelUsableBottom() - inspectorRemoveBottomGap - inspectorRemoveHeight
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

// GateControlRects returns the manual-open and automatic-mode controls for a
// selected finished gate. They are deliberately near the gate's short info
// block instead of bottom-docked beside demolition, so their state is visible
// and reachable without scrolling past the minimap.
func (l Layout) GateControlRects() (toggle, auto image.Rectangle) {
	r := l.RightPanel()
	left, right := r.Min.X+inspectorRemoveMargin, r.Max.X-inspectorRemoveMargin
	toggle = image.Rect(left, gateControlTop, right, gateControlTop+gateControlHeight)
	autoTop := toggle.Max.Y + gateControlGap
	auto = image.Rect(left, autoTop, right, autoTop+gateControlHeight)
	return toggle, auto
}

func (l Layout) GateToggleAt(x, y int) bool {
	toggle, _ := l.GateControlRects()
	return image.Pt(x, y).In(toggle)
}

func (l Layout) GateAutoAt(x, y int) bool {
	_, auto := l.GateControlRects()
	return image.Pt(x, y).In(auto)
}

// BarracksHireRects returns the three stacked hire buttons for a selected
// Barracks -- Sentry, Archer, Swordsman, in that order. They begin below the
// four equipment rows of the Barracks inspector, so its stock and actions
// cannot overlap at any window size.
func (l Layout) BarracksHireRects() (sentry, archer, swordsman image.Rectangle) {
	r := l.RightPanel()
	left, right := r.Min.X+inspectorRemoveMargin, r.Max.X-inspectorRemoveMargin
	sentry = image.Rect(left, barracksHireTop, right, barracksHireTop+barracksHireHeight)
	archerTop := sentry.Max.Y + gateControlGap
	archer = image.Rect(left, archerTop, right, archerTop+barracksHireHeight)
	swordsmanTop := archer.Max.Y + gateControlGap
	swordsman = image.Rect(left, swordsmanTop, right, swordsmanTop+barracksHireHeight)
	return
}

// BarracksHireIndexAt reports which hire button (0=Sentry, 1=Archer,
// 2=Swordsman) the cursor is over, if any.
func (l Layout) BarracksHireIndexAt(x, y int) (int, bool) {
	sentry, archer, swordsman := l.BarracksHireRects()
	pt := image.Pt(x, y)
	switch {
	case pt.In(sentry):
		return 0, true
	case pt.In(archer):
		return 1, true
	case pt.In(swordsman):
		return 2, true
	}
	return 0, false
}

// FormationLinesRects returns the three "1/2/3 ranks" buttons for a
// selected soldier group -- per the user's clarification that these are
// panel buttons, not keyboard shortcuts ("1/2/3 - это не клавиши на
// клавиатуре, а количество шеренг, кнопки в правой панели при выбранных
// юнитах"). Same top slot as the Barracks/Gate/Armory controls, never
// shown together with any of those.
func (l Layout) FormationLinesRects() (one, two, three image.Rectangle) {
	r := l.RightPanel()
	left := r.Min.X + inspectorRemoveMargin
	const size, gap = 30, 8
	one = image.Rect(left, gateControlTop, left+size, gateControlTop+size)
	two = image.Rect(left+size+gap, gateControlTop, left+2*size+gap, gateControlTop+size)
	three = image.Rect(left+2*(size+gap), gateControlTop, left+3*size+2*gap, gateControlTop+size)
	return
}

// FormationLinesAt reports which rank-count button (1, 2 or 3) the cursor
// is over, if any.
func (l Layout) FormationLinesAt(x, y int) (int, bool) {
	one, two, three := l.FormationLinesRects()
	pt := image.Pt(x, y)
	switch {
	case pt.In(one):
		return 1, true
	case pt.In(two):
		return 2, true
	case pt.In(three):
		return 3, true
	}
	return 0, false
}

// SoldierGroupActionRects returns the "разъединить"/"отряд по виду"
// buttons for a selected soldier group, stacked directly below
// FormationLinesRects -- per the user's explicit request for a way to
// split a merged group back apart, or select every soldier of one
// profession regardless of distance.
func (l Layout) SoldierGroupActionRects() (split, byProfession image.Rectangle) {
	r := l.RightPanel()
	left, right := r.Min.X+inspectorRemoveMargin, r.Max.X-inspectorRemoveMargin
	_, _, three := l.FormationLinesRects()
	top := three.Max.Y + gateControlGap
	half := (right - left - gateControlGap) / 2
	split = image.Rect(left, top, left+half, top+gateControlHeight)
	byProfession = image.Rect(left+half+gateControlGap, top, right, top+gateControlHeight)
	return
}

// SoldierGroupSplitAt/SoldierGroupByProfessionAt hit-test the two buttons
// SoldierGroupActionRects lays out.
func (l Layout) SoldierGroupSplitAt(x, y int) bool {
	split, _ := l.SoldierGroupActionRects()
	return image.Pt(x, y).In(split)
}

func (l Layout) SoldierGroupByProfessionAt(x, y int) bool {
	_, byProfession := l.SoldierGroupActionRects()
	return image.Pt(x, y).In(byProfession)
}

// armoryQueueRowHeight/armoryQueueRowGap/armoryQueueButtonWidth size the
// three item rows drawn below the Armory's own input/output inspector. A
// dedicated count cell sits between the name and the +/- buttons.
const (
	armoryQueueRowHeight    = 28
	armoryQueueRowGap       = 6
	armoryQueueButtonWidth  = 24
	armoryQueueButtonMargin = 4
	armoryQueueCountWidth   = 42
)

// ArmoryQueueRowRects returns row i's (0=Bow, 1=LeatherArmor, 2=Sword --
// see armoryQueueItems) full row rect plus its minus/plus button rects,
// for a selected Armory's production queue.
func (l Layout) ArmoryQueueRowRects(i int) (row, minus, plus image.Rectangle) {
	r := l.RightPanel()
	left, right := r.Min.X+inspectorRemoveMargin, r.Max.X-inspectorRemoveMargin
	top := armoryQueueTop + i*(armoryQueueRowHeight+armoryQueueRowGap)
	row = image.Rect(left, top, right, top+armoryQueueRowHeight)
	plus = image.Rect(right-armoryQueueButtonWidth, top, right, top+armoryQueueRowHeight)
	minus = image.Rect(plus.Min.X-armoryQueueButtonMargin-armoryQueueButtonWidth, top, plus.Min.X-armoryQueueButtonMargin, top+armoryQueueRowHeight)
	return row, minus, plus
}

// ArmoryQueueCountRect returns the dedicated count cell between a queue row's
// item label and its +/- buttons. Keeping it in Layout makes the displayed
// quantity and click targets independent instead of letting text overlap them.
func (l Layout) ArmoryQueueCountRect(i int) image.Rectangle {
	row, minus, _ := l.ArmoryQueueRowRects(i)
	right := minus.Min.X - armoryQueueButtonMargin
	return image.Rect(right-armoryQueueCountWidth, row.Min.Y, right, row.Max.Y)
}

// ArmoryQueueButtonAt reports which row's minus (delta -1) or plus (delta
// +1) button the cursor is over, if any -- cmd/game multiplies delta by 10
// itself on a right-click, per the user's explicit "клик ПКМ +/- 10 штук в
// очередь, ЛКМ +/- 1 в очередь".
func (l Layout) ArmoryQueueButtonAt(x, y int) (row int, delta int, ok bool) {
	pt := image.Pt(x, y)
	for i := 0; i < 3; i++ {
		_, minus, plus := l.ArmoryQueueRowRects(i)
		if pt.In(minus) {
			return i, -1, true
		}
		if pt.In(plus) {
			return i, 1, true
		}
	}
	return 0, 0, false
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
	rowY := l.rightPanelUsableBottom() - priorityRowHeight - priorityBottomGap
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
