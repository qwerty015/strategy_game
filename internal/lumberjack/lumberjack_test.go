package lumberjack

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

func TestLumberjackCutsNearestTreeAndStoresLogAtHut(t *testing.T) {
	grid := world.NewGrid(12, 4)
	hut := &building.Building{Kind: building.LumberjackHut, X: 0, Y: 0}
	tree := building.NewTree(5, 0)
	tree.GrowthTicks = tree.GrowthTargetTicks
	buildings := []*building.Building{hut, tree}
	controller := NewController()
	jack := controller.Spawn(hut)
	if jack == nil {
		t.Fatal("Spawn() returned nil")
	}

	var cut bool
	for tick := 0; tick < 100; tick++ {
		for _, event := range controller.Tick(grid, buildings) {
			if event.Kind != TreeCut || event.Tree != tree {
				t.Fatalf("unexpected tree-cut event: %#v", event)
			}
			cut = true
			// The game layer removes the object after receiving the event.
			buildings = []*building.Building{hut}
		}
		if cut && hut.OutputBuffer[resource.Log] == 1 {
			break
		}
	}

	if !cut {
		t.Fatal("lumberjack never finished cutting the mature tree")
	}
	if got := hut.OutputBuffer[resource.Log]; got != 1 {
		t.Fatalf("hut OutputBuffer[Log] = %d, want 1", got)
	}
	_, got := jack.Cargo()
	if got != 0 {
		t.Fatalf("lumberjack cargo after unloading = %d, want 0", got)
	}
}
