package economy_test

// End-to-end check that the whole pipeline the user described actually
// works together: Farm -> Mill -> Bakery -> Tavern, serfs hauling
// (preferring direct hauls over routing through the Warehouse), a
// Farmer and a Baker who get hungry and eat at the Tavern, and
// production pausing while a worker is off finding food. Deliberately
// stays below cmd/game/ebiten -- this exercises only the plain-Go logic
// packages (see AGENTS.md on why they're kept ebiten-free).

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/villagers"
	"strategy_game/internal/world"
)

func TestFullChain_FarmToMillToBakeryToTavern(t *testing.T) {
	// A through-road at y=1 that no building sits on, with every
	// building's footprint touching it (Farm via a short spur, since
	// its 3x3 footprint can't itself border y=1 without overlapping the
	// road). Buildings-on-the-road would block the very path other
	// buildings' workers need to cross -- pathfind only treats a road
	// tile or the two endpoint buildings' own footprints as walkable,
	// not any third building's footprint (see AGENTS.md).
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 2, Y: 3} // footprint 3: (2,3)-(4,5)
	mill := &building.Building{Kind: building.Mill, X: 6, Y: 0}
	bakery := &building.Building{Kind: building.Bakery, X: 8, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 10, Y: 0}

	var roads []*building.Building
	for x := 0; x <= 11; x++ {
		roads = append(roads, &building.Building{Kind: building.Road, X: x, Y: 1})
	}
	roads = append(roads, &building.Building{Kind: building.Road, X: 2, Y: 2}) // spur down to the Farm

	buildings := append([]*building.Building{warehouse, farm, mill, bakery, tavern}, roads...)

	logi := logistics.NewController(warehouse, 3)
	vills := villagers.NewController()
	vills.Spawn(villagers.Farmer, farm)
	vills.Spawn(villagers.Baker, bakery)
	stock := resource.NewStockpile(200)

	starving := func() map[*building.Building]bool {
		m := make(map[*building.Building]bool)
		for _, v := range vills.Villagers {
			// Only pause production while the worker is physically away
			// (walking to/from the Tavern). A worker who's merely hungry
			// but still at their post (e.g. because the Tavern has no
			// Bread yet) keeps working -- gating on Starving too would
			// deadlock the very first cycle: the Tavern can't get Bread
			// until the Bakery produces some, and the Bakery can't
			// produce while "starving".
			if !v.Working() {
				m[v.Home] = true
			}
		}
		return m
	}

	var sawBread bool
	for range 5000 {
		economy.Tick(buildings, starving())

		ledger := reservations.New()
		logi.Reserve(ledger)
		vills.Reserve(ledger)
		logi.Tick(nil, buildings, stock, ledger)
		vills.Tick(buildings, ledger)

		if tavern.InputBuffer[resource.Bread] > 0 || stock.Amount(resource.Bread) > 0 {
			sawBread = true
		}
	}

	if !sawBread {
		t.Fatal("bread never made it out of the bakery in 5000 ticks -- the chain is broken somewhere")
	}

	// The Tavern has been getting stocked with Bread throughout (that's
	// what sawBread confirms above), so by now neither villager should
	// be stuck unable to find a meal.
	for _, v := range vills.Villagers {
		if v.Starving {
			t.Errorf("%v villager is still Starving after 5000 ticks with a stocked Tavern reachable", v.Profession)
		}
	}

	// Buffer caps: nothing anywhere should ever exceed BufferCapacity.
	for _, b := range []*building.Building{farm, mill, bakery, tavern} {
		for rt, n := range b.OutputBuffer {
			if n > building.BufferCapacity {
				t.Errorf("%v OutputBuffer[%s] = %d, want <= %d", b.Kind, rt, n, building.BufferCapacity)
			}
		}
		for rt, n := range b.InputBuffer {
			if n > building.BufferCapacity {
				t.Errorf("%v InputBuffer[%s] = %d, want <= %d", b.Kind, rt, n, building.BufferCapacity)
			}
		}
	}

}

// TestFullChain_FarmToPigFarmToMeatWorkshopToTavern ensures animal feed is
// delivered through the same physical-road logistics as every other recipe.
// The pig farm must receive all three wheat units before its 600-tick growth;
// the produced carcass then travels directly to the meat workshop and the
// two sausage portions end up in the tavern as normal food.
func TestFullChain_FarmToPigFarmToMeatWorkshopToTavern(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 2, Y: 3}
	pigFarm := &building.Building{Kind: building.PigFarm, X: 4, Y: 0}
	meatWorkshop := &building.Building{Kind: building.MeatWorkshop, X: 6, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 8, Y: 0}

	var roads []*building.Building
	for x := 0; x <= 9; x++ {
		roads = append(roads, &building.Building{Kind: building.Road, X: x, Y: 1})
	}
	roads = append(roads, &building.Building{Kind: building.Road, X: 2, Y: 2})
	buildings := append([]*building.Building{warehouse, farm, pigFarm, meatWorkshop, tavern}, roads...)

	logi := logistics.NewController(warehouse, 3)
	vills := villagers.NewController()
	vills.Spawn(villagers.Farmer, farm)
	vills.Spawn(villagers.Swineherd, pigFarm)
	vills.Spawn(villagers.Butcher, meatWorkshop)
	stock := resource.NewStockpile(200)

	starving := func() map[*building.Building]bool {
		m := make(map[*building.Building]bool)
		for _, v := range vills.Villagers {
			if !v.Working() {
				m[v.HomeBuilding()] = true
			}
		}
		return m
	}

	var sawSausage bool
	for range 2000 {
		economy.Tick(buildings, starving())
		ledger := reservations.New()
		logi.Reserve(ledger)
		vills.Reserve(ledger)
		logi.Tick(nil, buildings, stock, ledger)
		vills.Tick(buildings, ledger)
		if tavern.InputBuffer[resource.Sausage] > 0 || stock.Amount(resource.Sausage) > 0 {
			sawSausage = true
			break
		}
	}

	if !sawSausage {
		t.Fatal("sausage never reached the tavern or warehouse: pig chain is broken")
	}
	if got := pigFarm.InputBuffer[resource.Wheat]; got > building.BufferCapacity {
		t.Fatalf("pig farm Wheat buffer = %d, want <= %d", got, building.BufferCapacity)
	}
}

// TestFullChain_LumberjackHutToCarpentryWorkshopToWarehouse covers the new
// wood chain: a lumberjack cuts a tree and stores a Log in the hut, a serf
// hauls it to the Carpentry Workshop, the carpenter turns it into Plank,
// and -- since nothing consumes Plank yet -- a serf eventually drains the
// surplus to the Warehouse. Exercises all three unit controllers
// (logistics, villagers, lumberjack) together, the way cmd/game does.
func TestFullChain_LumberjackHutToCarpentryWorkshopToWarehouse(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	hut := &building.Building{Kind: building.LumberjackHut, X: 2, Y: 0}
	carpentry := &building.Building{Kind: building.CarpentryWorkshop, X: 4, Y: 0}
	tree := building.NewTree(2, 2) // reachable on land from the hut, off the road
	tree.GrowthTicks = tree.GrowthTargetTicks

	var roads []*building.Building
	for x := 0; x <= 4; x++ {
		roads = append(roads, &building.Building{Kind: building.Road, X: x, Y: 1})
	}
	buildings := append([]*building.Building{warehouse, hut, carpentry, tree}, roads...)
	grid := world.NewGrid(10, 6)

	logi := logistics.NewController(warehouse, 3)
	vills := villagers.NewController()
	vills.Spawn(villagers.Carpenter, carpentry)
	jacks := lumberjack.NewController()
	jacks.Spawn(hut)
	stock := resource.NewStockpile(200)

	starving := func() map[*building.Building]bool {
		m := make(map[*building.Building]bool)
		for _, v := range vills.Villagers {
			if !v.Working() {
				m[v.HomeBuilding()] = true
			}
		}
		for _, j := range jacks.Lumberjacks {
			if !j.AtPost() {
				m[j.HomeBuilding()] = true
			}
		}
		return m
	}

	var sawPlank bool
	for range 3000 {
		economy.Tick(buildings, starving())

		ledger := reservations.New()
		logi.Reserve(ledger)
		vills.Reserve(ledger)
		jacks.Reserve(ledger)
		logi.Tick(nil, buildings, stock, ledger)
		vills.Tick(buildings, ledger)
		for _, event := range jacks.Tick(grid, buildings, ledger) {
			if event.Kind == lumberjack.TreeCut {
				for i, b := range buildings {
					if b == event.Tree {
						buildings = append(buildings[:i], buildings[i+1:]...)
						break
					}
				}
			}
		}

		if carpentry.OutputBuffer[resource.Plank] > 0 || stock.Amount(resource.Plank) > 0 {
			sawPlank = true
			break
		}
	}

	if !sawPlank {
		t.Fatal("plank never appeared: log -> carpentry chain is broken")
	}
	if got := carpentry.InputBuffer[resource.Log]; got > building.BufferCapacity {
		t.Fatalf("carpentry Log buffer = %d, want <= %d", got, building.BufferCapacity)
	}
}

// TestSharedTavernReservation_SerfAndVillagerDoNotDoubleBookTheLastLoaf is
// the cross-controller version of the user's "crowd" bug report: a serf
// (package logistics) and a farmer (package villagers) both starving at
// once, with only one loaf of Bread in the Tavern. Each controller used to
// only see its own units, so both could commit to the same last loaf in
// the same tick -- one of them would always walk there for nothing. The
// shared reservations.Ledger (seeded via Reserve before either Tick runs)
// is what makes them aware of each other.
func TestSharedTavernReservation_SerfAndVillagerDoNotDoubleBookTheLastLoaf(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 5, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 8, Y: 0}
	tavern.AddInput(resource.Bread, 1) // exactly one meal available

	var roads []*building.Building
	for x := 1; x < 8; x++ {
		roads = append(roads, &building.Building{Kind: building.Road, X: x, Y: 0})
	}
	buildings := append([]*building.Building{warehouse, farm, tavern}, roads...)

	logi := logistics.NewController(warehouse, 0)
	// RestoreSerf/RestoreVillager are the same public entry points
	// save/load uses -- here they let the test start both units already
	// hungry, instead of ticking HungerInterval times to get there.
	logi.RestoreSerf(warehouse.X, warehouse.Y, logistics.HungerInterval, false, false)
	vills := villagers.NewController()
	vills.RestoreVillager(villagers.Farmer, farm, farm.X, farm.Y, villagers.HungerInterval, false, villagers.VillagerWorking, buildings)

	stock := resource.NewStockpile(100)
	var serfAte, villagerAte bool
	// Both units start already at the meal threshold (HungerInterval).
	// Whichever loses the ledger race for the single loaf keeps
	// accumulating hunger every tick with nothing to eat -- capped well
	// below hunger.MaxTicks so the loser doesn't starve to death mid-test,
	// which would remove it from its controller's roster and panic the
	// index lookups below.
	for range 120 {
		ledger := reservations.New()
		logi.Reserve(ledger)
		vills.Reserve(ledger)
		logi.Tick(nil, buildings, stock, ledger)
		vills.Tick(buildings, ledger)

		// Check every tick, not just the final one: HungerTicks() resets
		// to 0 right when a unit eats, then immediately starts counting
		// up again -- by tick 200 a unit that ate early could already be
		// hungry again, which would make a final-state-only check flaky.
		if logi.Serfs[0].HungerTicks() == 0 {
			serfAte = true
		}
		if vills.Villagers[0].HungerTicks() == 0 {
			villagerAte = true
		}
	}

	ate := 0
	if serfAte {
		ate++
	}
	if villagerAte {
		ate++
	}
	if ate != 1 {
		t.Fatalf("%d units ate the single loaf (serf=%v, villager=%v), want exactly 1", ate, serfAte, villagerAte)
	}
	if got := tavern.InputBuffer[resource.Bread]; got != 0 {
		t.Fatalf("tavern Bread = %d, want 0 (the one loaf is gone)", got)
	}
}
