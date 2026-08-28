package ui

import (
	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/fishing"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/miner"
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
	SelectionBuilder
	SelectionMiner
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
	Builder    *builder.Builder
	Miner      *miner.Miner
}

// Clear removes the current selection.
func (s *Selection) Clear() {
	*s = Selection{}
}

// CanRemoveSelection reports whether the selected object can be removed by
// the inspector action. Trees, fish and mineral deposits are map resources,
// not player-owned buildings, so the action is deliberately hidden for them.
// A serf is dismissed only after completing any delivery already in progress.
func CanRemoveSelection(selection Selection) bool {
	switch selection.Kind {
	case SelectionSerf:
		return selection.Serf != nil
	case SelectionBuilding:
		if selection.Building == nil {
			return false
		}
		switch selection.Building.Kind {
		case building.Tree, building.Fish, building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
			return false
		}
		return true
	default:
		return false
	}
}
