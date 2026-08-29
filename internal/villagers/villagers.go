// Package villagers simulates Farmer, Baker, Winemaker, Swineherd, Butcher,
// Carpenter and Smelter units. Unlike serfs
// (package logistics) they don't haul goods -- they stand and work at
// one building (Home) -- but per the user's request they get the same
// treatment as every other unit "as in the reference game": they get
// hungry and must walk to a Tavern to eat, or their building's work
// stalls (see the starving map economy.Tick takes).
package villagers

import (
	"strategy_game/internal/building"
	"strategy_game/internal/hunger"
	"strategy_game/internal/meal"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
)

// Profession selects which sprite a Villager is drawn with (see
// render/villagers.go); behavior is identical for every profession.
type Profession int

const (
	Farmer Profession = iota
	Baker
	Winemaker
	Swineherd
	Butcher
	Carpenter
	Smelter
)

const (
	// HungerInterval is the 20% satiety threshold at which a villager
	// starts walking to the Tavern. Death happens at hunger.MaxTicks.
	HungerInterval = hunger.MealThresholdTicks

	// TicksPerTile is how many simulation ticks it takes a villager to
	// cross one tile of road, walking to/from the Tavern.
	TicksPerTile = 2

	// FarmWorkStepTicks controls the deliberately slow visible field-work loop.
	// A worker changes crop cells every four seconds at normal speed, so the
	// unit looks like it is tending the crop instead of teleporting around it.
	FarmWorkStepTicks = 8
)

// farmWorkRoute is the loop the farmer walks through while tending the eight
// field cells around the farmhouse. It is visual feedback only: production
// still advances through economy.Simulator, while the route makes sowing and
// harvesting readable on the map.
var farmWorkRoute = [...]building.Point{
	{X: 1, Y: 0},
	{X: 2, Y: 0},
	{X: 2, Y: 1},
	{X: 2, Y: 2},
	{X: 1, Y: 2},
	{X: 0, Y: 2},
	{X: 0, Y: 1},
	{X: 1, Y: 1},
}

type phase int

const (
	working phase = iota
	toTavern
	toHome
)

// Villager is a production worker such as a Farmer, Baker or Swineherd.
type Villager struct {
	Profession Profession
	Home       *building.Building

	X, Y int

	ph        phase
	path      []pathfind.Point
	pathIdx   int
	tileTicks int

	tavern *building.Building // which Tavern this trip is headed to/from, while ph != working
	meal   resource.Type      // food reserved for the current Tavern trip

	ticksSinceMeal int
	workTicks      int

	// Starving is true once HungerInterval has passed and there was
	// nowhere to actually go eat (no Tavern built yet, no road to one,
	// or all food is unavailable) -- exported so cmd/game can build the
	// starving-buildings map economy.Tick uses to pause this villager's
	// building, and so rendering can show it.
	Starving bool
}

// State is the public, read-only activity state used by the inspector and
// render layer. The movement bookkeeping itself remains private here.
type State int

const (
	VillagerWorking State = iota
	VillagerToTavern
	VillagerToHome
)

// State reports whether the villager is working or walking to eat.
func (v *Villager) State() State {
	return State(v.ph)
}

// HungerTicks returns simulation ticks since the villager's last meal.
func (v *Villager) HungerTicks() int {
	return v.ticksSinceMeal
}

// SatietyPercent returns the player-facing 0-100 satiety value.
func (v *Villager) SatietyPercent() int {
	return hunger.Percent(v.ticksSinceMeal)
}

// RemainingPath returns the tiles still ahead on the villager's current
// route, starting from (and including) the tile it's walking toward right
// now -- for the inspector's route-line overlay (see
// ui.DrawSelectedRoute). nil once idle or with no path assigned.
func (v *Villager) RemainingPath() []pathfind.Point {
	if v.pathIdx >= len(v.path) {
		return nil
	}
	return v.path[v.pathIdx:]
}

// HomeBuilding returns the building where this villager works.
func (v *Villager) HomeBuilding() *building.Building {
	return v.Home
}

// NewVillager creates a Villager standing at home, working.
func NewVillager(profession Profession, home *building.Building) *Villager {
	return &Villager{Profession: profession, Home: home, X: home.X, Y: home.Y, meal: resource.Bread}
}

// Working reports whether the villager is at its post right now, as
// opposed to out walking to/from a meal.
func (v *Villager) Working() bool {
	return v.ph == working
}

// VisibleOnMap reports whether the unit should be drawn as a person. Field
// workers remain visible while they tend their crop cells; bakers still use
// the compact worker marker while working inside their bakery. All workers
// appear as units when walking to or from the Tavern.
func (v *Villager) VisibleOnMap() bool {
	return !v.Working() || ((v.Profession == Farmer && v.Home != nil && v.Home.Kind == building.Farm) ||
		(v.Profession == Winemaker && v.Home != nil && v.Home.Kind == building.Winery))
}

// Meal returns the food reserved for the current or next Tavern trip.
func (v *Villager) Meal() resource.Type { return v.meal }

// Controller owns every production worker in town.
type Controller struct {
	Villagers []*Villager
	meals     meal.Selector
}

// NewController creates an empty roster; use Spawn to add villagers as
// their buildings get placed.
func NewController() *Controller {
	return &Controller{meals: meal.NewSelector(0x13198a2e)}
}

// MealSeed returns the persistent pseudo-random state for villager meal choices.
func (c *Controller) MealSeed() uint32 { return c.meals.Seed() }

// SetMealSeed restores the persistent pseudo-random state for villager meals.
func (c *Controller) SetMealSeed(seed uint32) { c.meals.SetSeed(seed) }

// Spawn adds a villager working at home.
func (c *Controller) Spawn(profession Profession, home *building.Building) {
	c.Villagers = append(c.Villagers, NewVillager(profession, home))
}

// HasHome reports whether a worker is already assigned to home. One
// production building may employ only its matching single resident.
func (c *Controller) HasHome(home *building.Building) bool {
	for _, v := range c.Villagers {
		if v.Home == home {
			return true
		}
	}
	return false
}

// RestoreVillager recreates a worker from a save snapshot. Unlike a new
// Spawn, it keeps the saved map position and hunger state. If the worker was
// walking to eat when saved, its route is rebuilt from that exact position;
// the old tile-by-tile path itself is not part of the save format.
func (c *Controller) RestoreVillager(profession Profession, home *building.Building, x, y, hungerTicks int, starving bool, state State, buildings []*building.Building, savedMeal ...resource.Type) *Villager {
	v := NewVillager(profession, home)
	v.X, v.Y = x, y
	if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
		v.meal = savedMeal[0]
	}
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	v.ticksSinceMeal = hungerTicks
	v.Starving = starving

	switch state {
	case VillagerToTavern:
		// From the saved (x, y), not home -- the villager may already have
		// been partway to the Tavern when the game was saved. No ledger
		// exists yet at load time, so the saved meal is used to find a
		// reachable Tavern that still physically holds that exact food.
		wanted := []resource.Type(nil)
		if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
			wanted = append(wanted, savedMeal[0])
		}
		if tavern, meal, path, ok := nearestTavernWithFood(buildings, pathfind.Point{X: x, Y: y}, nil, &c.meals, wanted...); ok {
			v.path, v.pathIdx, v.tileTicks = path, 0, 0
			v.tavern = tavern
			if len(savedMeal) == 0 || !resource.IsFood(savedMeal[0]) {
				v.meal = meal
			}
			v.ph = toTavern
		}
	case VillagerToHome:
		if path, ok := pathfind.FindPathFromPoint(buildings, pathfind.Point{X: x, Y: y}, home); ok {
			v.path, v.pathIdx, v.tileTicks = path, 0, 0
			v.ph = toHome
		}
	}

	c.Villagers = append(c.Villagers, v)
	return v
}

// RemoveHome removes the worker assigned to a building that was deleted.
// Workers are tied to their workplace in the current economy, so keeping a
// villager with a dangling Home pointer would make it continue working at a
// building that no longer exists.
func (c *Controller) RemoveHome(home *building.Building) {
	kept := c.Villagers[:0]
	for _, v := range c.Villagers {
		if v.Home != home {
			kept = append(kept, v)
		}
	}
	c.Villagers = kept
}

// CancelRouteTo resets any villager currently walking toward target as a
// Tavern back to its post at Home, instead of leaving it holding a
// dangling pointer to a building that's about to be removed from the
// world. Call this before deleting a building, in case it's a Tavern
// someone is mid-trip to eat at -- RemoveHome alone doesn't cover this,
// since a villager's Home is its farm/bakery, never the Tavern it eats
// at. A no-op for anyone not currently walking toward target.
func (c *Controller) CancelRouteTo(target *building.Building) {
	for _, v := range c.Villagers {
		if v.ph != toTavern || v.tavern != target {
			continue
		}
		v.tavern = nil
		v.path, v.pathIdx, v.tileTicks = nil, 0, 0
		v.X, v.Y = v.Home.X, v.Home.Y
		v.ph = working
		v.workTicks = 0
	}
}

// Reserve seeds ledger with every villager currently walking to eat, so
// other controllers sharing a Tavern (serfs, lumberjacks) see this claim
// before making their own commitments this tick. Call once per
// simulation tick, before this or any other controller's Tick runs.
func (c *Controller) Reserve(ledger *reservations.Ledger) {
	for _, v := range c.Villagers {
		if v.ph == toTavern && v.tavern != nil {
			ledger.ReservePickup(v.tavern, v.meal, 1)
		}
	}
}

// MaxWaitingHunger returns the highest HungerTicks among villagers that
// will actually try to eat this tick (working and HungerTicks >=
// HungerInterval), or -1 if none will. cmd/game compares this against the
// other unit controllers' MaxWaitingHunger to decide whose Tick runs
// first this simulation tick when the Tavern's food is scarce -- the
// unit that's been waiting longest gets first claim, instead of
// whichever controller happens to be first in a fixed call order.
//
// This only works because ticksSinceMeal keeps counting past
// HungerInterval instead of saturating there: once several units across
// different controllers are simultaneously overdue, a counter capped at
// HungerInterval would tie them all at the same value, and the ordering
// would silently fall back to the fixed call order it was built to
// replace.
func (c *Controller) MaxWaitingHunger() int {
	best := -1
	for _, v := range c.Villagers {
		if v.ph != working || v.ticksSinceMeal < HungerInterval {
			continue
		}
		if v.ticksSinceMeal > best {
			best = v.ticksSinceMeal
		}
	}
	return best
}

// Tick advances hunger and movement for every villager. Call once per
// simulation tick, after every controller sharing ledger has had a
// chance to Reserve its own pre-existing in-flight units.
func (c *Controller) Tick(buildings []*building.Building, ledger *reservations.Ledger) int {
	deaths := 0
	remaining := c.Villagers[:0]
	for _, v := range c.Villagers {
		v.ticksSinceMeal++
		if hunger.Dead(v.ticksSinceMeal) {
			deaths++
			continue
		}
		c.tick(v, buildings, ledger)
		remaining = append(remaining, v)
	}
	c.Villagers = remaining
	return deaths
}

// nearestTavernWithFood returns the nearest reachable Tavern that has any
// available food. The selector randomly chooses one item from that Tavern's
// currently available menu. Pass a nil ledger to search by reachability while
// rebuilding a saved route.
//
// `from` is a Point rather than a building precisely so RestoreVillager
// can search from the exact saved (x, y) -- which can differ from Home if
// the villager was already mid-walk to eat when the game was saved.
// Searching from Home instead used to make a restored, already-hungry
// villager jump back to the farmhouse tile before setting off again.
func nearestTavernWithFood(buildings []*building.Building, from pathfind.Point, ledger *reservations.Ledger, selector *meal.Selector, wanted ...resource.Type) (tavern *building.Building, selected resource.Type, path []pathfind.Point, ok bool) {
	bestLen := -1
	var available []resource.Type
	for _, b := range buildings {
		if b == nil || b.Kind != building.Tavern {
			continue
		}
		foods := make([]resource.Type, 0, len(resource.FoodTypes()))
		for _, food := range resource.FoodTypes() {
			if len(wanted) > 0 && food != wanted[0] {
				continue
			}
			if ledger != nil && ledger.AvailableInput(b, food) <= 0 {
				continue
			}
			if ledger == nil && b.InputBuffer[food] <= 0 {
				continue
			}
			foods = append(foods, food)
		}
		if len(foods) == 0 {
			continue
		}
		p, reachable := pathfind.FindPathFromPoint(buildings, from, b)
		if !reachable {
			continue
		}
		if bestLen == -1 || len(p) < bestLen {
			tavern, available, path, bestLen = b, foods, p, len(p)
		}
	}
	if tavern == nil {
		return nil, 0, nil, false
	}
	if selector == nil || len(wanted) > 0 {
		selected = available[0]
	} else {
		selected, _ = selector.Pick(available)
	}
	return tavern, selected, path, true
}

func (c *Controller) tick(v *Villager, buildings []*building.Building, ledger *reservations.Ledger) {
	switch v.ph {
	case working:
		c.tickWorking(v, buildings, ledger)
	case toTavern, toHome:
		tickWalking(v, buildings)
	}
}

func (c *Controller) tickWorking(v *Villager, buildings []*building.Building, ledger *reservations.Ledger) {
	if !hunger.NeedsMeal(v.ticksSinceMeal) {
		v.animateFieldWork()
		return
	}

	// Hungry enough to need a meal now.
	tavern, meal, path, ok := nearestTavernWithFood(buildings, pathfind.Point{X: v.Home.X, Y: v.Home.Y}, ledger, &c.meals)
	if !ok {
		v.Starving = true
		v.animateFieldWork()
		return
	}
	v.Starving = false
	// The road network starts at the farmhouse access tile. The farmer has
	// just finished the current field pass, so visually return to that tile
	// before starting the meal route.
	v.X, v.Y = v.Home.X, v.Home.Y
	v.tavern = tavern
	v.meal = meal
	v.path, v.pathIdx, v.tileTicks = path, 0, 0
	v.ph = toTavern
	ledger.ReservePickup(tavern, meal, 1)
}

func (v *Villager) animateFieldWork() {
	if v.Home == nil || (v.Profession != Farmer && v.Profession != Winemaker) {
		return
	}
	if v.Profession == Farmer && v.Home.Kind != building.Farm {
		return
	}
	if v.Profession == Winemaker && v.Home.Kind != building.Winery {
		return
	}
	v.workTicks++
	step := (v.workTicks / FarmWorkStepTicks) % len(farmWorkRoute)
	p := farmWorkRoute[step]
	v.X, v.Y = v.Home.X+p.X, v.Home.Y+p.Y
}

func tickWalking(v *Villager, buildings []*building.Building) {
	v.tileTicks++
	if v.tileTicks < TicksPerTile {
		return
	}
	v.tileTicks = 0

	if v.pathIdx < len(v.path)-1 {
		v.pathIdx++
		v.X, v.Y = v.path[v.pathIdx].X, v.path[v.pathIdx].Y
		return
	}

	if v.ph == toTavern {
		tavern := v.tavern
		v.X, v.Y = tavern.X, tavern.Y
		if tavern.TakeInput(v.meal, 1) {
			v.ticksSinceMeal = 0
		}
		// Whether or not there was still food by the time we arrived,
		// head home -- retrying immediately would just loop in place.
		if path, ok := pathfind.FindPath(buildings, tavern, v.Home); ok {
			v.path, v.pathIdx, v.tileTicks = path, 0, 0
			v.ph = toHome
		}
		v.tavern = nil
		// If home is somehow unreachable from here, just stay put at
		// the tavern; tickWorking will never run again until the road
		// is fixed, but that's an edge case not worth over-engineering.
		return
	}

	v.X, v.Y = v.Home.X, v.Home.Y
	v.ph = working
	v.workTicks = 0
}
