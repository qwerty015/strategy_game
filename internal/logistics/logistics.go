// Package logistics simulates serfs: workers that walk the road network
// (see package pathfind) carrying goods between producer/consumer
// buildings and the warehouse. This is the piece the MVP plan
// deliberately deferred -- see AGENTS.md -- now filled in because it's
// core to how the reference genre actually plays: a building not
// connected by road simply never gets serviced.
//
// Job queue order (see assign): keeping the Tavern fed comes first, then
// topping up a construction site short on Plank/StoneBlock/Iron -- straight
// from the Carpentry Workshop/Quarry Hut's own OutputBuffer when one has
// enough on hand, falling back to the Warehouse only if not -- then a
// direct producer -> consumer haul for the ordinary production chain (e.g.
// Mill's Flour straight to Bakery), then draining leftover OutputBuffer to
// the Warehouse, then pulling from the Warehouse to cover a shortage no
// producer can. This matches the reference behavior the user asked for:
// processing buildings feed each other (and construction sites) directly,
// the Warehouse is overflow/backup, not the only path.
//
// Every job search reads availability through a *reservations.Ledger
// (see package reservations) instead of raw buffer values, and every
// successful commitment (startLeg, tryStartMeal) records itself in that
// same ledger. Without this, several idle serfs in one Tick call -- or
// serfs from earlier ticks still walking -- would each independently
// "discover" the same limited stock and all set off for it, which is
// exactly the crowding bug this was built to fix (see AGENTS.md).
//
// A candidate source, Tavern, or Warehouse is only ever accepted once its
// reachability from the serf's (or, for a Warehouse dropoff, the pickup
// building's) current position has actually been checked via pathfind.
// Every job search tries every candidate in turn rather than stopping at
// the first match by buffer contents alone, and picks the nearest
// reachable one where more than one qualifies (see nearestTavernWithFood,
// nearestReachableWarehouse) -- see docs/DEVELOPMENT.md's "Job queue
// rules" section for the full writeup.
package logistics

import (
	"slices"

	"strategy_game/internal/building"
	"strategy_game/internal/hunger"
	"strategy_game/internal/meal"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/soldier"
	"strategy_game/internal/world"
)

// sortedResourceTypes returns a building's buffer/recipe resource keys in a
// fixed, deterministic order. Go randomizes map iteration order by design;
// without this, which resource a job search considers first (when a
// building holds more than one type at once) would vary from tick to tick
// for no game-visible reason.
func sortedResourceTypes[V any](m map[resource.Type]V) []resource.Type {
	types := make([]resource.Type, 0, len(m))
	for t := range m {
		types = append(types, t)
	}
	slices.Sort(types)
	return types
}

const (
	// CarryCapacity is how many units of one resource a serf hauls per
	// trip -- matches a typical recipe batch size, so one full
	// production cycle usually means one trip.
	CarryCapacity = 5

	// TicksPerTile is how many simulation ticks it takes a serf to
	// cross one tile of road.
	TicksPerTile = 2

	// HungerInterval is kept as the public "seek a meal" threshold for
	// callers and tests. A serf starts looking at 20% satiety; starvation
	// itself happens later at hunger.MaxTicks.
	HungerInterval = hunger.MealThresholdTicks
)

type phase int

const (
	idle phase = iota
	toPickup
	toDropoff

	// returning is the free-roam walk back to the nearest warehouse after
	// an off-road delivery (construction materials or soldier food -- see
	// Serf.construction/soldierTarget) ends away from the road network.
	// Without it a serf just stood wherever the delivery left it: every
	// ordinary job search after that routes over the road network only
	// (see findTavernSupplyJob/findDirectJob/etc.), so it could never find
	// a next job and starved on the spot -- a real bug the user reported.
	// The off-road privilege stays scoped to this one trip back, not to
	// job-search in general -- see startReturnToRoad.
	returning
)

// Serf is a single worker. X/Y is its current tile position, exported
// for rendering; everything else is job-assignment bookkeeping.
type Serf struct {
	X, Y int

	ph        phase
	path      []pathfind.Point
	pathIdx   int
	tileTicks int

	atBuilding *building.Building // where the serf is standing when idle
	pickup     *building.Building
	dropoff    *building.Building
	resource   resource.Type
	amount     int
	eating     bool // true: this trip is "walk to pickup (a Tavern) and eat", not haul

	// construction is true for a Plank/StoneBlock/Iron delivery to a building
	// site or road tile still under construction (package builder). Unlike
	// every other haul, both legs of this trip may cross open land instead
	// of requiring a road -- see startConstructionLeg's doc comment for why.
	construction bool

	// soldierTarget is set for a food delivery to a hungry Archer/Swordsman
	// (see findSoldierDeliveryJob/startSoldierLeg) instead of an ordinary
	// building dropoff -- dropoff stays nil for this trip. Both legs cross
	// open land like a construction delivery, per the user's explicit
	// "доставка провизии... даёт слугам право свободного передвижения по
	// карте аналогично доставке стройматериалов". soldierTargetPoint is the
	// soldier's position at the moment the trip started: the serf walks
	// there without re-tracking the (possibly still moving) soldier, the
	// same simplification already accepted for a sentry's stone-flight
	// target.
	soldierTarget      *soldier.Soldier
	soldierTargetPoint pathfind.Point

	ticksSinceMeal int
	dismissing     bool

	// Starving is true once HungerInterval has passed with nowhere to
	// actually go eat (no Tavern yet, no road to one, or all food is gone).
	// A starving serf keeps hauling rather than stand idle
	// forever -- see tryStartMeal's doc comment for why.
	Starving bool

	// killed -- see villagers.Villager's identical field doc comment.
	killed bool
}

// Kill marks this serf for removal on Controller's next Tick.
func (s *Serf) Kill() { s.killed = true }

// Alive -- see villagers.Villager's identical method doc comment.
func (s *Serf) Alive() bool { return s != nil && !s.killed }

// State is the public, read-only activity state used by the inspector and
// render layer. The movement bookkeeping itself remains private to this
// package.
type State int

const (
	SerfIdle State = iota
	SerfToPickup
	SerfToDropoff
)

// TickResult reports lifecycle changes that cmd/game stores in the town
// history counters.
type TickResult struct {
	Deaths    int
	Dismissed int
}

// State reports what the serf is doing right now.
func (s *Serf) State() State {
	return State(s.ph)
}

// Eating reports whether the current trip is a meal trip to the Tavern.
func (s *Serf) Eating() bool {
	return s.eating
}

// AtBuilding returns the building where the serf is currently stationed.
func (s *Serf) AtBuilding() *building.Building {
	return s.atBuilding
}

// PickupBuilding returns the current job's pickup building, if any.
func (s *Serf) PickupBuilding() *building.Building {
	return s.pickup
}

// DropoffBuilding returns the current job's destination, if any. nil while
// delivering food to a soldier -- see SoldierTarget.
func (s *Serf) DropoffBuilding() *building.Building {
	return s.dropoff
}

// SoldierTarget returns the Archer/Swordsman this serf is currently
// carrying food to, or nil for every other trip.
func (s *Serf) SoldierTarget() *soldier.Soldier {
	return s.soldierTarget
}

// Cargo returns the resource and amount currently assigned to the serf. The
// amount is also populated before pickup, so the inspector can show planned
// jobs as well as cargo already in hand.
func (s *Serf) Cargo() (resource.Type, int) {
	return s.resource, s.amount
}

// HungerTicks returns simulation ticks since the serf's last meal.
func (s *Serf) HungerTicks() int {
	return s.ticksSinceMeal
}

// SatietyPercent returns the player-facing 0-100 satiety value.
func (s *Serf) SatietyPercent() int {
	return hunger.Percent(s.ticksSinceMeal)
}

// Busy reports whether the serf is currently walking a job, for
// rendering (e.g. a different color for working vs. idle serfs).
func (s *Serf) Busy() bool {
	return s.ph != idle
}

// RemainingPath returns the tiles still ahead of the serf on its current
// route, starting from (and including) the tile it's walking toward right
// now -- for the inspector's route-line overlay (see
// ui.DrawSelectedRoute). nil once idle or with no path assigned.
func (s *Serf) RemainingPath() []pathfind.Point {
	if s.pathIdx >= len(s.path) {
		return nil
	}
	return s.path[s.pathIdx:]
}

// Dismissing reports whether the serf will leave the town after completing
// the currently assigned trip. A dismissal never interrupts a haul, so cargo
// that is already in the serf's hands still reaches its destination.
func (s *Serf) Dismissing() bool {
	return s.dismissing
}

func (s *Serf) reset() {
	s.ph = idle
	s.path = nil
	s.pathIdx = 0
	s.tileTicks = 0
	s.pickup, s.dropoff = nil, nil
	s.amount = 0
	s.eating = false
	s.construction = false
	s.soldierTarget = nil
}

// Controller owns every serf and the warehouse they work out of.
type Controller struct {
	Warehouse  *building.Building // primary spawn/root warehouse
	Warehouses []*building.Building
	Serfs      []*Serf
	meals      meal.Selector

	// priority holds a per-building-kind supply priority, set by the
	// player (see SetPriority). A kind with no entry defaults to
	// PriorityNormal (0).
	priority map[building.Kind]int

	// constructionBackoff holds a countdown (in simulation ticks) for
	// construction sites whose delivery route just failed the re-check in
	// arriveAtPickup -- real bug this fixes ("слуги ходили по кругу с
	// материалами... материалы на стройку не доставились"): the site's
	// access point was reachable when the job was assigned
	// (nearestReachableWarehouseOverLand), but something built in the
	// meantime blocked the only route by the time the serf actually
	// arrived at the pickup with cargo in hand. Without this, the very
	// next idle serf (or even the same one) immediately re-discovers the
	// exact same doomed job and repeats the failure forever, burning
	// every serf's time without the site ever making progress. Not
	// persisted across saves -- it's a short-lived throttle, not game
	// state; a freshly loaded save just retries once before backing off
	// again if the route is still blocked.
	constructionBackoff map[*building.Building]int
}

// constructionBackoffTicks is how long a construction site sits skipped
// after a failed delivery before another attempt is allowed -- about a
// minute at normal speed. Long enough that a permanently blocked route
// doesn't keep consuming serf trips every single tick; short enough that
// the site recovers quickly once the player notices and clears the
// obstruction (or builds a road around it).
const constructionBackoffTicks = 120

// Supply priority levels a player can assign to a building kind, from a
// five-step scale. Anything in between or beyond also works -- higher
// always wins a tie -- but these are what the UI slider offers.
const (
	PriorityLowest  = -2
	PriorityLow     = -1
	PriorityNormal  = 0
	PriorityHigh    = 1
	PriorityHighest = 2
)

// NewController spawns count serfs standing at the warehouse.
func NewController(warehouse *building.Building, count int) *Controller {
	c := &Controller{Warehouse: warehouse, Warehouses: []*building.Building{warehouse}, meals: meal.NewSelector(0x243f6a88), constructionBackoff: map[*building.Building]int{}}
	for range count {
		c.Hire()
	}
	return c
}

// SetPriority sets the supply priority for every building of kind. It
// breaks ties when several consumer kinds compete for the same limited
// resource -- e.g. a Mill and a Pig Farm both short on Wheat with only
// one batch available this tick: whichever kind has the higher priority
// gets served first, instead of whichever happens to be earlier in the
// buildings slice by accident of build order. Setting PriorityNormal (0)
// clears the entry rather than storing a redundant zero.
func (c *Controller) SetPriority(kind building.Kind, level int) {
	if level == PriorityNormal {
		delete(c.priority, kind)
		return
	}
	if c.priority == nil {
		c.priority = make(map[building.Kind]int)
	}
	c.priority[kind] = level
}

// Priority returns the current supply priority for a building kind
// (PriorityNormal if never set).
func (c *Controller) Priority(kind building.Kind) int {
	return c.priority[kind]
}

// Priorities returns every building kind with a non-default priority, for
// save serialization. The map is a copy; mutating it has no effect on the
// controller.
func (c *Controller) Priorities() map[building.Kind]int {
	out := make(map[building.Kind]int, len(c.priority))
	for k, v := range c.priority {
		out[k] = v
	}
	return out
}

// MealSeed returns the persistent pseudo-random state for serf meal choices.
func (c *Controller) MealSeed() uint32 { return c.meals.Seed() }

// SetMealSeed restores the persistent pseudo-random state for serf meals.
func (c *Controller) SetMealSeed(seed uint32) { c.meals.SetSeed(seed) }

// AddWarehouse registers another physical warehouse as a valid logistics
// endpoint. All warehouses share the same unlimited town stockpile, while the
// primary warehouse remains the spawn point for newly hired serfs.
func (c *Controller) AddWarehouse(warehouse *building.Building) {
	if !warehouse.IsOperationalWarehouse() {
		return
	}
	for _, existing := range c.Warehouses {
		if existing == warehouse {
			return
		}
	}
	c.Warehouses = append(c.Warehouses, warehouse)
}

// RemoveWarehouse unregisters a Warehouse that is about to be removed from
// the map. The final Warehouse is deliberately protected: the simulation
// always needs one physical logistics endpoint and spawn point. If the
// primary Warehouse goes away, the oldest remaining one becomes the new
// primary before active serfs are re-anchored by CancelAllJobs.
func (c *Controller) RemoveWarehouse(warehouse *building.Building) bool {
	if warehouse == nil || len(c.Warehouses) <= 1 {
		return false
	}
	index := -1
	for i, candidate := range c.Warehouses {
		if candidate == warehouse {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	c.Warehouses = append(c.Warehouses[:index], c.Warehouses[index+1:]...)
	if c.Warehouse == warehouse {
		c.Warehouse = c.Warehouses[0]
	}
	return true
}

// ForceRemoveWarehouse unregisters warehouse unconditionally, even if it
// is the controller's only one -- unlike RemoveWarehouse (which
// deliberately protects a player's last Warehouse, since the player must
// always keep one physical logistics endpoint to resupply from), this is
// for a Warehouse destroyed by combat: an AI faction reduced to zero
// Warehouses is a valid, ongoing state (see cmd/game's factionDefeated,
// which only requires every REAL building gone, not specifically the
// Warehouse -- a straggler faction can very much have no warehouse left).
//
// A real playtest bug found from an actual save ("красный вроде не
// осталось склада, но его слуги продолжают носить рыбу куда-то"):
// cmd/game's pruneDestroyedBuildings removes a combat-killed Warehouse
// from the map's building list, but nothing ever told this Controller to
// stop treating it as a live delivery destination -- c.Warehouse/
// Warehouses kept a raw pointer to the same now-detached struct forever
// (it's never garbage collected, nothing else references it), so every
// pickup/dropoff search kept finding and "delivering" to a building that
// no longer existed on the map. c.Warehouse itself is deliberately left
// pointing at warehouse when nothing else remains (never nil -- other
// code, e.g. Hire, still dereferences .X/.Y/.Owner directly), but
// warehouses() only ever returns c.Warehouses, so an emptied Warehouses
// list correctly stops offering any candidate at all from here on.
func (c *Controller) ForceRemoveWarehouse(warehouse *building.Building) {
	for i, candidate := range c.Warehouses {
		if candidate == warehouse {
			c.Warehouses = append(c.Warehouses[:i], c.Warehouses[i+1:]...)
			break
		}
	}
	if c.Warehouse == warehouse && len(c.Warehouses) > 0 {
		c.Warehouse = c.Warehouses[0]
	}
}

// warehouses returns every registered candidate delivery/pickup endpoint.
// NewController always seeds Warehouses with the primary Warehouse too,
// so in practice this is just c.Warehouses -- deliberately NOT falling
// back to c.Warehouse alone when Warehouses is empty: that emptiness is
// exactly what ForceRemoveWarehouse produces once a faction's last
// Warehouse is destroyed, and a fallback here would silently resurrect
// the very dangling reference that fix removes (see its own doc comment).
func (c *Controller) warehouses() []*building.Building {
	return c.Warehouses
}

// Hire creates one additional serf at the Warehouse. The current MVP does
// not charge a resource cost; the method is kept on the controller so a cost
// and a population limit can be added later without changing the UI wiring.
func (c *Controller) Hire() *Serf {
	s := &Serf{X: c.Warehouse.X, Y: c.Warehouse.Y, atBuilding: c.Warehouse}
	c.Serfs = append(c.Serfs, s)
	return s
}

// RestoreSerf adds a serf at the position stored in a save file. Its active
// haul is deliberately rebuilt on the next tick, but hunger and the visible
// position survive the load. atBuilding stays nil until the first restored
// route reaches a building; routing uses X/Y directly in the meantime.
func (c *Controller) RestoreSerf(x, y, hungerTicks int, starving, dismissing bool) *Serf {
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	s := &Serf{X: x, Y: y, ticksSinceMeal: hungerTicks, Starving: starving, dismissing: dismissing}
	c.Serfs = append(c.Serfs, s)
	return s
}

// RequestDismissal marks a live serf to leave after its current assignment.
// An idle serf has no assignment to finish and is removed on the next
// simulation tick. The method returns false for a stale pointer, which keeps
// UI actions harmless after a unit was already removed.
func (c *Controller) RequestDismissal(serf *Serf) bool {
	for _, s := range c.Serfs {
		if s == serf {
			s.dismissing = true
			return true
		}
	}
	return false
}

// CancelAllJobs safely interrupts every active route after the map changes
// (for example, when a road or building is deleted). A serf that already
// picked up a load returns it to the shared stockpile, then all serfs are
// re-anchored at the Warehouse. This prevents stale routes and lost cargo.
func (c *Controller) CancelAllJobs(stock *resource.Stockpile) {
	for _, s := range c.Serfs {
		if s.ph == toDropoff && !s.eating && s.amount > 0 {
			stock.Add(s.resource, s.amount)
		}
		s.reset()
		s.atBuilding = c.Warehouse
		s.X, s.Y = c.Warehouse.X, c.Warehouse.Y
	}
}

// Reserve seeds ledger with every serf currently mid-trip (assigned but
// not yet delivered), covering both the pickup and dropoff side of the
// trip. Call once per simulation tick, before this or any other
// controller's Tick runs, so a claim already in flight is visible to
// everyone sharing a building (namely the Tavern) before new jobs are
// handed out.
func (c *Controller) Reserve(ledger *reservations.Ledger) {
	for _, s := range c.Serfs {
		if s.ph == idle {
			continue
		}
		// Only reserve the pickup side while the serf is still walking
		// there (s.ph == toPickup). Once cargo is actually in hand
		// (s.ph == toDropoff), the goods are already physically removed
		// from the source's buffer -- s.pickup stays set until reset()
		// purely as trip bookkeeping, not as an outstanding claim.
		// Reserving it again here would double-subtract: once because
		// the buffer itself already shrank, and again via the ledger,
		// making the source look emptier than it really is until the
		// delivery finishes.
		if s.ph == toPickup && s.pickup != nil {
			ledger.ReservePickup(s.pickup, s.resource, s.amount)
		}
		if s.dropoff != nil {
			ledger.ReserveDropoff(s.dropoff, s.resource, s.amount)
		}
	}
}

// MaxWaitingHunger returns the highest HungerTicks among idle serfs that
// will actually try to eat this tick (HungerTicks >= HungerInterval), or
// -1 if none will. cmd/game compares this against the other unit
// controllers' MaxWaitingHunger to decide whose Tick runs first this
// simulation tick when the Tavern's food is scarce -- the unit that's
// been waiting longest gets first claim, instead of whichever controller
// happens to be first in a fixed call order.
//
// This only works because ticksSinceMeal keeps counting past
// HungerInterval instead of saturating there: once several units across
// different controllers are simultaneously overdue, a counter capped at
// HungerInterval would tie them all at the same value, and the ordering
// would silently fall back to the fixed call order it was built to
// replace.
func (c *Controller) MaxWaitingHunger() int {
	best := -1
	for _, s := range c.Serfs {
		if s.dismissing || s.ph != idle || s.ticksSinceMeal < HungerInterval {
			continue
		}
		if s.ticksSinceMeal > best {
			best = s.ticksSinceMeal
		}
	}
	return best
}

// Tick assigns jobs to idle serfs and advances every serf by one
// movement step. Call once per simulation tick (see economy.Simulator),
// after every controller sharing ledger has had a chance to Reserve its
// own pre-existing in-flight units. grid is only used for the off-road
// construction-material delivery leg (see startConstructionLeg) and the
// soldier food-delivery leg (see startSoldierLeg); every other job still
// routes exclusively over the road network. soldiers lists every current
// Archer/Swordsman so a hungry one can be found (NeedsDelivery) --
// nil/empty is fine before the Barracks has hired any.
//
// buildings and obstacles are deliberately different lists. buildings is
// this faction's own (plus shared-neutral: Road, natural resources --
// see cmd/game's ownedBuildingsWithRoads) and picks WHICH job candidates
// (producer, consumer, warehouse, construction site...) this serf may
// even consider -- unchanged, an opposing faction's buildings must never
// appear here. obstacles is the WHOLE map's buildings, every faction's,
// and is used only for the off-road (FindLandPathForFaction) legs'
// physical route: a real bug found from an actual playtest report
// ("вражеские слуги свободно проходят через" the player's own wall) --
// a soldier-delivery or construction-delivery leg can, by design, need to
// physically walk into or across an opposing faction's territory (that
// soldier being fed might be deep inside it, mid-attack), and that walk
// must actually be blocked by a wall that isn't this faction's own, the
// same as soldier movement already is (see cmd/game's Update/
// tickAIFaction passing g.buildings to soldiers.Tick). Passing buildings
// for obstacles too would work but silently make every foreign wall
// invisible again -- the bug this fixes.
func (c *Controller) Tick(grid *world.Grid, buildings []*building.Building, obstacles []*building.Building, stock *resource.Stockpile, ledger *reservations.Ledger, soldiers []*soldier.Soldier) TickResult {
	c.tickConstructionBackoff()

	// A soldier already claimed by a serf en route must not be picked
	// again by another idle serf later in this same loop -- recomputed
	// fresh every tick (nothing to persist between ticks) and mutated in
	// place as assign() commits new deliveries below.
	claimedSoldiers := map[*soldier.Soldier]bool{}
	for _, s := range c.Serfs {
		if s.soldierTarget != nil {
			claimedSoldiers[s.soldierTarget] = true
		}
	}

	var result TickResult
	remaining := c.Serfs[:0]
	for _, s := range c.Serfs {
		if s.killed {
			// See villagers.Villager's identical field doc comment.
			// Same cargo-return handling as a hunger death below, so an
			// enemy Sentry's kill doesn't silently destroy a resource
			// the serf was mid-haul with.
			if s.ph == toDropoff && !s.eating && s.amount > 0 {
				stock.Add(s.resource, s.amount)
			}
			result.Deaths++
			continue
		}
		s.ticksSinceMeal++
		if hunger.Dead(s.ticksSinceMeal) {
			// A carried haul has already left its source. Return it to the
			// common reserve, just as a cancelled route does, so starvation
			// never silently destroys resources.
			if s.ph == toDropoff && !s.eating && s.amount > 0 {
				stock.Add(s.resource, s.amount)
			}
			result.Deaths++
			continue
		}
		// A dismissal is checked before hunger and job assignment, so an
		// idle serf leaves immediately and a serf who just completed a haul
		// never starts a detour to the Tavern first.
		if s.dismissing && s.ph == idle {
			result.Dismissed++
			continue
		}
		if s.ph == idle {
			if !c.tryStartMeal(s, buildings, ledger) {
				c.assign(s, grid, buildings, obstacles, stock, ledger, soldiers, claimedSoldiers)
			}
		}
		c.advance(s, grid, buildings, obstacles, stock)
		remaining = append(remaining, s)
	}
	c.Serfs = remaining
	return result
}

// tickConstructionBackoff counts down every site currently sitting out a
// failed-delivery cooldown (see constructionBackoff's doc comment),
// dropping the entry once it reaches zero so the site becomes a normal
// candidate again.
func (c *Controller) tickConstructionBackoff() {
	for site, ticks := range c.constructionBackoff {
		if ticks <= 1 {
			delete(c.constructionBackoff, site)
			continue
		}
		c.constructionBackoff[site] = ticks - 1
	}
}

// nearestTavernWithFood returns the nearest reachable Tavern with at least one
// available food. The selector chooses randomly from that Tavern's available
// menu; wanted is used only to restore an already reserved saved meal.
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

// nearestReachableWarehouse returns the warehouse in candidates with the
// shortest road path from `from`, or false if none is reachable. A town
// can have several Warehouses; without this, a serf always picked
// whichever one happened to be first in Controller.Warehouses (the one
// registered first, normally the original spawn Warehouse), leaving
// every other Warehouse effectively unused as long as the first one
// stayed reachable at all.
func nearestReachableWarehouse(buildings []*building.Building, candidates []*building.Building, from pathfind.Point) (warehouse *building.Building, path []pathfind.Point, ok bool) {
	bestLen := -1
	for _, w := range candidates {
		p, reachable := pathfind.FindPathFromPoint(buildings, from, w)
		if !reachable {
			continue
		}
		if bestLen == -1 || len(p) < bestLen {
			warehouse, path, bestLen = w, p, len(p)
		}
	}
	return warehouse, path, warehouse != nil
}

// nearestReachableWarehouseTo is like nearestReachableWarehouse but also
// requires the candidate to be able to deliver onward to dest (e.g. the
// Tavern the pulled stock is meant for) -- both legs of the trip must
// work, not just the one from the serf's current position. Picking the
// nearest warehouse to the serf and only then discovering it can't reach
// the destination would waste the serf's whole walk there for nothing.
func nearestReachableWarehouseTo(buildings []*building.Building, candidates []*building.Building, from pathfind.Point, dest *building.Building) (warehouse *building.Building, path []pathfind.Point, ok bool) {
	bestLen := -1
	for _, w := range candidates {
		p, reachable := pathfind.FindPathFromPoint(buildings, from, w)
		if !reachable {
			continue
		}
		if _, deliverable := pathfind.FindPath(buildings, w, dest); !deliverable {
			continue
		}
		if bestLen == -1 || len(p) < bestLen {
			warehouse, path, bestLen = w, p, len(p)
		}
	}
	return warehouse, path, warehouse != nil
}

// tryStartMeal sends an idle, hungry serf to eat at the nearest reachable,
// stocked Tavern instead of taking a new haul job. Hunger only ever
// interrupts a serf between jobs, never mid-haul. If the serf is hungry
// but there's nowhere to actually go (no Tavern yet, none reachable, or
// all of them out of food once other units' claims are accounted for),
// it's marked Starving but keeps hauling anyway -- refusing to work would
// cripple the whole economy before a Tavern even exists, which is a
// worse outcome than a hungry serf.
func (c *Controller) tryStartMeal(s *Serf, buildings []*building.Building, ledger *reservations.Ledger) bool {
	if s.ticksSinceMeal < HungerInterval {
		return false
	}
	tavern, meal, path, ok := nearestTavernWithFood(buildings, pathfind.Point{X: s.X, Y: s.Y}, ledger, &c.meals)
	if !ok {
		s.Starving = true
		return false
	}
	s.Starving = false
	s.pickup = tavern
	s.resource = meal
	s.amount = 1
	s.eating = true
	s.path, s.pathIdx, s.tileTicks = path, 0, 0
	s.ph = toPickup
	ledger.ReservePickup(tavern, meal, 1)
	return true
}

// assign gives an idle serf a job, if one exists that it can currently
// reach, in priority order: a hungry soldier waiting on food comes first
// (highest priority, per the user's explicit request), then keep the
// Tavern supplied, then top up any construction site waiting on materials
// -- straight from a producer's OutputBuffer if one has enough on hand
// (findConstructionDirectJob), falling back to the Warehouse only if not
// (findConstructionSupplyJob) -- then direct producer->consumer haul for
// the ordinary production chain, then drain leftover OutputBuffer to the
// Warehouse, then pull from the Warehouse to cover a shortage no producer
// can. Construction is placed right after hunger and soldier delivery,
// ahead of the ordinary production chain, because a stalled build is what
// the player is actively watching, and because a road that only gets
// built "when a serf is otherwise idle" would make the very first road --
// the one everything else's connectivity depends on -- unreasonably slow
// to appear.
func (c *Controller) assign(s *Serf, grid *world.Grid, buildings []*building.Building, obstacles []*building.Building, stock *resource.Stockpile, ledger *reservations.Ledger, soldiers []*soldier.Soldier, claimedSoldiers map[*soldier.Soldier]bool) {
	from := pathfind.Point{X: s.X, Y: s.Y}
	if target, food, ok := findSoldierDeliveryJob(soldiers, claimedSoldiers, stock, ledger); ok {
		destPoint := pathfind.Point{X: target.X, Y: target.Y}
		if warehouse, path, ok := nearestReachableWarehouseOverLandToPoint(grid, obstacles, c.warehouses(), from, destPoint, c.Warehouse.Owner); ok {
			c.startSoldierLeg(s, warehouse, target, food, path, ledger)
			claimedSoldiers[target] = true
			return
		}
	}
	if pickup, dropoff, t, n, path, ok := findTavernSupplyJob(buildings, c.warehouses(), stock, ledger, from); ok {
		c.startLeg(s, pickup, dropoff, t, n, path, ledger)
		return
	}
	if pickup, site, t, n, path, ok := findConstructionDirectJob(grid, buildings, obstacles, ledger, from, c.constructionBackoff, c.Warehouse.Owner); ok {
		c.startConstructionLeg(s, pickup, site, t, n, path, ledger)
		return
	}
	if dropoff, t, n, ok := findConstructionSupplyJob(buildings, stock, ledger, c.constructionBackoff); ok {
		if warehouse, path, ok := nearestReachableWarehouseOverLand(grid, obstacles, c.warehouses(), from, dropoff, c.Warehouse.Owner); ok {
			c.startConstructionLeg(s, warehouse, dropoff, t, n, path, ledger)
			return
		}
	}
	if pickup, dropoff, t, n, path, ok := findDirectJob(buildings, c.Warehouse, ledger, from, c.priority); ok {
		c.startLeg(s, pickup, dropoff, t, n, path, ledger)
		return
	}
	// A warehouse-backed shortage must win over collecting a producer's
	// surplus. On a mature map there is almost always some output to collect;
	// putting collection first could therefore starve an Armory (or any other
	// consumer) forever despite the required resource already being in stock.
	if warehouse, dropoff, t, n, path, ok := findSupplyJob(buildings, c.warehouses(), stock, ledger, c.priority, from); ok {
		c.startLeg(s, warehouse, dropoff, t, n, path, ledger)
		return
	}
	if b, t, n, path, ok := findCollectJob(buildings, c.Warehouse, ledger, from, c.priority); ok {
		// The pickup (b) is already confirmed reachable from the serf;
		// only the dropoff warehouse still needs picking, from wherever
		// b itself can reach.
		bAccess := b.AccessPoint()
		if warehouse, _, ok := nearestReachableWarehouse(buildings, c.warehouses(), pathfind.Point{X: bAccess.X, Y: bAccess.Y}); ok {
			c.startLeg(s, b, warehouse, t, n, path, ledger)
			return
		}
	}
}

// findTavernSupplyJob is deliberately separate from the general consumer
// search. It keeps every Tavern supplied with any accepted food; Bread, Fish,
// Wine and Sausage are all valid and none has a gameplay priority.
//
// A town can have several Taverns, each with its own BufferCapacity-limited
// stock, so every one of them is considered (not just the first found in
// buildings). Every candidate source and warehouse is checked for BOTH legs
// of the trip -- serf to pickup (from), and pickup to the Tavern -- before
// being accepted: a producer or warehouse that merely looks promising on
// paper but can't actually reach this particular Tavern (a broken road
// between the two, even if each is independently reachable from the serf)
// must not block a serf from trying the next one. Without the second leg
// check, a serf could pick up a load, walk all the way to the producer,
// and only then discover it can't actually deliver -- the goods get
// returned safely (see arriveAtPickup), but the whole trip was wasted.
func findTavernSupplyJob(buildings []*building.Building, warehouses []*building.Building, stock *resource.Stockpile, ledger *reservations.Ledger, from pathfind.Point) (pickup, dropoff *building.Building, t resource.Type, amount int, path []pathfind.Point, ok bool) {
	accepted := building.Types[building.Tavern].AcceptedResources
	if len(accepted) == 0 {
		accepted = resource.FoodTypes()
	}
	for _, tavern := range buildings {
		if tavern.Kind != building.Tavern {
			continue
		}
		for _, rt := range accepted {
			room := ledger.RoomFor(tavern, rt, building.BufferCapacity)
			if room <= 0 {
				continue
			}
			// Among every producer holding rt, try the biggest backlog
			// first, not just the first one in buildings' slice order.
			// Real bug the user found by simulating their actual save: a
			// producer earlier in that slice (in practice, whichever one
			// happened to be built first) won this search every single
			// time it had *any* leftover output, even a sliver -- forever
			// starving a same-kind sibling sitting on a much bigger,
			// unrelieved backlog. Most visible on Winery, whose
			// OutputCapacity exactly equals one batch's OutputAmount, so
			// it jumps from empty straight to 100% full every cycle
			// instead of filling gradually like everything else.
			//
			// This costs no more pathfind calls than before in the
			// typical case: sorting a handful of candidates by backlog is
			// cheap (no BFS involved), and the expensive reachability/
			// deliverability check still stops at the first candidate
			// that actually works -- it just tries the biggest backlog
			// first instead of the slice-order-first one.
			type sourceCandidate struct {
				b    *building.Building
				have int
			}
			var candidates []sourceCandidate
			for _, producer := range buildings {
				if producer.Kind == building.Warehouse || producer.Kind == building.Road || producer.Kind == building.Tree {
					continue
				}
				if have := ledger.AvailableOutput(producer, rt); have > 0 {
					candidates = append(candidates, sourceCandidate{producer, have})
				}
			}
			slices.SortFunc(candidates, func(a, b sourceCandidate) int { return b.have - a.have })
			for _, c := range candidates {
				p, reachable := pathfind.FindPathFromPoint(buildings, from, c.b)
				if !reachable {
					continue // not reachable from here right now -- try the next source
				}
				if _, deliverable := pathfind.FindPath(buildings, c.b, tavern); !deliverable {
					continue // can't actually deliver from here to this Tavern -- try the next source
				}
				return c.b, tavern, rt, min(c.have, CarryCapacity, room), p, true
			}
			if avail := ledger.AvailableStock(stock, rt); avail > 0 {
				if warehouse, p, ok := nearestReachableWarehouseTo(buildings, warehouses, from, tavern); ok {
					return warehouse, tavern, rt, min(CarryCapacity, room, avail), p, true
				}
			}
		}
	}
	return nil, nil, 0, 0, nil, false
}

// findDirectJob looks for a producer with surplus output that some
// other (non-Warehouse) building directly wants as input, bypassing the
// Warehouse entirely. Building-to-building, not resource-type-specific:
// works for any Recipe pairing (Farm->Mill, Mill->Bakery, ...) because
// it just reads OutputBuffer against the candidate's Recipe.Inputs.
//
// Resource types are visited in a fixed sorted order (not Go's randomized
// map iteration) so which one gets hauled first, when a producer holds
// several at once, is stable from tick to tick. Each candidate producer is
// also checked for reachability from the serf's current position before
// being accepted, so an unreachable producer never blocks trying the next
// one. When more than one consumer wants the same producer's output,
// priority (see Controller.SetPriority) picks the winner; consumers tied
// on priority go to whichever is short the most, not just whichever
// happens to come first in buildings' slice order (see
// findTavernSupplyJob's doc comment for the bug that fixed).
func findDirectJob(buildings []*building.Building, warehouse *building.Building, ledger *reservations.Ledger, from pathfind.Point, priority map[building.Kind]int) (pickup, dropoff *building.Building, t resource.Type, amount int, path []pathfind.Point, ok bool) {
	for _, producer := range buildings {
		if producer == warehouse || producer.Kind == building.Road || producer.Kind == building.Tree {
			continue
		}
		for _, rt := range sortedResourceTypes(producer.OutputBuffer) {
			have := ledger.AvailableOutput(producer, rt)
			if have <= 0 {
				continue
			}
			p, reachable := pathfind.FindPathFromPoint(buildings, from, producer)
			if !reachable {
				continue // not reachable from here right now -- try the next producer
			}
			var bestConsumer *building.Building
			var bestAmt, bestShort int
			bestPriority := 0
			for _, consumer := range buildings {
				if consumer == warehouse || consumer == producer || consumer.Kind == building.Road || consumer.Kind == building.Tree {
					continue
				}
				need := building.Types[consumer.Kind].InputRequirement(rt)
				if need <= 0 {
					continue
				}
				short := ledger.RoomFor(consumer, rt, need)
				if short <= 0 {
					continue
				}
				amt := min(have, CarryCapacity, short)
				if amt <= 0 {
					continue
				}
				if bestConsumer != nil && priority[consumer.Kind] < bestPriority {
					continue // a strictly higher-priority candidate already won
				}
				// Among candidates tied on priority, the consumer with the
				// bigger shortfall wins -- see findTavernSupplyJob's doc
				// comment for the real bug this fixes (same bias, same
				// fix, applied here too).
				if bestConsumer != nil && priority[consumer.Kind] == bestPriority && short <= bestShort {
					continue
				}
				if _, deliverable := pathfind.FindPath(buildings, producer, consumer); !deliverable {
					continue // this producer can't actually deliver to this consumer -- try the next consumer
				}
				bestConsumer, bestAmt, bestPriority, bestShort = consumer, amt, priority[consumer.Kind], short
			}
			if bestConsumer != nil {
				return producer, bestConsumer, rt, bestAmt, p, true
			}
		}
	}
	return nil, nil, 0, 0, nil, false
}

// findCollectJob looks for any building with surplus output to drain to a
// warehouse. Resource types are visited in a fixed sorted order, and each
// candidate is checked for reachability before being accepted. Among
// several candidates with surplus, priority (see Controller.SetPriority)
// picks which one gets cleared out first; candidates tied on priority go
// to whichever has the biggest backlog, not just whichever happens to
// come first in buildings' slice order (see findTavernSupplyJob's doc
// comment for the bug that fixed).
func findCollectJob(buildings []*building.Building, warehouse *building.Building, ledger *reservations.Ledger, from pathfind.Point, priority map[building.Kind]int) (b *building.Building, t resource.Type, amount int, path []pathfind.Point, ok bool) {
	bestPriority := 0
	bestAmount := 0
	for _, cand := range buildings {
		if cand == warehouse || cand.Kind == building.Road || cand.Kind == building.Tree {
			continue
		}
		if b != nil && priority[cand.Kind] < bestPriority {
			continue // a strictly higher-priority candidate already won
		}
		for _, rt := range sortedResourceTypes(cand.OutputBuffer) {
			n := ledger.AvailableOutput(cand, rt)
			if n <= 0 {
				continue
			}
			// Among candidates tied on priority, the biggest backlog wins
			// -- see findTavernSupplyJob's doc comment for the real bug
			// this fixes (same bias, same fix, applied here too). Checked
			// before the reachability BFS below, so a losing candidate on
			// this tie-break never pays for that BFS at all.
			if b != nil && priority[cand.Kind] == bestPriority && n <= bestAmount {
				continue
			}
			p, reachable := pathfind.FindPathFromPoint(buildings, from, cand)
			if !reachable {
				continue // not reachable from here right now -- try the next candidate
			}
			b, t, amount, path, ok = cand, rt, min(n, CarryCapacity), p, true
			bestPriority, bestAmount = priority[cand.Kind], n
			break
		}
	}
	return
}

// findSupplyJob looks for a building falling short on an input that the
// Warehouse could actually cover right now, and an actually reachable
// warehouse to fetch it from. Resource types are visited in a fixed sorted
// order for the same reason as the other job searches. Among several
// buildings short on input, priority (see Controller.SetPriority) picks
// which one gets resupplied first -- this is the one that resolves
// "свиноферма/мельница" style contention over a shared input like Wheat;
// ties on priority go to the biggest shortfall, same as findTavernSupplyJob.
//
// A shortage is only accepted if stock actually has some of that resource
// available (per the shared ledger): a building short on a resource
// nobody has any of yet must not block trying the next shortage, whether
// that's a different resource at the same building or a different
// building entirely.
//
// Every ranked candidate is tried, in order, until one actually has a
// reachable warehouse (see nearestReachableWarehouseTo) -- a real bug
// found from an actual playtest report ("resources sit in the warehouse
// forever, nothing gets delivered to my Barracks"): this used to just
// pick the single best-ranked candidate and hand it to the caller with no
// fallback. A WatchTower whose only road connection had been walled off
// (a real, valid outcome once the player builds walls/gates around their
// base -- unlike a bug, that closed gate is meant to block traffic) still
// outranked the Barracks's shortfall every single tick, so the loop never
// even tried the Barracks -- forever, since the WatchTower's shortage
// never went away. The same "try the next-best candidate when the winner
// turns out unreachable" pattern findTavernSupplyJob already uses for
// picking a source producer, applied here to picking the shortage itself.
func findSupplyJob(buildings []*building.Building, warehouses []*building.Building, stock *resource.Stockpile, ledger *reservations.Ledger, priority map[building.Kind]int, from pathfind.Point) (warehouse, dropoff *building.Building, t resource.Type, amount int, path []pathfind.Point, ok bool) {
	type shortageCandidate struct {
		b        *building.Building
		t        resource.Type
		amount   int
		priority int
		short    int
	}
	var candidates []shortageCandidate
	for _, cand := range buildings {
		if cand.Kind == building.Warehouse || cand.Kind == building.Road || cand.Kind == building.Tree {
			continue
		}
		for _, rt := range resource.AllTypes() {
			need := building.Types[cand.Kind].InputRequirement(rt)
			if need <= 0 {
				continue
			}
			short := ledger.RoomFor(cand, rt, need)
			if short <= 0 {
				continue
			}
			amt := min(CarryCapacity, short, ledger.AvailableStock(stock, rt))
			if amt <= 0 {
				continue // nothing in stock for this shortage -- try the next one
			}
			candidates = append(candidates, shortageCandidate{cand, rt, amt, priority[cand.Kind], short})
			break // one shortage per building considered per pass, same as before
		}
	}
	slices.SortFunc(candidates, func(a, b shortageCandidate) int {
		if a.priority != b.priority {
			return b.priority - a.priority
		}
		return b.short - a.short
	})
	for _, c := range candidates {
		wh, p, reachable := nearestReachableWarehouseTo(buildings, warehouses, from, c.b)
		if !reachable {
			continue // this shortage's only warehouse route is blocked right now -- try the next one
		}
		return wh, c.b, c.t, c.amount, p, true
	}
	return nil, nil, 0, 0, nil, false
}

// constructionMaterials is the fixed pair a construction site ever wants,
// visited in this order so which one gets hauled first (when a site is
// short on both) is stable from tick to tick.
var constructionMaterials = building.ConstructionMaterialTypes()

// findConstructionSupplyJob looks for a building or road tile still under
// construction (see building.ConstructionStage) that's short on Plank or
// StoneBlock, with some of that resource actually available in the shared
// stockpile. It deliberately doesn't pick a specific warehouse or check
// reachability itself, for the same reason findSupplyJob doesn't: which
// registered warehouse is actually reachable depends on the serf's current
// position, and here "reachable" additionally means over open land, not
// necessarily a road -- see nearestReachableWarehouseOverLand. blocked is
// Controller.constructionBackoff -- a site whose delivery just failed
// stays skipped for a while instead of being immediately re-offered to
// the next idle serf.
func findConstructionSupplyJob(buildings []*building.Building, stock *resource.Stockpile, ledger *reservations.Ledger, blocked map[*building.Building]int) (site *building.Building, t resource.Type, amount int, ok bool) {
	for _, cand := range buildings {
		if cand.ConstructionStage == building.ConstructionNone || blocked[cand] > 0 {
			continue
		}
		for _, rt := range constructionMaterials {
			target := cand.ConstructionMaterialCost(rt)
			if target <= 0 {
				continue
			}
			short := ledger.RoomFor(cand, rt, target)
			if short <= 0 {
				continue
			}
			amt := min(CarryCapacity, short, ledger.AvailableStock(stock, rt))
			if amt <= 0 {
				continue // nothing in stock for this shortage -- try the next one
			}
			return cand, rt, amt, true
		}
	}
	return nil, 0, 0, false
}

// constructionMaterialProducer maps each construction material to the
// building kind that produces it, for findConstructionDirectJob. A site's
// material need isn't expressed as a Recipe.Input (see
// Building.ConstructionMaterialCost), so it can't reuse findDirectJob's
// generic recipe-driven consumer search -- this is its construction-site
// counterpart, kept as a small explicit map rather than derived from
// Recipe.Output since a producer's Recipe isn't guaranteed to exist purely
// to make this construction material (Plank/StoneBlock/Iron happen to be each
// one's only output today, but that's incidental, not a rule to lean on).
var constructionMaterialProducer = map[resource.Type]building.Kind{
	resource.Plank:      building.CarpentryWorkshop,
	resource.StoneBlock: building.QuarryHut,
	resource.Iron:       building.Smeltery,
}

// findConstructionDirectJob looks for a construction site short on Plank or
// StoneBlock that can be supplied straight from a producer's own
// OutputBuffer -- a Carpentry Workshop's planks, a Quarry Hut's stone or a Smeltery's iron
// blocks -- skipping the Warehouse entirely. Per the user's explicit
// request ("если требуется строителям - несем их им а не на склад"), this
// gives construction sites the same direct producer->consumer priority
// findDirectJob already gives ordinary buildings (see the package doc
// comment): the Warehouse should be a detour material passes through when
// nothing needs it right now, not a mandatory stop on the way to a site
// that does. Tried before findConstructionSupplyJob in assign, so a direct
// haul wins whenever one is actually available; the Warehouse-sourced job
// remains the fallback once nothing here reaches. Both legs use
// pathfind.FindLandPathForFaction, the same off-road exception every
// construction delivery gets (see startConstructionLeg) -- the site or
// its producer may have no road yet. owner is this serf's own faction
// (see assign's callers) -- see FindLandPathForFaction's doc comment for
// why an off-road leg specifically needs to know it, unlike the
// road-network-only jobs elsewhere in this file. buildings (this
// faction's own candidates) and obstacles (the whole map, every
// faction's, for the route itself) are deliberately different lists --
// see Controller.Tick's doc comment on the same split.
func findConstructionDirectJob(grid *world.Grid, buildings []*building.Building, obstacles []*building.Building, ledger *reservations.Ledger, from pathfind.Point, blocked map[*building.Building]int, owner int) (pickup, site *building.Building, t resource.Type, amount int, path []pathfind.Point, ok bool) {
	for _, cand := range buildings {
		if cand.ConstructionStage == building.ConstructionNone || blocked[cand] > 0 {
			continue
		}
		for _, rt := range constructionMaterials {
			target := cand.ConstructionMaterialCost(rt)
			if target <= 0 {
				continue
			}
			short := ledger.RoomFor(cand, rt, target)
			if short <= 0 {
				continue
			}
			producerKind, known := constructionMaterialProducer[rt]
			if !known {
				continue
			}
			siteAccess := cand.AccessPoint()
			sitePoint := pathfind.Point{X: siteAccess.X, Y: siteAccess.Y}

			var bestProducer *building.Building
			var bestAmt int
			var bestPath []pathfind.Point
			bestLen := -1
			for _, producer := range buildings {
				if producer.Kind != producerKind {
					continue
				}
				have := ledger.AvailableOutput(producer, rt)
				if have <= 0 {
					continue
				}
				amt := min(have, CarryCapacity, short)
				if amt <= 0 {
					continue
				}
				producerAccess := producer.AccessPoint()
				producerPoint := pathfind.Point{X: producerAccess.X, Y: producerAccess.Y}
				p, reachable := pathfind.FindLandPathForFaction(grid, obstacles, from, producerPoint, owner)
				if !reachable {
					continue
				}
				if _, deliverable := pathfind.FindLandPathForFaction(grid, obstacles, producerPoint, sitePoint, owner); !deliverable {
					continue
				}
				if bestLen == -1 || len(p) < bestLen {
					bestProducer, bestAmt, bestPath, bestLen = producer, amt, p, len(p)
				}
			}
			if bestProducer != nil {
				return bestProducer, cand, rt, bestAmt, bestPath, true
			}
		}
	}
	return nil, nil, 0, 0, nil, false
}

// findSoldierDeliveryJob returns the first living, hungry
// (NeedsDelivery()==true) soldier not already claimed by another serf this
// tick, together with a food resource currently available in the shared
// stockpile. Soldiers are tried in a fixed order (whatever order the
// caller's slice holds), not by whoever is hungriest -- with the 30%
// threshold applying to every soldier, and delivery already the top
// priority job of all, a fancier tie-break isn't worth the complexity.
func findSoldierDeliveryJob(soldiers []*soldier.Soldier, claimed map[*soldier.Soldier]bool, stock *resource.Stockpile, ledger *reservations.Ledger) (target *soldier.Soldier, food resource.Type, ok bool) {
	for _, sd := range soldiers {
		if sd == nil || !sd.Alive() || !sd.NeedsDelivery() || claimed[sd] {
			continue
		}
		for _, f := range resource.FoodTypes() {
			if ledger.AvailableStock(stock, f) > 0 {
				return sd, f, true
			}
		}
	}
	return nil, 0, false
}

// nearestReachableWarehouseOverLandToPoint is
// nearestReachableWarehouseOverLand's soldier-delivery counterpart: the
// destination is a raw tile (a soldier's position at dispatch time, see
// Serf.soldierTargetPoint) rather than a building. owner is this serf's
// own faction -- see FindLandPathForFaction's doc comment: a hungry
// soldier can, by design, be deep inside an opposing faction's walled
// territory (that's the whole point of combat working at all), so this
// off-road leg must actually be blocked by a wall that isn't this serf's
// own, the same real bug found from an actual playtest report ("вражеские
// слуги свободно проходят через" the player's own wall -- confirmed: it
// was exactly this leg, an opposing faction's serf walking off-road to
// feed its own soldier who'd fought its way inside).
func nearestReachableWarehouseOverLandToPoint(grid *world.Grid, buildings []*building.Building, candidates []*building.Building, from, destPoint pathfind.Point, owner int) (warehouse *building.Building, path []pathfind.Point, ok bool) {
	bestLen := -1
	for _, w := range candidates {
		access := w.AccessPoint()
		p, reachable := pathfind.FindLandPathForFaction(grid, buildings, from, pathfind.Point{X: access.X, Y: access.Y}, owner)
		if !reachable {
			continue
		}
		if _, deliverable := pathfind.FindLandPathForFaction(grid, buildings, pathfind.Point{X: access.X, Y: access.Y}, destPoint, owner); !deliverable {
			continue
		}
		if bestLen == -1 || len(p) < bestLen {
			warehouse, path, bestLen = w, p, len(p)
		}
	}
	return warehouse, path, warehouse != nil
}

// nearestReachableWarehouseOverLand is nearestReachableWarehouse's
// construction-delivery counterpart: both legs (serf to warehouse, warehouse
// to the site) are checked with pathfind.FindLandPathForFaction instead of
// the road-only pathfind.FindPath, since a site with no road to it yet must
// still be reachable -- that's the entire point of allowing this one job
// type off the road network. owner is this serf's own faction, same
// reasoning as nearestReachableWarehouseOverLandToPoint.
func nearestReachableWarehouseOverLand(grid *world.Grid, buildings []*building.Building, candidates []*building.Building, from pathfind.Point, dest *building.Building, owner int) (warehouse *building.Building, path []pathfind.Point, ok bool) {
	destAccess := dest.AccessPoint()
	destPoint := pathfind.Point{X: destAccess.X, Y: destAccess.Y}
	bestLen := -1
	for _, w := range candidates {
		access := w.AccessPoint()
		p, reachable := pathfind.FindLandPathForFaction(grid, buildings, from, pathfind.Point{X: access.X, Y: access.Y}, owner)
		if !reachable {
			continue
		}
		if _, deliverable := pathfind.FindLandPathForFaction(grid, buildings, pathfind.Point{X: access.X, Y: access.Y}, destPoint, owner); !deliverable {
			continue
		}
		if bestLen == -1 || len(p) < bestLen {
			warehouse, path, bestLen = w, p, len(p)
		}
	}
	return warehouse, path, warehouse != nil
}

func (c *Controller) startLeg(s *Serf, pickup, dropoff *building.Building, t resource.Type, amount int, path []pathfind.Point, ledger *reservations.Ledger) {
	s.pickup, s.dropoff = pickup, dropoff
	s.resource, s.amount = t, amount
	s.path, s.pathIdx, s.tileTicks = path, 0, 0
	s.ph = toPickup
	ledger.ReservePickup(pickup, t, amount)
	ledger.ReserveDropoff(dropoff, t, amount)
}

// startConstructionLeg is startLeg plus the one flag (Serf.construction)
// that lets arriveAtPickup route the second leg over open land instead of
// requiring a road. Construction material is the deliberate single
// exception to "serfs only walk the road network": a site with no road to
// it yet -- most importantly the very first road segment, which by
// definition has no finished road anywhere near it -- would otherwise be
// permanently unreachable by normal logistics. ReserveDropoff still applies
// normally; findConstructionSupplyJob passes the site's
// Building.ConstructionMaterialCost as the "target" reservations.Ledger.RoomFor
// expects, in place of the usual recipe input amount.
func (c *Controller) startConstructionLeg(s *Serf, pickup, dropoff *building.Building, t resource.Type, amount int, path []pathfind.Point, ledger *reservations.Ledger) {
	s.pickup, s.dropoff = pickup, dropoff
	s.resource, s.amount = t, amount
	s.path, s.pathIdx, s.tileTicks = path, 0, 0
	s.ph = toPickup
	s.construction = true
	ledger.ReservePickup(pickup, t, amount)
	ledger.ReserveDropoff(dropoff, t, amount)
}

// startSoldierLeg begins a food delivery to a hungry soldier: pickup is a
// Warehouse (always -- there's no per-building buffer to draw from, unlike
// every other job), target/food/amount describe what's being carried, and
// dropoff is deliberately left nil (see arriveAtDropoff's soldierTarget
// case). Only the pickup side is reserved in ledger; nothing building-side
// needs reserving for the second leg, and claimedSoldiers (mutated by the
// caller) is what stops a second serf from picking the same soldier this
// tick.
func (c *Controller) startSoldierLeg(s *Serf, pickup *building.Building, target *soldier.Soldier, food resource.Type, path []pathfind.Point, ledger *reservations.Ledger) {
	const amount = 1
	s.pickup, s.dropoff = pickup, nil
	s.soldierTarget = target
	s.soldierTargetPoint = pathfind.Point{X: target.X, Y: target.Y}
	s.resource, s.amount = food, amount
	s.path, s.pathIdx, s.tileTicks = path, 0, 0
	s.ph = toPickup
	ledger.ReservePickup(pickup, food, amount)
}

func (c *Controller) advance(s *Serf, grid *world.Grid, buildings []*building.Building, obstacles []*building.Building, stock *resource.Stockpile) {
	if s.ph == idle || len(s.path) == 0 {
		return
	}

	s.tileTicks++
	if s.tileTicks < TicksPerTile {
		return
	}
	s.tileTicks = 0

	if s.pathIdx < len(s.path)-1 {
		s.pathIdx++
		s.X, s.Y = s.path[s.pathIdx].X, s.path[s.pathIdx].Y
		return
	}

	switch s.ph {
	case toPickup:
		c.arriveAtPickup(s, grid, buildings, obstacles, stock)
	case toDropoff:
		c.arriveAtDropoff(s, grid, obstacles, stock)
	case returning:
		s.reset()
	}
}

func (c *Controller) arriveAtPickup(s *Serf, grid *world.Grid, buildings []*building.Building, obstacles []*building.Building, stock *resource.Stockpile) {
	s.atBuilding = s.pickup

	if s.eating {
		// No dropoff leg for a meal -- eat (or miss out, if someone beat
		// us to the last loaf) and go idle right here at the Tavern.
		if s.pickup.TakeInput(s.resource, s.amount) {
			s.ticksSinceMeal = 0
		}
		s.reset()
		return
	}

	var ok bool
	if s.pickup.IsOperationalWarehouse() {
		ok = stock.Remove(s.resource, s.amount)
	} else {
		ok = s.pickup.TakeOutput(s.resource, s.amount)
	}
	if !ok {
		// The reservation ledger should make this unreachable in the
		// normal case; kept as a defensive fallback (e.g. the map
		// changed mid-trip) -- go idle and try a fresh job next tick.
		s.reset()
		return
	}

	var path []pathfind.Point
	var found bool
	switch {
	case s.soldierTarget != nil:
		path, found = pathfind.FindLandPathForFaction(grid, obstacles, pathfind.Point{X: s.X, Y: s.Y}, s.soldierTargetPoint, c.Warehouse.Owner)
	case s.construction:
		dest := s.dropoff.AccessPoint()
		path, found = pathfind.FindLandPathForFaction(grid, obstacles, pathfind.Point{X: s.X, Y: s.Y}, pathfind.Point{X: dest.X, Y: dest.Y}, c.Warehouse.Owner)
	default:
		path, found = pathfind.FindPath(buildings, s.pickup, s.dropoff)
	}
	if !found {
		// Road got cut (or, for a construction delivery, the site became
		// unreachable over land) after the job was assigned. Return the
		// goods rather than lose them, then give up on the job.
		if s.pickup.IsOperationalWarehouse() {
			stock.Add(s.resource, s.amount)
		} else {
			s.pickup.AddOutput(s.resource, s.amount)
		}
		if s.construction && s.dropoff != nil {
			// Without this, the very next idle serf immediately
			// re-discovers this exact same doomed job and repeats the
			// failure forever -- see constructionBackoff's doc comment.
			c.constructionBackoff[s.dropoff] = constructionBackoffTicks
		}
		s.reset()
		return
	}
	s.path, s.pathIdx, s.tileTicks = path, 0, 0
	s.ph = toDropoff
}

func (c *Controller) arriveAtDropoff(s *Serf, grid *world.Grid, obstacles []*building.Building, stock *resource.Stockpile) {
	s.atBuilding = s.dropoff
	// Captured before the switch below: every case except reset()s at the
	// very end, and s.construction/s.soldierTarget still hold the values
	// this trip started with until then.
	offRoad := s.construction || s.soldierTarget != nil
	switch {
	case s.soldierTarget != nil:
		// Food handed straight to the soldier rather than deposited into a
		// building's InputBuffer. If it died, or another serf's delivery
		// already fed it, in the meantime, the unit's worth of food goes
		// back to the stockpile instead of silently vanishing.
		if s.soldierTarget.Alive() {
			s.soldierTarget.Feed()
		} else {
			stock.Add(s.resource, s.amount)
		}
	case s.dropoff.IsOperationalWarehouse():
		stock.Add(s.resource, s.amount)
	case s.construction:
		fit := s.dropoff.AddConstructionMaterial(s.resource, s.amount)
		if leftover := s.amount - fit; leftover > 0 {
			stock.Add(s.resource, leftover)
		}
	default:
		fit := s.dropoff.AddInput(s.resource, s.amount)
		if leftover := s.amount - fit; leftover > 0 {
			// The reservation ledger should prevent this in the normal
			// case, but if a race still slips through (the destination
			// changed underfoot, or a save/load edge case), don't
			// destroy the goods -- credit the shared stockpile instead,
			// exactly like a cancelled or road-cut delivery already does.
			stock.Add(s.resource, leftover)
		}
	}
	if offRoad {
		c.startReturnToRoad(s, grid, obstacles)
		return
	}
	s.reset()
}

// startReturnToRoad walks a serf that just finished an off-road delivery
// back to the nearest reachable warehouse over open land
// (pathfind.FindLandPathForFaction), so it lands back on the road network
// before going properly idle -- see the returning phase's doc comment for
// why this exists. Best-effort: if no warehouse is reachable at all (e.g.
// a freshly cut-off map), the serf simply goes idle right where it stands
// rather than loop forever chasing an impossible route. buildings here is
// actually the caller's obstacles (whole-map) list -- see Controller.
// Tick's doc comment on why an off-road leg needs that, not the
// per-faction one.
func (c *Controller) startReturnToRoad(s *Serf, grid *world.Grid, buildings []*building.Building) {
	from := pathfind.Point{X: s.X, Y: s.Y}
	s.pickup, s.dropoff = nil, nil
	s.resource, s.amount = 0, 0
	s.construction, s.soldierTarget = false, nil

	bestLen := -1
	var bestPath []pathfind.Point
	for _, w := range c.warehouses() {
		access := w.AccessPoint()
		p, reachable := pathfind.FindLandPathForFaction(grid, buildings, from, pathfind.Point{X: access.X, Y: access.Y}, c.Warehouse.Owner)
		if !reachable {
			continue
		}
		if bestLen == -1 || len(p) < bestLen {
			bestPath, bestLen = p, len(p)
		}
	}
	if bestLen == -1 {
		s.ph = idle
		s.path, s.pathIdx, s.tileTicks = nil, 0, 0
		return
	}
	s.path, s.pathIdx, s.tileTicks = bestPath, 0, 0
	s.ph = returning
}
