package fishing

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

func TestFishermanCatchesMatureFishAndReturnsItToHut(t *testing.T) {
	grid := world.NewGrid(8, 4)
	for x := 1; x <= 6; x++ {
		grid.Set(x, 1, world.Tile{Terrain: world.Water})
	}
	hut := &building.Building{Kind: building.FisherHut, X: 1, Y: 0}
	fish := building.NewFish(5, 1)
	fish.GrowthTicks = fish.GrowthTargetTicks
	buildings := []*building.Building{hut, fish}

	controller := NewController()
	fisherman := controller.Spawn(hut)
	if fisherman == nil {
		t.Fatal("Spawn() returned nil")
	}

	caught := false
	for range 100 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		for _, event := range controller.Tick(grid, buildings, ledger) {
			if event.Kind != FishCaught || event.Fish != fish {
				t.Fatalf("unexpected fishing event: %#v", event)
			}
			caught = true
			// The game layer normally removes the fish and schedules regrowth.
			buildings = []*building.Building{hut}
		}
		if caught && hut.OutputBuffer[resource.Fish] == 1 {
			break
		}
	}
	if !caught {
		t.Fatal("fisherman never caught the mature fish")
	}
	if got := hut.OutputBuffer[resource.Fish]; got != 1 {
		t.Fatalf("hut Fish output = %d, want 1", got)
	}
	if !fisherman.AtPost() {
		t.Fatal("fisherman did not return to the hut after unloading")
	}
}

func TestFishermanUsesRoadToEatWine(t *testing.T) {
	grid := world.NewGrid(7, 3)
	hut := &building.Building{Kind: building.FisherHut, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 4, Y: 0}
	tavern.AddInput(resource.Wine, 1)
	buildings := []*building.Building{hut, tavern}
	for x := 1; x < 4; x++ {
		buildings = append(buildings, &building.Building{Kind: building.Road, X: x, Y: 0})
	}

	controller := NewController()
	fisherman := controller.Spawn(hut)
	fisherman.hungerTick = HungerInterval - 1
	for range 30 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if fisherman.HungerTicks() == 0 {
			break
		}
	}
	if fisherman.HungerTicks() != 0 {
		t.Fatal("fisherman did not reach the Tavern over the road")
	}
	if got := tavern.InputBuffer[resource.Wine]; got != 0 {
		t.Fatalf("tavern Wine = %d, want 0 after the meal", got)
	}
}

func TestFishermanRestoreKeepsWineMeal(t *testing.T) {
	grid := world.NewGrid(7, 3)
	hut := &building.Building{Kind: building.FisherHut, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 4, Y: 0}
	tavern.AddInput(resource.Wine, 1)
	buildings := []*building.Building{hut, tavern}
	for x := 1; x < 4; x++ {
		buildings = append(buildings, &building.Building{Kind: building.Road, X: x, Y: 0})
	}

	controller := NewController()
	fisherman := controller.Restore(hut, 2, 0, HungerInterval, false, StateToTavern, nil, 0, 0, grid, buildings, resource.Wine)
	if fisherman.Meal() != resource.Wine {
		t.Fatalf("restored meal = %v, want wine", fisherman.Meal())
	}
	for range 20 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
		if fisherman.HungerTicks() == 0 {
			break
		}
	}
	if fisherman.HungerTicks() != 0 {
		t.Fatal("restored fisherman did not eat the saved wine")
	}
}

// TestController_SurvivesManyIdleTicksWithNoWork guards the "filter in
// place" roster bug directly: Tick used to append a survivor to the kept
// slice only when its state-machine switch fell through to the bottom of
// the loop body, but nearly every branch (still walking, still fishing,
// nothing to do yet) exits early via `continue` -- which skipped the
// append and silently dropped a perfectly alive, non-hungry worker from
// the roster after its very first tick.
func TestController_SurvivesManyIdleTicksWithNoWork(t *testing.T) {
	grid := world.NewGrid(8, 4)
	hut := &building.Building{Kind: building.FisherHut, X: 0, Y: 0}
	buildings := []*building.Building{hut} // no fish at all -- StateIdle finds nothing every tick
	controller := NewController()
	controller.Spawn(hut)

	for range 50 {
		ledger := reservations.New()
		controller.Reserve(ledger)
		controller.Tick(grid, buildings, ledger)
	}

	if got := len(controller.Fishermen); got != 1 {
		t.Fatalf("fishermen after 50 idle ticks = %d, want 1 (worker must not vanish while merely idle)", got)
	}
}
