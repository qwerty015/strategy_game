package lumberjack

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// TestLumberjack_EatsAtNearestReachableTavern covers "NPC кушают только в
// одной харчевне": a hungry lumberjack used to always walk to whichever
// Tavern happened to be first in the buildings slice, no matter how far
// away. tavernFar is listed before tavernNear specifically to rule that
// out. Lumberjacks aren't road-bound, so no roads are needed here -- just
// open ground on the grid.
func TestLumberjack_EatsAtNearestReachableTavern(t *testing.T) {
	grid := world.NewGrid(15, 4)
	hut := &building.Building{Kind: building.LumberjackHut, X: 0, Y: 0}
	tavernFar := &building.Building{Kind: building.Tavern, X: 12, Y: 0}
	tavernNear := &building.Building{Kind: building.Tavern, X: 3, Y: 0}
	tavernFar.AddInput(resource.Bread, 3)
	tavernNear.AddInput(resource.Bread, 3)

	buildings := []*building.Building{hut, tavernFar, tavernNear}
	controller := NewController()
	j := controller.Spawn(hut)

	var ate bool
	for range 500 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if j.hungerTick == 0 {
			ate = true
			break
		}
	}

	if !ate {
		t.Fatal("lumberjack never ate in 500 ticks")
	}
	if got := tavernNear.InputBuffer[resource.Bread]; got != 2 {
		t.Fatalf("nearer tavern Bread = %d, want 2 (lumberjack should have eaten there)", got)
	}
	if got := tavernFar.InputBuffer[resource.Bread]; got != 3 {
		t.Fatalf("farther tavern Bread = %d, want 3 (untouched)", got)
	}
}

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
		ledger := reservations.New()
		controller.Reserve(ledger)
		for _, event := range controller.Tick(grid, buildings, ledger) {
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
