// Package villagers simulates Farmer and Baker units. Unlike serfs
// (package logistics) they don't haul goods -- they stand and work at
// one building (Home) -- but per the user's request they get the same
// treatment as every other unit "as in the reference game": they get
// hungry and must walk to a Tavern to eat, or their building's work
// stalls (see the starving map economy.Tick takes).
package villagers

import (
	"strategy_game/internal/building"
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
)

const (
	// HungerInterval is how many simulation ticks a villager can go
	// between meals before heading to the Tavern. At normal speed this
	// is about 90 seconds, so eating does not dominate the work cycle.
	HungerInterval = 180

	// TicksPerTile is how many simulation ticks it takes a villager to
	// cross one tile of road, walking to/from the Tavern.
	TicksPerTile = 2

	// FarmWorkStepTicks controls the deliberately slow visible work loop.
	// A farmer changes field cells every four seconds at normal speed, so the
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

// Villager is a Farmer or Baker.
type Villager struct {
	Profession Profession
	Home       *building.Building

	X, Y int

	ph        phase
	path      []pathfind.Point
	pathIdx   int
	tileTicks int

	tavern *building.Building // which Tavern this trip is headed to/from, while ph != working

	ticksSinceMeal int
	workTicks      int

	// Starving is true once HungerInterval has passed and there was
	// nowhere to actually go eat (no Tavern built yet, no road to one,
	// or it's out of Bread) -- exported so cmd/game can build the
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

// HomeBuilding returns the building where this villager works.
func (v *Villager) HomeBuilding() *building.Building {
	return v.Home
}

// NewVillager creates a Villager standing at home, working.
func NewVillager(profession Profession, home *building.Building) *Villager {
	return &Villager{Profession: profession, Home: home, X: home.X, Y: home.Y}
}

// Working reports whether the villager is at its post right now, as
// opposed to out walking to/from a meal.
func (v *Villager) Working() bool {
	return v.ph == working
}

// VisibleOnMap reports whether the unit should be drawn as a person. Farmers
// remain visible while they tend their field; bakers still use the compact
// worker marker while working inside their bakery. Both professions appear
// as units when walking to or from the Tavern.
func (v *Villager) VisibleOnMap() bool {
	return !v.Working() || (v.Profession == Farmer && v.Home != nil && v.Home.Kind == building.Farm)
}

// Controller owns every Farmer/Baker in town.
type Controller struct {
	Villagers []*Villager
}

// NewController creates an empty roster; use Spawn to add villagers as
// their buildings get placed.
func NewController() *Controller {
	return &Controller{}
}

// Spawn adds a villager working at home (a Farm for Farmer, a Bakery
// for Baker).
func (c *Controller) Spawn(profession Profession, home *building.Building) {
	c.Villagers = append(c.Villagers, NewVillager(profession, home))
}

// RestoreVillager recreates a worker from a save snapshot. Unlike a new
// Spawn, it keeps the saved map position and hunger state. If the worker was
// walking to eat when saved, its route is rebuilt from that exact position;
// the old tile-by-tile path itself is not part of the save format.
func (c *Controller) RestoreVillager(profession Profession, home *building.Building, x, y, hungerTicks int, starving bool, state State, buildings []*building.Building) *Villager {
	v := NewVillager(profession, home)
	v.X, v.Y = x, y
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	v.ticksSinceMeal = hungerTicks
	v.Starving = starving

	switch state {
	case VillagerToTavern:
		// No ledger exists yet at load time, so this searches purely by
		// reachability (nil ledger skips the Bread-availability check) --
		// same as the original findTavern-based lookup it replaces.
		if tavern, path, ok := nearestTavernWithBread(buildings, home, nil); ok {
			v.path, v.pathIdx, v.tileTicks = path, 0, 0
			v.tavern = tavern
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

// Reserve seeds ledger with every villager currently walking to eat, so
// other controllers sharing a Tavern (serfs, lumberjacks) see this claim
// before making their own commitments this tick. Call once per
// simulation tick, before this or any other controller's Tick runs.
func (c *Controller) Reserve(ledger *reservations.Ledger) {
	for _, v := range c.Villagers {
		if v.ph == toTavern && v.tavern != nil {
			ledger.ReservePickup(v.tavern, resource.Bread, 1)
		}
	}
}

// MaxWaitingHunger returns the highest HungerTicks among villagers that
// will actually try to eat this tick (working and HungerTicks >=
// HungerInterval), or -1 if none will. cmd/game compares this against the
// other unit controllers' MaxWaitingHunger to decide whose Tick runs
// first this simulation tick when the Tavern's Bread is scarce -- the
// unit that's been waiting longest gets first claim, instead of
// whichever controller happens to be first in a fixed call order.
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
func (c *Controller) Tick(buildings []*building.Building, ledger *reservations.Ledger) {
	for _, v := range c.Villagers {
		tick(v, buildings, ledger)
	}
}

// nearestTavernWithBread returns the Tavern reachable from home by the
// shortest road path that also has at least one unit of Bread available,
// or false if none qualifies. A town can have several Taverns; without
// this, every hungry villager always walked to whichever one happened to
// be first in the buildings slice, even if a closer one existed or the
// first one was simply empty. Pass a nil ledger to search purely by
// reachability (used when rebuilding a route from a save, before any
// ledger exists for this tick).
func nearestTavernWithBread(buildings []*building.Building, home *building.Building, ledger *reservations.Ledger) (tavern *building.Building, path []pathfind.Point, ok bool) {
	bestLen := -1
	for _, b := range buildings {
		if b.Kind != building.Tavern {
			continue
		}
		if ledger != nil && ledger.AvailableInput(b, resource.Bread) <= 0 {
			continue
		}
		p, reachable := pathfind.FindPath(buildings, home, b)
		if !reachable {
			continue
		}
		if bestLen == -1 || len(p) < bestLen {
			tavern, path, bestLen = b, p, len(p)
		}
	}
	return tavern, path, tavern != nil
}

func tick(v *Villager, buildings []*building.Building, ledger *reservations.Ledger) {
	switch v.ph {
	case working:
		tickWorking(v, buildings, ledger)
	case toTavern, toHome:
		tickWalking(v, buildings)
	}
}

func tickWorking(v *Villager, buildings []*building.Building, ledger *reservations.Ledger) {
	if v.ticksSinceMeal < HungerInterval {
		v.ticksSinceMeal++
		v.animateFarmWork()
		return
	}

	// Hungry enough to need a meal now.
	tavern, path, ok := nearestTavernWithBread(buildings, v.Home, ledger)
	if !ok {
		v.Starving = true
		v.animateFarmWork()
		return
	}
	v.Starving = false
	// The road network starts at the farmhouse access tile. The farmer has
	// just finished the current field pass, so visually return to that tile
	// before starting the meal route.
	v.X, v.Y = v.Home.X, v.Home.Y
	v.tavern = tavern
	v.path, v.pathIdx, v.tileTicks = path, 0, 0
	v.ph = toTavern
	ledger.ReservePickup(tavern, resource.Bread, 1)
}

func (v *Villager) animateFarmWork() {
	if v.Profession != Farmer || v.Home == nil || v.Home.Kind != building.Farm {
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
		if tavern.TakeInput(resource.Bread, 1) {
			v.ticksSinceMeal = 0
		}
		// Whether or not there was still bread by the time we arrived,
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
