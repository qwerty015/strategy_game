package logistics

import (
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
