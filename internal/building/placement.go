package building

import "strategy_game/internal/world"

// CanPlace reports whether a building of kind kind could legally be
// placed with its top-left footprint corner at (x, y): every tile it
// would occupy must be in bounds and on allowed terrain, and it must not
// overlap any existing building's footprint.
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

	for _, b := range existing {
		if footprintsOverlap(x, y, bt.Footprint, b.X, b.Y, Types[b.Kind].Footprint) {
			return false
		}
	}

	return true
}

func footprintsOverlap(x1, y1, size1, x2, y2, size2 int) bool {
	return x1 < x2+size2 && x2 < x1+size1 && y1 < y2+size2 && y2 < y1+size1
}
