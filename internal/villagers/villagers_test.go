package villagers

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
)

// tickController runs one simulation step exactly like cmd/game does: a
// fresh ledger, seeded from this controller's own in-flight villagers,
// then Tick. (Named to avoid colliding with the package's own unexported
// per-villager tick helper.)
func tickController(c *Controller, buildings []*building.Building) {
	ledger := reservations.New()
	c.Reserve(ledger)
	c.Tick(buildings, ledger)
}

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
	// ticksSinceMeal resets to 0 back at the Tavern, several ticks before
	// the villager physically arrives home -- by the time ph flips back
	// to working it's already counting up again, so checking it here
	// would never coincide with the arrival tick. Position is the
	// reliable signal instead: none of animateFieldWork's eight field
	// cells is the farmhouse tile itself, so "working AND at home" is
	// true for exactly the one tick right after arrival, before the
	// first field-work step moves the villager away again.
	for range HungerInterval + 200 {
		tickController(c, buildings)
		if v.Working() && v.X == farm.X && v.Y == farm.Y {
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
		tickController(c, buildings)
	}

	if !v.Starving {
		t.Fatal("Starving = false with no tavern in town, want true")
	}
	if !v.Working() {
		t.Fatal("villager left its post with nowhere to actually go, want it to stay put")
	}
}

// TestRestoreVillager_SearchesFromSavedPositionNotHome reproduces the
// "визуально прыгает после загрузки" report: RestoreVillager used to
// search for a Tavern from v.Home instead of from the saved (x, y), so a
// villager saved mid-walk with an unreachable Home would fail to resume
// its route (or, if Home happened to be reachable via a different route,
// silently rebuild the wrong one) instead of continuing from where it
// actually stood. Home is placed with no road connection at all, while
// the saved position sits right next to a Tavern -- only searching from
// the saved position succeeds.
func TestRestoreVillager_SearchesFromSavedPositionNotHome(t *testing.T) {
	farm := &building.Building{Kind: building.Farm, X: 10, Y: 10} // no road anywhere near it
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 1}
	tavern.AddInput(resource.Bread, 3)

	buildings := append([]*building.Building{farm, tavern}, straightRoad(1, 6, 0)...)

	c := NewController()
	v := c.RestoreVillager(Farmer, farm, 4, 0, HungerInterval, false, VillagerToTavern, buildings)

	if v.X != 4 || v.Y != 0 {
		t.Fatalf("villager position after restore = (%d,%d), want to stay at the saved (4,0) instead of jumping", v.X, v.Y)
	}
	if v.State() != VillagerToTavern {
		t.Fatalf("villager state = %v, want VillagerToTavern (bug: searched for a route from the unreachable Home instead of the saved position)", v.State())
	}
}

// TestController_CancelRouteToResetsVillagerWithoutDanglingPointer covers
// "при удалении харчевни уже идущие к ней фермеры не получают отмену
// маршрута": deleting a Tavern a villager is mid-walk to eat at used to
// leave v.tavern pointing at a building no longer in the world.
func TestController_CancelRouteToResetsVillagerWithoutDanglingPointer(t *testing.T) {
	farm := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0}

	c := NewController()
	c.Spawn(Farmer, farm)
	v := c.Villagers[0]
	v.ph = toTavern
	v.tavern = tavern
	v.X, v.Y = 3, 0

	c.CancelRouteTo(tavern)

	if v.ph != working {
		t.Fatalf("villager phase = %v, want working (back at post)", v.ph)
	}
	if v.tavern != nil {
		t.Fatal("v.tavern is still set after CancelRouteTo -- dangling pointer to the deleted Tavern")
	}
	if v.X != farm.X || v.Y != farm.Y {
		t.Fatalf("villager position = (%d,%d), want back at home (%d,%d)", v.X, v.Y, farm.X, farm.Y)
	}
}

// TestVillager_EatsAtNearestReachableTavern covers "NPC кушают только в
// одной харчевне": a hungry farmer/baker used to always walk to whichever
// Tavern happened to be first in the buildings slice, no matter how far
// away. tavernFar is listed before tavernNear specifically to rule that
// out -- both sit off the same through-road at y=0 so neither blocks the
// path to the other.
func TestVillager_EatsAtNearestReachableTavern(t *testing.T) {
	farm := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	tavernFar := &building.Building{Kind: building.Tavern, X: 8, Y: 1}
	tavernNear := &building.Building{Kind: building.Tavern, X: 4, Y: 1}
	tavernFar.AddInput(resource.Bread, 3)
	tavernNear.AddInput(resource.Bread, 3)

	buildings := append([]*building.Building{farm, tavernFar, tavernNear}, straightRoad(1, 9, 0)...)

	c := NewController()
	c.Spawn(Farmer, farm)
	v := c.Villagers[0]

	for range HungerInterval + 200 {
		tickController(c, buildings)
		if v.Working() && v.ticksSinceMeal == 0 {
			break
		}
	}

	if got := tavernNear.InputBuffer[resource.Bread]; got != 2 {
		t.Fatalf("nearer tavern Bread = %d, want 2 (villager should have eaten there)", got)
	}
	if got := tavernFar.InputBuffer[resource.Bread]; got != 3 {
		t.Fatalf("farther tavern Bread = %d, want 3 (untouched)", got)
	}
}

func TestFarmerWalksAcrossItsFieldWhileWorking(t *testing.T) {
	farm := &building.Building{Kind: building.Farm, X: 4, Y: 6}
	c := NewController()
	c.Spawn(Farmer, farm)
	v := c.Villagers[0]

	for range FarmWorkStepTicks + 1 {
		tickController(c, []*building.Building{farm})
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
