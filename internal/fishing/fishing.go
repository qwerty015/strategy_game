// Package fishing simulates fishermen. A fisherman walks only on the road
// network while on land, then launches directly from a waterside Fisher Hut
// and uses a boat-only path through Water tiles to catch mature Fish.
package fishing

import (
	"strategy_game/internal/building"
	"strategy_game/internal/meal"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

const (
	// HungerInterval matches the other town workers. Hunger is checked between
	// fishing trips and never teleports a fisherman out of a boat.
	HungerInterval = 180
	TicksPerTile   = 2
	CatchTicks     = 12
)

// State is the visible activity state of a fisherman.
type State int

const (
	StateIdle State = iota
	StateToFish
	StateFishing
	StateToHome
	StateUnloading
	StateToTavern
	StateToHomeAfterMeal
)

// EventKind identifies a completed water-world interaction.
type EventKind int

const (
	FishCaught EventKind = iota
)

// Event tells the game layer to remove the caught Fish and schedule its
// delayed fry replacement. The controller never owns the buildings slice.
type Event struct {
	Kind EventKind
	Fish *building.Building
}

// Fisherman is one worker assigned to one waterside hut.
type Fisherman struct {
	Home *building.Building
	X, Y int

	state      State
	target     *building.Building
	tavern     *building.Building
	meal       resource.Type
	path       []pathfind.Point
	pathIdx    int
	tileTicks  int
	workTicks  int
	cargo      int
	hungerTick int

	// Starving is presentation/status information only. A fisherman who is
	// hungry but cannot reach a stocked Tavern still keeps the food chain
	// alive, exactly like the other worker controllers.
	Starving bool
}

// Controller owns all fishermen in the settlement.
type Controller struct {
	Fishermen []*Fisherman
	meals     meal.Selector
}

// NewController creates an empty fishing roster.
func NewController() *Controller { return &Controller{meals: meal.NewSelector(0x082efa98)} }

// MealSeed returns the persistent pseudo-random state for fisherman meals.
func (c *Controller) MealSeed() uint32 { return c.meals.Seed() }

// SetMealSeed restores the persistent pseudo-random state for fisherman meals.
func (c *Controller) SetMealSeed(seed uint32) { c.meals.SetSeed(seed) }

// NewFisherman creates a worker at the hut's road access point.
func NewFisherman(home *building.Building) *Fisherman {
	if home == nil {
		return &Fisherman{}
	}
	p := home.AccessPoint()
	return &Fisherman{Home: home, X: p.X, Y: p.Y, meal: resource.Bread}
}

// Spawn assigns one fisherman to a newly built FisherHut.
func (c *Controller) Spawn(home *building.Building) *Fisherman {
	if home == nil || home.Kind != building.FisherHut {
		return nil
	}
	f := NewFisherman(home)
	c.Fishermen = append(c.Fishermen, f)
	return f
}

// Restore recreates a fisherman from a save snapshot. A saved meal is kept
// for a Tavern trip so a worker heading for wine cannot silently arrive and
// try to eat bread after loading.
func (c *Controller) Restore(home *building.Building, x, y, hungerTicks int, starving bool, state State, target *building.Building, workTicks, cargo int, grid *world.Grid, buildings []*building.Building, savedMeal ...resource.Type) *Fisherman {
	f := NewFisherman(home)
	f.X, f.Y = x, y
	if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
		f.meal = savedMeal[0]
	}
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	if cargo < 0 {
		cargo = 0
	}
	f.hungerTick, f.Starving, f.workTicks, f.cargo = hungerTicks, starving, workTicks, cargo

	switch state {
	case StateToFish:
		if matureFish(target) && f.routeWaterTo(grid, pathfind.Point{X: target.X, Y: target.Y}) {
			f.target, f.state = target, StateToFish
		}
	case StateFishing:
		if matureFish(target) {
			f.target, f.state = target, StateFishing
		}
	case StateToHome:
		if f.cargo > 0 && f.routeWaterHome(grid) {
			f.state = StateToHome
		}
	case StateUnloading:
		if f.cargo > 0 && f.atHome() {
			f.state = StateUnloading
		}
	case StateToTavern:
		wanted := []resource.Type(nil)
		if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
			wanted = append(wanted, savedMeal[0])
		}
		if tavern, meal, path, ok := nearestTavernWithFood(buildings, pathfind.Point{X: x, Y: y}, nil, &c.meals, wanted...); ok {
			f.tavern, f.path = tavern, path
			f.pathIdx, f.tileTicks = 0, 0
			if len(savedMeal) == 0 || !resource.IsFood(savedMeal[0]) {
				f.meal = meal
			}
			f.state = StateToTavern
		}
	case StateToHomeAfterMeal:
		if f.routeRoadHome(buildings) {
			f.state = StateToHomeAfterMeal
		}
	}

	c.Fishermen = append(c.Fishermen, f)
	return f
}

// RemoveHome removes the worker tied to a deleted hut and preserves an
// already caught fish by returning it to the shared stockpile.
func (c *Controller) RemoveHome(home *building.Building, stock *resource.Stockpile) {
	kept := c.Fishermen[:0]
	for _, f := range c.Fishermen {
		if f.Home != home {
			kept = append(kept, f)
			continue
		}
		if stock != nil && f.cargo > 0 {
			stock.Add(resource.Fish, f.cargo)
		}
	}
	c.Fishermen = kept
}

// CancelRouteTo resets a meal trip whose target Tavern is being deleted.
func (c *Controller) CancelRouteTo(target *building.Building) {
	for _, f := range c.Fishermen {
		if f.state != StateToTavern || f.tavern != target {
			continue
		}
		f.resetAtHome()
	}
}

// State reports the current activity.
func (f *Fisherman) State() State { return f.state }

// HomeBuilding returns the assigned fishing hut.
func (f *Fisherman) HomeBuilding() *building.Building { return f.Home }

// TargetFish returns the fish currently being approached or caught.
func (f *Fisherman) TargetFish() *building.Building { return f.target }

// Cargo returns the caught Fish being carried back to the hut.
func (f *Fisherman) Cargo() (resource.Type, int) { return resource.Fish, f.cargo }

// Meal returns food reserved for the current Tavern trip.
func (f *Fisherman) Meal() resource.Type { return f.meal }

// HungerTicks returns simulation ticks since the last meal.
func (f *Fisherman) HungerTicks() int { return f.hungerTick }

// WorkTicks returns progress through the net-casting animation.
func (f *Fisherman) WorkTicks() int { return f.workTicks }

// AtPost reports whether the fisherman is inside the hut and available.
func (f *Fisherman) AtPost() bool { return f.state == StateIdle || f.state == StateUnloading }

// InBoat reports whether rendering should use the boat sprite instead of the
// land walker sprite.
func (f *Fisherman) InBoat() bool {
	return f.state == StateToFish || f.state == StateFishing || f.state == StateToHome
}

// VisibleOnMap hides an idle/unloading worker behind the compact hut marker.
func (f *Fisherman) VisibleOnMap() bool { return !f.AtPost() }

// Reserve records existing in-flight Tavern claims before controllers tick.
func (c *Controller) Reserve(ledger *reservations.Ledger) {
	for _, f := range c.Fishermen {
		if f.state == StateToTavern && f.tavern != nil {
			ledger.ReservePickup(f.tavern, f.meal, 1)
		}
	}
}

// MaxWaitingHunger supports fair cross-controller Tavern contention.
func (c *Controller) MaxWaitingHunger() int {
	best := -1
	for _, f := range c.Fishermen {
		if f.state == StateIdle && f.hungerTick >= HungerInterval && f.hungerTick > best {
			best = f.hungerTick
		}
	}
	return best
}

// Tick advances fishing, boating and road-bound meal trips by one simulation
// step and returns every fish caught this tick.
func (c *Controller) Tick(grid *world.Grid, buildings []*building.Building, ledger *reservations.Ledger) []Event {
	var events []Event
	for _, f := range c.Fishermen {
		f.hungerTick++
		switch f.state {
		case StateIdle:
			if f.cargo > 0 {
				f.state = StateUnloading
				continue
			}
			if f.hungerTick >= HungerInterval && c.tryStartMeal(f, buildings, ledger) {
				continue
			}
			c.startFishingJob(f, grid, buildings)
		case StateToFish:
			if !matureFish(f.target) {
				f.resetAtHome()
				continue
			}
			if f.advancePath() {
				f.state, f.workTicks = StateFishing, 0
			}
		case StateFishing:
			if !matureFish(f.target) {
				f.resetAtHome()
				continue
			}
			f.workTicks++
			if f.workTicks < CatchTicks {
				continue
			}
			f.cargo, f.workTicks = 1, 0
			events = append(events, Event{Kind: FishCaught, Fish: f.target})
			if f.routeWaterHome(grid) {
				f.state = StateToHome
			} else {
				f.resetAtHome()
			}
		case StateToHome:
			if f.advancePath() {
				f.setAtHome()
				f.state = StateUnloading
			}
		case StateUnloading:
			if f.Home != nil && f.Home.AddOutput(resource.Fish, f.cargo) == f.cargo {
				f.cargo = 0
				f.resetAtHome()
			}
		case StateToTavern:
			if !f.advancePath() {
				continue
			}
			if f.tavern != nil && f.tavern.TakeInput(f.meal, 1) {
				f.hungerTick, f.Starving = 0, false
			} else {
				f.Starving = true
			}
			f.tavern = nil
			if f.routeRoadHome(buildings) {
				f.state = StateToHomeAfterMeal
			} else {
				f.resetAtHome()
			}
		case StateToHomeAfterMeal:
			if f.advancePath() {
				f.resetAtHome()
			}
		}
	}
	return events
}

func (c *Controller) startFishingJob(f *Fisherman, grid *world.Grid, buildings []*building.Building) {
	if f.Home == nil || f.Home.OutputBuffer[resource.Fish] >= building.BufferCapacity {
		return
	}
	launch, ok := building.WaterAccessPoint(grid, f.Home)
	if !ok {
		return
	}
	bestLength := -1
	var target *building.Building
	var bestPath []pathfind.Point
	for _, candidate := range buildings {
		if !matureFish(candidate) || c.fishReserved(candidate, f) {
			continue
		}
		path, reachable := pathfind.FindWaterPath(grid, pathfind.Point{X: launch.X, Y: launch.Y}, pathfind.Point{X: candidate.X, Y: candidate.Y})
		if !reachable || (bestLength >= 0 && len(path) >= bestLength) {
			continue
		}
		target, bestPath, bestLength = candidate, path, len(path)
	}
	if target == nil {
		return
	}
	f.X, f.Y = launch.X, launch.Y
	f.target = target
	f.setPath(bestPath)
	f.state = StateToFish
}

func (c *Controller) fishReserved(target *building.Building, skip *Fisherman) bool {
	for _, f := range c.Fishermen {
		if f != skip && f.target == target && (f.state == StateToFish || f.state == StateFishing || f.state == StateToHome) {
			return true
		}
	}
	return false
}

func (c *Controller) tryStartMeal(f *Fisherman, buildings []*building.Building, ledger *reservations.Ledger) bool {
	tavern, meal, path, ok := nearestTavernWithFood(buildings, pathfind.Point{X: f.X, Y: f.Y}, ledger, &c.meals)
	if !ok {
		f.Starving = true
		return false
	}
	f.Starving, f.tavern, f.meal = false, tavern, meal
	f.setPath(path)
	f.state = StateToTavern
	ledger.ReservePickup(tavern, meal, 1)
	return true
}

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
			if ledger != nil {
				if ledger.AvailableInput(b, food) <= 0 {
					continue
				}
			} else if b.InputBuffer[food] <= 0 {
				// Route restoration has no reservation ledger yet, but it
				// must still pick food that really exists in the Tavern.
				continue
			}
			foods = append(foods, food)
		}
		if len(foods) == 0 {
			continue
		}
		p, reachable := pathfind.FindPathFromPoint(buildings, from, b)
		if !reachable || (bestLen >= 0 && len(p) >= bestLen) {
			continue
		}
		tavern, available, path, bestLen = b, foods, p, len(p)
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

func (f *Fisherman) routeWaterTo(grid *world.Grid, goal pathfind.Point) bool {
	path, ok := pathfind.FindWaterPath(grid, pathfind.Point{X: f.X, Y: f.Y}, goal)
	if !ok {
		return false
	}
	f.setPath(path)
	return true
}

func (f *Fisherman) routeWaterHome(grid *world.Grid) bool {
	launch, ok := building.WaterAccessPoint(grid, f.Home)
	if !ok {
		return false
	}
	return f.routeWaterTo(grid, pathfind.Point{X: launch.X, Y: launch.Y})
}

func (f *Fisherman) routeRoadHome(buildings []*building.Building) bool {
	if f.Home == nil {
		return false
	}
	path, ok := pathfind.FindPathFromPoint(buildings, pathfind.Point{X: f.X, Y: f.Y}, f.Home)
	if !ok {
		return false
	}
	f.setPath(path)
	return true
}

func (f *Fisherman) setPath(path []pathfind.Point) {
	f.path, f.pathIdx, f.tileTicks = path, 0, 0
}

func (f *Fisherman) advancePath() bool {
	if len(f.path) == 0 {
		return false
	}
	f.tileTicks++
	if f.tileTicks < TicksPerTile {
		return false
	}
	f.tileTicks = 0
	if f.pathIdx < len(f.path)-1 {
		f.pathIdx++
		f.X, f.Y = f.path[f.pathIdx].X, f.path[f.pathIdx].Y
		return false
	}
	return true
}

func (f *Fisherman) atHome() bool {
	if f.Home == nil {
		return false
	}
	p := f.Home.AccessPoint()
	return f.X == p.X && f.Y == p.Y
}

func (f *Fisherman) setAtHome() {
	if f.Home == nil {
		return
	}
	p := f.Home.AccessPoint()
	f.X, f.Y = p.X, p.Y
}

func (f *Fisherman) resetAtHome() {
	f.setAtHome()
	f.state, f.target, f.tavern = StateIdle, nil, nil
	f.path, f.pathIdx, f.tileTicks, f.workTicks = nil, 0, 0, 0
}

func matureFish(b *building.Building) bool {
	return b != nil && b.Kind == building.Fish && b.GrowthStage() >= 2
}
