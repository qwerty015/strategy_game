// Package miner simulates the worker assigned to a Miner Hut. Like a
// quarryman, a miner is not restricted to the road network: he walks over
// every non-water tile, mines a deposit, and carries the raw material home
// for collection by a serf -- unlike a quarryman, nothing is processed at
// the hut (that's the Smeltery's job for the two ores; Coal needs no
// further processing).
//
// Unlike every other gathering profession, a miner works three different
// deposit kinds (Coal, GoldOre, IronOre), not one. Always heading for
// whichever deposit is nearest would mean whichever resource happens to
// have the closest cell wins forever, starving the other two completely.
// Instead each miner follows a fixed round-robin quota (DefaultQuota): 1
// GoldOre, then 1 IronOre, then 3 Coal, then back to GoldOre -- so the
// three are gathered in a roughly fixed ratio regardless of which deposits
// happen to be closest. If the quota's current resource has nothing
// reachable right now (exhausted, or cut off), the miner tries the next
// entries in the cycle instead of standing idle while the other two sit
// untouched.
package miner

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

	// MineTicks is six seconds at normal speed, the same pace as a
	// lumberjack's ChopTicks/quarryman's MineTicks.
	MineTicks = 12

	// MaxWorkRadius mirrors lumberjack.MaxWorkRadius -- see its doc
	// comment for the full derivation from the hunger-budget math. Real
	// bug this guards against: user report of miners dying from distance
	// (map generation deliberately keeps every deposit at least
	// minDepositDistanceFromWarehouse away from the Warehouse, which made
	// this the most exposed profession of the three), since hunger is
	// never checked mid-walk to/from a deposit.
	MaxWorkRadius = 45
)

// Quota is one entry in DefaultQuota: mine Amount units of Resource (from
// Deposit-kind deposits) before moving on to the next entry.
type Quota struct {
	Deposit  building.Kind
	Resource resource.Type
	Amount   int
}

// DefaultQuota is the fixed cycle every miner follows, per the game
// design: gold and iron ore are rarer and wanted in smaller, steady
// amounts; coal is the most abundant deposit and feeds both smelting
// recipes, so it's gathered three units at a time.
var DefaultQuota = []Quota{
	{Deposit: building.GoldOreDeposit, Resource: resource.GoldOre, Amount: 1},
	{Deposit: building.IronOreDeposit, Resource: resource.IronOre, Amount: 1},
	{Deposit: building.CoalDeposit, Resource: resource.Coal, Amount: 3},
}

// State is the visible activity state of a miner.
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
	// DepositExhausted fires once a deposit's Reserve reaches zero. The
	// game layer owns the building slice, so it removes the object -- the
	// tile underneath is already ordinary buildable ground, exactly like a
	// mined-out StoneDeposit.
	DepositExhausted EventKind = iota
	WorkerDied
)

// Event is returned after a worker finishes a world interaction.
type Event struct {
	Kind    EventKind
	Deposit *building.Building
	Cargo   int
	// CargoResource identifies what Cargo counts (WorkerDied only -- a
	// miner can be carrying any of Coal/GoldOre/IronOre when hunger
	// catches up with him, unlike a quarryman's single fixed StoneBlock).
	CargoResource resource.Type
}

// Miner is one physical worker assigned to a Miner Hut.
type Miner struct {
	Home *building.Building
	X, Y int

	state      State
	target     *building.Building // the deposit currently being walked to or mined
	tavern     *building.Building
	meal       resource.Type
	path       []pathfind.Point
	pathIdx    int
	tileTicks  int
	workTicks  int
	hungerTick int

	// cargoResource is captured the moment mining finishes, from whichever
	// quota entry was active at the time -- not re-derived from the quota
	// at unload time, since the quota can advance while cargo is still in
	// hand (e.g. hunger interrupts the trip home). cargo is 0 or 1.
	cargoResource resource.Type
	cargo         int

	// quotaIndex/quotaProgress track this miner's own position in
	// DefaultQuota -- each miner cycles independently, the same way each
	// serf picks its own jobs independently.
	quotaIndex    int
	quotaProgress int

	// Starving is true when the worker needs food but currently cannot reach a
	// stocked Tavern. The worker keeps performing the current work loop so a
	// missing Tavern does not deadlock the mine.
	Starving bool
}

// Controller owns all miners in the settlement.
type Controller struct {
	Miners []*Miner
	meals  meal.Selector
}

// NewController creates an empty miner roster.
func NewController() *Controller {
	return &Controller{meals: meal.NewSelector(0x9e3779b1)}
}

// MealSeed returns the persistent pseudo-random state for miner meals.
func (c *Controller) MealSeed() uint32 { return c.meals.Seed() }

// SetMealSeed restores the persistent pseudo-random state for miner meals.
func (c *Controller) SetMealSeed(seed uint32) { c.meals.SetSeed(seed) }

// NewMiner creates a worker at the hut's access point.
func NewMiner(home *building.Building) *Miner {
	if home == nil {
		return &Miner{}
	}
	p := home.AccessPoint()
	return &Miner{Home: home, X: p.X, Y: p.Y, meal: resource.Bread}
}

// Spawn assigns one miner to a newly built hut.
func (c *Controller) Spawn(home *building.Building) *Miner {
	if home == nil || home.Kind != building.MinerHut {
		return nil
	}
	m := NewMiner(home)
	c.Miners = append(c.Miners, m)
	return m
}

// HasHome reports whether a miner is already assigned to home.
func (c *Controller) HasHome(home *building.Building) bool {
	for _, m := range c.Miners {
		if m.Home == home {
			return true
		}
	}
	return false
}

// Restore recreates a miner while preserving position, hunger, cargo, the
// selected meal, the current work state and quota progress. Routes are
// rebuilt from the saved tile because transient path slices are
// intentionally not part of the JSON format.
func (c *Controller) Restore(home *building.Building, x, y, hungerTicks int, starving bool, state State, target *building.Building, workTicks, cargoAmount int, cargoResource resource.Type, quotaIndex, quotaProgress int, grid *world.Grid, buildings []*building.Building, savedMeal ...resource.Type) *Miner {
	m := NewMiner(home)
	m.X, m.Y = x, y
	if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
		m.meal = savedMeal[0]
	}
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	if cargoAmount < 0 {
		cargoAmount = 0
	}
	m.hungerTick = hungerTicks
	m.Starving = starving
	m.workTicks = workTicks
	m.cargo = cargoAmount
	m.cargoResource = cargoResource
	if quotaIndex < 0 || quotaIndex >= len(DefaultQuota) {
		quotaIndex = 0
	}
	m.quotaIndex = quotaIndex
	if quotaProgress < 0 {
		quotaProgress = 0
	}
	m.quotaProgress = quotaProgress

	switch state {
	case StateToDeposit:
		if target != nil && isDeposit(target.Kind) && m.routeTo(grid, buildings, pathfind.Point{X: target.X, Y: target.Y}) {
			m.target = target
			m.state = StateToDeposit
		}
	case StateMining:
		if target != nil && isDeposit(target.Kind) {
			m.target = target
			m.state = StateMining
		}
	case StateToHome, StateUnloading:
		if m.cargo > 0 {
			if state == StateUnloading && atAccessPoint(m, home) {
				m.state = StateUnloading
			} else {
				m.routeHome(grid, buildings)
			}
		}
	case StateToTavern:
		wanted := []resource.Type(nil)
		if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
			wanted = append(wanted, savedMeal[0])
		}
		if tavern, meal, path, ok := nearestTavernWithFood(grid, buildings, pathfind.Point{X: x, Y: y}, nil, &c.meals, wanted...); ok {
			m.setPath(path)
			m.tavern = tavern
			if len(savedMeal) == 0 || !resource.IsFood(savedMeal[0]) {
				m.meal = meal
			}
			m.state = StateToTavern
		}
	case StateToHomeAfterMeal:
		if m.routeHome(grid, buildings) {
			m.state = StateToHomeAfterMeal
		}
	}

	c.Miners = append(c.Miners, m)
	return m
}

// RemoveHome removes the miner assigned to a deleted hut. Carried cargo is
// returned to the town stockpile so deleting a hut cannot destroy goods.
func (c *Controller) RemoveHome(home *building.Building, stock *resource.Stockpile) {
	kept := c.Miners[:0]
	for _, m := range c.Miners {
		if m.Home != home {
			kept = append(kept, m)
			continue
		}
		if stock != nil && m.cargo > 0 {
			stock.Add(m.cargoResource, m.cargo)
		}
	}
	c.Miners = kept
}

// CancelRouteTo resets any miner currently walking toward target as a
// Tavern back to idle at its hut, instead of leaving it holding a dangling
// pointer to a building that's about to be removed from the world. A
// no-op for anyone not currently walking toward target.
func (c *Controller) CancelRouteTo(target *building.Building) {
	for _, m := range c.Miners {
		if m.state != StateToTavern || m.tavern != target {
			continue
		}
		m.tavern = nil
		m.resetToIdle()
	}
}

// State reports what the miner is doing.
func (m *Miner) State() State { return m.state }

// HomeBuilding returns the assigned hut.
func (m *Miner) HomeBuilding() *building.Building { return m.Home }

// TargetDeposit returns the current deposit target, if one exists.
func (m *Miner) TargetDeposit() *building.Building { return m.target }

// Cargo returns the resource and amount currently carried (0 or 1).
func (m *Miner) Cargo() (resource.Type, int) { return m.cargoResource, m.cargo }

// Meal returns the food reserved for the current or next Tavern trip.
func (m *Miner) Meal() resource.Type { return m.meal }

// HungerTicks returns simulation ticks since the last meal.
func (m *Miner) HungerTicks() int { return m.hungerTick }

// SatietyPercent returns the player-facing 0-100 satiety value.
func (m *Miner) SatietyPercent() int { return hunger.Percent(m.hungerTick) }

// RemainingPath returns the tiles still ahead on the miner's current
// route, starting from (and including) the tile it's walking toward right
// now -- for the inspector's route-line overlay (see
// ui.DrawSelectedRoute). nil once idle or with no path assigned.
func (m *Miner) RemainingPath() []pathfind.Point {
	if m.pathIdx >= len(m.path) {
		return nil
	}
	return m.path[m.pathIdx:]
}

// WorkTicks returns progress through the current mining animation.
func (m *Miner) WorkTicks() int { return m.workTicks }

// QuotaProgress reports which DefaultQuota entry this miner is currently
// working toward, and how many units into it he already is -- for the
// inspector.
func (m *Miner) QuotaProgress() (index, progress int) { return m.quotaIndex, m.quotaProgress }

// AtPost reports whether the worker is inside/at the hut and available for a
// new assignment. It drives the hut's compact worker marker.
func (m *Miner) AtPost() bool {
	return m.state == StateIdle || m.state == StateUnloading
}

// VisibleOnMap hides the worker while idle inside the hut, but shows him on
// the map while walking, mining, eating, or returning with cargo.
func (m *Miner) VisibleOnMap() bool {
	return m.state != StateIdle && m.state != StateUnloading
}

// Reserve seeds ledger with every miner currently walking to eat, so other
// controllers sharing a Tavern see this claim before making their own
// commitments this tick. Call once per simulation tick, before this or any
// other controller's Tick runs.
func (c *Controller) Reserve(ledger *reservations.Ledger) {
	for _, m := range c.Miners {
		if m.state == StateToTavern && m.tavern != nil {
			ledger.ReservePickup(m.tavern, m.meal, 1)
		}
	}
}

// MaxWaitingHunger returns the highest HungerTicks among miners that will
// actually try to eat this tick (idle, not carrying cargo, and HungerTicks
// >= HungerInterval), or -1 if none will. See lumberjack.Controller's
// method of the same name for why this ordering matters when Tavern food
// is scarce.
func (c *Controller) MaxWaitingHunger() int {
	best := -1
	for _, m := range c.Miners {
		if m.state != StateIdle || m.cargo > 0 || m.hungerTick < HungerInterval {
			continue
		}
		if m.hungerTick > best {
			best = m.hungerTick
		}
	}
	return best
}

// Tick advances every miner and returns completed events. Call once per
// simulation tick, after every controller sharing ledger has had a chance
// to Reserve its own pre-existing in-flight units.
func (c *Controller) Tick(grid *world.Grid, buildings []*building.Building, ledger *reservations.Ledger) []Event {
	var events []Event
	remaining := c.Miners[:0]
	for _, m := range c.Miners {
		m.hungerTick++
		if hunger.Dead(m.hungerTick) {
			events = append(events, Event{Kind: WorkerDied, Cargo: m.cargo, CargoResource: m.cargoResource})
			continue
		}
		remaining = append(remaining, m)

		switch m.state {
		case StateIdle:
			// Unlike lumberjack/quarry, this branch used to be unreachable
			// with cargo > 0 -- StateUnloading never let go of a miner
			// while it was still carrying, so there was nothing to resume
			// here. Now that StateUnloading can also interrupt for a meal
			// (see below) and return here with cargo still in hand, this
			// must be checked first, the same way lumberjack/quarry
			// already do -- otherwise startDepositJob would send the miner
			// off on a fresh job while quietly overwriting the
			// undelivered cargo the next time it mines.
			if m.cargo > 0 {
				m.routeHome(grid, buildings)
				continue
			}
			if m.hungerTick >= HungerInterval && c.tryStartMeal(m, grid, buildings, ledger) {
				continue
			}
			c.startDepositJob(m, grid, buildings)
		case StateToDeposit:
			if !depositUsable(buildings, m.target) {
				m.resetToIdle()
				continue
			}
			if m.advancePath() {
				m.state = StateMining
				m.workTicks = 0
			}
		case StateMining:
			if !depositUsable(buildings, m.target) {
				m.resetToIdle()
				continue
			}
			m.workTicks++
			if m.workTicks < MineTicks {
				continue
			}
			deposit := m.target
			m.target = nil
			m.cargo = 1
			m.workTicks = 0
			m.state = StateToHome
			m.path = nil
			m.pathIdx = 0
			m.tileTicks = 0
			deposit.Reserve--
			if deposit.Reserve <= 0 {
				events = append(events, Event{Kind: DepositExhausted, Deposit: deposit})
			}
		case StateToHome:
			if len(m.path) == 0 {
				m.routeHome(grid, buildings)
				continue
			}
			if m.advancePath() {
				m.state = StateUnloading
			}
		case StateUnloading:
			if m.cargo == 0 {
				m.resetToIdle()
				continue
			}
			if m.Home != nil && m.Home.AddOutput(m.cargoResource, m.cargo) == m.cargo {
				c.advanceQuota(m)
				m.cargo = 0
				m.resetToIdle()
				continue
			}
			// Real bug this fixes: if the hut's OutputBuffer is full (no
			// serf has collected it yet), this wait has no upper bound --
			// hunger was never checked here, only at StateIdle. cargo is
			// untouched by tryStartMeal and survives resetToIdle, so after
			// eating, StateIdle's own "cargo > 0 -> routeHome" branch
			// naturally resumes trying to unload, with nothing lost.
			if m.hungerTick >= HungerInterval && c.tryStartMeal(m, grid, buildings, ledger) {
				continue
			}
		case StateToTavern:
			if len(m.path) == 0 {
				if m.tavern == nil || !m.routeToBuilding(grid, buildings, m.tavern, StateToTavern) {
					m.resetToIdle()
				}
				continue
			}
			if !m.advancePath() {
				continue
			}
			if m.tavern != nil && m.tavern.TakeInput(m.meal, 1) {
				m.hungerTick = 0
				m.Starving = false
			} else {
				m.Starving = true
			}
			m.tavern = nil
			m.state = StateToHomeAfterMeal
			m.path = nil
			m.pathIdx = 0
			m.tileTicks = 0
		case StateToHomeAfterMeal:
			if len(m.path) == 0 {
				if m.routeHome(grid, buildings) {
					m.state = StateToHomeAfterMeal
				}
				continue
			}
			if m.advancePath() {
				m.resetToIdle()
			}
		}
	}
	c.Miners = remaining
	return events
}

// advanceQuota moves a miner's quota position forward by the one unit it
// just delivered, rolling over to the next DefaultQuota entry (and back to
// the start) once the current entry's Amount is reached.
func (c *Controller) advanceQuota(m *Miner) {
	m.quotaProgress++
	if m.quotaProgress >= DefaultQuota[m.quotaIndex].Amount {
		m.quotaIndex = (m.quotaIndex + 1) % len(DefaultQuota)
		m.quotaProgress = 0
	}
}

func isDeposit(kind building.Kind) bool {
	switch kind {
	case building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
		return true
	default:
		return false
	}
}

// depositUsable reports whether target is still a live, unexhausted
// deposit -- it may have been mined out (and removed from the world) by
// another miner between this worker's ticks.
func depositUsable(buildings []*building.Building, target *building.Building) bool {
	if target == nil || !isDeposit(target.Kind) || target.Reserve <= 0 {
		return false
	}
	return containsBuilding(buildings, target)
}

// nearestTavernWithFood mirrors quarry's helper of the same name.
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

func (c *Controller) tryStartMeal(m *Miner, grid *world.Grid, buildings []*building.Building, ledger *reservations.Ledger) bool {
	tavern, meal, path, ok := nearestTavernWithFood(grid, buildings, pathfind.Point{X: m.X, Y: m.Y}, ledger, &c.meals)
	if !ok {
		m.Starving = true
		return false
	}
	m.Starving = false
	m.tavern = tavern
	m.meal = meal
	m.setPath(path)
	m.state = StateToTavern
	ledger.ReservePickup(tavern, meal, 1)
	return true
}

// startDepositJob finds the nearest reachable deposit for the miner's
// current quota entry. If nothing of that resource is reachable right now
// (exhausted, or cut off by water/buildings), it tries the remaining quota
// entries in cycle order instead of leaving the miner idle while the other
// two resources sit untouched -- and, if one of those succeeds, jumps the
// quota pointer to it so the skipped entry is retried fresh next time
// around rather than immediately re-blocking the very next cycle.
func (c *Controller) startDepositJob(m *Miner, grid *world.Grid, buildings []*building.Building) {
	start := pathfind.Point{X: m.X, Y: m.Y}
	for attempt := 0; attempt < len(DefaultQuota); attempt++ {
		idx := (m.quotaIndex + attempt) % len(DefaultQuota)
		q := DefaultQuota[idx]

		bestLength := int(^uint(0) >> 1)
		var bestDeposit *building.Building
		var bestPath []pathfind.Point
		for _, candidate := range buildings {
			if candidate == nil || candidate.Kind != q.Deposit || candidate.Reserve <= 0 {
				continue
			}
			path, ok := pathfind.FindLandPath(grid, buildings, start, pathfind.Point{X: candidate.X, Y: candidate.Y})
			if !ok || len(path) > MaxWorkRadius || len(path) >= bestLength {
				continue
			}
			bestDeposit, bestPath, bestLength = candidate, path, len(path)
		}
		if bestDeposit == nil {
			continue
		}
		if attempt > 0 {
			m.quotaIndex = idx
			m.quotaProgress = 0
		}
		m.target = bestDeposit
		m.cargoResource = q.Resource
		m.setPath(bestPath)
		m.state = StateToDeposit
		return
	}
}

func (m *Miner) routeTo(grid *world.Grid, buildings []*building.Building, goal pathfind.Point) bool {
	path, ok := pathfind.FindLandPath(grid, buildings, pathfind.Point{X: m.X, Y: m.Y}, goal)
	if !ok {
		return false
	}
	m.setPath(path)
	return true
}

func (m *Miner) routeToBuilding(grid *world.Grid, buildings []*building.Building, target *building.Building, state State) bool {
	if target == nil {
		return false
	}
	p := target.AccessPoint()
	if !m.routeTo(grid, buildings, pathfind.Point{X: p.X, Y: p.Y}) {
		return false
	}
	m.state = state
	return true
}

func (m *Miner) routeHome(grid *world.Grid, buildings []*building.Building) bool {
	if m.Home == nil {
		return false
	}
	p := m.Home.AccessPoint()
	if !m.routeTo(grid, buildings, pathfind.Point{X: p.X, Y: p.Y}) {
		return false
	}
	m.state = StateToHome
	return true
}

func (m *Miner) setPath(path []pathfind.Point) {
	m.path = path
	m.pathIdx = 0
	m.tileTicks = 0
}

func (m *Miner) advancePath() bool {
	if len(m.path) == 0 {
		return true
	}
	m.tileTicks++
	if m.tileTicks < TicksPerTile {
		return false
	}
	m.tileTicks = 0
	if m.pathIdx < len(m.path)-1 {
		m.pathIdx++
		m.X, m.Y = m.path[m.pathIdx].X, m.path[m.pathIdx].Y
		return false
	}
	return true
}

func (m *Miner) resetToIdle() {
	if m.Home != nil {
		p := m.Home.AccessPoint()
		m.X, m.Y = p.X, p.Y
	}
	m.state = StateIdle
	m.target = nil
	m.path = nil
	m.pathIdx = 0
	m.tileTicks = 0
	m.workTicks = 0
}

func atAccessPoint(m *Miner, home *building.Building) bool {
	if m == nil || home == nil {
		return false
	}
	p := home.AccessPoint()
	return m.X == p.X && m.Y == p.Y
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
