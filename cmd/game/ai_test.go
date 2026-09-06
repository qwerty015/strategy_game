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
