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

// BuildIndexAt returns the building palette card under the cursor.
func (l Layout) BuildIndexAt(x, y int, count int) (int, bool) {
	stride, height := l.buildCardGeometry(count)
	return l.menuIndexAtFrom(x, y, count, leftBuildCardsStartY, stride, height)
}

// DemolitionModeRect is the Build-tab switch for continuous removal. Keeping
// its rectangle in Layout gives input and rendering exactly the same bounds.
func (l Layout) DemolitionModeRect() image.Rectangle {
	return image.Rect(12, leftDemolitionY, l.LeftWidth-12, leftDemolitionY+leftDemolitionHeight)
}

func (l Layout) DemolitionModeAt(x, y int) bool {
	return image.Pt(x, y).In(l.DemolitionModeRect())
}

// HireIndexAt returns the hire-menu card under the cursor.
func (l Layout) HireIndexAt(x, y int, count int) (int, bool) {
	stride, height := l.cardGeometry(count)
	return l.menuIndexAtFrom(x, y, count, leftCardsStartY, stride, height)
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
