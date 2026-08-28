package quarry

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/hunger"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// TestController_CancelRouteToResetsQuarrymanWithoutDanglingPointer mirrors
// lumberjack's test of the same name: deleting a Tavern a quarryman is
// mid-walk to eat at must not leave j.tavern pointing at a building no
// longer in the world.
func TestController_CancelRouteToResetsQuarrymanWithoutDanglingPointer(t *testing.T) {
	hut := &building.Building{Kind: building.QuarryHut, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0}

	controller := NewController()
	q := controller.Spawn(hut)
	q.state = StateToTavern
	q.tavern = tavern
	q.X, q.Y = 3, 0

	controller.CancelRouteTo(tavern)

	if q.state != StateIdle {
		t.Fatalf("quarryman state = %v, want StateIdle", q.state)
	}
	if q.tavern != nil {
		t.Fatal("q.tavern is still set after CancelRouteTo -- dangling pointer to the deleted Tavern")
	}
	if q.X != hut.X || q.Y != hut.Y {
		t.Fatalf("quarryman position = (%d,%d), want back at hut (%d,%d)", q.X, q.Y, hut.X, hut.Y)
	}
}

// TestQuarryman_EatsAtNearestReachableTavern mirrors lumberjack's test: a
// hungry quarryman must walk to the nearest reachable Tavern, not just
// whichever one happens to be first in the buildings slice.
func TestQuarryman_EatsAtNearestReachableTavern(t *testing.T) {
	grid := world.NewGrid(15, 4)
	hut := &building.Building{Kind: building.QuarryHut, X: 0, Y: 0}
	tavernFar := &building.Building{Kind: building.Tavern, X: 12, Y: 0}
	tavernNear := &building.Building{Kind: building.Tavern, X: 3, Y: 0}
	tavernFar.AddInput(resource.Bread, 3)
	tavernNear.AddInput(resource.Bread, 3)

	buildings := []*building.Building{hut, tavernFar, tavernNear}
	controller := NewController()
	q := controller.Spawn(hut)

	var ate bool
	for range HungerInterval + 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if q.hungerTick == 0 {
			ate = true
			break
		}
	}

	if !ate {
		t.Fatal("quarryman never ate in time")
	}
	if got := tavernNear.InputBuffer[resource.Bread]; got != 2 {
		t.Fatalf("nearer tavern Bread = %d, want 2 (quarryman should have eaten there)", got)
	}
	if got := tavernFar.InputBuffer[resource.Bread]; got != 3 {
		t.Fatalf("farther tavern Bread = %d, want 3 (untouched)", got)
	}
}

// TestQuarryman_EatsWhileStuckUnloadingInsteadOfStarving is a regression
// guard for the same bug class as the builder's StateWaitingMaterials fix
// (see that package): StateUnloading had no upper bound (the hut's
// OutputBuffer can stay full indefinitely if no serf has collected it yet)
// and never checked hunger -- only StateIdle did. Runs well past
// hunger.MaxTicks with the hut deliberately kept full the whole time, and
// the quarryman must still be alive, having eaten more than once, with its
// carried cargo never lost or double-counted -- then, once room frees up,
// still deliver it normally.
func TestQuarryman_EatsWhileStuckUnloadingInsteadOfStarving(t *testing.T) {
	grid := world.NewGrid(15, 4)
	hut := &building.Building{Kind: building.QuarryHut, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 3, Y: 0}
	buildings := []*building.Building{hut, tavern}
	hut.AddOutput(resource.StoneBlock, building.BufferCapacity) // full: nowhere for the carried blocks to go

	controller := NewController()
	q := controller.Spawn(hut)
	if q == nil {
		t.Fatal("Spawn() returned nil")
	}
	q.X, q.Y = hut.X, hut.Y
	q.state = StateUnloading
	q.cargo = 1 // becomes 2 StoneBlocks on delivery

	meals := 0
	lastHunger := 0
	for range hunger.MaxTicks*2 + 100 {
		tavern.AddInput(resource.Bread, 1) // keep the Tavern stocked; food is never the constraint here
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if len(controller.Quarrymen) == 0 {
			t.Fatalf("quarryman died despite a stocked, reachable Tavern (last hunger tick observed: %d)", lastHunger)
		}
		if q.hungerTick == 0 && lastHunger > 0 {
			meals++
		}
		lastHunger = q.hungerTick
	}
	if meals < 2 {
		t.Fatalf("quarryman ate %d times over %d ticks, want at least 2 (should cycle to the Tavern repeatedly while stuck unloading)", meals, hunger.MaxTicks*2+100)
	}
	if q.cargo != 1 {
		t.Fatalf("cargo after surviving the long wait = %d, want 1 (never lost or double-counted across the meal trips)", q.cargo)
	}

	// Free up room in the hut and confirm the blocks finally get delivered.
	hut.OutputBuffer[resource.StoneBlock] = 0
	delivered := false
	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if q.cargo == 0 {
			delivered = true
			break
		}
	}
	if !delivered {
		t.Fatal("quarryman never delivered the blocks once room freed up")
	}
	if got := hut.OutputBuffer[resource.StoneBlock]; got != 2 {
		t.Fatalf("hut OutputBuffer[StoneBlock] = %d, want 2", got)
	}
}

// TestController_RestoreKeepsSavedMeal mirrors lumberjack's test: a save
// made while walking to a Tavern with wine but no bread must keep the meal
// type as wine after route reconstruction.
func TestController_RestoreKeepsSavedMeal(t *testing.T) {
	grid := world.NewGrid(8, 3)
	hut := &building.Building{Kind: building.QuarryHut, X: 0, Y: 1}
	tavern := &building.Building{Kind: building.Tavern, X: 4, Y: 1}
	tavern.AddInput(resource.Wine, 1)
	buildings := []*building.Building{hut, tavern}

	controller := NewController()
	worker := controller.Restore(hut, 0, 1, HungerInterval, false, StateToTavern, nil, 0, 0, grid, buildings, resource.Wine)
	if worker.Meal() != resource.Wine {
		t.Fatalf("restored meal = %v, want wine", worker.Meal())
	}

	for range 20 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if worker.HungerTicks() == 0 {
			break
		}
	}
	if worker.HungerTicks() != 0 {
		t.Fatal("restored quarryman did not eat the saved wine")
	}
	if got := tavern.InputBuffer[resource.Wine]; got != 0 {
		t.Fatalf("tavern Wine = %d, want 0 after the meal", got)
	}
}

// TestQuarrymanMinesNearestDepositAndStoresBlocksAtHut covers the core loop:
// one mining pass removes exactly one unit of Reserve and deposits two
// Stone Blocks at the hut, and -- since the deposit started with plenty of
// reserve left -- the deposit itself must still be standing afterward (no
// DepositExhausted event).
func TestQuarrymanMinesNearestDepositAndStoresBlocksAtHut(t *testing.T) {
	grid := world.NewGrid(12, 4)
	hut := &building.Building{Kind: building.QuarryHut, X: 0, Y: 0}
	deposit := building.NewStoneDeposit(5, 0)
	buildings := []*building.Building{hut, deposit}
	controller := NewController()
	worker := controller.Spawn(hut)
	if worker == nil {
		t.Fatal("Spawn() returned nil")
	}

	var mined bool
	for tick := 0; tick < 100; tick++ {
		ledger := reservations.New()
		controller.Reserve(ledger)
		for _, event := range controller.Tick(grid, buildings, ledger) {
			t.Fatalf("unexpected event with a nearly-full deposit: %#v", event)
		}
		if hut.OutputBuffer[resource.StoneBlock] == 2 {
			mined = true
			break
		}
	}

	if !mined {
		t.Fatal("quarryman never finished mining the deposit")
	}
	if got, want := deposit.Reserve, building.StoneDepositReserve-1; got != want {
		t.Fatalf("deposit Reserve = %d, want %d (exactly one unit mined)", got, want)
	}
	_, got := worker.Cargo()
	if got != 0 {
		t.Fatalf("quarryman cargo after unloading = %d, want 0", got)
	}
}

// TestQuarryman_SkipsDepositsBeyondMaxWorkRadius is a regression guard for
// the user's distance-death report: a deposit farther than MaxWorkRadius
// path tiles away must never be targeted at all (safety over exploiting
// every last deposit, since hunger is never checked mid-walk), while a
// nearer one within radius is still mined normally once one appears.
func TestQuarryman_SkipsDepositsBeyondMaxWorkRadius(t *testing.T) {
	grid := world.NewGrid(60, 4)
	hut := &building.Building{Kind: building.QuarryHut, X: 0, Y: 0}
	farDeposit := building.NewStoneDeposit(MaxWorkRadius+10, 0)
	buildings := []*building.Building{hut, farDeposit}
	controller := NewController()
	q := controller.Spawn(hut)
	if q == nil {
		t.Fatal("Spawn() returned nil")
	}

	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
	}
	if q.state != StateIdle {
		t.Fatalf("state with only an out-of-radius deposit available = %v, want StateIdle (must never target it)", q.state)
	}

	nearDeposit := building.NewStoneDeposit(10, 0)
	buildings = append(buildings, nearDeposit)

	mined := false
	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if nearDeposit.Reserve < building.StoneDepositReserve {
			mined = true
			break
		}
	}
	if !mined {
		t.Fatal("quarryman never mined the deposit within MaxWorkRadius once one appeared")
	}
}

// TestQuarrymanExhaustingADepositFiresEvent covers the other edge: a
// deposit down to its last unit must be removed (via DepositExhausted) the
// moment that last unit is mined, not left behind at Reserve 0.
func TestQuarrymanExhaustingADepositFiresEvent(t *testing.T) {
	grid := world.NewGrid(12, 4)
	hut := &building.Building{Kind: building.QuarryHut, X: 0, Y: 0}
	deposit := building.NewStoneDeposit(5, 0)
	deposit.Reserve = 1 // one mining pass empties it
	buildings := []*building.Building{hut, deposit}
	controller := NewController()
	controller.Spawn(hut)

	var exhausted bool
	for tick := 0; tick < 100 && !exhausted; tick++ {
		ledger := reservations.New()
		controller.Reserve(ledger)
		for _, event := range controller.Tick(grid, buildings, ledger) {
			if event.Kind != DepositExhausted || event.Deposit != deposit {
				t.Fatalf("unexpected event: %#v", event)
			}
			exhausted = true
		}
	}

	if !exhausted {
		t.Fatal("quarryman never exhausted the single-unit deposit")
	}
	if deposit.Reserve > 0 {
		t.Fatalf("deposit Reserve = %d after exhaustion, want <= 0", deposit.Reserve)
	}
}

// TestController_SurvivesManyIdleTicksWithNoWork mirrors lumberjack's guard
// against the "filter in place" roster bug: a quarryman with nothing to mine
// must not silently vanish from the roster while merely idle.
func TestController_SurvivesManyIdleTicksWithNoWork(t *testing.T) {
	grid := world.NewGrid(8, 4)
	hut := &building.Building{Kind: building.QuarryHut, X: 0, Y: 0}
	buildings := []*building.Building{hut} // no deposits at all
	controller := NewController()
	controller.Spawn(hut)

	for range 50 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
	}

	if got := len(controller.Quarrymen); got != 1 {
		t.Fatalf("quarrymen after 50 idle ticks = %d, want 1 (worker must not vanish while merely idle)", got)
	}
}

// TestTwoQuarrymenCanShareOneDeposit covers a deliberate difference from
// lumberjack's trees: a deposit holds thousands of units, so unlike a tree
// (single-claim, one lumberjack at a time) several quarrymen may legitimately
// target the very same deposit at once.
func TestTwoQuarrymenCanShareOneDeposit(t *testing.T) {
	grid := world.NewGrid(12, 4)
	hutA := &building.Building{Kind: building.QuarryHut, X: 0, Y: 0}
	hutB := &building.Building{Kind: building.QuarryHut, X: 10, Y: 0}
	deposit := building.NewStoneDeposit(5, 0)
	buildings := []*building.Building{hutA, hutB, deposit}
	controller := NewController()
	controller.Spawn(hutA)
	controller.Spawn(hutB)

	for range 20 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
	}

	if len(controller.Quarrymen) != 2 {
		t.Fatalf("quarrymen = %d, want 2", len(controller.Quarrymen))
	}
	both := controller.Quarrymen[0].target == deposit && controller.Quarrymen[1].target == deposit
	if !both {
		t.Fatalf("expected both quarrymen to target the shared deposit, got targets %v and %v", controller.Quarrymen[0].target, controller.Quarrymen[1].target)
	}
}
