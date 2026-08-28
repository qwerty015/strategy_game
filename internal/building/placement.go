package building

import "strategy_game/internal/world"

// CanPlace reports whether a building of kind kind could legally be
// placed with its top-left footprint corner at (x, y). Its footprint must be
// in bounds and on allowed terrain, it must not overlap anything already
// placed, and player buildings must keep one empty tile between their
// footprints in every direction. Roads and natural world objects are exempt
// from that visual gap: roads still need to meet a door, while a worker hut
// may reasonably stand beside a tree or deposit it is meant to service.
func CanPlace(g *world.Grid, existing []*Building, kind Kind, x, y int) bool {
	bt := Types[kind]

	for dy := 0; dy < bt.Footprint; dy++ {
		for dx := 0; dx < bt.Footprint; dx++ {
			tx, ty := x+dx, y+dy
			if !g.InBounds(tx, ty) {
				return false
			}
			if !bt.CanBuildOn(g.At(tx, ty)) {
				return false
			}
		}
	}

	// A fishing hut launches its boat straight from a pier. Diagonal contact
	// is intentionally not enough: the pier needs a cardinal neighbouring
	// water tile, not merely a corner of a pond.
	if kind == FisherHut && !hasCardinalWaterNeighbor(g, x, y) {
		return false
	}

	for _, b := range existing {
		if b == nil {
			continue
		}
		otherSize := Types[b.Kind].Footprint
		if footprintsOverlap(x, y, bt.Footprint, b.X, b.Y, otherSize) {
			return false
		}
		if requiresBuildingGap(kind) && requiresBuildingGap(b.Kind) &&
			footprintsOverlap(x-1, y-1, bt.Footprint+2, b.X, b.Y, otherSize) {
			return false
		}
	}

	return true
}

// FoundationRoad returns the completed one-tile road placed beside a newly
// created foundation. It chooses a cardinal neighbour of the building's
// marked access tile, never a tile inside the footprint. A pre-existing road
// is reused (nil, true); false means the building has no valid dry entrance
// and therefore must not be placed. The road is intentionally finished: it is
// the visible anchor from which the player extends the network while the
// building itself is still under construction.
func FoundationRoad(g *world.Grid, existing []*Building, kind Kind, x, y int) (*Building, bool) {
	if g == nil || kind == Road {
		return nil, kind == Road
	}
	candidate := &Building{Kind: kind, X: x, Y: y}
	access := candidate.AccessPoint()
	for _, d := range [...]Point{{X: 0, Y: 1}, {X: 1, Y: 0}, {X: 0, Y: -1}, {X: -1, Y: 0}} {
		roadX, roadY := access.X+d.X, access.Y+d.Y
		if footprintContains(candidate, roadX, roadY) || !g.InBounds(roadX, roadY) || !Types[Road].CanBuildOn(g.At(roadX, roadY)) {
			continue
		}
		if roadAt(existing, roadX, roadY) {
			return nil, true
		}
		if CanPlace(g, existing, Road, roadX, roadY) {
			return &Building{Kind: Road, X: roadX, Y: roadY}, true
		}
	}
	return nil, false
}

func requiresBuildingGap(kind Kind) bool {
	switch kind {
	case Road, Tree, Fish, StoneDeposit, CoalDeposit, GoldOreDeposit, IronOreDeposit:
		return false
	default:
		return true
	}
}

func footprintContains(b *Building, x, y int) bool {
	size := Types[b.Kind].Footprint
	return x >= b.X && x < b.X+size && y >= b.Y && y < b.Y+size
}

func roadAt(existing []*Building, x, y int) bool {
	for _, b := range existing {
		if b != nil && b.Kind == Road && b.X == x && b.Y == y && b.ConstructionStage == ConstructionNone {
			return true
		}
	}
	return false
}

func hasCardinalWaterNeighbor(g *world.Grid, x, y int) bool {
	for _, d := range [...]Point{{X: 0, Y: 1}, {X: -1, Y: 0}, {X: 0, Y: -1}, {X: 1, Y: 0}} {
		tx, ty := x+d.X, y+d.Y
		if g.InBounds(tx, ty) && g.At(tx, ty).Terrain == world.Water {
			return true
		}
	}
	return false
}

func footprintsOverlap(x1, y1, size1, x2, y2, size2 int) bool {
	return x1 < x2+size2 && x2 < x1+size1 && y1 < y2+size2 && y2 < y1+size1
}
