package logistics

import (
	"slices"
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// tick runs one simulation step exactly like cmd/game does: a fresh
// ledger, seeded from this controller's own in-flight serfs, then Tick.
// grid is nil-safe (pathfind.FindLandPath short-circuits on a nil grid) and
// only matters for the construction-supply job, which none of these tests
// exercise -- callers pass nil.
func tick(c *Controller, grid *world.Grid, buildings []*building.Building, stock *resource.Stockpile) {
	ledger := reservations.New()
	c.Reserve(ledger)
	c.Tick(grid, buildings, stock, ledger)
}

// straightRoad returns Road buildings filling every tile from x=fromX to
// x=toX-1 at row y, connecting whatever sits at fromX and toX.
func straightRoad(fromX, toX, y int) []*building.Building {
	var roads []*building.Building
	for x := fromX; x < toX; x++ {
		roads = append(roads, &building.Building{Kind: building.Road, X: x, Y: y})
	}
	return roads
}

// TestController_DoesNotOvercommitMultipleIdleSerfsToOneJob reproduces the
// user's bug report directly: a farm holding exactly one batch of Wheat
// (BufferCapacity units) and a crowd of idle serfs. Before the
// reservations.Ledger existed, every idle serf independently "discovered"
// the same OutputBuffer in the same Tick call and all set off for it --
// only the first to physically arrive could actually collect anything, and
// the rest walked back empty. See AGENTS.md.
func TestController_DoesNotOvercommitMultipleIdleSerfsToOneJob(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 5, Y: 0}
	farm.AddOutput(resource.Wheat, building.BufferCapacity) // 6 units, exactly the user's report

	buildings := append([]*building.Building{warehouse, farm}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 10) // a crowd, like the user described
	stock := resource.NewStockpile(100)
	tick(c, nil, buildings, stock)

	// CarryCapacity (5) is less than the 6 units sitting on the farm, so
	// it correctly takes two serfs to claim all of it (5 + 1) -- that's
	// not overcommitment. The bug was every idle serf seeing the full 6
	// and each claiming up to CarryCapacity of it, wildly exceeding what
	// the farm actually has.
	committed, claimed := 0, 0
	for _, s := range c.Serfs {
		if s.Busy() && s.PickupBuilding() == farm {
			committed++
			_, amount := s.Cargo()
			claimed += amount
		}
	}
	if claimed > building.BufferCapacity {
		t.Fatalf("%d serfs claimed %d units total from a %d-unit job, want <= %d", committed, claimed, building.BufferCapacity, building.BufferCapacity)
	}
	if committed > 2 {
		t.Fatalf("%d serfs committed to one %d-unit job (CarryCapacity %d), want at most 2", committed, building.BufferCapacity, CarryCapacity)
	}
}

// TestController_OverflowAtDropoffGoesToStock exercises arriveAtDropoff's
// safety net directly: if a serf shows up carrying more than the
// destination has room for (the reservation ledger should prevent this in
// the normal case, but a race could still slip through -- a deleted
// building, a save/load edge case), the goods must not be silently
// destroyed. AddInput's returned "how much actually fit" used to be
// discarded entirely.
func TestController_OverflowAtDropoffGoesToStock(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	mill := &building.Building{Kind: building.Mill, X: 5, Y: 0}
	mill.AddInput(resource.Wheat, building.BufferCapacity-1) // room for exactly 1 more

	c := NewController(warehouse, 1)
	s := c.Serfs[0]
	s.ph = toDropoff
	s.dropoff = mill
	s.resource = resource.Wheat
	s.amount = 5 // far more than the 1 unit of room left
	s.path = []pathfind.Point{{X: mill.X, Y: mill.Y}}
	s.pathIdx = 0
	s.tileTicks = TicksPerTile - 1

	stock := resource.NewStockpile(100)
	c.advance(s, nil, nil, stock)

	if got := mill.InputBuffer[resource.Wheat]; got != building.BufferCapacity {
		t.Fatalf("mill InputBuffer[Wheat] = %d, want %d (filled to capacity)", got, building.BufferCapacity)
	}
	if got := stock.Amount(resource.Wheat); got != 4 {
		t.Fatalf("warehouse Wheat = %d, want 4 (the 4 units the mill had no room for)", got)
	}
}

func TestController_SerfEatsAtTavernWhenHungry(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0}
	tavern.AddInput(resource.Bread, 3)

	buildings := append([]*building.Building{warehouse, tavern}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 1)
	s := c.Serfs[0]
	stock := resource.NewStockpile(100)

	for range HungerInterval + 200 {
		tick(c, nil, buildings, stock)
		if s.ph == idle && s.ticksSinceMeal == 0 {
			break
		}
	}

	if got := tavern.InputBuffer[resource.Bread]; got != 2 {
		t.Fatalf("tavern Bread = %d, want 2 (serf ate one loaf)", got)
	}
	if s.Starving {
		t.Fatal("Starving = true after a successful meal, want false")
	}
}

func TestController_SerfEatsWineAtTavern(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0}
	tavern.AddInput(resource.Bread, 1)
	tavern.AddInput(resource.Fish, 1)
	tavern.AddInput(resource.Wine, 1)

	buildings := append([]*building.Building{warehouse, tavern}, straightRoad(1, 5, 0)...)
	c := NewController(warehouse, 1)
	// Seed 2 selects the third entry from [Bread, Fish, Wine]. Wine must be
	// selectable even while the formerly preferred foods are present.
	c.SetMealSeed(2)
	s := c.Serfs[0]
	s.ticksSinceMeal = HungerInterval
	stock := resource.NewStockpile(100)

	for range 100 {
		tick(c, nil, buildings, stock)
		if s.ticksSinceMeal == 0 {
			break
		}
	}

	if got := tavern.InputBuffer[resource.Wine]; got != 0 {
		t.Fatalf("tavern Wine = %d, want 0 (wine is a valid meal)", got)
	}
	if got := tavern.InputBuffer[resource.Bread] + tavern.InputBuffer[resource.Fish]; got != 2 {
		t.Fatalf("bread and fish changed to %d, want 2 (serf should have chosen wine)", got)
	}
	if s.Starving {
		t.Fatal("Starving = true after a successful wine meal")
	}
}

func TestController_CollectsFromProducerToWarehouse(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0} // (0,0)-(1,1)
	farm := &building.Building{Kind: building.Farm, X: 5, Y: 0}           // (5,0)-(6,1)
	farm.AddOutput(resource.Wheat, 5)

	buildings := append([]*building.Building{warehouse, farm}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)

	for range 500 {
		tick(c, nil, buildings, stock)
		if stock.Amount(resource.Wheat) > 0 {
			break
		}
	}

	if got := stock.Amount(resource.Wheat); got != 5 {
		t.Fatalf("warehouse Wheat = %d, want 5 (serf should have collected the farm's output)", got)
	}
	if got := farm.OutputBuffer[resource.Wheat]; got != 0 {
		t.Fatalf("farm OutputBuffer[Wheat] = %d, want 0 (collected)", got)
	}
}

func TestController_CollectsLogsFromLumberjackHut(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	hut := &building.Building{Kind: building.LumberjackHut, X: 5, Y: 0}
	hut.AddOutput(resource.Log, 1)

	buildings := append([]*building.Building{warehouse, hut}, straightRoad(1, 5, 0)...)
	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(0)

	for range 500 {
		tick(c, nil, buildings, stock)
		if stock.Amount(resource.Log) > 0 {
			break
		}
	}

	if got := stock.Amount(resource.Log); got != 1 {
		t.Fatalf("warehouse Log = %d, want 1 (serf should collect the hut output)", got)
	}
	if got := hut.OutputBuffer[resource.Log]; got != 0 {
		t.Fatalf("hut OutputBuffer[Log] = %d, want 0 (collected)", got)
	}
}

func TestController_SuppliesConsumerFromWarehouse(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	mill := &building.Building{Kind: building.Mill, X: 5, Y: 0}

	buildings := append([]*building.Building{warehouse, mill}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)
	stock.Add(resource.Wheat, 20)

	for range 500 {
		tick(c, nil, buildings, stock)
		if mill.InputBuffer[resource.Wheat] > 0 {
			break
		}
	}

	if got := mill.InputBuffer[resource.Wheat]; got != 1 {
		t.Fatalf("mill InputBuffer[Wheat] = %d, want 1 (serf should have supplied it)", got)
	}
	if got := stock.Amount(resource.Wheat); got != 19 {
		t.Fatalf("warehouse Wheat = %d, want 19 (1 handed off to the mill)", got)
	}
}

// TestController_DeliversConstructionMaterialsDirectlyFromProducer guards
// the construction route that must not bounce Planks through the Warehouse:
// with a workshop buffer ready and no stockpile planks, the serf still has
// to complete one producer -> construction-site delivery.
func TestController_DeliversConstructionMaterialsDirectlyFromProducer(t *testing.T) {
	grid := world.NewGrid(12, 3)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	workshop := &building.Building{Kind: building.CarpentryWorkshop, X: 4, Y: 0}
	site := building.NewConstructionSite(building.Bakery, 8, 0)
	workshop.AddOutput(resource.Plank, CarryCapacity)
	buildings := []*building.Building{warehouse, workshop, site}

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(0)
	for range 100 {
		tick(c, grid, buildings, stock)
		if site.InputBuffer[resource.Plank] == CarryCapacity {
			break
		}
	}

	if got := site.InputBuffer[resource.Plank]; got != CarryCapacity {
		t.Fatalf("construction site Plank = %d, want %d (direct delivery from workshop)", got, CarryCapacity)
	}
	if got := workshop.OutputBuffer[resource.Plank]; got != 0 {
		t.Fatalf("workshop Plank output = %d, want 0 (cargo delivered)", got)
	}
	if got := stock.Amount(resource.Plank); got != 0 {
		t.Fatalf("warehouse Plank = %d, want 0 (construction must not detour through stock)", got)
	}
}

// TestController_DeliversConstructionMaterialsFromWarehouse covers the
// fallback route used when no producer currently has the material ready.
// The cargo must remain in the construction InputBuffer after arrival; a
// failed hand-off here would make the serf repeat Warehouse -> site trips
// forever and eventually starve.
func TestController_DeliversConstructionMaterialsFromWarehouse(t *testing.T) {
	grid := world.NewGrid(12, 3)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	site := building.NewConstructionSite(building.Bakery, 8, 0)
	buildings := []*building.Building{warehouse, site}

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(0)
	stock.Add(resource.Plank, CarryCapacity)
	for range 100 {
		tick(c, grid, buildings, stock)
		if site.InputBuffer[resource.Plank] == CarryCapacity {
			break
		}
	}

	if got := site.InputBuffer[resource.Plank]; got != CarryCapacity {
		t.Fatalf("construction site Plank = %d, want %d (warehouse delivery)", got, CarryCapacity)
	}
	if got := stock.Amount(resource.Plank); got != 0 {
		t.Fatalf("warehouse Plank = %d, want 0 (cargo must stay at the site)", got)
	}
}

// TestController_ConstructionWarehouseIsNotAnEndpointOrUnlimitedDropoff
// reproduces the new-warehouse loop: the unfinished Warehouse has the same
// Kind as a completed one, but must neither join the endpoint list nor bypass
// the construction material reservation. Exactly one five-plank trip may be
// assigned to its five-plank requirement, regardless of how many serfs wait.
func TestController_ConstructionWarehouseIsNotAnEndpointOrUnlimitedDropoff(t *testing.T) {
	grid := world.NewGrid(12, 3)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	site := building.NewConstructionSite(building.Warehouse, 8, 0)
	buildings := []*building.Building{warehouse, site}

	c := NewController(warehouse, 3)
	c.AddWarehouse(site)
	if got := len(c.Warehouses); got != 1 {
		t.Fatalf("registered warehouses = %d, want 1 (unfinished site is not an endpoint)", got)
	}

	stock := resource.NewStockpile(0)
	stock.Add(resource.Plank, 20)
	tick(c, grid, buildings, stock)

	assigned := 0
	for _, s := range c.Serfs {
		if s.DropoffBuilding() == site {
			assigned++
		}
	}
	if assigned != 1 {
		t.Fatalf("serfs assigned to one five-plank warehouse site = %d, want 1", assigned)
	}
}

func TestController_HaulsDirectlyBetweenProducerAndConsumer(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 20, Y: 20} // far away, off this road entirely
	farm := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	mill := &building.Building{Kind: building.Mill, X: 5, Y: 0}
	farm.AddOutput(resource.Wheat, 5)

	buildings := append([]*building.Building{warehouse, farm, mill}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 1)
	// The serf starts at the warehouse, far from this road -- give it
	// somewhere to actually start from for this test.
	c.Serfs[0].X, c.Serfs[0].Y = 0, 0
	c.Serfs[0].atBuilding = farm
	stock := resource.NewStockpile(100)

	for range 500 {
		tick(c, nil, buildings, stock)
		if mill.InputBuffer[resource.Wheat] > 0 {
			break
		}
	}

	if got := mill.InputBuffer[resource.Wheat]; got != 1 {
		t.Fatalf("mill InputBuffer[Wheat] = %d, want 1 (hauled straight from the farm)", got)
	}
	if got := farm.OutputBuffer[resource.Wheat]; got != 4 {
		t.Fatalf("farm OutputBuffer[Wheat] = %d, want 4 (one unit delivered, four remain)", got)
	}
	if got := stock.Amount(resource.Wheat); got != 0 {
		t.Fatalf("warehouse Wheat = %d, want 0 (never should have routed through the warehouse)", got)
	}
}

// TestController_DeliversConstructionMaterialDirectlyFromProducer covers
// the user's explicit request ("если требуется строителям - несем их им а
// не на склад"): a construction site short on Plank must be suppliable
// straight from a Carpentry Workshop's own OutputBuffer -- the same
// producer->consumer priority ordinary buildings already get via
// findDirectJob -- instead of being forced through the Warehouse first,
// even though the Warehouse has room and would otherwise happily take it.
// Uses a real grid (not nil, unlike most of this file's tests): unlike an
// ordinary haul, construction delivery is checked with
// pathfind.FindLandPath, which short-circuits false on a nil grid.
func TestController_DeliversConstructionMaterialDirectlyFromProducer(t *testing.T) {
	grid := world.NewGrid(15, 4)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	workshop := &building.Building{Kind: building.CarpentryWorkshop, X: 5, Y: 0}
	site := building.NewConstructionSite(building.LumberjackHut, 10, 0)
	want := building.Types[building.LumberjackHut].PlankCost
	// AddOutput lazily inits the map but caps at BufferCapacity (6); the
	// site needs 10, more than one Workshop ever holds at once in normal
	// play. Set the raw buffer directly to stand in for "the workshop kept
	// producing while the serf made more than one trip" without also
	// having to simulate its Recipe tick in this test.
	workshop.AddOutput(resource.Plank, 1)
	workshop.OutputBuffer[resource.Plank] = want

	buildings := []*building.Building{warehouse, workshop, site}
	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100) // plenty of room, but never actually gets any Plank

	for range 500 {
		tick(c, grid, buildings, stock)
		if site.InputBuffer[resource.Plank] >= want {
			break
		}
	}

	if got := site.InputBuffer[resource.Plank]; got != want {
		t.Fatalf("site InputBuffer[Plank] = %d, want %d (full cost delivered)", got, want)
	}
	if got := workshop.OutputBuffer[resource.Plank]; got != 0 {
		t.Fatalf("workshop OutputBuffer[Plank] = %d, want 0 (all of it hauled straight to the site)", got)
	}
	if got := stock.Amount(resource.Plank); got != 0 {
		t.Fatalf("warehouse Plank = %d, want 0 (never should have routed through the warehouse)", got)
	}
}

// TestController_FallsBackToWarehouseForConstructionMaterialWhenNoProducerHasIt
// covers the other half of the same priority order: with no Carpentry
// Workshop in town at all, a construction site's Plank must still come
// from the Warehouse -- the fallback findConstructionSupplyJob provided
// even before findConstructionDirectJob existed.
func TestController_FallsBackToWarehouseForConstructionMaterialWhenNoProducerHasIt(t *testing.T) {
	grid := world.NewGrid(15, 4)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	site := building.NewConstructionSite(building.LumberjackHut, 10, 0)

	buildings := []*building.Building{warehouse, site}
	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)
	stock.Add(resource.Plank, building.Types[building.LumberjackHut].PlankCost)

	want := building.Types[building.LumberjackHut].PlankCost
	for range 500 {
		tick(c, grid, buildings, stock)
		if site.InputBuffer[resource.Plank] >= want {
			break
		}
	}

	if got := site.InputBuffer[resource.Plank]; got != want {
		t.Fatalf("site InputBuffer[Plank] = %d, want %d (delivered from the warehouse)", got, want)
	}
	if got := stock.Amount(resource.Plank); got != 0 {
		t.Fatalf("warehouse Plank = %d, want 0 (fully delivered)", got)
	}
}

func TestController_DisconnectedBuildingIsNeverServiced(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 5, Y: 0}
	farm.AddOutput(resource.Wheat, 5)

	// No road at all between them.
	buildings := []*building.Building{warehouse, farm}

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)

	for range 200 {
		tick(c, nil, buildings, stock)
	}

	if got := stock.Amount(resource.Wheat); got != 0 {
		t.Fatalf("warehouse Wheat = %d, want 0 (farm has no road connection)", got)
	}
	if got := farm.OutputBuffer[resource.Wheat]; got != 5 {
		t.Fatalf("farm OutputBuffer[Wheat] = %d, want 5 (still sitting there, uncollected)", got)
	}
}

func TestController_HireAndCancelJobs(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 3, Y: 4}
	farm := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)

	if got := c.Hire(); got == nil || len(c.Serfs) != 2 {
		t.Fatalf("Hire() produced %v serfs, want 2", len(c.Serfs))
	}

	s := c.Serfs[0]
	s.ph = toDropoff
	s.pickup = farm
	s.resource = resource.Wheat
	s.amount = 2
	s.X, s.Y = 1, 1
	c.CancelAllJobs(stock)

	if got := stock.Amount(resource.Wheat); got != 2 {
		t.Fatalf("stock Wheat after CancelAllJobs = %d, want 2", got)
	}
	if s.ph != idle || s.X != warehouse.X || s.Y != warehouse.Y {
		t.Fatalf("serf after CancelAllJobs = phase %v at (%d,%d), want idle at warehouse", s.ph, s.X, s.Y)
	}
}

// TestController_DoesNotDoubleReservePickupAfterCargoCollected reproduces
// the "залипший резерв источника" report directly: a serf that has already
// picked up cargo (ph == toDropoff) still had its Reserve() call re-reserve
// the pickup side every tick, on top of the buffer already having shrunk
// when TakeOutput ran. That made the source look emptier than it really
// was until the delivery finished, blocking a second idle serf from seeing
// -- and claiming -- the goods actually still sitting there.
func TestController_DoesNotDoubleReservePickupAfterCargoCollected(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 5, Y: 0}
	farm.AddOutput(resource.Wheat, 1) // as if an earlier trip already collected the rest

	buildings := append([]*building.Building{warehouse, farm}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 2)
	hauler := c.Serfs[0]
	hauler.ph = toDropoff
	hauler.pickup = farm
	hauler.dropoff = warehouse
	hauler.resource = resource.Wheat
	hauler.amount = 5 // cargo already in hand; farm's OutputBuffer already reflects this

	idle := c.Serfs[1]

	tick(c, nil, buildings, resource.NewStockpile(100))

	if !idle.Busy() || idle.PickupBuilding() != farm {
		t.Fatalf("idle serf did not claim the farm's remaining Wheat (busy=%v pickup=%v) -- the hauler's Reserve() is still reserving the pickup side after cargo was already collected", idle.Busy(), idle.PickupBuilding())
	}
	if _, amount := idle.Cargo(); amount != 1 {
		t.Fatalf("idle serf claimed %d units, want 1 (the actual remaining amount)", amount)
	}
}

// TestController_SkipsUnreachableSourceForReachableOne reproduces "поиск не
// всегда переходит к следующему доступному источнику": an unreachable
// producer listed first in the buildings slice used to make the job search
// return it anyway (no reachability check), so the serf just failed to
// commit and stood idle even though a second, perfectly reachable producer
// with the same surplus existed.
func TestController_SkipsUnreachableSourceForReachableOne(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	unreachableFarm := &building.Building{Kind: building.Farm, X: 20, Y: 20} // no road anywhere near it
	reachableFarm := &building.Building{Kind: building.Farm, X: 5, Y: 0}
	unreachableFarm.AddOutput(resource.Wheat, 5)
	reachableFarm.AddOutput(resource.Wheat, 5)

	// unreachableFarm listed first, so a naive "first match" search would
	// pick it and never try reachableFarm.
	buildings := append([]*building.Building{warehouse, unreachableFarm, reachableFarm}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)

	for range 500 {
		tick(c, nil, buildings, stock)
		if stock.Amount(resource.Wheat) > 0 {
			break
		}
	}

	if got := stock.Amount(resource.Wheat); got != 5 {
		t.Fatalf("warehouse Wheat = %d, want 5 (serf should have skipped the unreachable farm and collected from the reachable one)", got)
	}
	if got := unreachableFarm.OutputBuffer[resource.Wheat]; got != 5 {
		t.Fatalf("unreachable farm OutputBuffer[Wheat] = %d, want 5 (untouched)", got)
	}
	if got := reachableFarm.OutputBuffer[resource.Wheat]; got != 0 {
		t.Fatalf("reachable farm OutputBuffer[Wheat] = %d, want 0 (collected)", got)
	}
}

// TestSortedResourceTypesIsDeterministic guards against "выбор задания
// зависит от порядка обхода Go-map": job searches used to range directly
// over a buffer/recipe map, whose iteration order Go deliberately
// randomizes. sortedResourceTypes must return the same order every time
// for the same keys, regardless of how many times it's called.
func TestSortedResourceTypesIsDeterministic(t *testing.T) {
	m := map[resource.Type]int{resource.Bread: 3, resource.Wheat: 1, resource.Flour: 2}
	want := []resource.Type{resource.Wheat, resource.Flour, resource.Bread}

	for i := range 20 {
		got := sortedResourceTypes(m)
		if !slices.Equal(got, want) {
			t.Fatalf("run %d: sortedResourceTypes(%v) = %v, want %v", i, m, got, want)
		}
	}
}

// TestController_EatsAtNearestReachableTavern covers "NPC кушают только в
// одной харчевне": a hungry serf used to always walk to whichever Tavern
// happened to be first in the buildings slice, no matter how far away.
// tavernFar is listed before tavernNear specifically to rule out that.
func TestController_EatsAtNearestReachableTavern(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	tavernFar := &building.Building{Kind: building.Tavern, X: 8, Y: 1}
	tavernNear := &building.Building{Kind: building.Tavern, X: 3, Y: 1}
	tavernFar.AddInput(resource.Bread, 3)
	tavernNear.AddInput(resource.Bread, 3)

	buildings := append([]*building.Building{warehouse, tavernFar, tavernNear}, straightRoad(0, 9, 0)...)

	c := NewController(warehouse, 1)
	s := c.Serfs[0]
	stock := resource.NewStockpile(100)

	for range HungerInterval + 200 {
		tick(c, nil, buildings, stock)
		if s.ph == idle && s.ticksSinceMeal == 0 {
			break
		}
	}

	if got := tavernNear.InputBuffer[resource.Bread]; got != 2 {
		t.Fatalf("nearer tavern Bread = %d, want 2 (serf should have eaten there)", got)
	}
	if got := tavernFar.InputBuffer[resource.Bread]; got != 3 {
		t.Fatalf("farther tavern Bread = %d, want 3 (untouched)", got)
	}
}

// TestController_CollectsToNearestReachableWarehouse covers the same
// report for Warehouses: surplus output used to always route to whichever
// Warehouse was registered first, leaving every other one untouched even
// when it sat much closer to the source. warehouseFar is both listed and
// registered before warehouseNear to rule that out.
func TestController_CollectsToNearestReachableWarehouse(t *testing.T) {
	hut := &building.Building{Kind: building.LumberjackHut, X: 0, Y: 1}
	hut.AddOutput(resource.Log, 1)
	warehouseFar := &building.Building{Kind: building.Warehouse, X: 8, Y: 1}
	warehouseNear := &building.Building{Kind: building.Warehouse, X: 3, Y: 1}

	buildings := append([]*building.Building{hut, warehouseFar, warehouseNear}, straightRoad(0, 9, 0)...)

	c := NewController(warehouseFar, 1)
	c.AddWarehouse(warehouseNear)
	// Start the serf right at the hut so its own position can't
	// accidentally bias which warehouse looks closer -- only the hut's
	// distance to each warehouse should matter for the dropoff choice.
	c.Serfs[0].X, c.Serfs[0].Y = hut.X, hut.Y
	c.Serfs[0].atBuilding = hut

	tick(c, nil, buildings, resource.NewStockpile(100))

	s := c.Serfs[0]
	if s.DropoffBuilding() != warehouseNear {
		t.Fatalf("dropoff = %v, want the nearer warehouse (bug: still always picks the first-registered one)", s.DropoffBuilding())
	}
}

func TestController_RemoveWarehousePromotesAnotherWarehouse(t *testing.T) {
	first := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	second := &building.Building{Kind: building.Warehouse, X: 6, Y: 0}

	c := NewController(first, 0)
	c.AddWarehouse(second)
	if !c.RemoveWarehouse(first) {
		t.Fatal("RemoveWarehouse(first) = false, want true with another warehouse available")
	}
	if c.Warehouse != second {
		t.Fatalf("primary warehouse = %v, want remaining warehouse %v", c.Warehouse, second)
	}
	if len(c.Warehouses) != 1 || c.Warehouses[0] != second {
		t.Fatalf("registered warehouses = %v, want only %v", c.Warehouses, second)
	}
	if got := c.Hire(); got.X != second.X || got.Y != second.Y {
		t.Fatalf("new serf spawned at (%d,%d), want new primary warehouse (%d,%d)", got.X, got.Y, second.X, second.Y)
	}
	if c.RemoveWarehouse(second) {
		t.Fatal("RemoveWarehouse(last) = true, want false")
	}
}

func TestController_DismissalWaitsForCurrentHaul(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 5, Y: 0}
	farm.AddOutput(resource.Wheat, 1)
	buildings := append([]*building.Building{warehouse, farm}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(0)
	tick(c, nil, buildings, stock) // assigns the farm -> warehouse haul
	serf := c.Serfs[0]
	if !serf.Busy() {
		t.Fatal("serf did not receive the expected haul before dismissal")
	}
	if !c.RequestDismissal(serf) || !serf.Dismissing() {
		t.Fatal("RequestDismissal did not mark the live serf")
	}

	for range 100 {
		tick(c, nil, buildings, stock)
		if len(c.Serfs) == 0 {
			break
		}
	}
	if got := len(c.Serfs); got != 0 {
		t.Fatalf("serfs after completed dismissal = %d, want 0", got)
	}
	if got := stock.Amount(resource.Wheat); got != 1 {
		t.Fatalf("warehouse Wheat = %d, want 1: dismissal must not lose carried cargo", got)
	}
}

func TestController_DismissalRemovesIdleSerfBeforeNewJob(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	c := NewController(warehouse, 1)
	if !c.RequestDismissal(c.Serfs[0]) {
		t.Fatal("RequestDismissal(idle serf) = false")
	}
	tick(c, nil, []*building.Building{warehouse}, resource.NewStockpile(0))
	if got := len(c.Serfs); got != 0 {
		t.Fatalf("idle dismissed serfs = %d, want 0", got)
	}
}

// TestController_TavernSupplySkipsProducerThatCannotReachTavern covers
// "проверка только пути слуги к производителю, но не производителя к
// харчевне": producer1 sits on a dead-end spur reachable from the serf but
// with no road continuing on to the Tavern; producer2 shares the Tavern's
// through-road, so it can actually deliver. producer1 is listed first, so
// a naive "first match with surplus and a reachable first leg" search
// would commit to it, walk there, then discover on arrival it can't
// deliver (goods get returned safely, but the whole trip was wasted).
func TestController_TavernSupplySkipsProducerThatCannotReachTavern(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 20, Y: 20} // only the controller anchor; no route role in this test
	producer1 := &building.Building{Kind: building.Bakery, X: -4, Y: 0}
	producer1.AddOutput(resource.Bread, 5)
	// The non-road start point lies between two branches. A serf can reach
	// either branch from there, while the branches cannot reach each other,
	// even diagonally.
	spur := []*building.Building{
		{Kind: building.Road, X: -3, Y: 0},
		{Kind: building.Road, X: -2, Y: 0},
		{Kind: building.Road, X: -1, Y: 0},
	}

	producer2 := &building.Building{Kind: building.Bakery, X: 5, Y: 1}
	producer2.AddOutput(resource.Bread, 5)
	tavern := &building.Building{Kind: building.Tavern, X: 10, Y: 1}

	mainRoad := straightRoad(1, 11, 0) // x=1..10 at y=0, touches producer2 and the Tavern

	buildings := append(append([]*building.Building{warehouse, producer1, producer2, tavern}, spur...), mainRoad...)

	c := NewController(warehouse, 1)
	c.Serfs[0].X, c.Serfs[0].Y = 0, 0
	tick(c, nil, buildings, resource.NewStockpile(100))

	s := c.Serfs[0]
	if s.PickupBuilding() != producer2 {
		t.Fatalf("pickup = %v, want producer2 (bug: committed to a producer that can't actually deliver to the Tavern)", s.PickupBuilding())
	}
}

// TestController_DirectHaulSkipsProducerThatCannotReachConsumer is the
// same fix for direct producer->consumer hauls: producer1 is reachable
// from the serf but sits on a dead-end spur with no road onward to the
// Mill; producer2 shares the Mill's through-road.
func TestController_DirectHaulSkipsProducerThatCannotReachConsumer(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 20, Y: 20} // off to the side, irrelevant here
	producer1 := &building.Building{Kind: building.Farm, X: -4, Y: 0}
	producer1.AddOutput(resource.Wheat, 5)
	spur := []*building.Building{
		{Kind: building.Road, X: -3, Y: 0},
		{Kind: building.Road, X: -2, Y: 0},
		{Kind: building.Road, X: -1, Y: 0},
	}

	producer2 := &building.Building{Kind: building.Farm, X: 5, Y: 1}
	producer2.AddOutput(resource.Wheat, 5)
	mill := &building.Building{Kind: building.Mill, X: 10, Y: 1}

	mainRoad := straightRoad(1, 11, 0)

	buildings := append(append([]*building.Building{warehouse, producer1, producer2, mill}, spur...), mainRoad...)

	c := NewController(warehouse, 1)
	c.Serfs[0].X, c.Serfs[0].Y = 0, 0
	tick(c, nil, buildings, resource.NewStockpile(100))

	s := c.Serfs[0]
	if s.PickupBuilding() != producer2 || s.DropoffBuilding() != mill {
		t.Fatalf("job = %v -> %v, want producer2 -> mill (bug: committed to a producer that can't actually deliver to the consumer)", s.PickupBuilding(), s.DropoffBuilding())
	}
}

// TestController_SupplySkipsShortageNotInStock covers "findSupplyJob
// может выбрать первую нехватку ресурса, которого нет на складе": the
// Mill is listed first and short on Wheat, but the warehouse has none at
// all; the Bakery is short on Flour, which the warehouse does have. The
// search must not get stuck on the Mill's unfulfillable shortage.
func TestController_SupplySkipsShortageNotInStock(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	mill := &building.Building{Kind: building.Mill, X: 5, Y: 1}     // short on Wheat; none in stock
	bakery := &building.Building{Kind: building.Bakery, X: 9, Y: 1} // short on Flour; in stock

	buildings := append([]*building.Building{warehouse, mill, bakery}, straightRoad(1, 10, 0)...)

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)
	stock.Add(resource.Flour, 20) // no Wheat anywhere

	for range 500 {
		tick(c, nil, buildings, stock)
		if bakery.InputBuffer[resource.Flour] > 0 {
			break
		}
	}

	if got := bakery.InputBuffer[resource.Flour]; got != 1 {
		t.Fatalf("bakery InputBuffer[Flour] = %d, want 1 (serf should have skipped the Mill's Wheat shortage -- nothing in stock for it -- and supplied the Bakery instead)", got)
	}
	if got := mill.InputBuffer[resource.Wheat]; got != 0 {
		t.Fatalf("mill InputBuffer[Wheat] = %d, want 0 (no Wheat was ever in stock to deliver)", got)
	}
}

// TestController_MaxWaitingHungerKeepsGrowingPastInterval covers "счётчик
// голода ограничивается HungerInterval, поэтому несколько голодных юнитов
// получают одинаковый приоритет": with no Tavern reachable at all, the
// serf stays hungry indefinitely: HungerTicks (and MaxWaitingHunger) must
// keep growing past HungerInterval instead of saturating there, or the
// cross-controller priority ordering degrades back into ties broken by
// call order.
func TestController_MaxWaitingHungerKeepsGrowingPastInterval(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	buildings := []*building.Building{warehouse} // no Tavern at all

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)

	for range HungerInterval + 50 {
		tick(c, nil, buildings, stock)
	}

	if got := c.Serfs[0].HungerTicks(); got <= HungerInterval {
		t.Fatalf("HungerTicks() = %d, want > %d (the counter must keep counting past HungerInterval instead of saturating there)", got, HungerInterval)
	}
	if got := c.MaxWaitingHunger(); got != c.Serfs[0].HungerTicks() {
		t.Fatalf("MaxWaitingHunger() = %d, want %d (should reflect the true uncapped wait time)", got, c.Serfs[0].HungerTicks())
	}
}

func TestController_PrioritizesTavernSupply(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	bakery := &building.Building{Kind: building.Bakery, X: 5, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 7, Y: 0}
	bakery.AddOutput(resource.Bread, 5)

	buildings := append([]*building.Building{warehouse, bakery, tavern}, straightRoad(1, 5, 0)...)
	buildings = append(buildings, &building.Building{Kind: building.Road, X: 6, Y: 0})
	c := NewController(warehouse, 1)
	tick(c, nil, buildings, resource.NewStockpile(100))

	s := c.Serfs[0]
	if s.PickupBuilding() != bakery || s.DropoffBuilding() != tavern {
		t.Fatalf("first job = %v -> %v, want bakery -> tavern", s.PickupBuilding(), s.DropoffBuilding())
	}
}

// TestController_PriorityAPI covers Controller.SetPriority/Priority/
// Priorities directly: default is PriorityNormal, setting a level is
// reflected back, and resetting to PriorityNormal clears the entry
// entirely rather than storing a redundant zero.
func TestController_PriorityAPI(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	c := NewController(warehouse, 0)

	if got := c.Priority(building.Mill); got != PriorityNormal {
		t.Fatalf("default priority = %d, want PriorityNormal", got)
	}
	c.SetPriority(building.Mill, PriorityHigh)
	if got := c.Priority(building.Mill); got != PriorityHigh {
		t.Fatalf("priority after SetPriority = %d, want PriorityHigh", got)
	}
	if got := c.Priorities(); len(got) != 1 || got[building.Mill] != PriorityHigh {
		t.Fatalf("Priorities() = %v, want {Mill: PriorityHigh}", got)
	}
	c.SetPriority(building.Mill, PriorityNormal)
	if got := c.Priorities(); len(got) != 0 {
		t.Fatalf("Priorities() after resetting to PriorityNormal = %v, want empty", got)
	}
}

// TestController_PriorityBreaksDirectHaulTie covers the exact scenario the
// user asked about: a Farm's Wheat could go to either the Mill or the Pig
// Farm this tick -- without a priority set, the first one in the
// buildings slice wins (an accident of build order); with the Pig Farm
// prioritized, it wins instead, regardless of slice order.
func TestController_PriorityBreaksDirectHaulTie(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 20, Y: 20} // off to the side, irrelevant here
	farm := &building.Building{Kind: building.Farm, X: 0, Y: 1}
	farm.AddOutput(resource.Wheat, 5)
	mill := &building.Building{Kind: building.Mill, X: 5, Y: 1}
	pigFarm := &building.Building{Kind: building.PigFarm, X: 8, Y: 1}

	buildings := append([]*building.Building{warehouse, farm, mill, pigFarm}, straightRoad(0, 9, 0)...)

	c := NewController(warehouse, 1)
	c.Serfs[0].X, c.Serfs[0].Y = farm.X, farm.Y
	tick(c, nil, buildings, resource.NewStockpile(100))
	if got := c.Serfs[0].DropoffBuilding(); got != mill {
		t.Fatalf("without priority, dropoff = %v, want mill (first in buildings, tie-broken by order)", got)
	}

	c2 := NewController(warehouse, 1)
	c2.Serfs[0].X, c2.Serfs[0].Y = farm.X, farm.Y
	c2.SetPriority(building.PigFarm, PriorityHigh)
	tick(c2, nil, buildings, resource.NewStockpile(100))
	if got := c2.Serfs[0].DropoffBuilding(); got != pigFarm {
		t.Fatalf("with Pig Farm prioritized, dropoff = %v, want pigFarm", got)
	}
}

// TestController_PriorityBreaksSupplyTie is the same scenario one queue
// tier down: no direct producer, both buildings short on Wheat the
// Warehouse could cover.
func TestController_PriorityBreaksSupplyTie(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	mill := &building.Building{Kind: building.Mill, X: 5, Y: 1}
	pigFarm := &building.Building{Kind: building.PigFarm, X: 8, Y: 1}
	buildings := append([]*building.Building{warehouse, mill, pigFarm}, straightRoad(0, 9, 0)...)

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)
	stock.Add(resource.Wheat, 20)
	tick(c, nil, buildings, stock)
	if got := c.Serfs[0].DropoffBuilding(); got != mill {
		t.Fatalf("without priority, dropoff = %v, want mill (first in buildings, tie-broken by order)", got)
	}

	c2 := NewController(warehouse, 1)
	c2.SetPriority(building.PigFarm, PriorityHigh)
	stock2 := resource.NewStockpile(100)
	stock2.Add(resource.Wheat, 20)
	tick(c2, nil, buildings, stock2)
	if got := c2.Serfs[0].DropoffBuilding(); got != pigFarm {
		t.Fatalf("with Pig Farm prioritized, dropoff = %v, want pigFarm", got)
	}
}

// TestArriveAtPickup_ConstructionDeliveryFailureSetsBackoff is a
// regression guard for a real bug the user reported ("слуги ходили по
// кругу с материалами... материалы на стройку не доставились"): a
// construction site's delivery route can pass the pre-check
// (nearestReachableWarehouseOverLand, at job-assignment time) but then
// fail the re-check in arriveAtPickup -- something built in the way by
// the time the serf actually arrives at the pickup with cargo in hand.
// Without a backoff, the very next idle serf immediately re-discovers the
// exact same doomed job and repeats the failure forever. This checks the
// backoff is actually set when that re-check fails, and that the cargo is
// returned to the Warehouse rather than lost.
func TestArriveAtPickup_ConstructionDeliveryFailureSetsBackoff(t *testing.T) {
	grid := world.NewGrid(15, 4)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	site := building.NewConstructionSite(building.LumberjackHut, 10, 0)
	// A solid wall of ordinary (footprint-1) buildings across every row
	// blocks every route from the warehouse to the site -- landWalkable
	// treats only the start/goal tiles themselves as always walkable,
	// everything else in between must be clear.
	var wall []*building.Building
	for x := 3; x < 8; x++ {
		for y := 0; y < 4; y++ {
			wall = append(wall, &building.Building{Kind: building.Mill, X: x, Y: y})
		}
	}
	buildings := append([]*building.Building{warehouse, site}, wall...)

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)
	stock.Add(resource.Plank, 10)

	s := c.Serfs[0]
	s.pickup, s.dropoff = warehouse, site
	s.resource, s.amount = resource.Plank, 5
	s.construction = true
	s.X, s.Y = warehouse.X, warehouse.Y

	c.arriveAtPickup(s, grid, buildings, stock)

	if got := stock.Amount(resource.Plank); got != 10 {
		t.Fatalf("warehouse Plank after a failed delivery = %d, want 10 (cargo returned, not lost)", got)
	}
	if got := c.constructionBackoff[site]; got != constructionBackoffTicks {
		t.Fatalf("constructionBackoff[site] = %d, want %d (a failed delivery must back the site off)", got, constructionBackoffTicks)
	}
	if s.ph != idle {
		t.Fatalf("serf phase after a failed delivery = %v, want idle", s.ph)
	}
}

// TestFindConstructionSupplyJob_SkipsBlockedSite checks the other half of
// the same fix: a site currently sitting out its backoff must not be
// re-offered to another serf, even though it would otherwise qualify.
func TestFindConstructionSupplyJob_SkipsBlockedSite(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	site := building.NewConstructionSite(building.LumberjackHut, 5, 0)
	buildings := []*building.Building{warehouse, site}
	stock := resource.NewStockpile(100)
	stock.Add(resource.Plank, 10)

	if _, _, _, ok := findConstructionSupplyJob(buildings, stock, reservations.New(), nil); !ok {
		t.Fatal("findConstructionSupplyJob with no blocked sites = not found, want found")
	}

	blocked := map[*building.Building]int{site: constructionBackoffTicks}
	if _, _, _, ok := findConstructionSupplyJob(buildings, stock, reservations.New(), blocked); ok {
		t.Fatal("findConstructionSupplyJob found a site that's currently backed off, want it skipped")
	}
}

// TestFindConstructionDirectJob_SkipsBlockedSite mirrors the warehouse-
// sourced test above for the direct-from-producer path.
func TestFindConstructionDirectJob_SkipsBlockedSite(t *testing.T) {
	grid := world.NewGrid(15, 4)
	workshop := &building.Building{Kind: building.CarpentryWorkshop, X: 0, Y: 0}
	site := building.NewConstructionSite(building.LumberjackHut, 5, 0)
	workshop.AddOutput(resource.Plank, 5)
	buildings := []*building.Building{workshop, site}
	from := pathfind.Point{X: 0, Y: 0}

	if _, _, _, _, _, ok := findConstructionDirectJob(grid, buildings, reservations.New(), from, nil); !ok {
		t.Fatal("findConstructionDirectJob with no blocked sites = not found, want found")
	}

	blocked := map[*building.Building]int{site: constructionBackoffTicks}
	if _, _, _, _, _, ok := findConstructionDirectJob(grid, buildings, reservations.New(), from, blocked); ok {
		t.Fatal("findConstructionDirectJob found a site that's currently backed off, want it skipped")
	}
}

// TestTickConstructionBackoff_CountsDownAndExpires checks the countdown
// itself: an entry decrements by one per call and disappears once it
// reaches zero, making the site a normal candidate again.
func TestTickConstructionBackoff_CountsDownAndExpires(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	c := NewController(warehouse, 0)
	site := &building.Building{Kind: building.LumberjackHut, X: 5, Y: 0}
	c.constructionBackoff[site] = 2

	c.tickConstructionBackoff()
	if got := c.constructionBackoff[site]; got != 1 {
		t.Fatalf("backoff after one tick = %d, want 1", got)
	}

	c.tickConstructionBackoff()
	if _, still := c.constructionBackoff[site]; still {
		t.Fatal("backoff entry still present after it should have expired")
	}
}

// TestController_DirectHaulSuppliesSmelteryWithIronOre proves that direct
// producer-to-consumer logistics considers AltRecipes. Before this regression
// guard, findDirectJob checked only Smeltery.Recipe (GoldOre + Coal), so it
// sent IronOre from the Miner Hut to the Warehouse instead of the Smeltery.
func TestController_DirectHaulSuppliesSmelteryWithIronOre(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	minerHut := &building.Building{Kind: building.MinerHut, X: 3, Y: 0}
	smeltery := &building.Building{Kind: building.Smeltery, X: 6, Y: 0}
	minerHut.AddOutput(resource.IronOre, 1)
	buildings := []*building.Building{warehouse, minerHut, smeltery}
	buildings = append(buildings, straightRoad(1, 3, 0)...)
	buildings = append(buildings, straightRoad(4, 6, 0)...)

	controller := NewController(warehouse, 1)
	tick(controller, nil, buildings, resource.NewStockpile(100))

	serf := controller.Serfs[0]
	if serf.PickupBuilding() != minerHut || serf.DropoffBuilding() != smeltery {
		t.Fatalf("job = %v -> %v, want MinerHut -> Smeltery for IronOre", serf.PickupBuilding(), serf.DropoffBuilding())
	}
	if cargo, amount := serf.Cargo(); cargo != resource.IronOre || amount != 1 {
		t.Fatalf("reserved cargo = %v x%d, want IronOre x1", cargo, amount)
	}
}

// TestController_SuppliesSmelteryWithIronOreFromWarehouse covers the fallback
// leg after ore has already reached storage. findSupplyJob must consider the
// Smeltery's alternative iron recipe, otherwise the iron stays in stock
// forever even when the smeltery is empty and connected.
func TestController_SuppliesSmelteryWithIronOreFromWarehouse(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	smeltery := &building.Building{Kind: building.Smeltery, X: 3, Y: 0}
	buildings := append([]*building.Building{warehouse, smeltery}, straightRoad(1, 3, 0)...)
	stock := resource.NewStockpile(100)
	stock.Add(resource.IronOre, 1)

	controller := NewController(warehouse, 1)
	for range 100 {
		tick(controller, nil, buildings, stock)
		if smeltery.InputBuffer[resource.IronOre] == 1 {
			return
		}
	}
	t.Fatalf("smeltery IronOre input = %d, want 1 after a warehouse supply run", smeltery.InputBuffer[resource.IronOre])
}

// TestSerf_RemainingPath is a regression guard for the inspector's
// route-line overlay (ui.DrawSelectedRoute, and the same pattern
// mirrored across villagers/lumberjack/fishing/quarry/builder/miner): an
// idle serf reports no path, a serf mid-haul reports the tiles still
// ahead of it (not the whole route from the start -- the player wants to
// see where it's *going*, not where it's already been), and that
// remaining length only ever shrinks as it walks, never grows or resets
// mid-trip.
func TestSerf_RemainingPath(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 5, Y: 0}
	farm.AddOutput(resource.Wheat, 5)
	buildings := append([]*building.Building{warehouse, farm}, straightRoad(1, 5, 0)...)

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)
	s := c.Serfs[0]

	if got := s.RemainingPath(); len(got) != 0 {
		t.Fatalf("idle serf RemainingPath = %v, want empty", got)
	}

	// A full haul is two legs (walk to the farm, then walk back to the
	// warehouse with cargo) and arriveAtPickup hands the serf a brand new
	// path for the second leg in the very same tick the first leg's
	// path empties -- so RemainingPath legitimately jumps back up once,
	// exactly when State() changes. It must still only shrink within a
	// single leg (same State value).
	lastState, lastLen := State(-1), -1
	sawMidTrip := false
	for range 500 {
		tick(c, nil, buildings, stock)
		remaining := s.RemainingPath()
		if len(remaining) == 0 {
			if sawMidTrip {
				break // the whole haul just finished -- stop before an unrelated second trip could start
			}
			continue
		}
		sawMidTrip = true
		if s.State() == lastState && len(remaining) > lastLen {
			t.Fatalf("RemainingPath grew from %d to %d tiles within the same leg (state %v), want monotonically non-increasing", lastLen, len(remaining), s.State())
		}
		lastState, lastLen = s.State(), len(remaining)
	}
	if !sawMidTrip {
		t.Fatal("serf never reported a non-empty RemainingPath during its haul")
	}
}
