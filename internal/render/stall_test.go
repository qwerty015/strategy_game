package render

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

// TestBuildingStallReason_OrdinaryRecipe covers the Mill-style path
// (economy.tickRecipe): not stalled before the timer completes even with
// an empty input buffer (a serf might still deliver in time), then
// correctly reports which of the two blockers applies once the timer is
// done.
func TestBuildingStallReason_OrdinaryRecipe(t *testing.T) {
	bt := building.Types[building.Mill]

	stillTicking := &building.Building{Kind: building.Mill, ProgressTicks: bt.Recipe.TicksToProduce - 1}
	if got := buildingStallReason(stillTicking); got != stallNone {
		t.Errorf("mid-cycle with no input = %v, want stallNone (still has time to arrive)", got)
	}

	shortOnWheat := &building.Building{Kind: building.Mill, ProgressTicks: bt.Recipe.TicksToProduce}
	if got := buildingStallReason(shortOnWheat); got != stallNoInput {
		t.Errorf("timer done, no wheat = %v, want stallNoInput", got)
	}

	outputFull := &building.Building{
		Kind:          building.Mill,
		ProgressTicks: bt.Recipe.TicksToProduce,
		InputBuffer:   map[resource.Type]int{resource.Wheat: 1},
		OutputBuffer:  map[resource.Type]int{resource.Flour: building.BufferCapacity},
	}
	if got := buildingStallReason(outputFull); got != stallOutputFull {
		t.Errorf("timer done, wheat ready, output full = %v, want stallOutputFull", got)
	}

	readyToProduce := &building.Building{
		Kind:          building.Mill,
		ProgressTicks: bt.Recipe.TicksToProduce,
		InputBuffer:   map[resource.Type]int{resource.Wheat: 1},
	}
	if got := buildingStallReason(readyToProduce); got != stallNone {
		t.Errorf("timer done, everything ready = %v, want stallNone", got)
	}
}

// TestBuildingStallReason_PrepaidRecipe covers the PigFarm-style path
// (economy.tickPrepaidRecipe): an empty InputBuffer mid-cycle
// (ProgressTicks > 0) is the normal "already fed, growing" state, not a
// stall -- only ProgressTicks == 0 with insufficient feed counts.
func TestBuildingStallReason_PrepaidRecipe(t *testing.T) {
	bt := building.Types[building.PigFarm]

	notYetFed := &building.Building{Kind: building.PigFarm, ProgressTicks: 0}
	if got := buildingStallReason(notYetFed); got != stallNoInput {
		t.Errorf("not started, no feed = %v, want stallNoInput", got)
	}

	growing := &building.Building{Kind: building.PigFarm, ProgressTicks: 1}
	if got := buildingStallReason(growing); got != stallNone {
		t.Errorf("mid-growth with empty input buffer = %v, want stallNone (feed was already spent)", got)
	}

	doneButFull := &building.Building{
		Kind:          building.PigFarm,
		ProgressTicks: bt.Recipe.TicksToProduce,
		OutputBuffer:  map[resource.Type]int{resource.Carcass: building.BufferCapacity},
	}
	if got := buildingStallReason(doneButFull); got != stallOutputFull {
		t.Errorf("grown, output full = %v, want stallOutputFull", got)
	}
}

// TestBuildingStallReason_MultiRecipe covers the Smeltery-style path
// (economy.tickMultiRecipe): stallNoInput only when *neither* recipe can
// start, matching pickRecipe's own scan.
func TestBuildingStallReason_MultiRecipe(t *testing.T) {
	neitherOreNorFeed := &building.Building{Kind: building.Smeltery, ProgressTicks: 0}
	if got := buildingStallReason(neitherOreNorFeed); got != stallNoInput {
		t.Errorf("no ore of either kind = %v, want stallNoInput", got)
	}

	oneRecipeReady := &building.Building{
		Kind:          building.Smeltery,
		ProgressTicks: 0,
		InputBuffer:   map[resource.Type]int{resource.GoldOre: 1, resource.Coal: 1},
	}
	if got := buildingStallReason(oneRecipeReady); got != stallNone {
		t.Errorf("gold ore + coal on hand = %v, want stallNone", got)
	}
}

// TestBuildingStallReason_UnaffectedCases covers the buildings and states
// that must never report a stall: still under construction, and a
// building with no recipe at all (a Warehouse).
func TestBuildingStallReason_UnaffectedCases(t *testing.T) {
	underConstruction := &building.Building{Kind: building.Mill, ConstructionStage: building.ConstructionFoundation, ProgressTicks: 999}
	if got := buildingStallReason(underConstruction); got != stallNone {
		t.Errorf("under construction = %v, want stallNone", got)
	}

	warehouse := &building.Building{Kind: building.Warehouse}
	if got := buildingStallReason(warehouse); got != stallNone {
		t.Errorf("warehouse (no recipe) = %v, want stallNone", got)
	}
}
