package main

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/soldier"
)

// TestNearestRealBuildingOwnedBy_SkipsNeutralAndInfrastructureKinds is
// part of the fix for a real bug found from an actual playtest report
// ("красный уничтожил не все постройки других ботов"): aiConsiderAttack
// used to stop considering an opponent entirely once its Warehouse was
// destroyed, even with other real buildings still standing. This checks
// the fallback target-finder alone: it must skip natural resources (owner
// is meaningless for them) and Road/StoneWall/Gate (factionDefeated
// itself never counts these either), landing on the one real building.
func TestNearestRealBuildingOwnedBy_SkipsNeutralAndInfrastructureKinds(t *testing.T) {
	g := &Game{buildings: []*building.Building{
		{Kind: building.Tree, Owner: 0, X: 1, Y: 0},
		{Kind: building.Road, Owner: 2, X: 2, Y: 0},
		{Kind: building.StoneWall, Owner: 2, X: 3, Y: 0},
		{Kind: building.Gate, Owner: 2, X: 4, Y: 0},
		{Kind: building.FisherHut, Owner: 2, X: 5, Y: 0},
	}}
	got := g.nearestRealBuildingOwnedBy(2, 0, 0)
	if got == nil || got.Kind != building.FisherHut {
		t.Fatalf("nearestRealBuildingOwnedBy = %+v, want the FisherHut", got)
	}
}

// TestNearestRealBuildingOwnedBy_PicksTheClosestOne confirms the "nearest"
// half of the name -- aiConsiderAttack's whole point is marching on the
// closest threat/straggler, not a random or first-found one.
func TestNearestRealBuildingOwnedBy_PicksTheClosestOne(t *testing.T) {
	g := &Game{buildings: []*building.Building{
		{Kind: building.FisherHut, Owner: 1, X: 100, Y: 0},
		{Kind: building.Armory, Owner: 1, X: 5, Y: 0},
	}}
	got := g.nearestRealBuildingOwnedBy(1, 0, 0)
	if got == nil || got.Kind != building.Armory {
		t.Fatalf("nearestRealBuildingOwnedBy = %+v, want the closer Armory", got)
	}
}

// TestNearestRealBuildingOwnedBy_NilWhenNothingRealLeft confirms this
// correctly reports "truly nothing left" (the caller then treats that
// opponent as not worth attacking, same as an already-defeated one)
// rather than latching onto a Road tile that pruneDestroyedBuildings
// would never even remove.
func TestNearestRealBuildingOwnedBy_NilWhenNothingRealLeft(t *testing.T) {
	g := &Game{buildings: []*building.Building{
		{Kind: building.Road, Owner: 1, X: 1, Y: 0},
		{Kind: building.StoneWall, Owner: 1, X: 2, Y: 0},
		{Kind: building.Tree, Owner: 0, X: 3, Y: 0},
	}}
	if got := g.nearestRealBuildingOwnedBy(1, 0, 0); got != nil {
		t.Fatalf("nearestRealBuildingOwnedBy = %+v, want nil (nothing real left)", got)
	}
}

// TestAIConsiderAttack_KeepsHuntingAfterTheWarehouseFalls is the
// end-to-end version: once an opponent's Warehouse is gone but a real
// building of theirs survives elsewhere, an idle attack squad must still
// be sent after it -- the exact scenario the user's real playtest report
// described (a rival's Warehouse destroyed, other buildings left
// standing forever because nothing ever attacked them again).
func TestAIConsiderAttack_KeepsHuntingAfterTheWarehouseFalls(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIHard, AIHard})
	f := g.ais[0]        // owner 1, AIHard -> attackSquadSize() == 3
	opponent := g.ais[1] // owner 2

	for i := 0; i < 3; i++ {
		f.soldiers.Spawn(soldier.Archer, f.logi.Warehouse.X, f.logi.Warehouse.Y)
	}

	// A straggler building of the opponent's, placed right next to their
	// own (soon to be destroyed) warehouse -- same quadrant, so reachable
	// the same way the warehouse itself was. Tries a handful of nearby
	// offsets since the exact tiles right beside a freshly generated
	// warehouse can be uneven ground, water, or already occupied.
	oh := opponent.logi.Warehouse
	var straggler *building.Building
	for _, d := range [][2]int{{2, 0}, {-2, 0}, {0, 2}, {0, -2}, {3, 1}, {-3, -1}, {1, 3}, {-1, -3}} {
		x, y := oh.X+d[0], oh.Y+d[1]
		if building.CanPlace(g.grid, g.buildings, building.Armory, x, y) {
			straggler = &building.Building{Kind: building.Armory, X: x, Y: y, Owner: opponent.owner, ConstructionStage: building.ConstructionNone, HP: building.MaxHP}
			break
		}
	}
	if straggler == nil {
		t.Fatal("test setup: no nearby offset was placeable for the straggler building")
	}
	g.buildings = append(g.buildings, straggler)

	oh.HP = 0
	g.pruneDestroyedBuildings()
	if findWarehouseOwnedBy(g.buildings, opponent.owner) != nil {
		t.Fatal("test setup: opponent's warehouse should be gone")
	}

	f.brain.tick(g, f, g.grid)

	for _, s := range f.soldiers.Soldiers {
		path := s.RemainingPath()
		if len(path) == 0 {
			continue
		}
		last := path[len(path)-1]
		if last.X == straggler.X && last.Y == straggler.Y {
			return // found it -- at least one soldier is headed there
		}
	}
	t.Fatal("no soldier was routed toward the opponent's last remaining building once its warehouse was destroyed")
}
