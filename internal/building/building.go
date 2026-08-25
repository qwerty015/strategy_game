// Package building defines building types (as data: footprint, terrain
// rules, production recipe) and placed Building instances. Adding a new
// production chain later means adding entries to Types, not new logic.
package building

import (
	"slices"

	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// Kind identifies a type of building.
type Kind int

const (
	Farm Kind = iota
	Mill
	Bakery

	// Warehouse is the town's single logistics hub: serfs (see package
	// logistics) deposit collected goods here and draw from it to supply
	// other buildings. It has no Recipe -- it produces nothing itself.
	Warehouse

	// Road is not really a "building" gameplay-wise, but reusing the
	// Building/placement machinery for it (1x1 footprint, buildable
	// anywhere) means the whole grid/overlap/save system already works
	// for roads for free. Serfs may only walk across Road tiles and the
	// footprints of the two buildings they're travelling between --
	// buildings not connected to the road network by an unbroken chain
	// of Road tiles can't be serviced. See package pathfind.
	Road
)

// Recipe describes how a building turns raw resources into a product
// over a number of simulation ticks. An empty Inputs map means the
// building gathers from the land instead of consuming stockpiled goods
// (e.g. a Farm).
type Recipe struct {
	Inputs         map[resource.Type]int
	Output         resource.Type
	OutputAmount   int
	TicksToProduce int
}

// Type describes a kind of building: how big it is, what terrain it can
// sit on, and what it produces.
type Type struct {
	Kind      Kind
	Name      string
	Footprint int // buildings are Footprint x Footprint tiles

	// AllowedTerrain lists the terrain types this building may be placed
	// on. An empty slice means "any buildable (non-water) terrain".
	AllowedTerrain []world.TerrainType

	Recipe Recipe
}

// CanBuildOn reports whether a single tile satisfies this building
// type's terrain requirement. It does not check footprint or overlap;
// see CanPlace for the full placement check.
func (t Type) CanBuildOn(tile world.Tile) bool {
	if !tile.Buildable() {
		return false
	}
	if len(t.AllowedTerrain) == 0 {
		return true
	}
	return slices.Contains(t.AllowedTerrain, tile.Terrain)
}

// BufferCapacity caps how many units of any single resource type a
// building can hold in its InputBuffer or OutputBuffer. Small on
// purpose: a producer with a full OutputBuffer pauses production until a
// serf clears space, which is what makes logistics (roads + serfs)
// matter instead of being invisible plumbing.
const BufferCapacity = 10

// Building is a placed instance of a Type on the grid.
//
// Buildings do not talk to the shared warehouse stockpile directly --
// see AGENTS.md. A producer accumulates its product in OutputBuffer; a
// consumer draws its raw materials from InputBuffer. Moving goods
// between these buffers and the warehouse is entirely the job of serfs
// (package logistics), which is why a building disconnected from the
// road network simply stalls instead of producing.
type Building struct {
	Kind          Kind
	X, Y          int // top-left tile of the footprint
	ProgressTicks int // ticks accumulated toward the current production cycle

	InputBuffer  map[resource.Type]int
	OutputBuffer map[resource.Type]int
}

// AddOutput deposits up to n units of t into OutputBuffer, capped by
// BufferCapacity, and returns how many units actually fit.
func (b *Building) AddOutput(t resource.Type, n int) int {
	return addCapped(&b.OutputBuffer, t, n)
}

// TakeOutput removes n units of t from OutputBuffer if available
// (all-or-nothing) and reports whether it succeeded.
func (b *Building) TakeOutput(t resource.Type, n int) bool {
	return takeAvailable(b.OutputBuffer, t, n)
}

// AddInput deposits up to n units of t into InputBuffer, capped by
// BufferCapacity, and returns how many units actually fit.
func (b *Building) AddInput(t resource.Type, n int) int {
	return addCapped(&b.InputBuffer, t, n)
}

// TakeInput removes n units of t from InputBuffer if available
// (all-or-nothing) and reports whether it succeeded.
func (b *Building) TakeInput(t resource.Type, n int) bool {
	return takeAvailable(b.InputBuffer, t, n)
}

func addCapped(buf *map[resource.Type]int, t resource.Type, n int) int {
	if n <= 0 {
		return 0
	}
	if *buf == nil {
		*buf = make(map[resource.Type]int)
	}
	room := BufferCapacity - (*buf)[t]
	if room <= 0 {
		return 0
	}
	if n > room {
		n = room
	}
	(*buf)[t] += n
	return n
}

func takeAvailable(buf map[resource.Type]int, t resource.Type, n int) bool {
	if n <= 0 {
		return true
	}
	if buf[t] < n {
		return false
	}
	buf[t] -= n
	return true
}
