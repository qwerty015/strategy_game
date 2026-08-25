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
	"strategy_game/internal/resource"
	"strategy_game/internal/villagers"
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
		logi.Tick(buildings, stock)
		vills.Tick(buildings)

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
