package main

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/combat"
	"strategy_game/internal/soldier"
	"strategy_game/internal/ui"
	"strategy_game/internal/villagers"
)

// TestOpposingBuildingAt_FindsAnyTileOfAMultiTileFootprint confirms
// opposingBuildingAt matches the same footprint-aware convention as
// buildingSelectionAt -- a click anywhere on a >1-tile building's
// footprint must resolve to it, not just its (X, Y) origin tile.
func TestOpposingBuildingAt_FindsAnyTileOfAMultiTileFootprint(t *testing.T) {
	g := &Game{
		soldiers: soldier.NewController(),
		ais:      []*faction{{owner: 1, soldiers: soldier.NewController()}},
	}
	rival := &building.Building{Kind: building.Farm, Owner: 1, X: 5, Y: 5, HP: building.MaxHP}
	g.buildings = []*building.Building{rival}

	footprint := building.Types[building.Farm].Footprint
	if footprint < 2 {
		t.Fatal("test setup: expected Farm to have a >1 footprint")
	}
	if got := g.opposingBuildingAt(rival.X+footprint-1, rival.Y+footprint-1); got != rival {
		t.Fatalf("opposingBuildingAt(far corner of footprint) = %v, want the rival warehouse", got)
	}
	if got := g.opposingBuildingAt(rival.X-1, rival.Y); got != nil {
		t.Fatalf("opposingBuildingAt(just outside footprint) = %v, want nil", got)
	}
}

// TestOpposingBuildingAt_IgnoresADestroyedBuilding confirms a building
// already at 0 HP (pending pruneDestroyedBuildings) is never offered up
// as a click-to-attack target -- matching opposingBuildingsFor's own
// candidate list, which auto-engage already respects.
func TestOpposingBuildingAt_IgnoresADestroyedBuilding(t *testing.T) {
	g := &Game{
		soldiers: soldier.NewController(),
		ais:      []*faction{{owner: 1, soldiers: soldier.NewController()}},
	}
	rival := &building.Building{Kind: building.Warehouse, Owner: 1, X: 5, Y: 5, HP: 0}
	g.buildings = []*building.Building{rival}

	if got := g.opposingBuildingAt(rival.X, rival.Y); got != nil {
		t.Fatalf("opposingBuildingAt(destroyed building) = %v, want nil", got)
	}
}

// TestCommandSoldierGroupAttackFaction_OrdersEverySoldierInTheGroup is the
// regression test for the real playtest bug ("клик боевым юнитом на
// постройку противника - перемещает юнитов, но не уничтожает постройку
// врага"): every soldier in the current SelectionSoldierGroup must come
// away with a standing AttackFactionOrder against the clicked opposing
// building, not merely a move order toward it.
func TestCommandSoldierGroupAttackFaction_OrdersEverySoldierInTheGroup(t *testing.T) {
	g := &Game{soldiers: soldier.NewController()}
	rival := &building.Building{Kind: building.Warehouse, Owner: 1, X: 5, Y: 5, HP: building.MaxHP}

	a := g.soldiers.Spawn(soldier.Archer, 0, 0)
	b := g.soldiers.Spawn(soldier.Swordsman, 1, 0)
	g.selection = ui.Selection{Kind: ui.SelectionSoldierGroup, SoldierGroup: []*soldier.Soldier{a, b}}

	g.commandSoldierGroupAttackFaction(rival)

	if !a.HasFactionTarget() || !b.HasFactionTarget() {
		t.Fatal("commandSoldierGroupAttackFaction did not set a standing faction target on every soldier in the group")
	}
}

// TestOpposingSoldierAt_FindsALivingRivalSoldier and
// TestOpposingIntruderAt_FindsALivingRivalVillager are the regression
// tests for the follow-up request "клик боевым юнитом на любого
// юнита/постройку противника, должен переходить в режим атаки" -- a
// right-click on an opposing rival soldier, or on any other opposing
// unit (a rival villager here), must resolve to an attack target the
// same way opposingBuildingAt already does for a building.
func TestOpposingSoldierAt_FindsALivingRivalSoldier(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, Owner: 1, X: 0, Y: 0, HP: building.MaxHP}
	rivalFaction := newFaction(1, warehouse, AIEasy)
	rival := rivalFaction.soldiers.Spawn(soldier.Swordsman, 5, 5)
	g := &Game{soldiers: soldier.NewController(), ais: []*faction{rivalFaction}}

	if got := g.opposingSoldierAt(5, 5); got != rival {
		t.Fatalf("opposingSoldierAt(rival's tile) = %v, want the rival soldier", got)
	}
	if got := g.opposingSoldierAt(6, 6); got != nil {
		t.Fatalf("opposingSoldierAt(empty tile) = %v, want nil", got)
	}

	rival.HP = 0
	if got := g.opposingSoldierAt(5, 5); got != nil {
		t.Fatalf("opposingSoldierAt(dead rival's tile) = %v, want nil", got)
	}
}

func TestOpposingIntruderAt_FindsALivingRivalVillager(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, Owner: 1, X: 0, Y: 0, HP: building.MaxHP}
	rivalFaction := newFaction(1, warehouse, AIEasy)
	rivalVillager := &villagers.Villager{Profession: villagers.Farmer, X: 7, Y: 7}
	rivalFaction.vills.Villagers = append(rivalFaction.vills.Villagers, rivalVillager)
	g := &Game{soldiers: soldier.NewController(), ais: []*faction{rivalFaction}}

	got, ok := g.opposingIntruderAt(7, 7)
	if !ok || got.X != 7 || got.Y != 7 {
		t.Fatalf("opposingIntruderAt(rival villager's tile) = (%+v, %v), want the rival villager", got, ok)
	}
	if _, ok := g.opposingIntruderAt(8, 8); ok {
		t.Fatal("opposingIntruderAt(empty tile) = true, want false")
	}

	rivalVillager.Kill()
	if _, ok := g.opposingIntruderAt(7, 7); ok {
		t.Fatal("opposingIntruderAt(dead villager's tile) = true, want false")
	}
}

// TestCommandSoldierGroupAttackSoldier_OrdersEverySoldierInTheGroup and
// its intruder counterpart mirror
// TestCommandSoldierGroupAttackFaction_OrdersEverySoldierInTheGroup for
// the two remaining opposing-target kinds.
func TestCommandSoldierGroupAttackSoldier_OrdersEverySoldierInTheGroup(t *testing.T) {
	g := &Game{soldiers: soldier.NewController()}
	rivalController := soldier.NewController()
	rival := rivalController.Spawn(soldier.Swordsman, 5, 5)

	a := g.soldiers.Spawn(soldier.Archer, 0, 0)
	b := g.soldiers.Spawn(soldier.Swordsman, 1, 0)
	g.selection = ui.Selection{Kind: ui.SelectionSoldierGroup, SoldierGroup: []*soldier.Soldier{a, b}}

	g.commandSoldierGroupAttackSoldier(rival)

	if !a.HasFactionTarget() || !b.HasFactionTarget() {
		t.Fatal("commandSoldierGroupAttackSoldier did not set a standing faction target on every soldier in the group")
	}
}

func TestCommandSoldierGroupAttackIntruder_OrdersEverySoldierInTheGroup(t *testing.T) {
	g := &Game{soldiers: soldier.NewController()}
	intruder := combat.IntruderTarget{X: 5, Y: 5, Alive: func() bool { return true }, Kill: func() {}}

	a := g.soldiers.Spawn(soldier.Archer, 0, 0)
	b := g.soldiers.Spawn(soldier.Swordsman, 1, 0)
	g.selection = ui.Selection{Kind: ui.SelectionSoldierGroup, SoldierGroup: []*soldier.Soldier{a, b}}

	g.commandSoldierGroupAttackIntruder(intruder)

	if !a.HasFactionTarget() || !b.HasFactionTarget() {
		t.Fatal("commandSoldierGroupAttackIntruder did not set a standing faction target on every soldier in the group")
	}
}
