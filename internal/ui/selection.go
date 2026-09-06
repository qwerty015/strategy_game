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

	// SelectionOpposingBuilding/SelectionOpposingUnit are read-only,
	// per the user's explicit request ("разреши клик на юнитов
	// противника, и отображай в правом окне информацию о нем, но без
	// управления им"): every action control in this package (gate/
	// Barracks/Armory controls, the remove button, soldier-group
	// commands) is gated behind one of the OTHER, player-owned Kind
	// values above, so introducing these as distinct Kinds means none of
	// that existing gating needs to change at all -- an opposing
	// selection simply never matches any of it, by construction.
	SelectionOpposingBuilding
	SelectionOpposingUnit
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

	// OpposingUnitOwner/OpposingUnitKind/OpposingUnitHP/OpposingUnitMaxHP
	// describe a SelectionOpposingUnit -- see that Kind's own doc
	// comment. Deliberately a plain description (no live pointer into
	// whichever package's own unit struct) rather than one more typed
	// field per opposing profession: this selection can never issue a
	// command, so it only ever needs enough to render a short summary.
	// OpposingUnitMaxHP <= 0 means "no HP concept" (any civilian worker,
	// which dies in one hit -- see combat.IntruderTarget -- rather than
	// having a percentage to show).
	OpposingUnitOwner                 int
	OpposingUnitKind                  string
	OpposingUnitHP, OpposingUnitMaxHP int
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
