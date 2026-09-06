package building

import "testing"

// TestGatePassableTo_OnlyTheGatesOwnOwnerEverPasses is the unit-level
// regression for a real playtest report ("юниты противника спокойно
// проходят через мои ворота"): plain GatePassable has no notion of who is
// walking through, so an Open or Auto gate let any faction's units through
// -- see pathfind.FindLandPathForFaction, which is what actually consumes
// this for soldier movement.
func TestGatePassableTo_OnlyTheGatesOwnOwnerEverPasses(t *testing.T) {
	gate := &Building{Kind: Gate, ConstructionStage: ConstructionNone, Owner: 0, GateOpen: true}
	if !GatePassableTo(gate, 0) {
		t.Fatal("gate owner could not pass its own open gate")
	}
	if GatePassableTo(gate, 1) {
		t.Fatal("a foreign owner passed an open gate that isn't theirs")
	}

	gate.GateOpen = false
	gate.GateAuto = true
	if !GatePassableTo(gate, 0) {
		t.Fatal("gate owner could not pass its own automatic gate")
	}
	if GatePassableTo(gate, 1) {
		t.Fatal("a foreign owner passed an automatic gate that isn't theirs")
	}

	gate.GateAuto = false
	if GatePassableTo(gate, 0) {
		t.Fatal("a manually closed gate let even its own owner through")
	}
}

func TestGatePassableTo_NonGateOrUnfinishedNeverPasses(t *testing.T) {
	if GatePassableTo(nil, 0) {
		t.Fatal("nil building reported passable")
	}
	wall := &Building{Kind: StoneWall, ConstructionStage: ConstructionNone, Owner: 0}
	if GatePassableTo(wall, 0) {
		t.Fatal("a plain StoneWall reported passable")
	}
	unfinished := &Building{Kind: Gate, ConstructionStage: ConstructionFoundation, Owner: 0, GateOpen: true, GateAuto: true}
	if GatePassableTo(unfinished, 0) {
		t.Fatal("a gate still under construction reported passable")
	}
}
