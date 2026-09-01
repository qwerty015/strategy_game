package soldier

import (
	"testing"

	"strategy_game/internal/combat"
	"strategy_game/internal/enemy"
	"strategy_game/internal/hunger"
	"strategy_game/internal/world"
)

func TestMoveTo_WalksToTheClickedTileAndStops(t *testing.T) {
	grid := world.NewGrid(10, 4)
	c := NewController()
	s := c.Spawn(Swordsman, 0, 0)

	if !s.MoveTo(grid, nil, 5, 0) {
		t.Fatal("MoveTo(5, 0) = false, want a valid route over open land")
	}
	if len(s.RemainingPath()) == 0 {
		t.Fatal("RemainingPath() is empty right after a successful MoveTo")
	}

	for range 50 {
		c.Tick(grid, nil, nil)
	}

	if s.X != 5 || s.Y != 0 {
		t.Fatalf("position after ticking = (%d, %d), want (5, 0)", s.X, s.Y)
	}
}

// TestSwordsman_MeleeOnlyHitsAdjacent covers the user's explicit "мечник —
// ближний бой (клетки врага и наша — одинаковые)" rule: SwordsmanRange is
// 1. A single hit only does combat.UnitDamagePerHit (50%), not a kill --
// see TestTwoHitsKillTheTarget for that.
func TestSwordsman_MeleeOnlyHitsAdjacent(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	s := c.Spawn(Swordsman, 5, 5)
	target := enemy.New(5, 6) // adjacent
	s.AttackOrder(grid, nil, target)

	c.Tick(grid, nil, nil)
	if target.HP != combat.MaxHP-combat.UnitDamagePerHit {
		t.Fatalf("adjacent target HP after one swordsman tick = %d, want %d", target.HP, combat.MaxHP-combat.UnitDamagePerHit)
	}
}

// TestArcher_HitsAtRangeThreeButNotFour covers the user's explicit
// "лучник — может атаковать за 3 клетки от себя".
func TestArcher_HitsAtRangeThreeButNotFour(t *testing.T) {
	grid := world.NewGrid(10, 10)

	near := enemy.New(5, 5+ArcherRange)
	c := NewController()
	s := c.Spawn(Archer, 5, 5)
	s.AttackOrder(grid, nil, near)
	c.Tick(grid, nil, nil)
	if near.HP != combat.MaxHP-combat.UnitDamagePerHit {
		t.Fatalf("target at exactly ArcherRange (%d) HP = %d, want a landed hit (%d)", ArcherRange, near.HP, combat.MaxHP-combat.UnitDamagePerHit)
	}

	far := enemy.New(5, 5+ArcherRange+1)
	c2 := NewController()
	s2 := c2.Spawn(Archer, 5, 5)
	s2.AttackOrder(grid, nil, far)
	c2.Tick(grid, nil, nil)
	if far.HP != combat.MaxHP {
		t.Fatal("target one tile beyond ArcherRange took damage -- should still be out of range on the first tick")
	}
}

// TestTwoHitsKillTheTarget covers the user's explicit unit-damage rule
// (50% per hit, reused from package combat): two hits kill. The second
// hit lands on the tick *after* the cooldown reaches zero -- the same
// decrement-then-return-early cooldown shape package sentry's
// ShotCooldownTicks already uses, so it needs one extra tick beyond the
// cooldown length itself, not exactly SwordsmanCooldownTicks.
func TestTwoHitsKillTheTarget(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	s := c.Spawn(Swordsman, 5, 5)
	target := enemy.New(5, 6)
	s.AttackOrder(grid, nil, target)

	c.Tick(grid, nil, nil)
	if target.HP != combat.MaxHP-combat.UnitDamagePerHit {
		t.Fatalf("target HP after 1 hit = %d, want %d", target.HP, combat.MaxHP-combat.UnitDamagePerHit)
	}

	for range SwordsmanCooldownTicks + 1 {
		c.Tick(grid, nil, nil)
	}
	if target.Alive() {
		t.Fatal("target still alive after cooldown elapsed and a second hit landed")
	}
}

// TestAttackOrder_ChasesATargetThatMovesOutOfRange covers the standing
// order re-approaching on its own, no new player click needed.
func TestAttackOrder_ChasesATargetThatMovesOutOfRange(t *testing.T) {
	grid := world.NewGrid(20, 10)
	c := NewController()
	s := c.Spawn(Swordsman, 0, 0)
	target := enemy.New(10, 0) // far out of melee range
	s.AttackOrder(grid, nil, target)

	if len(s.RemainingPath()) == 0 {
		t.Fatal("swordsman did not start approaching a distant attack target")
	}

	for range 60 {
		c.Tick(grid, nil, nil)
	}
	if target.Alive() {
		t.Fatal("swordsman never closed the distance to melee range")
	}
}

// TestController_AutoEngagesAnEnemyWithinEngageRangeWithNoPlayerOrder
// covers the user's explicit "при враге в 2 клетки от группы/юнита -
// вступать в бой": a soldier with no standing attack order opens fire on
// its own once an enemy comes within EngageRange, no right-click needed.
func TestController_AutoEngagesAnEnemyWithinEngageRangeWithNoPlayerOrder(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	// Archer: EngageRange (2) sits within ArcherRange (3), so the very
	// tick that auto-engages also lands a hit -- Swordsman's melee-only
	// range (1) would need an extra approach tick first, covered
	// separately by TestAttackOrder_ChasesATargetThatMovesOutOfRange.
	s := c.Spawn(Archer, 5, 5)
	target := enemy.New(5, 5+EngageRange) // exactly at the auto-engage edge

	c.Tick(grid, nil, []*enemy.Enemy{target})
	if !s.HasAttackOrder() {
		t.Fatal("soldier did not auto-engage an enemy within EngageRange")
	}
	if target.HP != combat.MaxHP-combat.UnitDamagePerHit {
		t.Fatalf("target HP after the auto-engage tick = %d, want a landed hit (%d)", target.HP, combat.MaxHP-combat.UnitDamagePerHit)
	}
}

// TestController_DoesNotAutoEngageBeyondEngageRange is the counterpart:
// an enemy just outside EngageRange is left alone until it (or the
// player) closes the distance.
func TestController_DoesNotAutoEngageBeyondEngageRange(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	s := c.Spawn(Swordsman, 5, 5)
	target := enemy.New(5, 5+EngageRange+1)

	c.Tick(grid, nil, []*enemy.Enemy{target})
	if s.HasAttackOrder() {
		t.Fatal("soldier auto-engaged an enemy beyond EngageRange")
	}
}

func TestNeedsDelivery_TriggersAtThirtyPercentSatiety(t *testing.T) {
	s := New(Archer, 0, 0)
	if s.NeedsDelivery() {
		t.Fatal("a freshly created soldier already needs delivery")
	}
	// Push satiety down to exactly the 30% threshold.
	for s.SatietyPercent() > DeliveryThresholdPercent {
		s.ticksSinceMeal++
	}
	if !s.NeedsDelivery() {
		t.Fatalf("NeedsDelivery() = false at satiety %d%%, want true at threshold %d%%", s.SatietyPercent(), DeliveryThresholdPercent)
	}
	s.Feed()
	if s.NeedsDelivery() {
		t.Fatal("NeedsDelivery() = true immediately after Feed()")
	}
}

func TestController_StarvationStillKills(t *testing.T) {
	grid := world.NewGrid(4, 4)
	c := NewController()
	c.Spawn(Archer, 0, 0)

	deaths := 0
	for range hunger.MaxTicks + 10 {
		deaths += c.Tick(grid, nil, nil)
	}
	if deaths != 1 {
		t.Fatalf("deaths after starving out a soldier with no delivery = %d, want 1", deaths)
	}
	if len(c.Soldiers) != 0 {
		t.Fatalf("roster after starvation = %d, want 0", len(c.Soldiers))
	}
}
