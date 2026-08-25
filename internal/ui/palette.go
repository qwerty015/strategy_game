// Package ui draws HUD elements (resource bar, building palette,
// placement preview) and holds the small bits of UI state (which
// building is selected) that don't belong in the game-logic packages.
package ui

import "strategy_game/internal/building"

// Palette is the set of building kinds the player can choose from, plus
// which one is currently selected for placement.
type Palette struct {
	Kinds    []building.Kind
	Selected int // index into Kinds
}

// NewPalette creates the default palette: every player-placeable
// building kind, in the order they should appear (and be bound to
// number keys) in the HUD. Warehouse is deliberately excluded -- the
// town has exactly one, auto-placed at game start.
func NewPalette() *Palette {
	return &Palette{Kinds: []building.Kind{building.Farm, building.Mill, building.Bakery, building.Tavern, building.Road}}
}

// SelectedKind returns the building kind currently chosen for placement.
func (p *Palette) SelectedKind() building.Kind {
	return p.Kinds[p.Selected]
}

// Select changes the selected building by index (e.g. from a number-key
// press). Out-of-range indices are ignored.
func (p *Palette) Select(i int) {
	if i >= 0 && i < len(p.Kinds) {
		p.Selected = i
	}
}
