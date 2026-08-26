package logistics

import (
	"slices"
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
)

// tick runs one simulation step exactly like cmd/game does: a fresh
// ledger, seeded from this controller's own in-flight serfs, then Tick.
func tick(c *Controller, buildings []*building.Building, stock *resource.Stockpile) {
	ledger := reservations.New()
	c.Reserve(ledger)
	c.Tick(buildings, stock, ledger)
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
	tick(c, buildings, stock)

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
	c.advance(s, nil, stock)

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

	for range 500 {
		tick(c, buildings, stock)
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
	tavern.AddInput(resource.Wine, 1)

	buildings := append([]*building.Building{warehouse, tavern}, straightRoad(1, 5, 0)...)
	c := NewController(warehouse, 1)
	s := c.Serfs[0]
	s.ticksSinceMeal = HungerInterval
	stock := resource.NewStockpile(100)

	for range 100 {
		tick(c, buildings, stock)
		if s.ticksSinceMeal == 0 {
			break
		}
	}

	if got := tavern.InputBuffer[resource.Wine]; got != 0 {
		t.Fatalf("tavern Wine = %d, want 0 (wine is a valid meal)", got)
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
		tick(c, buildings, stock)
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
		tick(c, buildings, stock)
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
		tick(c, buildings, stock)
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
		tick(c, buildings, stock)
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

func TestController_DisconnectedBuildingIsNeverServiced(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 5, Y: 0}
	farm.AddOutput(resource.Wheat, 5)

	// No road at all between them.
	buildings := []*building.Building{warehouse, farm}

	c := NewController(warehouse, 1)
	stock := resource.NewStockpile(100)

	for range 200 {
		tick(c, buildings, stock)
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

	tick(c, buildings, resource.NewStockpile(100))

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
		tick(c, buildings, stock)
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

	for range 500 {
		tick(c, buildings, stock)
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

	tick(c, buildings, resource.NewStockpile(100))

	s := c.Serfs[0]
	if s.DropoffBuilding() != warehouseNear {
		t.Fatalf("dropoff = %v, want the nearer warehouse (bug: still always picks the first-registered one)", s.DropoffBuilding())
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
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	producer1 := &building.Building{Kind: building.Bakery, X: 0, Y: 2}
	producer1.AddOutput(resource.Bread, 5)
	spur := &building.Building{Kind: building.Road, X: 0, Y: 1} // dead end, connects nowhere else

	producer2 := &building.Building{Kind: building.Bakery, X: 5, Y: 1}
	producer2.AddOutput(resource.Bread, 5)
	tavern := &building.Building{Kind: building.Tavern, X: 10, Y: 1}

	mainRoad := straightRoad(1, 11, 0) // x=1..10 at y=0, touches producer2 and the Tavern

	buildings := append([]*building.Building{warehouse, producer1, spur, producer2, tavern}, mainRoad...)

	c := NewController(warehouse, 1)
	c.Serfs[0].X, c.Serfs[0].Y = 0, 0
	tick(c, buildings, resource.NewStockpile(100))

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
	producer1 := &building.Building{Kind: building.Farm, X: 0, Y: 2}
	producer1.AddOutput(resource.Wheat, 5)
	spur := &building.Building{Kind: building.Road, X: 0, Y: 1}

	producer2 := &building.Building{Kind: building.Farm, X: 5, Y: 1}
	producer2.AddOutput(resource.Wheat, 5)
	mill := &building.Building{Kind: building.Mill, X: 10, Y: 1}

	mainRoad := straightRoad(1, 11, 0)

	buildings := append([]*building.Building{warehouse, producer1, spur, producer2, mill}, mainRoad...)

	c := NewController(warehouse, 1)
	c.Serfs[0].X, c.Serfs[0].Y = 0, 0
	tick(c, buildings, resource.NewStockpile(100))

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
		tick(c, buildings, stock)
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
		tick(c, buildings, stock)
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
	tick(c, buildings, resource.NewStockpile(100))

	s := c.Serfs[0]
	if s.PickupBuilding() != bakery || s.DropoffBuilding() != tavern {
		t.Fatalf("first job = %v -> %v, want bakery -> tavern", s.PickupBuilding(), s.DropoffBuilding())
	}
}
