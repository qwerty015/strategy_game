// Package quarry simulates the worker assigned to a Quarry Hut. Like a
// lumberjack, a quarryman is not restricted to the road network: he walks
// over every non-water tile, finds the nearest stone deposit with reserve
// left, mines one unit of raw stone, and carries it back to his hut, where
// it becomes two finished Stone Blocks for collection by a serf.
package quarry

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
	// between jobs and never interrupts a mining pass or a return trip.
	HungerInterval = hunger.MealThresholdTicks
	TicksPerTile   = 2

	// MineTicks is six seconds at normal speed (two simulation ticks/sec),
	// the same pace as a lumberjack's ChopTicks.
	MineTicks = 12
)

// State is the visible activity state of a quarryman.
type State int

const (
	StateIdle State = iota
	StateToDeposit
	StateMining
	StateToHome
	StateUnloading
	StateToTavern
	StateToHomeAfterMeal
)

// EventKind identifies a world change emitted by the controller.
type EventKind int

const (
	// DepositExhausted fires once a deposit's Reserve reaches zero. The game
	// layer owns the building slice, so it removes the object -- the tile
	// underneath is already ordinary buildable ground, exactly like the
	// space left behind by a felled tree.
	DepositExhausted EventKind = iota
	WorkerDied
)

// Event is returned after a worker finishes a world interaction.
type Event struct {
	Kind    EventKind
	Deposit *building.Building
	Cargo   int
}

// Quarryman is one physical worker assigned to a Quarry Hut.
type Quarryman struct {
	Home *building.Building
	X, Y int

	state      State
	target     *building.Building // the stone deposit currently being walked to or mined
	tavern     *building.Building
	meal       resource.Type
	path       []pathfind.Point
	pathIdx    int
	tileTicks  int
	workTicks  int
	cargo      int // 0 or 1 raw stone, converted to 2 Stone Blocks on unload
	hungerTick int

	// Starving is true when the worker needs food but currently cannot reach a
	// stocked Tavern. The worker keeps performing the current work loop so a
	// missing Tavern does not deadlock the quarry.
	Starving bool
}

// Controller owns all quarrymen in the settlement.
type Controller struct {
	Quarrymen []*Quarryman
	meals     meal.Selector
}

// NewController creates an empty quarryman roster.
func NewController() *Controller {
	return &Controller{meals: meal.NewSelector(0x2f4a2b6d)}
}

// MealSeed returns the persistent pseudo-random state for quarryman meals.
func (c *Controller) MealSeed() uint32 { return c.meals.Seed() }

// SetMealSeed restores the persistent pseudo-random state for quarryman meals.
func (c *Controller) SetMealSeed(seed uint32) { c.meals.SetSeed(seed) }

// NewQuarryman creates a worker at the hut's access point.
func NewQuarryman(home *building.Building) *Quarryman {
	if home == nil {
		return &Quarryman{}
	}
	p := home.AccessPoint()
	return &Quarryman{Home: home, X: p.X, Y: p.Y, meal: resource.Bread}
}

// Spawn assigns one quarryman to a newly built hut.
func (c *Controller) Spawn(home *building.Building) *Quarryman {
	if home == nil || home.Kind != building.QuarryHut {
		return nil
	}
	q := NewQuarryman(home)
	c.Quarrymen = append(c.Quarrymen, q)
	return q
}

// HasHome reports whether a quarryman is already assigned to home.
func (c *Controller) HasHome(home *building.Building) bool {
	for _, q := range c.Quarrymen {
		if q.Home == home {
			return true
		}
	}
	return false
}

// Restore recreates a quarryman while preserving position, hunger, cargo,
// the selected meal and the current work state. Routes are rebuilt from the
// saved tile because transient path slices are intentionally not part of the
// JSON format.
func (c *Controller) Restore(home *building.Building, x, y, hungerTicks int, starving bool, state State, target *building.Building, workTicks, cargo int, grid *world.Grid, buildings []*building.Building, savedMeal ...resource.Type) *Quarryman {
	q := NewQuarryman(home)
	q.X, q.Y = x, y
	if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
		q.meal = savedMeal[0]
	}
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	if cargo < 0 {
		cargo = 0
	}
	q.hungerTick = hungerTicks
	q.Starving = starving
	q.workTicks = workTicks
	q.cargo = cargo

	switch state {
	case StateToDeposit:
		if target != nil && target.Kind == building.StoneDeposit && q.routeTo(grid, buildings, pathfind.Point{X: target.X, Y: target.Y}) {
			q.target = target
			q.state = StateToDeposit
		}
	case StateMining:
		if target != nil && target.Kind == building.StoneDeposit {
			q.target = target
			q.state = StateMining
		}
	case StateToHome, StateUnloading:
		if q.cargo > 0 {
			if state == StateUnloading && atAccessPoint(q, home) {
				q.state = StateUnloading
			} else {
				q.routeHome(grid, buildings)
			}
		}
	case StateToTavern:
		wanted := []resource.Type(nil)
		if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
			wanted = append(wanted, savedMeal[0])
		}
		if tavern, meal, path, ok := nearestTavernWithFood(grid, buildings, pathfind.Point{X: x, Y: y}, nil, &c.meals, wanted...); ok {
			q.setPath(path)
			q.tavern = tavern
			if len(savedMeal) == 0 || !resource.IsFood(savedMeal[0]) {
				q.meal = meal
			}
			q.state = StateToTavern
		}
	case StateToHomeAfterMeal:
		if q.routeHome(grid, buildings) {
			q.state = StateToHomeAfterMeal
		}
	}

	c.Quarrymen = append(c.Quarrymen, q)
	return q
}

// RemoveHome removes the quarryman assigned to a deleted hut. Carried stone
// is returned to the town stockpile as finished Stone Blocks so deleting a
// hut cannot destroy goods.
func (c *Controller) RemoveHome(home *building.Building, stock *resource.Stockpile) {
	kept := c.Quarrymen[:0]
	for _, q := range c.Quarrymen {
		if q.Home != home {
			kept = append(kept, q)
			continue
		}
		if stock != nil && q.cargo > 0 {
			stock.Add(resource.StoneBlock, q.cargo*2)
		}
	}
	c.Quarrymen = kept
}

// CancelRouteTo resets any quarryman currently walking toward target as a
// Tavern back to idle at its hut, instead of leaving it holding a dangling
// pointer to a building that's about to be removed from the world.
func (c *Controller) CancelRouteTo(target *building.Building) {
	for _, q := range c.Quarrymen {
		if q.state != StateToTavern || q.tavern != target {
			continue
		}
		q.tavern = nil
		q.resetToIdle()
	}
}

// State reports what the quarryman is doing.
func (q *Quarryman) State() State { return q.state }

// HomeBuilding returns the assigned hut.
func (q *Quarryman) HomeBuilding() *building.Building { return q.Home }

// TargetDeposit returns the current stone-deposit target, if one exists.
func (q *Quarryman) TargetDeposit() *building.Building { return q.target }

// Cargo returns the number of Stone Blocks currently carried (0 or 2 --
// mining always yields one raw unit internally, reported here already
// converted since StoneBlock is the only unit that ever reaches a buffer or
// the player-facing UI).
func (q *Quarryman) Cargo() (resource.Type, int) { return resource.StoneBlock, q.cargo * 2 }

// RawCargo returns the internal pre-conversion cargo count (0 or 1). Save
// serialization uses this, not Cargo, because Restore's cargo parameter
// reconstructs the same internal field -- passing the already-doubled
// amount back in would double the conversion on the next unload.
func (q *Quarryman) RawCargo() int { return q.cargo }

// Meal returns the food reserved for the current or next Tavern trip.
func (q *Quarryman) Meal() resource.Type { return q.meal }

// HungerTicks returns simulation ticks since the last meal.
func (q *Quarryman) HungerTicks() int { return q.hungerTick }

// SatietyPercent returns the player-facing 0-100 satiety value.
func (q *Quarryman) SatietyPercent() int { return hunger.Percent(q.hungerTick) }

// WorkTicks returns progress through the current mining animation.
func (q *Quarryman) WorkTicks() int { return q.workTicks }

// AtPost reports whether the worker is inside/at the hut and available for a
// new assignment. It drives the hut's compact worker marker.
func (q *Quarryman) AtPost() bool {
	return q.state == StateIdle || q.state == StateUnloading
}

// VisibleOnMap hides the worker while idle inside the hut, but shows him on
// the map while walking, mining, eating, or returning with stone.
func (q *Quarryman) VisibleOnMap() bool {
	return q.state != StateIdle && q.state != StateUnloading
}

// Reserve seeds ledger with every quarryman currently walking to eat, so
// other controllers sharing a Tavern see this claim before making their own
// commitments this tick. Call once per simulation tick, before this or any
// other controller's Tick runs.
func (c *Controller) Reserve(ledger *reservations.Ledger) {
	for _, q := range c.Quarrymen {
		if q.state == StateToTavern && q.tavern != nil {
			ledger.ReservePickup(q.tavern, q.meal, 1)
		}
	}
}

// MaxWaitingHunger returns the highest HungerTicks among quarrymen that will
// actually try to eat this tick (idle, not carrying stone, and HungerTicks
// >= HungerInterval), or -1 if none will. See lumberjack.Controller's
// method of the same name for why this ordering matters when Tavern food is
// scarce.
func (c *Controller) MaxWaitingHunger() int {
	best := -1
	for _, q := range c.Quarrymen {
		if q.state != StateIdle || q.cargo > 0 || q.hungerTick < HungerInterval {
			continue
		}
		if q.hungerTick > best {
			best = q.hungerTick
		}
	}
	return best
}

// Tick advances every quarryman and returns completed events. Call once per
// simulation tick, after every controller sharing ledger has had a chance to
// Reserve its own pre-existing in-flight units.
func (c *Controller) Tick(grid *world.Grid, buildings []*building.Building, ledger *reservations.Ledger) []Event {
	var events []Event
	remaining := c.Quarrymen[:0]
	for _, q := range c.Quarrymen {
		q.hungerTick++
		if hunger.Dead(q.hungerTick) {
			// Cargo is reported already converted to Stone Blocks, like
			// every other cargo accessor on Quarryman -- see Cargo's doc
			// comment.
			events = append(events, Event{Kind: WorkerDied, Cargo: q.cargo * 2})
			continue
		}
		remaining = append(remaining, q)

		switch q.state {
		case StateIdle:
			if q.cargo > 0 {
				q.routeHome(grid, buildings)
				continue
			}
			if q.hungerTick >= HungerInterval && c.tryStartMeal(q, grid, buildings, ledger) {
				continue
			}
			c.startDepositJob(q, grid, buildings)
		case StateToDeposit:
			if !depositUsable(buildings, q.target) {
				q.resetToIdle()
				continue
			}
			if q.advancePath() {
				q.state = StateMining
				q.workTicks = 0
			}
		case StateMining:
			if !depositUsable(buildings, q.target) {
				q.resetToIdle()
				continue
			}
			q.workTicks++
			if q.workTicks < MineTicks {
				continue
			}
			deposit := q.target
			q.target = nil
			q.cargo = 1
			q.workTicks = 0
			q.state = StateToHome
			q.path = nil
			q.pathIdx = 0
			q.tileTicks = 0
			deposit.Reserve--
			if deposit.Reserve <= 0 {
				events = append(events, Event{Kind: DepositExhausted, Deposit: deposit})
			}
		case StateToHome:
			if len(q.path) == 0 {
				q.routeHome(grid, buildings)
				continue
			}
			if q.advancePath() {
				q.state = StateUnloading
			}
		case StateUnloading:
			if q.cargo == 0 {
				q.resetToIdle()
				continue
			}
			// One raw unit of mined stone becomes two finished Stone Blocks
			// the moment it reaches the hut -- the quarryman is both miner
			// and stonecutter, so there is no separate processing building.
			blocks := q.cargo * 2
			if q.Home != nil && q.Home.AddOutput(resource.StoneBlock, blocks) == blocks {
				q.cargo = 0
				q.resetToIdle()
			}
		case StateToTavern:
			if len(q.path) == 0 {
				if q.tavern == nil || !q.routeToBuilding(grid, buildings, q.tavern, StateToTavern) {
					q.resetToIdle()
				}
				continue
			}
			if !q.advancePath() {
				continue
			}
			if q.tavern != nil && q.tavern.TakeInput(q.meal, 1) {
				q.hungerTick = 0
				q.Starving = false
			} else {
				q.Starving = true
			}
			q.tavern = nil
			q.state = StateToHomeAfterMeal
			q.path = nil
			q.pathIdx = 0
			q.tileTicks = 0
		case StateToHomeAfterMeal:
			if len(q.path) == 0 {
				if q.routeHome(grid, buildings) {
					q.state = StateToHomeAfterMeal
				}
				continue
			}
			if q.advancePath() {
				q.resetToIdle()
			}
		}
	}
	c.Quarrymen = remaining
	return events
}

// depositUsable reports whether target is still a live stone deposit with
// reserve left to mine -- it may have been exhausted (and removed from the
// world) by another quarryman between this worker's ticks.
func depositUsable(buildings []*building.Building, target *building.Building) bool {
	if target == nil || target.Kind != building.StoneDeposit || target.Reserve <= 0 {
		return false
	}
	return containsBuilding(buildings, target)
}

// nearestTavernWithFood mirrors lumberjack's helper of the same name.
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

func (c *Controller) tryStartMeal(q *Quarryman, grid *world.Grid, buildings []*building.Building, ledger *reservations.Ledger) bool {
	tavern, meal, path, ok := nearestTavernWithFood(grid, buildings, pathfind.Point{X: q.X, Y: q.Y}, ledger, &c.meals)
	if !ok {
		q.Starving = true
		return false
	}
	q.Starving = false
	q.tavern = tavern
	q.meal = meal
	q.setPath(path)
	q.state = StateToTavern
	ledger.ReservePickup(tavern, meal, 1)
	return true
}

// startDepositJob finds the nearest reachable stone deposit with reserve
// left. Unlike a tree, a deposit holds thousands of units, so several
// quarrymen may legitimately head for the same one at once -- there is no
// equivalent of the lumberjack's single-claim tree reservation.
func (c *Controller) startDepositJob(q *Quarryman, grid *world.Grid, buildings []*building.Building) {
	start := pathfind.Point{X: q.X, Y: q.Y}
	bestLength := int(^uint(0) >> 1)
	var bestDeposit *building.Building
	var bestPath []pathfind.Point
	for _, candidate := range buildings {
		if candidate == nil || candidate.Kind != building.StoneDeposit || candidate.Reserve <= 0 {
			continue
		}
		path, ok := pathfind.FindLandPath(grid, buildings, start, pathfind.Point{X: candidate.X, Y: candidate.Y})
		if !ok || len(path) >= bestLength {
			continue
		}
		bestDeposit, bestPath, bestLength = candidate, path, len(path)
	}
	if bestDeposit == nil {
		return
	}
	q.target = bestDeposit
	q.setPath(bestPath)
	q.state = StateToDeposit
}

func (q *Quarryman) routeTo(grid *world.Grid, buildings []*building.Building, goal pathfind.Point) bool {
	path, ok := pathfind.FindLandPath(grid, buildings, pathfind.Point{X: q.X, Y: q.Y}, goal)
	if !ok {
		return false
	}
	q.setPath(path)
	return true
}

func (q *Quarryman) routeToBuilding(grid *world.Grid, buildings []*building.Building, target *building.Building, state State) bool {
	if target == nil {
		return false
	}
	p := target.AccessPoint()
	if !q.routeTo(grid, buildings, pathfind.Point{X: p.X, Y: p.Y}) {
		return false
	}
	q.state = state
	return true
}

func (q *Quarryman) routeHome(grid *world.Grid, buildings []*building.Building) bool {
	if q.Home == nil {
		return false
	}
	p := q.Home.AccessPoint()
	if !q.routeTo(grid, buildings, pathfind.Point{X: p.X, Y: p.Y}) {
		return false
	}
	q.state = StateToHome
	return true
}

func (q *Quarryman) setPath(path []pathfind.Point) {
	q.path = path
	q.pathIdx = 0
	q.tileTicks = 0
}

func (q *Quarryman) advancePath() bool {
	if len(q.path) == 0 {
		return true
	}
	q.tileTicks++
	if q.tileTicks < TicksPerTile {
		return false
	}
	q.tileTicks = 0
	if q.pathIdx < len(q.path)-1 {
		q.pathIdx++
		q.X, q.Y = q.path[q.pathIdx].X, q.path[q.pathIdx].Y
		return false
	}
	return true
}

func (q *Quarryman) resetToIdle() {
	if q.Home != nil {
		p := q.Home.AccessPoint()
		q.X, q.Y = p.X, p.Y
	}
	q.state = StateIdle
	q.target = nil
	q.path = nil
	q.pathIdx = 0
	q.tileTicks = 0
	q.workTicks = 0
}

func atAccessPoint(q *Quarryman, home *building.Building) bool {
	if q == nil || home == nil {
		return false
	}
	p := home.AccessPoint()
	return q.X == p.X && q.Y == p.Y
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
