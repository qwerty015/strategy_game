package ui

import (
	"testing"

	"strategy_game/internal/economy"
	"strategy_game/internal/i18n"
)

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

// TestMenuTabAtCoversAllThreeTabs checks the tab bar resolves a click in the
// middle of each of the three tab buttons (Build, Hire, Settings) to that
// tab, with no gaps between them wide enough to swallow a click.
func TestMenuTabAtCoversAllThreeTabs(t *testing.T) {
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

// TestSettingsSlotActionAtCoversAllFiveSlots checks every slot's Save and
// Load buttons resolve to the right 1-indexed slot and action, matching how
// drawSettingsContent lays the row out (see settings* constants in layout.go).
func TestSettingsSlotActionAtCoversAllFiveSlots(t *testing.T) {
	layout := NewLayout(1024, 768)
	startX := 12
	w := layout.LeftWidth - 24
	half := w / 2

	for i := 0; i < settingsSlotCount; i++ {
		rowY := settingsSlotsStartY + i*settingsSlotStride
		btnY := rowY + settingsSlotNameH + settingsSlotButtonH/2

		slot, action, ok := layout.SettingsSlotActionAt(startX+half/2, btnY)
		if !ok || slot != i+1 || action != SettingsSlotSave {
			t.Fatalf("slot %d save-half resolved to slot=%d action=%v ok=%v; want slot=%d action=Save ok=true", i+1, slot, action, ok, i+1)
		}

		slot, action, ok = layout.SettingsSlotActionAt(startX+half+half/2, btnY)
		if !ok || slot != i+1 || action != SettingsSlotLoad {
			t.Fatalf("slot %d load-half resolved to slot=%d action=%v ok=%v; want slot=%d action=Load ok=true", i+1, slot, action, ok, i+1)
		}
	}

	if _, _, ok := layout.SettingsSlotActionAt(startX+half/2, settingsSlotsStartY); ok {
		t.Fatal("a click on the name line (above the button row) resolved to an action, want false")
	}
}

// TestSettingsSpeedAtCoversAllFiveSpeeds mirrors the bottom panel's SpeedAt
// test but for the settings tab's own copy of the speed control.
func TestSettingsSpeedAtCoversAllFiveSpeeds(t *testing.T) {
	layout := NewLayout(1024, 768)
	startX := 12
	w := layout.LeftWidth - 24
	segW := w / 5

	for i := 0; i <= int(economy.Quadruple); i++ {
		got, ok := layout.SettingsSpeedAt(startX+i*segW+segW/2, settingsSpeedRowY+settingsSpeedRowH/2)
		if !ok || got != economy.Speed(i) {
			t.Fatalf("speed segment %d resolved to %v, %v; want %v, true", i, got, ok, economy.Speed(i))
		}
	}
	if _, ok := layout.SettingsSpeedAt(startX, settingsSpeedRowY-5); ok {
		t.Fatal("a click above the settings speed row resolved to a speed, want false")
	}
}

// TestSettingsLangAtTogglesBothLanguages checks both halves of the
// settings tab's language row resolve to the language they're labeled with.
func TestSettingsLangAtTogglesBothLanguages(t *testing.T) {
	layout := NewLayout(1024, 768)
	startX := 12
	w := layout.LeftWidth - 24
	half := w / 2

	got, ok := layout.SettingsLangAt(startX+half/2, settingsLangRowY+settingsLangRowH/2)
	if !ok || got != i18n.RU {
		t.Fatalf("left language button resolved to %v, %v; want %v, true", got, ok, i18n.RU)
	}
	got, ok = layout.SettingsLangAt(startX+half+half/2, settingsLangRowY+settingsLangRowH/2)
	if !ok || got != i18n.EN {
		t.Fatalf("right language button resolved to %v, %v; want %v, true", got, ok, i18n.EN)
	}
}

// TestSettingsDialogButtonAtCoversBothButtons checks the shared modal
// button row (naming and overwrite-confirmation both use it) resolves left
// vs right correctly, since a wrong read here would let a player who means
// to cancel accidentally overwrite a slot instead.
func TestSettingsDialogButtonAtCoversBothButtons(t *testing.T) {
	layout := NewLayout(1024, 768)
	startX := 12
	w := layout.LeftWidth - 24
	half := w / 2

	left, ok := layout.SettingsDialogButtonAt(startX+half/2, settingsDialogButtonY+settingsDialogButtonH/2)
	if !ok || !left {
		t.Fatalf("left dialog button resolved to left=%v ok=%v; want left=true ok=true", left, ok)
	}
	left, ok = layout.SettingsDialogButtonAt(startX+half+half/2, settingsDialogButtonY+settingsDialogButtonH/2)
	if !ok || left {
		t.Fatalf("right dialog button resolved to left=%v ok=%v; want left=false ok=true", left, ok)
	}
	if _, ok := layout.SettingsDialogButtonAt(startX, settingsDialogButtonY-5); ok {
		t.Fatal("a click above the dialog button row resolved to a button, want false")
	}
}
