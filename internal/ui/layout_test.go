package ui

import (
	"testing"

	"strategy_game/internal/building"
)

// TestMinimapRectStaysInsideRightPanel guards the docked bottom-center
// minimap against ever drifting outside its own panel or overlapping the
// title bar, across the same window sizes TestLayoutKeepsPanelsAtWindowEdges
// checks.
func TestMinimapRectStaysInsideRightPanel(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}, {800, 600}} {
		layout := NewLayout(size[0], size[1])
		right := layout.RightPanel()
		m := layout.MinimapRect()
		if !m.In(right) {
			t.Fatalf("size %v: minimap %v not inside right panel %v", size, m, right)
		}
		c := layout.ClockRect()
		if c.Min.Y >= m.Min.Y {
			t.Fatalf("size %v: clock row %v does not sit above minimap %v", size, c, m)
		}
		if !c.In(right) {
			t.Fatalf("size %v: clock row %v not inside right panel %v", size, c, right)
		}
	}
}

// TestPriorityRowClearsTheMinimap locks in the fix for the real bug this
// docking introduced: before rightPanelUsableBottom existed, the priority
// row and minimap could overlap because both were independently anchored to
// the panel's raw bottom edge.
func TestPriorityRowClearsTheMinimap(t *testing.T) {
	layout := NewLayout(1024, 768)
	rowY := layout.rightPanelUsableBottom() - priorityRowHeight - priorityBottomGap
	priorityBottom := rowY + priorityRowHeight
	if minimapTop := layout.MinimapRect().Min.Y; priorityBottom > minimapTop {
		t.Fatalf("priority row bottom %d overlaps minimap top %d", priorityBottom, minimapTop)
	}
}

func TestLayoutKeepsPanelsAtWindowEdges(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {800, 600}, {640, 480}} {
		layout := NewLayout(size[0], size[1])
		if got := layout.LeftPanel().Min.X; got != 0 {
			t.Fatalf("left panel starts at %d, want 0", got)
		}
		if got := layout.RightPanel().Max.X; got != size[0] {
			t.Fatalf("right panel ends at %d, want window width %d", got, size[0])
		}
		if got := layout.RightPanel().Max.Y; got != size[1] {
			t.Fatalf("right panel ends at y=%d, want window height %d", got, size[1])
		}
		if got := layout.MapRect().Max.Y; got != size[1] {
			t.Fatalf("map ends at y=%d, want window height %d", got, size[1])
		}
		if layout.MapRect().Dx() <= 0 || layout.MapRect().Dy() <= 0 {
			t.Fatalf("map rect is not usable for window %dx%d: %v", size[0], size[1], layout.MapRect())
		}
	}
}

// TestLeftListWindow_ScrollsOnceTheFloorStrideCantFitEverything covers the
// user's explicit request for a scrollable left panel: a short window
// forces even the floor stride to overflow, and scroll must reveal the
// later items instead of everything staying permanently off-screen.
func TestLeftListWindow_ScrollsOnceTheFloorStrideCantFitEverything(t *testing.T) {
	layout := NewLayout(1024, 300) // short window: floor stride still overflows
	count := 20

	stride, height, start, visible := layout.leftListWindow(leftCardsStartY, count, 0)
	if visible >= count {
		t.Fatalf("visible = %d, want fewer than count (%d) so scrolling is actually needed", visible, count)
	}
	if stride < leftCardMinStride || height < leftCardMinHeight {
		t.Fatalf("stride/height = %d/%d, want at least the floor (%d/%d)", stride, height, leftCardMinStride, leftCardMinHeight)
	}
	if start != 0 {
		t.Fatalf("start at scroll=0 = %d, want 0", start)
	}

	// Scrolling past the end clamps to the last full page, not an empty tail.
	_, _, start, visible = layout.leftListWindow(leftCardsStartY, count, count)
	if start != count-visible {
		t.Fatalf("start at an over-large scroll = %d, want %d (clamped to the last page)", start, count-visible)
	}

	// A point resolved via HireIndexAt at a mid-scroll offset must return
	// an index from the scrolled window, not the unscrolled one.
	scroll := 5
	_, _, start, _ = layout.leftListWindow(leftCardsStartY, count, scroll)
	x, y := 20, leftCardsStartY+4
	got, ok := layout.HireIndexAt(x, y, count, scroll)
	if !ok || got != start {
		t.Fatalf("HireIndexAt at the top row with scroll=%d = (%d, %v), want (%d, true)", scroll, got, ok, start)
	}
}

func TestBuildPaletteKeepsEveryCardClickable(t *testing.T) {
	layout := NewLayout(1024, 768)
	palette := NewPalette()
	stride, _ := layout.buildCardGeometry(len(palette.Kinds))
	for i := range palette.Kinds {
		x, y := 20, leftBuildCardsStartY+4+i*stride
		got, ok := layout.BuildIndexAt(x, y, len(palette.Kinds), 0)
		if !ok || got != i {
			t.Fatalf("card %d at (%d,%d) resolved to %d, %v", i, x, y, got, ok)
		}
		if y >= layout.Height {
			t.Fatalf("card %d starts outside the left panel at y=%d", i, y)
		}
	}
}

func TestInspectorConfirmRemoveButtonsCoverBothActions(t *testing.T) {
	layout := NewLayout(1024, 768)
	confirm, cancel := layout.InspectorConfirmRemoveButtons()
	if got, ok := layout.InspectorConfirmRemoveAt(confirm.Min.X+confirm.Dx()/2, confirm.Min.Y+confirm.Dy()/2); !ok || !got {
		t.Fatalf("confirm button resolved to confirm=%v ok=%v, want true true", got, ok)
	}
	if got, ok := layout.InspectorConfirmRemoveAt(cancel.Min.X+cancel.Dx()/2, cancel.Min.Y+cancel.Dy()/2); !ok || got {
		t.Fatalf("cancel button resolved to confirm=%v ok=%v, want false true", got, ok)
	}
	if _, ok := layout.InspectorConfirmRemoveAt(confirm.Min.X, confirm.Min.Y-1); ok {
		t.Fatal("a click above the confirmation buttons resolved to an action")
	}
}

// TestPriorityLevelAtCoversAllFiveSegments checks the five-segment supply
// priority control (see DrawInspectorPanel's priority row) maps clicks to
// levels -2..2 left to right, with no gaps and nothing outside the row.
// TestPalettePlacesTheWholeFoodChainFirst keeps the construction menu ordered
// around how a settlement is built: food sources and processing lead to the
// Tavern, while roads, storage and extraction follow afterwards.
func TestPalettePlacesTheWholeFoodChainFirst(t *testing.T) {
	want := []building.Kind{
		building.Farm,
		building.Mill,
		building.Bakery,
		building.Winery,
		building.FisherHut,
		building.PigFarm,
		building.MeatWorkshop,
		building.Tavern,
		building.Road,
		building.StoneWall,
		building.Gate,
		building.WatchTower,
		building.Barracks,
		building.Armory,
		building.Warehouse,
		building.LumberjackHut,
		building.CarpentryWorkshop,
		building.QuarryHut,
		building.MinerHut,
		building.Smeltery,
	}
	got := NewPalette().Kinds
	if len(got) != len(want) {
		t.Fatalf("palette length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("palette item %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestPriorityLevelAtCoversAllFiveSegments checks the five-segment supply
// priority control (see DrawInspectorPanel's priority row) maps clicks to
// levels -2..2 left to right, with no gaps and nothing outside the row.
func TestPriorityLevelAtCoversAllFiveSegments(t *testing.T) {
	layout := NewLayout(1024, 768)
	r := layout.RightPanel()
	// Anchored to rightPanelUsableBottom, not r.Max.Y directly, since the
	// minimap/clock now reserve a fixed band at the panel's actual bottom
	// (see Layout.MinimapRect) that this row must stay clear of.
	rowY := layout.rightPanelUsableBottom() - priorityRowHeight - priorityBottomGap
	startX := r.Min.X + priorityMargin
	segW := (r.Dx() - 2*priorityMargin) / 5

	for i := range 5 {
		x := startX + i*segW + segW/2
		got, ok := layout.PriorityLevelAt(x, rowY+priorityRowHeight/2)
		want := i - 2
		if !ok || got != want {
			t.Fatalf("segment %d at x=%d resolved to %d, %v; want %d, true", i, x, got, ok, want)
		}
	}

	if _, ok := layout.PriorityLevelAt(startX, rowY-5); ok {
		t.Fatal("a click above the priority row resolved to a level, want false")
	}
	if _, ok := layout.PriorityLevelAt(startX, rowY+priorityRowHeight+5); ok {
		t.Fatal("a click below the priority row resolved to a level, want false")
	}
	if _, ok := layout.PriorityLevelAt(startX-10, rowY+5); ok {
		t.Fatal("a click left of the priority row resolved to a level, want false")
	}
}

// TestMenuTabAtCoversVisibleTabs checks the tab bar resolves a click in the
// middle of each visible tab button (Build and Hire) to that
// tab, with no gaps between them wide enough to swallow a click.
// TestInspectorRemoveButtonMovesAbovePriority keeps the selected object's
// removal action clickable without covering the priority label or segments.
func TestInspectorRemoveButtonMovesAbovePriority(t *testing.T) {
	layout := NewLayout(1024, 768)
	plain := layout.InspectorRemoveRect(false)
	if !layout.InspectorRemoveAt(plain.Min.X+plain.Dx()/2, plain.Min.Y+plain.Dy()/2, false) {
		t.Fatal("plain inspector removal button is not clickable at its center")
	}

	withPriority := layout.InspectorRemoveRect(true)
	if !layout.InspectorRemoveAt(withPriority.Min.X+withPriority.Dx()/2, withPriority.Min.Y+withPriority.Dy()/2, true) {
		t.Fatal("priority inspector removal button is not clickable at its center")
	}
	rowY := layout.RightPanel().Max.Y - priorityRowHeight - priorityBottomGap
	if withPriority.Max.Y > rowY-18 {
		t.Fatalf("removal button ends at y=%d, overlaps priority label beginning at y=%d", withPriority.Max.Y, rowY-18)
	}
	if withPriority.Min.Y >= plain.Min.Y {
		t.Fatalf("priority removal button y=%d, want above plain y=%d", withPriority.Min.Y, plain.Min.Y)
	}
}

func TestMenuTabAtCoversVisibleTabs(t *testing.T) {
	layout := NewLayout(1024, 768)
	for i := 0; i < tabCount; i++ {
		x, w := layout.tabRect(i)
		got, ok := layout.MenuTabAt(x+w/2, leftTabY+leftTabHeight/2)
		if !ok || got != LeftTab(i) {
			t.Fatalf("tab %d center resolved to %d, %v; want %d, true", i, got, ok, i)
		}
	}
	if _, ok := layout.MenuTabAt(0, leftTabY+5); ok {
		t.Fatal("a click at the panel's far left edge resolved to a tab, want false (outside the 12px margin)")
	}
	if _, ok := layout.MenuTabAt(20, leftTabY-5); ok {
		t.Fatal("a click above the tab row resolved to a tab, want false")
	}
}

func TestDemolitionModeHitAreaMatchesItsRectangle(t *testing.T) {
	layout := NewLayout(1024, 768)
	r := layout.DemolitionModeRect()
	if !layout.DemolitionModeAt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2) {
		t.Fatal("centre of demolition button is not clickable")
	}
	if layout.DemolitionModeAt(r.Max.X, r.Min.Y+r.Dy()/2) {
		t.Fatal("point outside demolition button is clickable")
	}
}

// TestGateControlRects keeps the rendered gate controls and their hit testing
// coupled through Layout, just like the removal confirmation and priorities.
func TestGateControlRects(t *testing.T) {
	layout := NewLayout(1024, 768)
	toggle, auto := layout.GateControlRects()
	if !layout.GateToggleAt(toggle.Min.X+toggle.Dx()/2, toggle.Min.Y+toggle.Dy()/2) {
		t.Fatal("gate toggle center did not hit its control")
	}
	if !layout.GateAutoAt(auto.Min.X+auto.Dx()/2, auto.Min.Y+auto.Dy()/2) {
		t.Fatal("gate auto center did not hit its control")
	}
	if layout.GateToggleAt(toggle.Min.X, toggle.Min.Y-1) || layout.GateAutoAt(auto.Max.X, auto.Max.Y) {
		t.Fatal("outside gate control edge was accepted")
	}
}
