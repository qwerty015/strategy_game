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
	// between meals before heading to the Tavern.
	HungerInterval = 40

	// TicksPerTile is how many simulation ticks it takes a villager to
	// cross one tile of road, walking to/from the Tavern.
	TicksPerTile = 2
)

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

	ticksSinceMeal int

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

// Tick advances hunger and movement for every villager. Call once per
// simulation tick.
func (c *Controller) Tick(buildings []*building.Building) {
	tavern := findTavern(buildings)
	for _, v := range c.Villagers {
		tick(v, buildings, tavern)
	}
}

func findTavern(buildings []*building.Building) *building.Building {
	for _, b := range buildings {
		if b.Kind == building.Tavern {
			return b
		}
	}
	return nil
}

func tick(v *Villager, buildings []*building.Building, tavern *building.Building) {
	switch v.ph {
	case working:
		tickWorking(v, buildings, tavern)
	case toTavern, toHome:
		tickWalking(v, buildings, tavern)
	}
}

func tickWorking(v *Villager, buildings []*building.Building, tavern *building.Building) {
	if v.ticksSinceMeal < HungerInterval {
		v.ticksSinceMeal++
		return
	}

	// Hungry enough to need a meal now.
	if tavern == nil || tavern.InputBuffer[resource.Bread] <= 0 {
		v.Starving = true
		return
	}
	path, ok := pathfind.FindPath(buildings, v.Home, tavern)
	if !ok {
		v.Starving = true
		return
	}
	v.Starving = false
	v.path, v.pathIdx, v.tileTicks = path, 0, 0
	v.ph = toTavern
}

func tickWalking(v *Villager, buildings []*building.Building, tavern *building.Building) {
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
		// If home is somehow unreachable from here, just stay put at
		// the tavern; tickWorking will never run again until the road
		// is fixed, but that's an edge case not worth over-engineering.
		return
	}

	v.X, v.Y = v.Home.X, v.Home.Y
	v.ph = working
}
