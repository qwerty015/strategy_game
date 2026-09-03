package soldier

import (
	"testing"

	"strategy_game/internal/enemy"
	"strategy_game/internal/world"
)

func TestAttackVisualStartsOnALandedHitAndExpires(t *testing.T) {
	grid := world.NewGrid(8, 8)
	controller := NewController()
	unit := controller.Spawn(Archer, 2, 2)
	target := enemy.New(2, 4)
	unit.AttackOrder(grid, nil, target)

	if _, _, _, ok := unit.AttackVisual(); ok {
		t.Fatal("AttackVisual() is present before the first hit")
	}
	controller.Tick(grid, nil, nil, nil, nil, nil)
	x, y, progress, ok := unit.AttackVisual()
	if !ok {
		t.Fatal("AttackVisual() is absent immediately after a landed hit")
	}
	if x != target.X || y != target.Y || progress <= 0 || progress > 1 {
		t.Fatalf("AttackVisual() = (%d,%d,%v), want target (%d,%d) and progress in (0,1]", x, y, progress, target.X, target.Y)
	}
	// Cancel the standing order so the zero-cooldown attack rate (see
	// ArcherCooldownTicks) doesn't fire -- and reset the visual with -- a
	// second, lethal hit here; that cascade is covered on its own by
	// TestTwoHitsKillTheTarget. This test is only about one hit's own
	// visual lifecycle.
	unit.AttackOrder(grid, nil, nil)
	for range attackVisualLifetime {
		controller.Tick(grid, nil, nil, nil, nil, nil)
	}
	if _, _, _, ok := unit.AttackVisual(); ok {
		t.Fatal("AttackVisual() remained after its visual lifetime")
	}
}
