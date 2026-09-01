package ui

import (
	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/enemy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/miner"
	"strategy_game/internal/quarry"
	"strategy_game/internal/soldier"
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

	// SelectionEnemy is the debug test-attacker (package enemy) -- see the
	// user's explicit request to select and right-click-move it, meant to
	// carry over to future player-controlled combat units too.
	SelectionEnemy

	// SelectionSoldierGroup is one or more Archers/Swordsmen of the same
	// Profession, selected together -- see cmd/game's selectionAt, which
	// clicking a single soldier expands into every same-profession soldier
	// within 4 tiles (Chebyshev), per the user's explicit request ("клик
	// на лучника - выделяются и управляются сразу все лучники в радиусе 4
	// клеток"). A right-click then orders the whole group at once (move or
	// attack) -- see commandSoldierGroupTo/commandSoldierGroupAttack.
	SelectionSoldierGroup
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
	Enemy      *enemy.Enemy

	// SoldierGroup is set only for SelectionSoldierGroup -- see that
	// constant's doc comment. Shift-clicking another soldier merges its
	// own proximity group into this one (see cmd/game's mergeSoldierGroups),
	// so a group may end up mixing Archers and Swordsmen.
	SoldierGroup []*soldier.Soldier

	// SoldierGroupAnchor is the specific soldier a plain (non-shift) click
	// most recently selected -- the "разъединить" inspector button
	// collapses SoldierGroup back down to just this one, per the user's
	// explicit request for a way to split a merged group apart again.
	SoldierGroupAnchor *soldier.Soldier
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
