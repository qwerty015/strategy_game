// Package logistics simulates serfs: workers that walk the road network
// (see package pathfind) carrying goods between producer/consumer
// buildings and the warehouse. This is the piece the MVP plan
// deliberately deferred -- see AGENTS.md -- now filled in because it's
// core to how the reference genre actually plays: a building not
// connected by road simply never gets serviced.
//
// Everything routes through the warehouse (collect: building ->
// warehouse, supply: warehouse -> building). There's no direct
// building-to-building hauling; that keeps job assignment a simple
// single pass instead of a full transport-matching problem, at the cost
// of some realism/efficiency that could be added later.
package logistics

import (
	"strategy_game/internal/building"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/resource"
)

const (
	// CarryCapacity is how many units of one resource a serf hauls per
	// trip -- matches a typical recipe batch size, so one full
	// production cycle usually means one trip.
	CarryCapacity = 5

	// TicksPerTile is how many simulation ticks it takes a serf to
	// cross one tile of road.
	TicksPerTile = 2
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
}

// Controller owns every serf and the warehouse they work out of.
type Controller struct {
	Warehouse *building.Building
	Serfs     []*Serf
}

// NewController spawns count serfs standing at the warehouse.
func NewController(warehouse *building.Building, count int) *Controller {
	c := &Controller{Warehouse: warehouse}
	for range count {
		c.Serfs = append(c.Serfs, &Serf{X: warehouse.X, Y: warehouse.Y, atBuilding: warehouse})
	}
	return c
}

// Tick assigns jobs to idle serfs and advances every serf by one
// movement step. Call once per simulation tick (see economy.Simulator).
func (c *Controller) Tick(buildings []*building.Building, stock *resource.Stockpile) {
	for _, s := range c.Serfs {
		if s.ph == idle {
			c.assign(s, buildings, stock)
		}
		c.advance(s, buildings, stock)
	}
}

// assign gives an idle serf a job, if one exists that it can currently
// reach. Draining a producer's OutputBuffer takes priority over
// supplying a consumer, so a full buffer (which stalls production)
// clears before starting new deliveries.
func (c *Controller) assign(s *Serf, buildings []*building.Building, stock *resource.Stockpile) {
	if b, t, n, ok := findCollectJob(buildings, c.Warehouse); ok {
		c.startLeg(s, b, c.Warehouse, t, n, buildings)
		return
	}
	if b, t, n, ok := findSupplyJob(buildings, c.Warehouse, stock); ok {
		c.startLeg(s, c.Warehouse, b, t, n, buildings)
	}
}

func findCollectJob(buildings []*building.Building, warehouse *building.Building) (b *building.Building, t resource.Type, amount int, ok bool) {
	for _, cand := range buildings {
		if cand == warehouse || cand.Kind == building.Road {
			continue
		}
		for rt, n := range cand.OutputBuffer {
			if n > 0 {
				return cand, rt, min(n, CarryCapacity), true
			}
		}
	}
	return nil, 0, 0, false
}

func findSupplyJob(buildings []*building.Building, warehouse *building.Building, stock *resource.Stockpile) (b *building.Building, t resource.Type, amount int, ok bool) {
	for _, cand := range buildings {
		if cand == warehouse || cand.Kind == building.Road {
			continue
		}
		recipe := building.Types[cand.Kind].Recipe
		for rt, need := range recipe.Inputs {
			have := cand.InputBuffer[rt]
			if have >= need {
				continue
			}
			amount := min(CarryCapacity, need-have, stock.Amount(rt))
			if amount > 0 {
				return cand, rt, amount, true
			}
		}
	}
	return nil, 0, 0, false
}

func (c *Controller) startLeg(s *Serf, pickup, dropoff *building.Building, t resource.Type, amount int, buildings []*building.Building) {
	path, ok := pathfind.FindPath(buildings, s.atBuilding, pickup)
	if !ok {
		return // not reachable from here right now; try again next tick
	}
	s.pickup, s.dropoff = pickup, dropoff
	s.resource, s.amount = t, amount
	s.path, s.pathIdx, s.tileTicks = path, 0, 0
	s.ph = toPickup
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

	var ok bool
	if s.pickup == c.Warehouse {
		ok = stock.Remove(s.resource, s.amount)
	} else {
		ok = s.pickup.TakeOutput(s.resource, s.amount)
	}
	if !ok {
		// Another serf beat us to it (see assign's doc comment); go idle
		// and try a fresh job next tick.
		s.reset()
		return
	}

	path, found := pathfind.FindPath(buildings, s.pickup, s.dropoff)
	if !found {
		// Road got cut after the job was assigned. Return the goods
		// rather than lose them, then give up on the job.
		if s.pickup == c.Warehouse {
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
	if s.dropoff == c.Warehouse {
		stock.Add(s.resource, s.amount)
	} else {
		s.dropoff.AddInput(s.resource, s.amount)
	}
	s.reset()
}
