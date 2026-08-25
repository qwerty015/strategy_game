package villagers

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

func straightRoad(fromX, toX, y int) []*building.Building {
	var roads []*building.Building
	for x := fromX; x < toX; x++ {
		roads = append(roads, &building.Building{Kind: building.Road, X: x, Y: y})
	}
	return roads
}

func TestVillager_WalksToTavernWhenHungryAndBack(t *testing.T) {
	farm := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0}
	tavern.AddInput(resource.Bread, 3)

	buildings := append([]*building.Building{farm, tavern}, straightRoad(1, 5, 0)...)

	c := NewController()
	c.Spawn(Farmer, farm)
	v := c.Villagers[0]

	// Run past HungerInterval plus enough ticks for a full round trip.
	for range 500 {
		c.Tick(buildings)
		if v.Working() && v.ticksSinceMeal == 0 {
			break // has eaten and returned home
		}
	}

	if !v.Working() {
		t.Fatalf("villager phase = %v, want back to working", v.ph)
	}
	if v.X != farm.X || v.Y != farm.Y {
		t.Fatalf("villager position = (%d,%d), want back at home (%d,%d)", v.X, v.Y, farm.X, farm.Y)
	}
	if got := tavern.InputBuffer[resource.Bread]; got != 2 {
		t.Fatalf("tavern Bread = %d, want 2 (one loaf eaten)", got)
	}
	if v.Starving {
		t.Fatal("Starving = true after a successful meal, want false")
	}
}

func TestVillager_StarvingWhenNoTavernReachable(t *testing.T) {
	farm := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	// No tavern at all.
	buildings := []*building.Building{farm}

	c := NewController()
	c.Spawn(Farmer, farm)
	v := c.Villagers[0]

	for range HungerInterval + 5 {
		c.Tick(buildings)
	}

	if !v.Starving {
		t.Fatal("Starving = false with no tavern in town, want true")
	}
	if !v.Working() {
		t.Fatal("villager left its post with nowhere to actually go, want it to stay put")
	}
}

func TestFarmerWalksAcrossItsFieldWhileWorking(t *testing.T) {
	farm := &building.Building{Kind: building.Farm, X: 4, Y: 6}
	c := NewController()
	c.Spawn(Farmer, farm)
	v := c.Villagers[0]

	for range FarmWorkStepTicks + 1 {
		c.Tick([]*building.Building{farm})
	}

	if !v.Working() {
		t.Fatal("farmer left work without a reachable tavern")
	}
	if v.X == farm.X && v.Y == farm.Y {
		t.Fatalf("farmer stayed on farmhouse tile at (%d,%d), want a field tile", v.X, v.Y)
	}
	if v.X < farm.X || v.X >= farm.X+3 || v.Y < farm.Y || v.Y >= farm.Y+3 {
		t.Fatalf("farmer position = (%d,%d), want inside farm footprint", v.X, v.Y)
	}
}

func TestController_RemoveHome(t *testing.T) {
	farm := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	bakery := &building.Building{Kind: building.Bakery, X: 3, Y: 0}
	c := NewController()
	c.Spawn(Farmer, farm)
	c.Spawn(Baker, bakery)

	c.RemoveHome(farm)

	if len(c.Villagers) != 1 || c.Villagers[0].Home != bakery {
		t.Fatalf("villagers after RemoveHome = %+v, want only bakery worker", c.Villagers)
	}
}
