// Package world holds the map: a grid of tiles and their terrain.
package world

import "fmt"

// TerrainType identifies what kind of ground a tile has, which in turn
// controls what can be built on it.
type TerrainType int

const (
	Grass   TerrainType = iota
	Fertile             // buildable by a Farm
	Forest
	Water
	Stone
)

// Tile is a single cell of the map.
type Tile struct {
	Terrain TerrainType
}

// Buildable reports whether anything can be placed on this tile at all.
// Individual building types layer additional rules on top (e.g. a Farm
// also requires Fertile specifically).
func (t Tile) Buildable() bool {
	return t.Terrain != Water
}

// Grid is a fixed-size rectangular map of tiles, addressed by (x, y) with
// (0, 0) at the top-left.
type Grid struct {
	Width, Height int
	tiles         []Tile
}

// NewGrid creates a Width x Height grid where every tile is Grass.
func NewGrid(width, height int) *Grid {
	return &Grid{
		Width:  width,
		Height: height,
		tiles:  make([]Tile, width*height),
	}
}

// InBounds reports whether (x, y) is a valid tile coordinate.
func (g *Grid) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < g.Width && y < g.Height
}

// At returns the tile at (x, y). It panics if the coordinate is out of
// bounds; callers should check InBounds first when the coordinate isn't
// already known-good.
func (g *Grid) At(x, y int) Tile {
	return g.tiles[y*g.Width+x]
}

// Set overwrites the tile at (x, y).
func (g *Grid) Set(x, y int, t Tile) {
	g.tiles[y*g.Width+x] = t
}

// Tiles returns a copy of the grid's tiles in row-major order, suitable
// for serialization (see save.GameState). Mutating the result does not
// affect the grid.
func (g *Grid) Tiles() []Tile {
	out := make([]Tile, len(g.tiles))
	copy(out, g.tiles)
	return out
}

// NewGridFromTiles reconstructs a grid from a previously-saved tile
// slice (see Tiles). len(tiles) must equal width*height.
func NewGridFromTiles(width, height int, tiles []Tile) (*Grid, error) {
	if len(tiles) != width*height {
		return nil, fmt.Errorf("world: NewGridFromTiles: got %d tiles, want %d (%dx%d)", len(tiles), width*height, width, height)
	}
	g := &Grid{Width: width, Height: height, tiles: make([]Tile, len(tiles))}
	copy(g.tiles, tiles)
	return g, nil
}

// NewTestGrid builds a small hardcoded map for early development: mostly
// grass, with a fertile patch (for farms), a forest patch, and a pond.
func NewTestGrid() *Grid {
	g := NewGrid(40, 30)

	// Fertile patch for farms.
	for y := 4; y < 9; y++ {
		for x := 3; x < 12; x++ {
			g.Set(x, y, Tile{Terrain: Fertile})
		}
	}

	// Forest patch.
	for y := 15; y < 25; y++ {
		for x := 25; x < 35; x++ {
			g.Set(x, y, Tile{Terrain: Forest})
		}
	}

	// Pond.
	for y := 2; y < 6; y++ {
		for x := 30; x < 36; x++ {
			g.Set(x, y, Tile{Terrain: Water})
		}
	}

	// A stone deposit strip.
	for x := 15; x < 20; x++ {
		g.Set(x, 20, Tile{Terrain: Stone})
	}

	return g
}
