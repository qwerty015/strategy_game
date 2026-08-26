package ui

import "testing"

func TestLayoutKeepsPanelsAtWindowEdges(t *testing.T) {
	for _, size := range [][2]int{{1024, 768}, {800, 600}, {640, 480}} {
		layout := NewLayout(size[0], size[1])
		if got := layout.LeftPanel().Min.X; got != 0 {
			t.Fatalf("left panel starts at %d, want 0", got)
		}
		if got := layout.RightPanel().Max.X; got != size[0] {
			t.Fatalf("right panel ends at %d, want window width %d", got, size[0])
		}
		if got := layout.BottomPanel().Max.X; got != size[0] {
			t.Fatalf("bottom panel ends at %d, want window width %d", got, size[0])
		}
		if layout.MapRect().Dx() <= 0 || layout.MapRect().Dy() <= 0 {
			t.Fatalf("map rect is not usable for window %dx%d: %v", size[0], size[1], layout.MapRect())
		}
	}
}

func TestBuildPaletteKeepsEveryCardClickable(t *testing.T) {
	layout := NewLayout(1024, 768)
	palette := NewPalette()
	for i := range palette.Kinds {
		x, y := 20, leftCardsStartY+4+i*leftCardStride
		got, ok := layout.BuildIndexAt(x, y, len(palette.Kinds))
		if !ok || got != i {
			t.Fatalf("card %d at (%d,%d) resolved to %d, %v", i, x, y, got, ok)
		}
		if y >= layout.BottomPanel().Min.Y {
			t.Fatalf("card %d starts inside bottom panel at y=%d", i, y)
		}
	}
}

// TestPriorityLevelAtCoversAllFiveSegments checks the five-segment supply
// priority control (see DrawInspectorPanel's priority row) maps clicks to
// levels -2..2 left to right, with no gaps and nothing outside the row.
func TestPriorityLevelAtCoversAllFiveSegments(t *testing.T) {
	layout := NewLayout(1024, 768)
	r := layout.RightPanel()
	rowY := r.Max.Y - priorityRowHeight - priorityBottomGap
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
