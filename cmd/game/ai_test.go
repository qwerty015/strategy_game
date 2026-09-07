package main

import (
	"testing"

	"strategy_game/internal/building"
)

// TestPruneDestroyedBuildings_ForceRemovesADestroyedAIWarehouse is the
// regression test for a real playtest bug found from an actual save
// ("красный вроде не осталось склада, но его слуги продолжают носить
// рыбу куда-то"): destroying an AI faction's only Warehouse in combat
// used to leave that faction's logistics.Controller holding a stale
// pointer to it forever, since only the manual player-delete path ever
// called RemoveWarehouse/CancelAllJobs. pruneDestroyedBuildings must now
// do the same cleanup for whichever faction actually owned the building.
func TestPruneDestroyedBuildings_ForceRemovesADestroyedAIWarehouse(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, Owner: 1, X: 0, Y: 0, HP: 0}
	f := newFaction(1, warehouse, AIEasy)
	g := &Game{buildings: []*building.Building{warehouse}, ais: []*faction{f}}

	g.pruneDestroyedBuildings()

	if len(g.buildings) != 0 {
		t.Fatalf("g.buildings after pruning the destroyed warehouse = %v, want empty", g.buildings)
	}
	if len(f.logi.Warehouses) != 0 {
		t.Fatalf("faction's registered warehouses after its only warehouse was destroyed = %v, want none", f.logi.Warehouses)
	}
	for _, s := range f.logi.Serfs {
		if s.Busy() {
			t.Fatalf("serf still mid-haul after its faction's only warehouse was force-removed: %+v", s)
		}
	}
}

// TestPruneDestroyedBuildings_LeavesAnUnrelatedFactionsWarehouseAlone
// confirms the new per-owner lookup in pruneDestroyedBuildings actually
// targets the right faction -- destroying owner 1's warehouse must not
// touch owner 2's own logistics.Controller at all.
func TestPruneDestroyedBuildings_LeavesAnUnrelatedFactionsWarehouseAlone(t *testing.T) {
	destroyed := &building.Building{Kind: building.Warehouse, Owner: 1, X: 0, Y: 0, HP: 0}
	survivor := &building.Building{Kind: building.Warehouse, Owner: 2, X: 10, Y: 0, HP: building.MaxHP}
	fDestroyed := newFaction(1, destroyed, AIEasy)
	fSurvivor := newFaction(2, survivor, AIEasy)
	g := &Game{buildings: []*building.Building{destroyed, survivor}, ais: []*faction{fDestroyed, fSurvivor}}

	g.pruneDestroyedBuildings()

	if len(fDestroyed.logi.Warehouses) != 0 {
		t.Fatalf("destroyed faction's registered warehouses = %v, want none", fDestroyed.logi.Warehouses)
	}
	if len(fSurvivor.logi.Warehouses) != 1 || fSurvivor.logi.Warehouses[0] != survivor {
		t.Fatalf("unrelated faction's registered warehouses = %v, want untouched [%v]", fSurvivor.logi.Warehouses, survivor)
	}
}

// TestPruneDestroyedBuildings_RemovesADestroyedWallOrGate is the
// regression test for a real playtest report ("это чужая стена с
// чужими воротами, я должен иметь возможность её уничтожить!"): a
// StoneWall/Gate reduced to 0 HP in combat used to be exempted from
// pruning right alongside Road, so it stayed on the map forever --
// still physically blocking movement (pathfind's occupancy checks never
// look at HP) while also becoming permanently unattackable
// (opposingBuildingAt skips any HP<=0 candidate). Road stays exempt --
// it's never a combat target at all.
func TestPruneDestroyedBuildings_RemovesADestroyedWallOrGate(t *testing.T) {
	wall := &building.Building{Kind: building.StoneWall, Owner: 1, X: 0, Y: 0, HP: 0}
	gate := &building.Building{Kind: building.Gate, Owner: 1, X: 1, Y: 0, HP: 0}
	road := &building.Building{Kind: building.Road, Owner: 1, X: 2, Y: 0, HP: 0}
	aliveWall := &building.Building{Kind: building.StoneWall, Owner: 1, X: 3, Y: 0, HP: building.MaxHP}
	g := &Game{buildings: []*building.Building{wall, gate, road, aliveWall}}

	g.pruneDestroyedBuildings()

	for _, b := range g.buildings {
		if b == wall || b == gate {
			t.Fatalf("destroyed %v is still on the map after pruneDestroyedBuildings", b.Kind)
		}
	}
	foundRoad, foundAliveWall := false, false
	for _, b := range g.buildings {
		if b == road {
			foundRoad = true
		}
		if b == aliveWall {
			foundAliveWall = true
		}
	}
	if !foundRoad {
		t.Fatal("a Road at HP<=0 must stay exempt -- it's never a combat target in the first place")
	}
	if !foundAliveWall {
		t.Fatal("a StoneWall with real HP left must not be removed")
	}
}
