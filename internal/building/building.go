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

// Building is a placed instance of a Type on the grid.
type Building struct {
	Kind          Kind
	X, Y          int // top-left tile of the footprint
	ProgressTicks int // ticks accumulated toward the current production cycle
}
