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

	// Warehouse is a logistics hub: serfs (see package logistics) deposit
	// collected goods here and draw from it to supply other buildings. The
	// first warehouse is the serf spawn point; additional warehouses share the
	// same town stockpile. It has no Recipe -- it produces nothing itself.
	Warehouse

	// Road is not really a "building" gameplay-wise, but reusing the
	// Building/placement machinery for it (1x1 footprint, buildable
	// anywhere) means the whole grid/overlap/save system already works
	// for roads for free. Serfs may only walk across Road tiles and the
	// footprints of the two buildings they're travelling between --
	// buildings not connected to the road network by an unbroken chain
	// of Road tiles can't be serviced. See package pathfind.
	Road

	// Tavern is where villagers (serfs, farmers, bakers -- see package
	// villagers) go to eat. Like Warehouse it has no Recipe.Output (it
	// produces nothing), but it does declare Recipe.Inputs so the
	// logistics system knows to keep it stocked with Bread the same way
	// it stocks any other consumer -- see Types[Tavern].
	Tavern

	// Tree is a world object rather than a player-buildable structure. It
	// occupies a tile and grows over time. A lumberjack can harvest it; the
	// player still cannot remove it directly.
	Tree

	// LumberjackHut is the workplace and temporary log store for one
	// lumberjack. It is appended after Tree to preserve the numeric Tree value
	// in existing save files.
	LumberjackHut
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

// Point is a world-grid coordinate used for building access points.
type Point struct{ X, Y int }

// Type describes a kind of building: how big it is, what terrain it can
// sit on, and what it produces.
type Type struct {
	Kind      Kind
	Name      string
	Footprint int // buildings are Footprint x Footprint tiles

	// AccessX and AccessY identify the single tile where a road must meet
	// the building. For a Farm this is the farmhouse tile; for a 1x1
	// building it is (0, 0).
	AccessX, AccessY int

	// AllowedTerrain lists the terrain types this building may be placed
	// on. An empty slice means "any buildable (non-water) terrain".
	AllowedTerrain []world.TerrainType

	Recipe Recipe

	// AcceptedResources is used by service buildings that can consume more
	// than one food type. Tavern currently consumes Bread, but declaring the
	// complete future menu here lets logistics and the inspector already
	// understand Fish, Wine and Sausage without pretending they are produced.
	AcceptedResources []resource.Type
}

// AccessPoint returns the world tile that serves as this building's door or
// loading point. Roads connected to another part of a multi-tile building do
// not make that building reachable.
func (b *Building) AccessPoint() Point {
	bt := Types[b.Kind]
	return Point{X: b.X + bt.AccessX, Y: b.Y + bt.AccessY}
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
// matter instead of being invisible plumbing. 6 matches the reference
// the user asked to follow (wheat at farm/mill, flour at mill/bakery,
// bread at bakery/tavern -- 6 each; only the Warehouse is unlimited).
const BufferCapacity = 6

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

	// Tree growth is kept on the placed object so it survives save/load and
	// each tree can have its own deterministic random-looking lifetime.
	GrowthTicks       int
	GrowthTargetTicks int
}

const (
	// TreeGrowthMinTicks is two minutes at the normal two simulation ticks
	// per second. The extra random-looking part makes a grove grow unevenly.
	TreeGrowthMinTicks       = 240
	TreeGrowthVariationTicks = 240
)

// NewTree creates an indestructible tree with a stable per-coordinate growth
// target. The target is deterministic, so saving and loading never changes
// how long that particular tree takes to mature.
func NewTree(x, y int) *Building {
	return &Building{
		Kind:              Tree,
		X:                 x,
		Y:                 y,
		GrowthTargetTicks: treeGrowthTarget(x, y),
	}
}

// TickGrowth advances a tree by one simulation tick. Non-tree buildings are
// ignored so the caller can safely tick the whole building slice.
func (b *Building) TickGrowth() {
	if b == nil || b.Kind != Tree {
		return
	}
	if b.GrowthTargetTicks <= 0 {
		b.GrowthTargetTicks = treeGrowthTarget(b.X, b.Y)
	}
	if b.GrowthTicks < b.GrowthTargetTicks {
		b.GrowthTicks++
	}
}

// GrowthProgress returns a clamped 0..1 value for rendering and UI.
func (b *Building) GrowthProgress() float64 {
	if b == nil || b.Kind != Tree || b.GrowthTargetTicks <= 0 {
		return 0
	}
	progress := float64(b.GrowthTicks) / float64(b.GrowthTargetTicks)
	if progress > 1 {
		return 1
	}
	if progress < 0 {
		return 0
	}
	return progress
}

// GrowthStage returns 0 for a sapling and 2 for a mature tree.
func (b *Building) GrowthStage() int {
	progress := b.GrowthProgress()
	switch {
	case progress >= 0.66:
		return 2
	case progress >= 0.33:
		return 1
	default:
		return 0
	}
}

func treeGrowthTarget(x, y int) int {
	seed := uint32(x)*73856093 ^ uint32(y)*19349663 ^ 0x9e3779b9
	return TreeGrowthMinTicks + int(seed%TreeGrowthVariationTicks)
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
