package building

import (
	"testing"

	"strategy_game/internal/resource"
)

func TestProductionRatios(t *testing.T) {
	mill := Types[Mill].Recipe
	if got := mill.Inputs[resource.Wheat]; got != 1 {
		t.Fatalf("mill Wheat input = %d, want 1", got)
	}
	if mill.OutputAmount != 1 || mill.Output != resource.Flour {
		t.Fatalf("mill recipe = %+v, want 1 Flour", mill)
	}

	bakery := Types[Bakery].Recipe
	if got := bakery.Inputs[resource.Flour]; got != 1 {
		t.Fatalf("bakery Flour input = %d, want 1", got)
	}
	if bakery.OutputAmount != 2 || bakery.Output != resource.Bread {
		t.Fatalf("bakery recipe = %+v, want 2 Bread", bakery)
	}

	// Per the user's explicit request, Wine comes in small batches (2 at
	// a time) rather than one 8-unit harvest at the end of the cycle --
	// see the Recipe's own doc comment in types.go for the full reasoning
	// (it fixed a real "buffer instantly at 100% full" side effect a
	// single all-at-once 8-unit batch had). OutputCapacity stays at 8
	// (unchanged): with 2-unit batches, that's still room for 4
	// uncollected cycles before the buffer would actually block progress,
	// same margin of safety as before, just no longer front-loaded.
	winery := Types[Winery].Recipe
	if winery.OutputAmount != 2 || winery.Output != resource.Wine {
		t.Fatalf("winery recipe = %+v, want 2 Wine per batch", winery)
	}
	if Types[Winery].Footprint != 3 || Types[Winery].OutputCapacity != 8 {
		t.Fatalf("winery type = %+v, want 3x3 footprint and output capacity 8", Types[Winery])
	}

	pigFarm := Types[PigFarm].Recipe
	if got := pigFarm.Inputs[resource.Wheat]; got != 3 {
		t.Fatalf("pig farm Wheat input = %d, want 3", got)
	}
	if pigFarm.Output != resource.Carcass || pigFarm.OutputAmount != 1 || pigFarm.TicksToProduce != 600 || !pigFarm.ConsumeInputsAtStart {
		t.Fatalf("pig farm recipe = %+v, want 3 Wheat before 600 ticks -> 1 Carcass", pigFarm)
	}

	meatWorkshop := Types[MeatWorkshop].Recipe
	if got := meatWorkshop.Inputs[resource.Carcass]; got != 1 {
		t.Fatalf("meat workshop Carcass input = %d, want 1", got)
	}
	if meatWorkshop.Output != resource.Sausage || meatWorkshop.OutputAmount != 2 {
		t.Fatalf("meat workshop recipe = %+v, want 1 Carcass -> 2 Sausage", meatWorkshop)
	}

	carpentry := Types[CarpentryWorkshop].Recipe
	if got := carpentry.Inputs[resource.Log]; got != 1 {
		t.Fatalf("carpentry workshop Log input = %d, want 1", got)
	}
	if carpentry.Output != resource.Plank || carpentry.OutputAmount != 2 {
		t.Fatalf("carpentry workshop recipe = %+v, want 1 Log -> 2 Plank", carpentry)
	}
	if Types[CarpentryWorkshop].Footprint != 1 || !Types[CarpentryWorkshop].RequiresWorker {
		t.Fatalf("carpentry workshop type = %+v, want 1x1 footprint with a resident worker", Types[CarpentryWorkshop])
	}
	if Types[CarpentryWorkshop].OutputCapacity != 0 {
		t.Fatalf("carpentry workshop OutputCapacity = %d, want 0 (default BufferCapacity, same as everywhere else)", Types[CarpentryWorkshop].OutputCapacity)
	}
}
