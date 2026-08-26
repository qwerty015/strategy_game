package ui

import (
	"strategy_game/internal/building"
	"strategy_game/internal/fishing"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/quarry"
	"strategy_game/internal/villagers"
)

// SelectionKind identifies the kind of object currently shown in the
// inspector. A selection stores pointers to live game objects; it is cleared
// when the object is removed or a save is loaded.
type SelectionKind int

const (
	SelectionNone SelectionKind = iota
	SelectionBuilding
	SelectionSerf
	SelectionVillager
	SelectionLumberjack
	SelectionFisherman
	SelectionQuarryman
)

// Selection is the UI-facing selection state. Only one object can be
// inspected at a time.
type Selection struct {
	Kind       SelectionKind
	Building   *building.Building
	Serf       *logistics.Serf
	Villager   *villagers.Villager
	Lumberjack *lumberjack.Lumberjack
	Fisherman  *fishing.Fisherman
	Quarryman  *quarry.Quarryman
}

// Clear removes the current selection.
func (s *Selection) Clear() {
	*s = Selection{}
}
