package soldier

import (
	"testing"

	"strategy_game/internal/building"
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
		c.Tick(grid, nil, nil, nil, nil, nil)
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

	c.Tick(grid, nil, nil, nil, nil, nil)
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
	c.Tick(grid, nil, nil, nil, nil, nil)
	if near.HP != combat.MaxHP-combat.UnitDamagePerHit {
		t.Fatalf("target at exactly ArcherRange (%d) HP = %d, want a landed hit (%d)", ArcherRange, near.HP, combat.MaxHP-combat.UnitDamagePerHit)
	}

	far := enemy.New(5, 5+ArcherRange+1)
	c2 := NewController()
	s2 := c2.Spawn(Archer, 5, 5)
	s2.AttackOrder(grid, nil, far)
	c2.Tick(grid, nil, nil, nil, nil, nil)
	if far.HP != combat.MaxHP {
		t.Fatal("target one tile beyond ArcherRange took damage -- should still be out of range on the first tick")
	}
}

// TestTwoHitsKillTheTarget covers both the user's explicit unit-damage
// rule (50% per hit, reused from package combat) and the "1 hit per tick"
// combat-pacing rule (ArcherCooldownTicks/SwordsmanCooldownTicks == 0,
// per the user's explicit "за 1 тик юнит наносит 1 удар. 2 удара == 2
// тика"): with no cooldown, two hits land on two consecutive ticks. The
// second (killing) hit's HP zeroing is deferred until its own attack
// visual finishes (see pendingKillTarget's doc comment -- the same fix as
// the user's reported "стрела убивает раньше чем долетает" bug), so the
// target must not report dead the instant that second hit registers, only
// attackVisualLifetime ticks later.
func TestTwoHitsKillTheTarget(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	s := c.Spawn(Swordsman, 5, 5)
	target := enemy.New(5, 6)
	s.AttackOrder(grid, nil, target)

	c.Tick(grid, nil, nil, nil, nil, nil)
	if target.HP != combat.MaxHP-combat.UnitDamagePerHit {
		t.Fatalf("target HP after 1 hit = %d, want %d", target.HP, combat.MaxHP-combat.UnitDamagePerHit)
	}

	c.Tick(grid, nil, nil, nil, nil, nil) // the second, lethal hit -- HP not applied yet
	if !target.Alive() {
		t.Fatal("target died the instant the killing blow registered -- its death must wait for the attack visual (pendingKillTarget)")
	}

	for range attackVisualLifetime {
		c.Tick(grid, nil, nil, nil, nil, nil)
	}
	if target.Alive() {
		t.Fatal("target still alive after its killing blow's attack visual finished")
	}
}

// TestTwoHitsKillTheTarget_ExactThreeTickTimeline locks in the precise
// tick-by-tick schedule the user explicitly asked for: "1 тик удар/стрела
// 1, 2 тик - второй удар/стрела и всё, 3 тик уже анимация смерти юнита".
// A regression here (e.g. attackVisualLifetime creeping back up) would
// silently stretch this exchange back out.
func TestTwoHitsKillTheTarget_ExactThreeTickTimeline(t *testing.T) {
	grid := world.NewGrid(10, 10)
	c := NewController()
	s := c.Spawn(Archer, 5, 5)
	target := enemy.New(5, 8) // within ArcherRange (3)
	s.AttackOrder(grid, nil, target)

	c.Tick(grid, nil, nil, nil, nil, nil) // tick 1: first shot
	if target.HP != combat.MaxHP-combat.UnitDamagePerHit {
		t.Fatalf("HP after tick 1 = %d, want %d", target.HP, combat.MaxHP-combat.UnitDamagePerHit)
	}

	c.Tick(grid, nil, nil, nil, nil, nil) // tick 2: second (lethal) shot registers
	if !target.Alive() {
		t.Fatal("target died on tick 2 -- the killing blow's death must not land until tick 3")
	}

	c.Tick(grid, nil, nil, nil, nil, nil) // tick 3: the deferred kill resolves
	if target.Alive() {
		t.Fatal("target still alive on tick 3 -- want it dead by exactly this tick")
	}
}

// TestMoveTo_SurvivesAutoEngageWhenTheOldTargetIsStillNearby reproduces the
// user's explicit bug report: after landing one (non-lethal) hit on an
// adjacent enemy, the swordsman "froze" and could no longer be moved at
// all. Root cause: MoveTo clears attackTarget as its retreat signal, but
// the very next tick auto-engage saw attackTarget == nil, immediately
// found the same still-nearby enemy (right after a melee exchange it is
// almost always still within EngageRange) and re-issued an AttackOrder,
// whose approach() then found the soldier already in range and cancelled
// the just-started path right back out. A soldier with an order already
// in progress (a non-empty path) must not have it overridden this way.
func TestMoveTo_SurvivesAutoEngageWhenTheOldTargetIsStillNearby(t *testing.T) {
	grid := world.NewGrid(20, 20)
	c := NewController()
	s := c.Spawn(Swordsman, 5, 5)
	target := enemy.New(5, 6) // adjacent -- within AttackRange and EngageRange
	enemies := []*enemy.Enemy{target}
	s.AttackOrder(grid, nil, target)
	c.Tick(grid, nil, enemies, nil, nil, nil) // lands the first, non-lethal hit

	if !s.MoveTo(grid, nil, 15, 5) {
		t.Fatal("MoveTo failed to find a route away from the enemy")
	}
	if len(s.RemainingPath()) == 0 {
		t.Fatal("MoveTo did not start a route")
	}

	c.Tick(grid, nil, enemies, nil, nil, nil) // the tick that used to cancel it
	if len(s.RemainingPath()) == 0 {
		t.Fatal("the move order was cancelled by auto-engage even though the player just issued it")
	}
	if s.HasAttackOrder() {
		t.Fatal("auto-engage re-locked onto the enemy the player was trying to move away from")
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
		c.Tick(grid, nil, nil, nil, nil, nil)
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

	c.Tick(grid, nil, []*enemy.Enemy{target}, nil, nil, nil)
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

	c.Tick(grid, nil, []*enemy.Enemy{target}, nil, nil, nil)
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
		deaths += c.Tick(grid, nil, nil, nil, nil, nil).Deaths
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

	c.Tick(grid, nil, nil, opposingBuildings, nil, nil) // auto-engage + kill in one hit (HP == one hit's worth)
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

	result := c.Tick(grid, nil, nil, []*building.Building{rival}, nil, nil)
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

	result := c.Tick(grid, nil, nil, nil, rivalController.Soldiers, nil)
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

	result := c.Tick(grid, nil, nil, nil, nil, []combat.IntruderTarget{intruder})
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
		c.Tick(grid, nil, nil, []*building.Building{rival}, nil, nil)
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
		c.Tick(grid, nil, nil, nil, rivalController.Soldiers, nil)
		if !rival.Alive() {
			break
		}
	}
	if rival.Alive() {
		t.Fatal("swordsman never closed the distance and destroyed the ordered rival soldier")
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
		c.Tick(grid, nil, nil, nil, nil, []combat.IntruderTarget{intruder})
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

	c.Tick(grid, nil, nil, nil, nil, []combat.IntruderTarget{intruder})
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

	c.Tick(grid, nil, nil, nil, rivalController.Soldiers, nil)
	if rival.HP != combat.MaxHP-combat.UnitDamagePerHit {
		t.Fatalf("rival soldier HP = %d, want %d after one auto-engaged hit", rival.HP, combat.MaxHP-combat.UnitDamagePerHit)
	}
	if !s.HasFactionTarget() {
		t.Fatal("soldier did not lock onto the rival soldier as its faction target")
	}
}
