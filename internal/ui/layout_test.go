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
