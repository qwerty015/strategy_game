package soldier

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/combat"
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
		c.Tick(grid, nil, nil, nil, nil)
	}

	if s.X != 5 || s.Y != 0 {
		t.Fatalf("position after ticking = (%d, %d), want (5, 0)", s.X, s.Y)
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
		deaths += c.Tick(grid, nil, nil, nil, nil).Deaths
	}
	if deaths != 1 {
		t.Fatalf("deaths after starving out a soldier with no delivery = %d, want 1", deaths)
	}
	if len(c.Soldiers) != 0 {
		t.Fatalf("roster after starvation = %d, want 0", len(c.Soldiers))
	}
}

// TestController_AutoEngagesAnOpposingBuildingWithinFactionEngageRange
// covers "1×1 против ИИ" mode's actual fighting: a soldier with no debug
// enemy target and no move order auto-attacks a rival building within
// FactionEngageRange, chipping it down with combat.DamagePerHit per hit
// until it's destroyed -- the mechanism the win condition (all rival
// buildings/units destroyed) depends on.
func TestController_AutoEngagesAnOpposingBuildingWithinFactionEngageRange(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	c.Spawn(Swordsman, 5, 5)
	rival := &building.Building{Kind: building.Warehouse, X: 5, Y: 6, HP: combat.DamagePerHit}
	opposingBuildings := []*building.Building{rival}

	c.Tick(grid, nil, opposingBuildings, nil, nil) // auto-engage + kill in one hit (HP == one hit's worth)
	if rival.HP > 0 {
		t.Fatalf("rival building HP = %d, want 0 after one hit at exactly combat.DamagePerHit health", rival.HP)
	}
}

// TestController_Tick_ReportsBuildingsDestroyedOnLethalHit,
// TestController_Tick_ReportsKillOnLethalSoldierHit and
// TestController_Tick_ReportsKillOnIntruderKill are the regression tests
// for a real playtest report ("счетчик убито врагов не считает юнитов,
// нужно считать убитых с помощью башни или убитых боевыми юнитами, и
// введи новый счетчик: 'Разрушено построек'"): Controller.Tick's
// TickResult must actually report Kills/BuildingsDestroyed for real
// cross-faction combat, not just Deaths -- cmd/game had no way to credit
// economy.Population.Kills/EnemyBuildingsDestroyed for anything beyond
// the sandbox debug enemy before this.
func TestController_Tick_ReportsBuildingsDestroyedOnLethalHit(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	c.Spawn(Swordsman, 5, 5)
	rival := &building.Building{Kind: building.Warehouse, X: 5, Y: 6, HP: combat.DamagePerHit}

	result := c.Tick(grid, nil, []*building.Building{rival}, nil, nil)
	if result.BuildingsDestroyed != 1 {
		t.Fatalf("TickResult.BuildingsDestroyed = %d, want 1", result.BuildingsDestroyed)
	}
	if result.Kills != 0 {
		t.Fatalf("TickResult.Kills = %d, want 0 (a building was destroyed, not a unit killed)", result.Kills)
	}
}

func TestController_Tick_ReportsKillOnLethalSoldierHit(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	c.Spawn(Archer, 5, 5)
	rivalController := NewController()
	rival := rivalController.Spawn(Swordsman, 5, 5+FactionEngageRange)
	rival.HP = combat.UnitDamagePerHit // one hit is lethal

	result := c.Tick(grid, nil, nil, rivalController.Soldiers, nil)
	if result.Kills != 1 {
		t.Fatalf("TickResult.Kills = %d, want 1", result.Kills)
	}
	if result.BuildingsDestroyed != 0 {
		t.Fatalf("TickResult.BuildingsDestroyed = %d, want 0 (a soldier was killed, not a building destroyed)", result.BuildingsDestroyed)
	}
}

func TestController_Tick_ReportsKillOnIntruderKill(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	c.Spawn(Swordsman, 5, 5)
	alive := true
	intruder := combat.IntruderTarget{
		X: 5, Y: 6,
		Alive: func() bool { return alive },
		Kill:  func() { alive = false },
	}

	result := c.Tick(grid, nil, nil, nil, []combat.IntruderTarget{intruder})
	if result.Kills != 1 {
		t.Fatalf("TickResult.Kills = %d, want 1", result.Kills)
	}
}

// TestAttackFactionOrder_ChasesAndDestroysADistantOpposingBuilding is the
// manual (right-click) counterpart to
// TestController_AutoEngagesAnOpposingBuildingWithinFactionEngageRange: a
// real bug found from an actual playtest report ("клик боевым юнитом на
// постройку противника - перемещает юнитов, но не уничтожает постройку
// врага, хотя они должны подойти для дистанции атаки и атаковать"). Before
// AttackFactionOrder existed, cmd/game's right-click handler had no way to
// set a cross-faction combat target at all, so a click on a distant
// opposing building fell through to a plain move order that walked the
// squad up to it without ever fighting. This confirms the standing order
// alone (target far outside FactionEngageRange, no auto-engage possible on
// the first tick) makes the soldier approach on its own and keep attacking
// until the building is destroyed.
func TestAttackFactionOrder_ChasesAndDestroysADistantOpposingBuilding(t *testing.T) {
	grid := world.NewGrid(20, 20)
	c := NewController()
	s := c.Spawn(Swordsman, 0, 0)
	rival := &building.Building{Kind: building.Warehouse, X: 10, Y: 0, HP: combat.MaxHP}
	s.AttackFactionOrder(rival)

	if !s.HasFactionTarget() {
		t.Fatal("AttackFactionOrder did not set a standing faction target")
	}

	for range 200 {
		c.Tick(grid, nil, []*building.Building{rival}, nil, nil)
		if rival.HP <= 0 {
			break
		}
	}
	if rival.HP > 0 {
		t.Fatal("swordsman never closed the distance and destroyed the ordered building")
	}
}

// TestAttackFactionOrder_ClearsOnNilOrDeadTarget mirrors AttackOrder's own
// clearing convention (see AttackOrder's doc comment) -- a nil target, or
// one already at 0 HP, must clear any existing standing order instead of
// leaving the soldier fighting a corpse/nothing.
func TestAttackFactionOrder_ClearsOnNilOrDeadTarget(t *testing.T) {
	c := NewController()
	s := c.Spawn(Swordsman, 0, 0)
	rival := &building.Building{Kind: building.Warehouse, X: 1, Y: 0, HP: combat.MaxHP}
	s.AttackFactionOrder(rival)
	if !s.HasFactionTarget() {
		t.Fatal("test setup: expected a standing faction target")
	}

	s.AttackFactionOrder(nil)
	if s.HasFactionTarget() {
		t.Fatal("AttackFactionOrder(nil) did not clear the standing faction target")
	}

	s.AttackFactionOrder(rival)
	rival.HP = 0
	s.AttackFactionOrder(rival)
	if s.HasFactionTarget() {
		t.Fatal("AttackFactionOrder(deadTarget) did not clear the standing faction target")
	}
}

// TestAttackFactionSoldierOrder_ChasesAndDestroysADistantRival mirrors
// TestAttackFactionOrder_ChasesAndDestroysADistantOpposingBuilding for a
// rival Soldier target instead of a building -- part of the follow-up
// request "клик боевым юнитом на любого юнита/постройку противника,
// должен переходить в режим атаки" (a rival soldier, not just a
// building, must also become a real standing attack order).
func TestAttackFactionSoldierOrder_ChasesAndDestroysADistantRival(t *testing.T) {
	grid := world.NewGrid(20, 20)
	c := NewController()
	s := c.Spawn(Swordsman, 0, 0)
	rivalController := NewController()
	rival := rivalController.Spawn(Swordsman, 10, 0)
	s.AttackFactionSoldierOrder(rival)

	if !s.HasFactionTarget() {
		t.Fatal("AttackFactionSoldierOrder did not set a standing faction target")
	}

	for range 200 {
		c.Tick(grid, nil, nil, rivalController.Soldiers, nil)
		if !rival.Alive() {
			break
		}
	}
	if rival.Alive() {
		t.Fatal("swordsman never closed the distance and destroyed the ordered rival soldier")
	}
}

// TestController_Tick_RemovesASoldierKilledByAnotherController is the
// regression test for a real, serious playtest bug found from an actual
// siege ("куда они все смотрели?" -- the player's own archers/WatchTower
// genuinely reduced an invading soldier's HP to 0, confirmed via
// combat.ApplyDamage, but that "zombie" soldier then kept marching and
// destroyed the warehouse anyway): Controller.Tick never checked whether
// the soldier it was about to move/fight with was still alive -- only a
// starved soldier ever got dropped from the roster. A rival hit always
// lands on some OTHER controller's Tick call earlier in the same overall
// simulation tick, so by the time THIS soldier's own controller ticks,
// its HP already reflects the hit; this confirms it's now removed (and
// stops moving toward its own standing order) the very next tick.
func TestController_Tick_RemovesASoldierKilledByAnotherController(t *testing.T) {
	grid := world.NewGrid(20, 20)
	c := NewController()
	s := c.Spawn(Swordsman, 0, 0)
	target := &building.Building{Kind: building.Warehouse, X: 15, Y: 0, HP: building.MaxHP}
	s.AttackFactionOrder(target)

	// Simulate the lethal hit that just landed on s from some other
	// attacker (a rival soldier, or a WatchTower) -- exactly what
	// combat.ApplyDamage driving HP to 0 looks like from s's own
	// perspective, mid-battle.
	s.HP = 0

	result := c.Tick(grid, nil, nil, nil, nil)
	if result.Deaths != 1 {
		t.Fatalf("TickResult.Deaths = %d, want 1", result.Deaths)
	}
	if len(c.Soldiers) != 0 {
		t.Fatalf("roster after ticking a soldier already at 0 HP = %d, want 0 (must be removed, not left to keep fighting)", len(c.Soldiers))
	}
	if s.X != 0 {
		t.Fatal("a soldier already at 0 HP moved toward its standing order -- it must be inert, not a functioning zombie")
	}
}

// TestAttackFactionIntruderOrder_KillsADistantIntruder is the same
// coverage for the third and last opposing-target kind: any other rival
// unit with no HP concept of its own (see combat.IntruderTarget).
func TestAttackFactionIntruderOrder_KillsADistantIntruder(t *testing.T) {
	grid := world.NewGrid(20, 20)
	c := NewController()
	s := c.Spawn(Swordsman, 0, 0)

	killed := false
	alive := true
	intruder := combat.IntruderTarget{
		X: 10, Y: 0,
		Alive: func() bool { return alive },
		Kill:  func() { killed = true; alive = false },
	}
	s.AttackFactionIntruderOrder(intruder)

	if !s.HasFactionTarget() {
		t.Fatal("AttackFactionIntruderOrder did not set a standing faction target")
	}

	for range 200 {
		c.Tick(grid, nil, nil, nil, []combat.IntruderTarget{intruder})
		if killed {
			break
		}
	}
	if !killed {
		t.Fatal("swordsman never closed the distance and killed the ordered intruder")
	}
}

// TestAttackFactionSoldierOrder_ClearsOnNilOrDeadTarget and its intruder
// counterpart mirror AttackFactionOrder's own clearing convention.
func TestAttackFactionSoldierOrder_ClearsOnNilOrDeadTarget(t *testing.T) {
	c := NewController()
	s := c.Spawn(Swordsman, 0, 0)
	rivalController := NewController()
	rival := rivalController.Spawn(Swordsman, 1, 0)

	s.AttackFactionSoldierOrder(rival)
	if !s.HasFactionTarget() {
		t.Fatal("test setup: expected a standing faction target")
	}
	s.AttackFactionSoldierOrder(nil)
	if s.HasFactionTarget() {
		t.Fatal("AttackFactionSoldierOrder(nil) did not clear the standing faction target")
	}

	s.AttackFactionSoldierOrder(rival)
	rival.HP = 0
	s.AttackFactionSoldierOrder(rival)
	if s.HasFactionTarget() {
		t.Fatal("AttackFactionSoldierOrder(deadTarget) did not clear the standing faction target")
	}
}

func TestAttackFactionIntruderOrder_ClearsOnDeadOrEmptyTarget(t *testing.T) {
	c := NewController()
	s := c.Spawn(Swordsman, 0, 0)
	alive := true
	intruder := combat.IntruderTarget{X: 1, Y: 0, Alive: func() bool { return alive }, Kill: func() { alive = false }}

	s.AttackFactionIntruderOrder(intruder)
	if !s.HasFactionTarget() {
		t.Fatal("test setup: expected a standing faction target")
	}

	alive = false
	s.AttackFactionIntruderOrder(intruder)
	if s.HasFactionTarget() {
		t.Fatal("AttackFactionIntruderOrder(deadTarget) did not clear the standing faction target")
	}

	s.AttackFactionIntruderOrder(intruder) // Alive still false from above
	s.AttackFactionIntruderOrder(combat.IntruderTarget{})
	if s.HasFactionTarget() {
		t.Fatal("AttackFactionIntruderOrder(zero value) did not clear the standing faction target")
	}
}

// TestController_AutoEngagesAnOpposingIntruder covers the user's explicit
// request "боевые юниты могут уничтожать любых юнитов противника - это
// враги!": a Soldier must be able to auto-engage ANY opposing unit, not
// only a rival Soldier or building -- an unarmed rival serf/villager/etc.
// (represented here the same way cmd/game's real intruderTargetsFrom
// wraps one, and sentry_test.go's own intruder test does) is a one-hit
// kill via its own Kill callback, same as a WatchTower's shot.
func TestController_AutoEngagesAnOpposingIntruder(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	c.Spawn(Swordsman, 5, 5)

	killed := false
	alive := true
	intruder := combat.IntruderTarget{
		X: 5, Y: 6, // within FactionEngageRange
		Alive: func() bool { return alive },
		Kill:  func() { killed = true; alive = false },
	}

	c.Tick(grid, nil, nil, nil, []combat.IntruderTarget{intruder})
	if !killed {
		t.Fatal("intruder was not killed after one auto-engaged tick")
	}
}

// TestController_AutoEngagesAnOpposingSoldier is the same coverage for a
// rival soldier instead of a building.
func TestController_AutoEngagesAnOpposingSoldier(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	s := c.Spawn(Archer, 5, 5)
	rivalController := NewController()
	rival := rivalController.Spawn(Swordsman, 5, 5+FactionEngageRange) // within both FactionEngageRange and ArcherRange

	c.Tick(grid, nil, nil, rivalController.Soldiers, nil)
	if rival.HP != combat.MaxHP-combat.UnitDamagePerHit {
		t.Fatalf("rival soldier HP = %d, want %d after one auto-engaged hit", rival.HP, combat.MaxHP-combat.UnitDamagePerHit)
	}
	if !s.HasFactionTarget() {
		t.Fatal("soldier did not lock onto the rival soldier as its faction target")
	}
}
