// Package lumberjack simulates the worker assigned to a Lumberjack Hut.
// Unlike serfs, a lumberjack is not restricted to the road network: he walks
// over every non-water tile, finds a mature tree, cuts it, and carries one
// finished Log back to his hut for collection by a serf.
package lumberjack

import (
	"strategy_game/internal/building"
	"strategy_game/internal/hunger"
	"strategy_game/internal/meal"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

const (
	// HungerInterval is the shared 20% satiety threshold. Hunger is checked
	// between jobs and never interrupts a tree-cutting or return trip.
	HungerInterval = hunger.MealThresholdTicks
	TicksPerTile   = 2

	// ChopTicks is six seconds at normal speed (two simulation ticks/sec).
	ChopTicks = 12
)

// State is the visible activity state of a lumberjack.
type State int

const (
	StateIdle State = iota
	StateToTree
	StateChopping
	StateToHome
	StateUnloading
	StateToTavern
	StateToHomeAfterMeal
)

// EventKind identifies a world change emitted by the controller.
type EventKind int

const (
	TreeCut EventKind = iota
	WorkerDied
)

// Event is returned after a worker finishes a world interaction. The game
// layer owns the building slice, so it removes the cut tree and schedules its
// regrowth after receiving TreeCut.
type Event struct {
	Kind  EventKind
	Tree  *building.Building
	Cargo int
}

// Lumberjack is one physical worker assigned to a Lumberjack Hut.
type Lumberjack struct {
	Home *building.Building
	X, Y int

	state      State
	target     *building.Building
	tavern     *building.Building // which Tavern this trip is headed to/from, while state == StateToTavern
	meal       resource.Type      // food reserved for the current Tavern trip
	path       []pathfind.Point
	pathIdx    int
	tileTicks  int
	workTicks  int
	cargo      int
	hungerTick int

	// Starving is true when the worker needs food but currently cannot reach a
	// stocked Tavern. The worker keeps performing the current work loop so a
	// missing Tavern does not deadlock the forestry chain.
	Starving bool
}

// Controller owns all lumberjacks in the settlement.
type Controller struct {
	Lumberjacks []*Lumberjack
	meals       meal.Selector
}

// NewController creates an empty lumberjack roster.
func NewController() *Controller {
	return &Controller{meals: meal.NewSelector(0xa4093822)}
}

// MealSeed returns the persistent pseudo-random state for lumberjack meals.
func (c *Controller) MealSeed() uint32 { return c.meals.Seed() }

// SetMealSeed restores the persistent pseudo-random state for lumberjack meals.
func (c *Controller) SetMealSeed(seed uint32) { c.meals.SetSeed(seed) }

// NewLumberjack creates a worker at the hut's access point.
func NewLumberjack(home *building.Building) *Lumberjack {
	if home == nil {
		return &Lumberjack{}
	}
	p := home.AccessPoint()
	return &Lumberjack{Home: home, X: p.X, Y: p.Y, meal: resource.Bread}
}

// Spawn assigns one lumberjack to a newly built hut.
func (c *Controller) Spawn(home *building.Building) *Lumberjack {
	if home == nil || home.Kind != building.LumberjackHut {
		return nil
	}
	j := NewLumberjack(home)
	c.Lumberjacks = append(c.Lumberjacks, j)
	return j
}

// HasHome reports whether a lumberjack is already assigned to home.
func (c *Controller) HasHome(home *building.Building) bool {
	for _, j := range c.Lumberjacks {
		if j.Home == home {
			return true
		}
	}
	return false
}

// Restore recreates a lumberjack while preserving position, hunger, cargo,
// the selected meal and the current work state. Routes are rebuilt from the
// saved tile because transient path slices are intentionally not part of the
// JSON format. savedMeal is variadic so older callers that do not have the
// field can keep using the previous signature; new saves pass UnitState.Meal.
func (c *Controller) Restore(home *building.Building, x, y, hungerTicks int, starving bool, state State, target *building.Building, workTicks, cargo int, grid *world.Grid, buildings []*building.Building, savedMeal ...resource.Type) *Lumberjack {
	j := NewLumberjack(home)
	j.X, j.Y = x, y
	if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
		j.meal = savedMeal[0]
	}
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	if cargo < 0 {
		cargo = 0
	}
	j.hungerTick = hungerTicks
	j.Starving = starving
	j.workTicks = workTicks
	j.cargo = cargo

	switch state {
	case StateToTree:
		if target != nil && target.Kind == building.Tree && j.routeTo(grid, buildings, pathfind.Point{X: target.X, Y: target.Y}) {
			j.target = target
			j.state = StateToTree
		}
	case StateChopping:
		if target != nil && target.Kind == building.Tree {
			j.target = target
			j.state = StateChopping
		}
	case StateToHome, StateUnloading:
		if j.cargo > 0 {
			if state == StateUnloading && atAccessPoint(j, home) {
				j.state = StateUnloading
			} else {
				j.routeHome(grid, buildings)
			}
		}
	case StateToTavern:
		wanted := []resource.Type(nil)
		if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
			wanted = append(wanted, savedMeal[0])
		}
		if tavern, meal, path, ok := nearestTavernWithFood(grid, buildings, pathfind.Point{X: x, Y: y}, nil, &c.meals, wanted...); ok {
			j.setPath(path)
			j.tavern = tavern
			if len(savedMeal) == 0 || !resource.IsFood(savedMeal[0]) {
				j.meal = meal
			}
			j.state = StateToTavern
		}
	case StateToHomeAfterMeal:
		if j.routeHome(grid, buildings) {
			j.state = StateToHomeAfterMeal
		}
	}

	c.Lumberjacks = append(c.Lumberjacks, j)
	return j
}

// RemoveHome removes the lumberjack assigned to a deleted hut. A carried log
// is returned to the town stockpile so deleting a hut cannot destroy goods.
func (c *Controller) RemoveHome(home *building.Building, stock *resource.Stockpile) {
	kept := c.Lumberjacks[:0]
	for _, j := range c.Lumberjacks {
		if j.Home != home {
			kept = append(kept, j)
			continue
		}
		if stock != nil && j.cargo > 0 {
			stock.Add(resource.Log, j.cargo)
		}
	}
	c.Lumberjacks = kept
}

// CancelRouteTo resets any lumberjack currently walking toward target as a
// Tavern back to idle at its hut, instead of leaving it holding a
// dangling pointer to a building that's about to be removed from the
// world. Call this before deleting a building, in case it's a Tavern
// someone is mid-trip to eat at -- RemoveHome alone doesn't cover this,
// since a lumberjack's Home is its hut, never the Tavern it eats at. A
// no-op for anyone not currently walking toward target.
func (c *Controller) CancelRouteTo(target *building.Building) {
	for _, j := range c.Lumberjacks {
		if j.state != StateToTavern || j.tavern != target {
			continue
		}
		j.tavern = nil
		j.resetToIdle()
	}
}

// State reports what the lumberjack is doing.
func (j *Lumberjack) State() State { return j.state }

// HomeBuilding returns the assigned hut.
func (j *Lumberjack) HomeBuilding() *building.Building { return j.Home }

// TargetTree returns the current tree target, if one exists.
func (j *Lumberjack) TargetTree() *building.Building { return j.target }

// Cargo returns the number of Logs currently carried.
func (j *Lumberjack) Cargo() (resource.Type, int) { return resource.Log, j.cargo }

// Meal returns the food reserved for the current or next Tavern trip.
func (j *Lumberjack) Meal() resource.Type { return j.meal }

// HungerTicks returns simulation ticks since the last meal.
func (j *Lumberjack) HungerTicks() int { return j.hungerTick }

// SatietyPercent returns the player-facing 0-100 satiety value.
func (j *Lumberjack) SatietyPercent() int { return hunger.Percent(j.hungerTick) }

// WorkTicks returns progress through the current chopping animation.
func (j *Lumberjack) WorkTicks() int { return j.workTicks }

// AtPost reports whether the worker is inside/at the hut and available for a
// new assignment. It drives the hut's compact worker marker.
func (j *Lumberjack) AtPost() bool {
	return j.state == StateIdle || j.state == StateUnloading
}

// VisibleOnMap hides the worker while idle inside the hut, but shows him on
// the map while walking, chopping, eating, or returning with a log.
func (j *Lumberjack) VisibleOnMap() bool {
	return j.state != StateIdle && j.state != StateUnloading
}

// Reserve seeds ledger with every lumberjack currently walking to eat, so
// other controllers sharing a Tavern (serfs, villagers) see this claim
// before making their own commitments this tick. Call once per
// simulation tick, before this or any other controller's Tick runs.
func (c *Controller) Reserve(ledger *reservations.Ledger) {
	for _, j := range c.Lumberjacks {
		if j.state == StateToTavern && j.tavern != nil {
			ledger.ReservePickup(j.tavern, j.meal, 1)
		}
	}
}

// MaxWaitingHunger returns the highest HungerTicks among lumberjacks that
// will actually try to eat this tick (idle, not carrying a log, and
// HungerTicks >= HungerInterval), or -1 if none will. cmd/game compares
// this against the other unit controllers' MaxWaitingHunger to decide
// whose Tick runs first this simulation tick when the Tavern's food is
// scarce -- the unit that's been waiting longest gets first claim,
// instead of whichever controller happens to be first in a fixed call
// order.
//
// This only works because hungerTick keeps counting past HungerInterval
// instead of saturating there: once several units across different
// controllers are simultaneously overdue, a counter capped at
// HungerInterval would tie them all at the same value, and the ordering
// would silently fall back to the fixed call order it was built to
// replace.
func (c *Controller) MaxWaitingHunger() int {
	best := -1
	for _, j := range c.Lumberjacks {
		if j.state != StateIdle || j.cargo > 0 || j.hungerTick < HungerInterval {
			continue
		}
		if j.hungerTick > best {
			best = j.hungerTick
		}
	}
	return best
}

// Tick advances every lumberjack and returns completed tree-cut events.
// Call once per simulation tick, after every controller sharing ledger
// has had a chance to Reserve its own pre-existing in-flight units.
func (c *Controller) Tick(grid *world.Grid, buildings []*building.Building, ledger *reservations.Ledger) []Event {
	var events []Event
	remaining := c.Lumberjacks[:0]
	for _, j := range c.Lumberjacks {
		j.hungerTick++
		if hunger.Dead(j.hungerTick) {
			events = append(events, Event{Kind: WorkerDied, Cargo: j.cargo})
			continue
		}
		// Appended here, before the switch below, because that switch is
		// full of `continue` statements for the ordinary "still in
		// progress this tick" case (walking, chopping, ...) -- each of
		// those used to skip straight past a trailing append and silently
		// drop a perfectly alive worker from the roster.
		remaining = append(remaining, j)

		switch j.state {
		case StateIdle:
			if j.cargo > 0 {
				j.routeHome(grid, buildings)
				continue
			}
			if j.hungerTick >= HungerInterval && c.tryStartMeal(j, grid, buildings, ledger) {
				continue
			}
			c.startTreeJob(j, grid, buildings)
		case StateToTree:
			if !containsBuilding(buildings, j.target) || j.target.Kind != building.Tree {
				j.resetToIdle()
				continue
			}
			if j.advancePath() {
				j.state = StateChopping
				j.workTicks = 0
			}
		case StateChopping:
			if !containsBuilding(buildings, j.target) || j.target.Kind != building.Tree {
				j.resetToIdle()
				continue
			}
			j.workTicks++
			if j.workTicks < ChopTicks {
				continue
			}
			tree := j.target
			j.target = nil
			j.cargo = 1
			j.workTicks = 0
			j.state = StateToHome
			j.path = nil
			j.pathIdx = 0
			j.tileTicks = 0
			events = append(events, Event{Kind: TreeCut, Tree: tree})
		case StateToHome:
			if len(j.path) == 0 {
				j.routeHome(grid, buildings)
				continue
			}
			if j.advancePath() {
				j.state = StateUnloading
			}
		case StateUnloading:
			if j.cargo == 0 {
				j.resetToIdle()
				continue
			}
			if j.Home != nil && j.Home.AddOutput(resource.Log, j.cargo) == j.cargo {
				j.cargo = 0
				j.resetToIdle()
			}
		case StateToTavern:
			if len(j.path) == 0 {
				// Defensive fallback: normally unreachable, since
				// tryStartMeal/Restore only ever set this state together
				// with a non-empty path. If the path was somehow cleared
				// out from under the worker, try to re-route to the same
				// committed Tavern before giving up on the meal entirely.
				if j.tavern == nil || !j.routeToBuilding(grid, buildings, j.tavern, StateToTavern) {
					j.resetToIdle()
				}
				continue
			}
			if !j.advancePath() {
				continue
			}
			if j.tavern != nil && j.tavern.TakeInput(j.meal, 1) {
				j.hungerTick = 0
				j.Starving = false
			} else {
				j.Starving = true
			}
			j.tavern = nil
			j.state = StateToHomeAfterMeal
			j.path = nil
			j.pathIdx = 0
			j.tileTicks = 0
		case StateToHomeAfterMeal:
			if len(j.path) == 0 {
				if j.routeHome(grid, buildings) {
					j.state = StateToHomeAfterMeal
				}
				continue
			}
			if j.advancePath() {
				j.resetToIdle()
			}
		}
	}
	c.Lumberjacks = remaining
	return events
}

// nearestTavernWithFood returns the nearest reachable Tavern with food. The
// selector randomly chooses from its available menu; wanted restores an
// already-reserved saved meal without consuming another random choice.
func nearestTavernWithFood(grid *world.Grid, buildings []*building.Building, from pathfind.Point, ledger *reservations.Ledger, selector *meal.Selector, wanted ...resource.Type) (tavern *building.Building, selected resource.Type, path []pathfind.Point, ok bool) {
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
		p := b.AccessPoint()
		route, reachable := pathfind.FindLandPath(grid, buildings, from, pathfind.Point{X: p.X, Y: p.Y})
		if !reachable {
			continue
		}
		if bestLen == -1 || len(route) < bestLen {
			tavern, available, path, bestLen = b, foods, route, len(route)
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

func (c *Controller) tryStartMeal(j *Lumberjack, grid *world.Grid, buildings []*building.Building, ledger *reservations.Ledger) bool {
	tavern, meal, path, ok := nearestTavernWithFood(grid, buildings, pathfind.Point{X: j.X, Y: j.Y}, ledger, &c.meals)
	if !ok {
		j.Starving = true
		return false
	}
	j.Starving = false
	j.tavern = tavern
	j.meal = meal
	j.setPath(path)
	j.state = StateToTavern
	ledger.ReservePickup(tavern, meal, 1)
	return true
}

func (c *Controller) startTreeJob(j *Lumberjack, grid *world.Grid, buildings []*building.Building) {
	start := pathfind.Point{X: j.X, Y: j.Y}
	bestLength := int(^uint(0) >> 1)
	var bestTree *building.Building
	var bestPath []pathfind.Point
	for _, candidate := range buildings {
		if candidate == nil || candidate.Kind != building.Tree || candidate.GrowthStage() < 2 || c.treeReserved(candidate, j) {
			continue
		}
		path, ok := pathfind.FindLandPath(grid, buildings, start, pathfind.Point{X: candidate.X, Y: candidate.Y})
		if !ok || len(path) >= bestLength {
			continue
		}
		bestTree, bestPath, bestLength = candidate, path, len(path)
	}
	if bestTree == nil {
		return
	}
	j.target = bestTree
	j.setPath(bestPath)
	j.state = StateToTree
}

func (c *Controller) treeReserved(tree *building.Building, except *Lumberjack) bool {
	for _, j := range c.Lumberjacks {
		if j != except && j.target == tree && (j.state == StateToTree || j.state == StateChopping) {
			return true
		}
	}
	return false
}

func (j *Lumberjack) routeTo(grid *world.Grid, buildings []*building.Building, goal pathfind.Point) bool {
	path, ok := pathfind.FindLandPath(grid, buildings, pathfind.Point{X: j.X, Y: j.Y}, goal)
	if !ok {
		return false
	}
	j.setPath(path)
	return true
}

func (j *Lumberjack) routeToBuilding(grid *world.Grid, buildings []*building.Building, target *building.Building, state State) bool {
	if target == nil {
		return false
	}
	p := target.AccessPoint()
	if !j.routeTo(grid, buildings, pathfind.Point{X: p.X, Y: p.Y}) {
		return false
	}
	j.state = state
	return true
}

func (j *Lumberjack) routeHome(grid *world.Grid, buildings []*building.Building) bool {
	if j.Home == nil {
		return false
	}
	p := j.Home.AccessPoint()
	if !j.routeTo(grid, buildings, pathfind.Point{X: p.X, Y: p.Y}) {
		return false
	}
	j.state = StateToHome
	return true
}

func (j *Lumberjack) setPath(path []pathfind.Point) {
	j.path = path
	j.pathIdx = 0
	j.tileTicks = 0
}

func (j *Lumberjack) advancePath() bool {
	if len(j.path) == 0 {
		return true
	}
	j.tileTicks++
	if j.tileTicks < TicksPerTile {
		return false
	}
	j.tileTicks = 0
	if j.pathIdx < len(j.path)-1 {
		j.pathIdx++
		j.X, j.Y = j.path[j.pathIdx].X, j.path[j.pathIdx].Y
		return false
	}
	return true
}

func (j *Lumberjack) resetToIdle() {
	if j.Home != nil {
		p := j.Home.AccessPoint()
		j.X, j.Y = p.X, p.Y
	}
	j.state = StateIdle
	j.target = nil
	j.path = nil
	j.pathIdx = 0
	j.tileTicks = 0
	j.workTicks = 0
}

func atAccessPoint(j *Lumberjack, home *building.Building) bool {
	if j == nil || home == nil {
		return false
	}
	p := home.AccessPoint()
	return j.X == p.X && j.Y == p.Y
}

func containsBuilding(buildings []*building.Building, target *building.Building) bool {
	if target == nil {
		return false
	}
	for _, b := range buildings {
		if b == target {
			return true
		}
	}
	return false
}
