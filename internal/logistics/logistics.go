// Package logistics simulates serfs: workers that walk the road network
// (see package pathfind) carrying goods between producer/consumer
// buildings and the warehouse. This is the piece the MVP plan
// deliberately deferred -- see AGENTS.md -- now filled in because it's
// core to how the reference genre actually plays: a building not
// connected by road simply never gets serviced.
//
// Job queue order (see assign): keeping the Tavern fed comes first, then a
// direct producer -> consumer haul (e.g. Mill's Flour straight to
// Bakery), then draining leftover OutputBuffer to the Warehouse, then
// pulling from the Warehouse to cover a shortage no producer can. This
// matches the reference behavior the user asked for: processing
// buildings feed each other directly, the Warehouse is overflow/backup,
// not the only path.
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
// reachable one where more than one qualifies (see nearestTavernWithBread,
// nearestReachableWarehouse) -- see docs/DEVELOPMENT.md's "Job queue
// rules" section for the full writeup.
package logistics

import (
	"slices"

	"strategy_game/internal/building"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
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

	// HungerInterval is how many simulation ticks a serf can go between
	// meals before heading to the Tavern between jobs (never mid-haul --
	// see tryStartMeal). At normal speed this is about 90 seconds.
	HungerInterval = 180
)

type phase int

const (
	idle phase = iota
	toPickup
	toDropoff
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

	ticksSinceMeal int

	// Starving is true once HungerInterval has passed with nowhere to
	// actually go eat (no Tavern yet, no road to one, or all food is gone).
	// A starving serf keeps hauling rather than stand idle
	// forever -- see tryStartMeal's doc comment for why.
	Starving bool
}

// State is the public, read-only activity state used by the inspector and
// render layer. The movement bookkeeping itself remains private to this
// package.
type State int

const (
	SerfIdle State = iota
	SerfToPickup
	SerfToDropoff
)

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

// DropoffBuilding returns the current job's destination, if any.
func (s *Serf) DropoffBuilding() *building.Building {
	return s.dropoff
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

// Busy reports whether the serf is currently walking a job, for
// rendering (e.g. a different color for working vs. idle serfs).
func (s *Serf) Busy() bool {
	return s.ph != idle
}

func (s *Serf) reset() {
	s.ph = idle
	s.path = nil
	s.pathIdx = 0
	s.tileTicks = 0
	s.pickup, s.dropoff = nil, nil
	s.amount = 0
	s.eating = false
}

// Controller owns every serf and the warehouse they work out of.
type Controller struct {
	Warehouse  *building.Building // primary spawn/root warehouse
	Warehouses []*building.Building
	Serfs      []*Serf
}

// NewController spawns count serfs standing at the warehouse.
func NewController(warehouse *building.Building, count int) *Controller {
	c := &Controller{Warehouse: warehouse, Warehouses: []*building.Building{warehouse}}
	for range count {
		c.Hire()
	}
	return c
}

// AddWarehouse registers another physical warehouse as a valid logistics
// endpoint. All warehouses share the same unlimited town stockpile, while the
// primary warehouse remains the spawn point for newly hired serfs.
func (c *Controller) AddWarehouse(warehouse *building.Building) {
	if warehouse == nil || warehouse.Kind != building.Warehouse {
		return
	}
	for _, existing := range c.Warehouses {
		if existing == warehouse {
			return
		}
	}
	c.Warehouses = append(c.Warehouses, warehouse)
}

func (c *Controller) warehouses() []*building.Building {
	if len(c.Warehouses) > 0 {
		return c.Warehouses
	}
	if c.Warehouse != nil {
		return []*building.Building{c.Warehouse}
	}
	return nil
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
func (c *Controller) RestoreSerf(x, y, hungerTicks int, starving bool) *Serf {
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	s := &Serf{X: x, Y: y, ticksSinceMeal: hungerTicks, Starving: starving}
	c.Serfs = append(c.Serfs, s)
	return s
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
// simulation tick when the Tavern's Bread is scarce -- the unit that's
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
		if s.ph != idle || s.ticksSinceMeal < HungerInterval {
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
// own pre-existing in-flight units.
func (c *Controller) Tick(buildings []*building.Building, stock *resource.Stockpile, ledger *reservations.Ledger) {
	for _, s := range c.Serfs {
		// Deliberately uncapped: see MaxWaitingHunger's doc comment for
		// why this must keep counting past HungerInterval instead of
		// saturating there. HungerTicks() still returns the true value;
		// UI code clamps it for the "N/HungerInterval" display.
		s.ticksSinceMeal++
		if s.ph == idle {
			if !c.tryStartMeal(s, buildings, ledger) {
				c.assign(s, buildings, stock, ledger)
			}
		}
		c.advance(s, buildings, stock)
	}
}

// nearestTavernWithFood returns the nearest reachable Tavern with any food.
// Bread, fish, wine and sausage are interchangeable meals; the stable
// resource order only makes job selection reproducible.
func nearestTavernWithFood(buildings []*building.Building, from pathfind.Point, ledger *reservations.Ledger) (tavern *building.Building, meal resource.Type, path []pathfind.Point, ok bool) {
	bestLen := -1
	for _, b := range buildings {
		if b.Kind != building.Tavern {
			continue
		}
		for _, food := range resource.FoodTypes() {
			if ledger != nil && ledger.AvailableInput(b, food) <= 0 {
				continue
			}
			p, reachable := pathfind.FindPathFromPoint(buildings, from, b)
			if !reachable {
				continue
			}
			if bestLen == -1 || len(p) < bestLen {
				tavern, meal, path, bestLen = b, food, p, len(p)
			}
		}
	}
	return tavern, meal, path, tavern != nil
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
// all of them out of Bread once other units' claims are accounted for),
// it's marked Starving but keeps hauling anyway -- refusing to work would
// cripple the whole economy before a Tavern even exists, which is a
// worse outcome than a hungry serf.
func (c *Controller) tryStartMeal(s *Serf, buildings []*building.Building, ledger *reservations.Ledger) bool {
	if s.ticksSinceMeal < HungerInterval {
		return false
	}
	tavern, meal, path, ok := nearestTavernWithFood(buildings, pathfind.Point{X: s.X, Y: s.Y}, ledger)
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
// reach, in priority order: keep the Tavern supplied first, then direct
// producer->consumer haul, then drain leftover OutputBuffer to the Warehouse,
// then pull from the Warehouse to cover a shortage no producer can.
func (c *Controller) assign(s *Serf, buildings []*building.Building, stock *resource.Stockpile, ledger *reservations.Ledger) {
	from := pathfind.Point{X: s.X, Y: s.Y}
	if pickup, dropoff, t, n, path, ok := findTavernSupplyJob(buildings, c.warehouses(), stock, ledger, from); ok {
		c.startLeg(s, pickup, dropoff, t, n, path, ledger)
		return
	}
	if pickup, dropoff, t, n, path, ok := findDirectJob(buildings, c.Warehouse, ledger, from); ok {
		c.startLeg(s, pickup, dropoff, t, n, path, ledger)
		return
	}
	if b, t, n, path, ok := findCollectJob(buildings, c.Warehouse, ledger, from); ok {
		// The pickup (b) is already confirmed reachable from the serf;
		// only the dropoff warehouse still needs picking, from wherever
		// b itself can reach.
		bAccess := b.AccessPoint()
		if warehouse, _, ok := nearestReachableWarehouse(buildings, c.warehouses(), pathfind.Point{X: bAccess.X, Y: bAccess.Y}); ok {
			c.startLeg(s, b, warehouse, t, n, path, ledger)
			return
		}
	}
	if b, t, n, ok := findSupplyJob(buildings, stock, ledger); ok {
		// Here it's the pickup side (which warehouse) that varies, so pick
		// whichever registered warehouse is actually nearest the serf
		// right now AND can still reach b -- not just the one nearest the
		// serf.
		if warehouse, path, ok := nearestReachableWarehouseTo(buildings, c.warehouses(), from, b); ok {
			c.startLeg(s, warehouse, b, t, n, path, ledger)
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
			for _, producer := range buildings {
				if producer.Kind == building.Warehouse || producer.Kind == building.Road || producer.Kind == building.Tree {
					continue
				}
				have := ledger.AvailableOutput(producer, rt)
				if have <= 0 {
					continue
				}
				p, reachable := pathfind.FindPathFromPoint(buildings, from, producer)
				if !reachable {
					continue // not reachable from here right now -- try the next source
				}
				if _, deliverable := pathfind.FindPath(buildings, producer, tavern); !deliverable {
					continue // can't actually deliver from here to this Tavern -- try the next source
				}
				return producer, tavern, rt, min(have, CarryCapacity, room), p, true
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
// one.
func findDirectJob(buildings []*building.Building, warehouse *building.Building, ledger *reservations.Ledger, from pathfind.Point) (pickup, dropoff *building.Building, t resource.Type, amount int, path []pathfind.Point, ok bool) {
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
			for _, consumer := range buildings {
				if consumer == warehouse || consumer == producer || consumer.Kind == building.Road || consumer.Kind == building.Tree {
					continue
				}
				need, wants := building.Types[consumer.Kind].Recipe.Inputs[rt]
				if !wants {
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
				if _, deliverable := pathfind.FindPath(buildings, producer, consumer); !deliverable {
					continue // this producer can't actually deliver to this consumer -- try the next consumer
				}
				return producer, consumer, rt, amt, p, true
			}
		}
	}
	return nil, nil, 0, 0, nil, false
}

// findCollectJob looks for any building with surplus output to drain to a
// warehouse. Resource types are visited in a fixed sorted order, and each
// candidate is checked for reachability before being accepted.
func findCollectJob(buildings []*building.Building, warehouse *building.Building, ledger *reservations.Ledger, from pathfind.Point) (b *building.Building, t resource.Type, amount int, path []pathfind.Point, ok bool) {
	for _, cand := range buildings {
		if cand == warehouse || cand.Kind == building.Road || cand.Kind == building.Tree {
			continue
		}
		for _, rt := range sortedResourceTypes(cand.OutputBuffer) {
			n := ledger.AvailableOutput(cand, rt)
			if n <= 0 {
				continue
			}
			p, reachable := pathfind.FindPathFromPoint(buildings, from, cand)
			if !reachable {
				continue // not reachable from here right now -- try the next candidate
			}
			return cand, rt, min(n, CarryCapacity), p, true
		}
	}
	return nil, 0, 0, nil, false
}

// findSupplyJob looks for a building falling short on an input that the
// Warehouse could actually cover right now. It deliberately doesn't pick a
// specific warehouse or check reachability itself -- assign tries every
// registered warehouse as the pickup point, since which one is actually
// reachable depends on the serf's position, not on which building is short
// on input. Resource types are visited in a fixed sorted order for the
// same reason as the other job searches.
//
// A shortage is only accepted if stock actually has some of that resource
// available (per the shared ledger): a building short on a resource
// nobody has any of yet must not block trying the next shortage, whether
// that's a different resource at the same building or a different
// building entirely.
func findSupplyJob(buildings []*building.Building, stock *resource.Stockpile, ledger *reservations.Ledger) (b *building.Building, t resource.Type, amount int, ok bool) {
	for _, cand := range buildings {
		if cand.Kind == building.Warehouse || cand.Kind == building.Road || cand.Kind == building.Tree {
			continue
		}
		recipe := building.Types[cand.Kind].Recipe
		for _, rt := range sortedResourceTypes(recipe.Inputs) {
			short := ledger.RoomFor(cand, rt, recipe.Inputs[rt])
			if short <= 0 {
				continue
			}
			amount := min(CarryCapacity, short, ledger.AvailableStock(stock, rt))
			if amount <= 0 {
				continue // nothing in stock for this shortage -- try the next one
			}
			return cand, rt, amount, true
		}
	}
	return nil, 0, 0, false
}

func (c *Controller) startLeg(s *Serf, pickup, dropoff *building.Building, t resource.Type, amount int, path []pathfind.Point, ledger *reservations.Ledger) {
	s.pickup, s.dropoff = pickup, dropoff
	s.resource, s.amount = t, amount
	s.path, s.pathIdx, s.tileTicks = path, 0, 0
	s.ph = toPickup
	ledger.ReservePickup(pickup, t, amount)
	ledger.ReserveDropoff(dropoff, t, amount)
}

func (c *Controller) advance(s *Serf, buildings []*building.Building, stock *resource.Stockpile) {
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
		c.arriveAtPickup(s, buildings, stock)
	case toDropoff:
		c.arriveAtDropoff(s, stock)
	}
}

func (c *Controller) arriveAtPickup(s *Serf, buildings []*building.Building, stock *resource.Stockpile) {
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
	if s.pickup.Kind == building.Warehouse {
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

	path, found := pathfind.FindPath(buildings, s.pickup, s.dropoff)
	if !found {
		// Road got cut after the job was assigned. Return the goods
		// rather than lose them, then give up on the job.
		if s.pickup.Kind == building.Warehouse {
			stock.Add(s.resource, s.amount)
		} else {
			s.pickup.AddOutput(s.resource, s.amount)
		}
		s.reset()
		return
	}
	s.path, s.pathIdx, s.tileTicks = path, 0, 0
	s.ph = toDropoff
}

func (c *Controller) arriveAtDropoff(s *Serf, stock *resource.Stockpile) {
	s.atBuilding = s.dropoff
	if s.dropoff.Kind == building.Warehouse {
		stock.Add(s.resource, s.amount)
	} else {
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
	s.reset()
}
