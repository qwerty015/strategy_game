package render

import (
	"testing"

	"strategy_game/internal/building"
)

// TestFinishedRoadPositions_OnlyCountsCompletedRoads is a regression guard
// for drawRoadConnections' road-connector correction (see
// assets.RoadFadeN's doc comment): a Road still under construction must
// not count as a connected neighbour, matching the "not a real road yet"
// rule pathfind.roadSet and the ground-pass renderer already use.
func TestFinishedRoadPositions_OnlyCountsCompletedRoads(t *testing.T) {
	finished := &building.Building{Kind: building.Road, X: 1, Y: 1, ConstructionStage: building.ConstructionNone}
	unfinished := &building.Building{Kind: building.Road, X: 2, Y: 1, ConstructionStage: building.ConstructionFoundation}
	other := &building.Building{Kind: building.Warehouse, X: 3, Y: 1}

	got := finishedRoadPositions([]*building.Building{finished, unfinished, other})

	if !got[roadTile{1, 1}] {
		t.Error("a finished road must be indexed")
	}
	if got[roadTile{2, 1}] {
		t.Error("a road still under construction must not be indexed")
	}
	if got[roadTile{3, 1}] {
		t.Error("a non-road building must not be indexed")
	}
	if len(got) != 1 {
		t.Errorf("finishedRoadPositions returned %d entries, want 1", len(got))
	}
}
