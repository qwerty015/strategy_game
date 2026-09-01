package enemy

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/world"
)

func TestNewIsAliveAtFullHealth(t *testing.T) {
	e := New(3, 4)
	if e.X != 3 || e.Y != 4 {
		t.Fatalf("New(3, 4) position = (%d, %d), want (3, 4)", e.X, e.Y)
	}
	if e.HP != MaxHP {
		t.Fatalf("New(...).HP = %d, want %d", e.HP, MaxHP)
	}
	if !e.Alive() {
		t.Fatal("Alive() = false for a freshly created enemy")
	}
}

func TestAliveIsFalseAtZeroHP(t *testing.T) {
	e := New(0, 0)
	e.HP = 0
	if e.Alive() {
		t.Fatal("Alive() = true at HP 0")
	}
}

func TestAliveIsFalseOnNilEnemy(t *testing.T) {
	var e *Enemy
	if e.Alive() {
		t.Fatal("Alive() = true on a nil *Enemy")
	}
}

func TestTick_DamagesAnAdjacentFinishedBuilding(t *testing.T) {
	target := &building.Building{Kind: building.Farm, X: 5, Y: 5, ConstructionStage: building.ConstructionNone, HP: building.MaxHP}
	e := New(target.X+1, target.Y) // adjacent, within AttackRange=1

	Tick([]*Enemy{e}, []*building.Building{target})

	if target.HP != building.MaxHP-10 {
		t.Fatalf("target.HP = %d, want %d (one hit)", target.HP, building.MaxHP-10)
	}
}

func TestTick_IgnoresBuildingsOutOfRange(t *testing.T) {
	target := &building.Building{Kind: building.Farm, X: 5, Y: 5, ConstructionStage: building.ConstructionNone, HP: building.MaxHP}
	e := New(target.X+5, target.Y)

	Tick([]*Enemy{e}, []*building.Building{target})

	if target.HP != building.MaxHP {
		t.Fatalf("target.HP = %d, want unchanged %d (out of range)", target.HP, building.MaxHP)
	}
}

func TestTick_IgnoresUnfinishedConstructionSites(t *testing.T) {
	site := building.NewConstructionSite(building.Farm, 5, 5)
	e := New(site.X+1, site.Y)

	Tick([]*Enemy{e}, []*building.Building{site})

	if site.HP != building.MaxHP {
		t.Fatalf("site.HP = %d, want unchanged %d (still under construction)", site.HP, building.MaxHP)
	}
}

func TestTick_RespectsAttackCooldown(t *testing.T) {
	target := &building.Building{Kind: building.Farm, X: 5, Y: 5, ConstructionStage: building.ConstructionNone, HP: building.MaxHP}
	e := New(target.X+1, target.Y)

	Tick([]*Enemy{e}, []*building.Building{target})
	Tick([]*Enemy{e}, []*building.Building{target})

	if target.HP != building.MaxHP-10 {
		t.Fatalf("target.HP after two ticks = %d, want %d (second hit still cooling down)", target.HP, building.MaxHP-10)
	}
}

// TestMoveTo_WalksToTheClickedTileAndStops covers the user's explicit
// "выбрал противника, кликнул ПКМ, противник идёт туда" request: a move
// order actually advances the enemy's position tile by tile and clears
// itself on arrival, rather than just recording an intent.
func TestMoveTo_WalksToTheClickedTileAndStops(t *testing.T) {
	grid := world.NewGrid(8, 4)
	e := New(0, 0)

	if !e.MoveTo(grid, nil, 3, 0) {
		t.Fatal("MoveTo(3, 0) = false, want a valid route over open land")
	}
	if len(e.RemainingPath()) == 0 {
		t.Fatal("RemainingPath() is empty right after a successful MoveTo")
	}

	// A generous number of ticks: enough to arrive and then, one more
	// step later, clear the order -- the same one-tick-late clearing
	// every other unit's walk-to-target loop in this codebase already
	// has (see e.g. package sentry's tickWalking).
	for range 50 {
		Tick([]*Enemy{e}, nil)
	}

	if e.X != 3 || e.Y != 0 {
		t.Fatalf("position after ticking = (%d, %d), want (3, 0)", e.X, e.Y)
	}
	if len(e.RemainingPath()) != 0 {
		t.Fatal("RemainingPath() still non-empty long after arriving -- order should clear")
	}
}

// TestMoveTo_UnreachableTargetLeavesExistingOrderIntact matches the doc
// comment's contract: a failed MoveTo (e.g. a click on an unreachable
// tile) must not cancel a route the enemy was already following.
func TestMoveTo_UnreachableTargetLeavesExistingOrderIntact(t *testing.T) {
	grid := world.NewGrid(8, 4)
	e := New(0, 0)
	if !e.MoveTo(grid, nil, 3, 0) {
		t.Fatal("MoveTo(3, 0) = false, want a valid route over open land")
	}
	before := len(e.RemainingPath())

	if e.MoveTo(grid, nil, 999, 999) {
		t.Fatal("MoveTo(999, 999) = true, want false (out of bounds/unreachable)")
	}
	if got := len(e.RemainingPath()); got != before {
		t.Fatalf("RemainingPath() length after a failed MoveTo = %d, want unchanged %d", got, before)
	}
}
