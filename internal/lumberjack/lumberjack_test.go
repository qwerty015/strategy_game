package lumberjack

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/hunger"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// TestController_IsBlockedByAForeignWall is the regression test for a real
// playtest bug found from an actual save ("дровосек синего через стену
// спокойно попадает на мой квадрат и рубит дерево"): before Tick gained
// its separate obstacles parameter, a lumberjack's route was computed
// against its own faction's buildings alone, so a RIVAL faction's wall
// was never even present in the obstacle map -- not "passable", simply
// invisible to the pathfinder. buildings (the job/candidate list) here
// deliberately omits the wall entirely -- a natural resource like a Tree
// is always a valid candidate regardless of who "owns" the map region --
// while obstacles (the whole map) includes it, mirroring how cmd/game
// actually calls this in "N против ИИ" mode.
func TestController_IsBlockedByAForeignWall(t *testing.T) {
	grid := world.NewGrid(5, 3)
	hut := &building.Building{Kind: building.LumberjackHut, X: 0, Y: 1}
	tree := building.NewTree(4, 1)
	tree.GrowthTicks = tree.GrowthTargetTicks
	buildings := []*building.Building{hut, tree}

	// A complete wall across the whole map height, owned by a different
	// faction -- no gate, no gap, nothing to route around.
	wall := []*building.Building{
		{Kind: building.StoneWall, X: 2, Y: 0, Owner: 1, ConstructionStage: building.ConstructionNone},
		{Kind: building.StoneWall, X: 2, Y: 1, Owner: 1, ConstructionStage: building.ConstructionNone},
		{Kind: building.StoneWall, X: 2, Y: 2, Owner: 1, ConstructionStage: building.ConstructionNone},
	}
	obstacles := append(append([]*building.Building{}, buildings...), wall...)

	controller := NewController()
	jack := controller.Spawn(hut)
	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, obstacles, ledger)
	}
	if jack.state != StateIdle {
		t.Fatalf("state with the target tree behind a foreign wall = %v, want StateIdle (must never cross it)", jack.state)
	}

	// Same geometry, no wall this time -- confirms the block above was
	// really the wall, not a setup mistake (e.g. the tree being
	// unreachable for some other reason).
	var cut bool
	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		for _, event := range controller.Tick(grid, buildings, buildings, ledger) {
			if event.Tree == tree {
				cut = true
			}
		}
		if cut {
			break
		}
	}
	if !cut {
		t.Fatal("lumberjack never cut the tree with no wall in its way -- confirms the previous block was really the wall")
	}
}

// TestController_CancelRouteToResetsLumberjackWithoutDanglingPointer
// covers "при удалении харчевни уже идущие к ней лесорубы не получают
// отмену маршрута": deleting a Tavern a lumberjack is mid-walk to eat at
// used to leave j.tavern pointing at a building no longer in the world.
func TestController_CancelRouteToResetsLumberjackWithoutDanglingPointer(t *testing.T) {
	hut := &building.Building{Kind: building.LumberjackHut, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0}

	controller := NewController()
	j := controller.Spawn(hut)
	j.state = StateToTavern
	j.tavern = tavern
	j.X, j.Y = 3, 0

	controller.CancelRouteTo(tavern)

	if j.state != StateIdle {
		t.Fatalf("lumberjack state = %v, want StateIdle", j.state)
	}
	if j.tavern != nil {
		t.Fatal("j.tavern is still set after CancelRouteTo -- dangling pointer to the deleted Tavern")
	}
	if j.X != hut.X || j.Y != hut.Y {
		t.Fatalf("lumberjack position = (%d,%d), want back at hut (%d,%d)", j.X, j.Y, hut.X, hut.Y)
	}
}

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
	for range HungerInterval + 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, buildings, ledger)
		if j.hungerTick == 0 {
			ate = true
			break
		}
	}

	if !ate {
		t.Fatal("lumberjack never ate in time")
	}
	if got := tavernNear.InputBuffer[resource.Bread]; got != 2 {
		t.Fatalf("nearer tavern Bread = %d, want 2 (lumberjack should have eaten there)", got)
	}
	if got := tavernFar.InputBuffer[resource.Bread]; got != 3 {
		t.Fatalf("farther tavern Bread = %d, want 3 (untouched)", got)
	}
}

// TestLumberjack_EatsWhileStuckUnloadingInsteadOfStarving is a regression
// guard for the same bug class as the builder's StateWaitingMaterials fix:
// StateUnloading also had no upper bound (the hut's OutputBuffer can stay
// full indefinitely if no serf has collected it yet) and never checked
// hunger -- only StateIdle did -- so a lumberjack stuck unloading simply
// starved with zero chance to eat, no matter how close a stocked Tavern
// was. Runs well past hunger.MaxTicks with the hut deliberately kept full
// the whole time, and the lumberjack must still be alive, having eaten
// more than once, with its carried log never lost or double-counted --
// then, once room frees up, still deliver it normally.
func TestLumberjack_EatsWhileStuckUnloadingInsteadOfStarving(t *testing.T) {
	grid := world.NewGrid(15, 4)
	hut := &building.Building{Kind: building.LumberjackHut, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 3, Y: 0}
	buildings := []*building.Building{hut, tavern}
	hut.AddOutput(resource.Log, building.BufferCapacity) // full: nowhere for the carried log to go

	controller := NewController()
	j := controller.Spawn(hut)
	if j == nil {
		t.Fatal("Spawn() returned nil")
	}
	j.X, j.Y = hut.X, hut.Y
	j.state = StateUnloading
	j.cargo = 1

	meals := 0
	lastHunger := 0
	for range hunger.MaxTicks*2 + 100 {
		tavern.AddInput(resource.Bread, 1) // keep the Tavern stocked; food is never the constraint here
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, buildings, ledger)
		if len(controller.Lumberjacks) == 0 {
			t.Fatalf("lumberjack died despite a stocked, reachable Tavern (last hunger tick observed: %d)", lastHunger)
		}
		if j.hungerTick == 0 && lastHunger > 0 {
			meals++
		}
		lastHunger = j.hungerTick
	}
	if meals < 2 {
		t.Fatalf("lumberjack ate %d times over %d ticks, want at least 2 (should cycle to the Tavern repeatedly while stuck unloading)", meals, hunger.MaxTicks*2+100)
	}
	if j.cargo != 1 {
		t.Fatalf("cargo after surviving the long wait = %d, want 1 (never lost or double-counted across the meal trips)", j.cargo)
	}

	// Free up room in the hut and confirm the log finally gets delivered.
	hut.OutputBuffer[resource.Log] = 0
	delivered := false
	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, buildings, ledger)
		if j.cargo == 0 {
			delivered = true
			break
		}
	}
	if !delivered {
		t.Fatal("lumberjack never delivered the log once room freed up")
	}
	if got := hut.OutputBuffer[resource.Log]; got != 1 {
		t.Fatalf("hut OutputBuffer[Log] = %d, want 1", got)
	}
}

// TestController_RestoreKeepsSavedMeal covers a save made while a lumberjack
// is walking to a Tavern that has wine but no bread. The meal type must remain
// wine after route reconstruction; otherwise arrival incorrectly reports the
// worker as starving.
func TestController_RestoreKeepsSavedMeal(t *testing.T) {
	grid := world.NewGrid(8, 3)
	hut := &building.Building{Kind: building.LumberjackHut, X: 0, Y: 1}
	tavern := &building.Building{Kind: building.Tavern, X: 4, Y: 1}
	tavern.AddInput(resource.Wine, 1)
	buildings := []*building.Building{hut, tavern}

	controller := NewController()
	jack := controller.Restore(hut, 0, 1, HungerInterval, false, StateToTavern, nil, 0, 0, grid, buildings, buildings, resource.Wine)
	if jack.Meal() != resource.Wine {
		t.Fatalf("restored meal = %v, want wine", jack.Meal())
	}

	for range 20 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, buildings, ledger)
		if jack.HungerTicks() == 0 {
			break
		}
	}
	if jack.HungerTicks() != 0 {
		t.Fatal("restored lumberjack did not eat the saved wine")
	}
	if got := tavern.InputBuffer[resource.Wine]; got != 0 {
		t.Fatalf("tavern Wine = %d, want 0 after the meal", got)
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
		for _, event := range controller.Tick(grid, buildings, buildings, ledger) {
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

// TestLumberjack_SkipsTreesBeyondMaxWorkRadius is a regression guard for
// the user's distance-death report: a tree farther than MaxWorkRadius path
// tiles away must never be targeted at all (safety over exploiting every
// last tree, since hunger is never checked mid-walk), while a nearer one
// within radius is still harvested normally once one appears.
func TestLumberjack_SkipsTreesBeyondMaxWorkRadius(t *testing.T) {
	grid := world.NewGrid(60, 4)
	hut := &building.Building{Kind: building.LumberjackHut, X: 0, Y: 0}
	farTree := building.NewTree(MaxWorkRadius+10, 0)
	farTree.GrowthTicks = farTree.GrowthTargetTicks
	buildings := []*building.Building{hut, farTree}
	controller := NewController()
	jack := controller.Spawn(hut)
	if jack == nil {
		t.Fatal("Spawn() returned nil")
	}

	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, buildings, ledger)
	}
	if jack.state != StateIdle {
		t.Fatalf("state with only an out-of-radius tree available = %v, want StateIdle (must never target it)", jack.state)
	}

	nearTree := building.NewTree(10, 0)
	nearTree.GrowthTicks = nearTree.GrowthTargetTicks
	buildings = append(buildings, nearTree)

	var cut bool
	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		for _, event := range controller.Tick(grid, buildings, buildings, ledger) {
			if event.Tree == nearTree {
				cut = true
			}
		}
		if cut {
			break
		}
	}
	if !cut {
		t.Fatal("lumberjack never harvested the tree within MaxWorkRadius once one appeared")
	}
}

// TestController_SurvivesManyIdleTicksWithNoWork guards the "filter in
// place" roster bug directly: Tick used to append a survivor to the kept
// slice only when its state-machine switch fell through to the bottom of
// the loop body, but nearly every branch (still walking, still chopping,
// nothing to do yet) exits early via `continue` -- which skipped the
// append and silently dropped a perfectly alive, non-hungry worker from
// the roster after its very first tick.
func TestController_SurvivesManyIdleTicksWithNoWork(t *testing.T) {
	grid := world.NewGrid(8, 4)
	hut := &building.Building{Kind: building.LumberjackHut, X: 0, Y: 0}
	buildings := []*building.Building{hut} // no trees at all -- StateIdle finds nothing every tick
	controller := NewController()
	controller.Spawn(hut)

	for range 50 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, buildings, ledger)
	}

	if got := len(controller.Lumberjacks); got != 1 {
		t.Fatalf("lumberjacks after 50 idle ticks = %d, want 1 (worker must not vanish while merely idle)", got)
	}
}
