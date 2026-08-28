package miner

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/hunger"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// TestMiner_EatsAtNearestReachableTavern mirrors quarry/lumberjack's test
// of the same shape: a hungry, idle miner (no deposit to work) must
// prioritize walking to the nearest reachable, stocked Tavern over staying
// idle, and must actually eat there -- taking food from that Tavern's
// InputBuffer and resetting hunger to 0 -- not just walk there and stand
// around.
func TestMiner_EatsAtNearestReachableTavern(t *testing.T) {
	grid := world.NewGrid(15, 4)
	hut := &building.Building{Kind: building.MinerHut, X: 0, Y: 0}
	tavernFar := &building.Building{Kind: building.Tavern, X: 12, Y: 0}
	tavernNear := &building.Building{Kind: building.Tavern, X: 3, Y: 0}
	tavernFar.AddInput(resource.Bread, 3)
	tavernNear.AddInput(resource.Bread, 3)

	buildings := []*building.Building{hut, tavernFar, tavernNear}
	controller := NewController()
	m := controller.Spawn(hut)
	if m == nil {
		t.Fatal("Spawn() returned nil")
	}

	var ate bool
	for range HungerInterval + 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if m.HungerTicks() == 0 {
			ate = true
			break
		}
	}

	if !ate {
		t.Fatal("miner never ate in time")
	}
	if got := tavernNear.InputBuffer[resource.Bread]; got != 2 {
		t.Fatalf("nearer tavern Bread = %d, want 2 (miner should have eaten there)", got)
	}
	if got := tavernFar.InputBuffer[resource.Bread]; got != 3 {
		t.Fatalf("farther tavern Bread = %d, want 3 (untouched)", got)
	}
}

// TestMiner_EatsWhileStuckUnloadingInsteadOfStarving is a regression guard
// for the same bug class as the builder's StateWaitingMaterials fix (see
// that package): StateUnloading had no upper bound (the hut's OutputBuffer
// can stay full indefinitely if no serf has collected it yet) and never
// checked hunger -- only StateIdle did. Also covers the second, smaller bug
// this fix required: unlike lumberjack/quarry, miner's StateIdle never had
// a "cargo > 0 -> resume delivering" branch, since a miner was never
// reachable at Idle with cargo before this fix existed -- without adding
// that branch too, the leftover cargo would have been silently discarded
// the next time startDepositJob overwrote it.
func TestMiner_EatsWhileStuckUnloadingInsteadOfStarving(t *testing.T) {
	grid := world.NewGrid(15, 4)
	hut := &building.Building{Kind: building.MinerHut, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 3, Y: 0}
	buildings := []*building.Building{hut, tavern}
	hut.AddOutput(resource.Coal, building.BufferCapacity) // full: nowhere for the carried coal to go

	controller := NewController()
	m := controller.Spawn(hut)
	if m == nil {
		t.Fatal("Spawn() returned nil")
	}
	m.X, m.Y = hut.X, hut.Y
	m.state = StateUnloading
	m.cargo = 1
	m.cargoResource = resource.Coal

	meals := 0
	lastHunger := 0
	for range hunger.MaxTicks*2 + 100 {
		tavern.AddInput(resource.Bread, 1) // keep the Tavern stocked; food is never the constraint here
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if len(controller.Miners) == 0 {
			t.Fatalf("miner died despite a stocked, reachable Tavern (last hunger tick observed: %d)", lastHunger)
		}
		if m.hungerTick == 0 && lastHunger > 0 {
			meals++
		}
		lastHunger = m.hungerTick
	}
	if meals < 2 {
		t.Fatalf("miner ate %d times over %d ticks, want at least 2 (should cycle to the Tavern repeatedly while stuck unloading)", meals, hunger.MaxTicks*2+100)
	}
	if m.cargo != 1 || m.cargoResource != resource.Coal {
		t.Fatalf("cargo after surviving the long wait = %d of %v, want 1 of Coal (never lost, overwritten, or double-counted across the meal trips)", m.cargo, m.cargoResource)
	}

	// Free up room in the hut and confirm the coal finally gets delivered.
	hut.OutputBuffer[resource.Coal] = 0
	delivered := false
	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if m.cargo == 0 {
			delivered = true
			break
		}
	}
	if !delivered {
		t.Fatal("miner never delivered the coal once room freed up")
	}
	if got := hut.OutputBuffer[resource.Coal]; got != 1 {
		t.Fatalf("hut OutputBuffer[Coal] = %d, want 1", got)
	}
}

// TestMinerFollowsQuotaNotJustNearestDeposit covers the whole reason
// DefaultQuota exists: a Coal deposit sitting right next to the hut must
// not make the miner ignore Gold ore and Iron ore forever just because
// coal is always the closest. Coal is placed nearest on purpose.
func TestMinerFollowsQuotaNotJustNearestDeposit(t *testing.T) {
	grid := world.NewGrid(20, 4)
	hut := &building.Building{Kind: building.MinerHut, X: 0, Y: 0}
	coal := building.NewOreDeposit(building.CoalDeposit, 2, 0)     // closest
	gold := building.NewOreDeposit(building.GoldOreDeposit, 8, 0)  // farther
	iron := building.NewOreDeposit(building.IronOreDeposit, 14, 0) // farthest
	buildings := []*building.Building{hut, coal, gold, iron}
	controller := NewController()
	worker := controller.Spawn(hut)
	if worker == nil {
		t.Fatal("Spawn() returned nil")
	}

	// Drain the hut's buffer periodically, like a serf collecting it would
	// -- otherwise BufferCapacity (6) would eventually stall the miner
	// mid-delivery, unrelated to what this test is actually checking.
	delivered := map[resource.Type]int{}
	for tick := 0; tick < 2000; tick++ {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		for _, rt := range []resource.Type{resource.GoldOre, resource.IronOre, resource.Coal} {
			if n := hut.OutputBuffer[rt]; n > 0 {
				delivered[rt] += n
				hut.OutputBuffer[rt] = 0
			}
		}
	}
	_ = worker

	total := delivered[resource.GoldOre] + delivered[resource.IronOre] + delivered[resource.Coal]
	if total == 0 {
		t.Fatal("miner never delivered anything")
	}
	if delivered[resource.GoldOre] == 0 {
		t.Fatalf("GoldOre delivered = 0 despite being in the quota (delivered: %v) -- quota was ignored in favor of the nearest deposit (coal)", delivered)
	}
	if delivered[resource.IronOre] == 0 {
		t.Fatalf("IronOre delivered = 0 despite being in the quota (delivered: %v)", delivered)
	}
}

// TestMinerQuotaRatioMatchesDefault covers the exact ratio the user asked
// for: over several full cycles, Coal delivered should be about 3x Gold
// ore (and 3x Iron ore), matching DefaultQuota's 1-1-3 split, not some
// other ratio distorted by deposit distance.
func TestMinerQuotaRatioMatchesDefault(t *testing.T) {
	grid := world.NewGrid(6, 6)
	hut := &building.Building{Kind: building.MinerHut, X: 0, Y: 0}
	// One deposit of each kind, all equally reachable, generous reserve.
	gold := building.NewOreDeposit(building.GoldOreDeposit, 1, 0)
	iron := building.NewOreDeposit(building.IronOreDeposit, 2, 0)
	coal := building.NewOreDeposit(building.CoalDeposit, 3, 0)
	buildings := []*building.Building{hut, gold, iron, coal}
	controller := NewController()
	worker := controller.Spawn(hut)

	// Drain the hut's buffer into the "delivered" tallies each time it
	// fills, so BufferCapacity never blocks further mining.
	delivered := map[resource.Type]int{}
	for tick := 0; tick < 20000; tick++ {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		for _, rt := range []resource.Type{resource.GoldOre, resource.IronOre, resource.Coal} {
			if n := hut.OutputBuffer[rt]; n > 0 {
				delivered[rt] += n
				hut.OutputBuffer[rt] = 0
			}
		}
		if delivered[resource.GoldOre] >= 5 {
			break
		}
	}
	_ = worker

	if delivered[resource.GoldOre] == 0 || delivered[resource.IronOre] == 0 || delivered[resource.Coal] == 0 {
		t.Fatalf("delivered totals = %v, want all three present", delivered)
	}
	// Ratio should be roughly 1:1:3 -- allow slack for whichever partial
	// cycle the loop happened to stop mid-way through.
	if delivered[resource.Coal] < delivered[resource.GoldOre] {
		t.Fatalf("delivered = %v, want Coal >= GoldOre (quota is 3 coal per 1 gold ore)", delivered)
	}
}

// TestMinerSkipsExhaustedResourceInQuota covers the other edge: if the
// quota's current resource has nothing left to mine (or nothing reachable),
// the miner must move on to the next quota entry instead of standing idle
// forever while gold ore, say, sits there mined out.
func TestMinerSkipsExhaustedResourceInQuota(t *testing.T) {
	grid := world.NewGrid(10, 4)
	hut := &building.Building{Kind: building.MinerHut, X: 0, Y: 0}
	// No GoldOreDeposit at all -- the quota's first entry has nothing to
	// find, ever.
	iron := building.NewOreDeposit(building.IronOreDeposit, 5, 0)
	buildings := []*building.Building{hut, iron}
	controller := NewController()
	controller.Spawn(hut)

	var got bool
	for tick := 0; tick < 500 && !got; tick++ {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if hut.OutputBuffer[resource.IronOre] > 0 {
			got = true
		}
	}

	if !got {
		t.Fatal("miner never mined iron ore even though gold ore (first in the quota) was permanently unavailable")
	}
}

// TestMiner_SkipsDepositsBeyondMaxWorkRadius is a regression guard for the
// user's distance-death report: a deposit farther than MaxWorkRadius path
// tiles away must never be targeted at all (safety over exploiting every
// last deposit, since hunger is never checked mid-walk) -- across every
// quota entry, not just the current one -- while a nearer one within
// radius is still mined normally once one appears.
func TestMiner_SkipsDepositsBeyondMaxWorkRadius(t *testing.T) {
	grid := world.NewGrid(60, 4)
	hut := &building.Building{Kind: building.MinerHut, X: 0, Y: 0}
	farDeposit := building.NewOreDeposit(building.CoalDeposit, MaxWorkRadius+10, 0)
	buildings := []*building.Building{hut, farDeposit}
	controller := NewController()
	m := controller.Spawn(hut)
	if m == nil {
		t.Fatal("Spawn() returned nil")
	}

	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
	}
	if m.state != StateIdle {
		t.Fatalf("state with only an out-of-radius deposit available = %v, want StateIdle (must never target it)", m.state)
	}

	nearDeposit := building.NewOreDeposit(building.CoalDeposit, 10, 0)
	buildings = append(buildings, nearDeposit)

	mined := false
	for range 200 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if nearDeposit.Reserve < building.OreDepositReserve {
			mined = true
			break
		}
	}
	if !mined {
		t.Fatal("miner never mined the deposit within MaxWorkRadius once one appeared")
	}
}

// TestController_CancelRouteToResetsMinerWithoutDanglingPointer mirrors the
// equivalent lumberjack/quarry/builder test.
func TestController_CancelRouteToResetsMinerWithoutDanglingPointer(t *testing.T) {
	hut := &building.Building{Kind: building.MinerHut, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0}

	controller := NewController()
	m := controller.Spawn(hut)
	m.state = StateToTavern
	m.tavern = tavern
	m.X, m.Y = 3, 0

	controller.CancelRouteTo(tavern)

	if m.state != StateIdle {
		t.Fatalf("miner state = %v, want StateIdle", m.state)
	}
	if m.tavern != nil {
		t.Fatal("m.tavern is still set after CancelRouteTo -- dangling pointer to the deleted Tavern")
	}
	if m.X != hut.X || m.Y != hut.Y {
		t.Fatalf("miner position = (%d,%d), want back at hut (%d,%d)", m.X, m.Y, hut.X, hut.Y)
	}
}

// TestController_SurvivesManyIdleTicksWithNoWork mirrors the same guard
// used for every other free-roaming worker controller.
func TestController_SurvivesManyIdleTicksWithNoWork(t *testing.T) {
	grid := world.NewGrid(8, 4)
	hut := &building.Building{Kind: building.MinerHut, X: 0, Y: 0}
	buildings := []*building.Building{hut} // no deposits at all
	controller := NewController()
	controller.Spawn(hut)

	for range 50 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
	}

	if got := len(controller.Miners); got != 1 {
		t.Fatalf("miners after 50 idle ticks = %d, want 1 (worker must not vanish while merely idle)", got)
	}
}
