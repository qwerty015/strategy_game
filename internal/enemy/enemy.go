// Package enemy is a minimal, engine-free stand-in for an attacking unit.
// There is no AI, movement, or pathfinding here -- a real attacker
// (archer/swordsman) is a separate, later pass, see AGENTS.md's roadmap
// notes. This package exists solely so the defensive side of the game
// (WatchTower + package sentry, HP, package combat, the Builder's
// auto-repair) can be exercised end to end in a real game session before
// real attackers exist -- see cmd/game's debug spawn hotkey.
package enemy

import (
	"strategy_game/internal/building"
	"strategy_game/internal/combat"
)

// MaxHP is full health, 0-100 scale, matching building.MaxHP/combat.MaxHP.
const MaxHP = 100

const (
	// AttackRange is how close (in tiles, from any footprint tile) a
	// finished building must be for an Enemy to damage it -- deliberately
	// short (adjacent only): this whole package exists to let the
	// Builder's auto-repair be exercised in a real session, not to be a
	// real siege AI, see this package's doc comment.
	AttackRange = 1

	// AttackCooldownTicks paces damage the same way package sentry paces
	// return fire, so a building doesn't drop to 0 HP in an instant.
	AttackCooldownTicks = 30
)

// Enemy is a stationary target with health at a fixed world tile
// position. It never moves or chooses a target on its own beyond the
// single static Tick below -- something with real AI (a later pass, see
// AGENTS.md's roadmap) would have to seek things out instead.
type Enemy struct {
	X, Y int
	HP   int

	attackCooldown int
}

// New creates an enemy at (x, y) with full health.
func New(x, y int) *Enemy {
	return &Enemy{X: x, Y: y, HP: MaxHP}
}

// Alive reports whether the enemy still has health remaining.
func (e *Enemy) Alive() bool {
	return e != nil && e.HP > 0
}

// Tick lets every living enemy damage the nearest finished building within
// AttackRange, once every AttackCooldownTicks -- see AttackRange's doc
// comment for why this exists at all despite "no AI". Call once per
// simulation tick from cmd/game, alongside every other controller's Tick.
func Tick(enemies []*Enemy, buildings []*building.Building) {
	for _, e := range enemies {
		if !e.Alive() {
			continue
		}
		if e.attackCooldown > 0 {
			e.attackCooldown--
			continue
		}
		target := nearestBuildingInRange(e.X, e.Y, buildings)
		if target == nil {
			continue
		}
		target.HP = combat.ApplyDamage(target.HP, combat.DamagePerHit)
		e.attackCooldown = AttackCooldownTicks
	}
}

// nearestBuildingInRange returns the first finished building whose
// footprint comes within AttackRange tiles of (ex, ey). "Nearest" only in
// the sense of "first found close enough" -- with AttackRange=1 there is
// essentially never more than one candidate at once, so no distance
// comparison is worth the extra code.
func nearestBuildingInRange(ex, ey int, buildings []*building.Building) *building.Building {
	for _, b := range buildings {
		if b == nil || b.ConstructionStage != building.ConstructionNone {
			continue
		}
		footprint := building.Types[b.Kind].Footprint
		if ex >= b.X-AttackRange && ex <= b.X+footprint-1+AttackRange &&
			ey >= b.Y-AttackRange && ey <= b.Y+footprint-1+AttackRange {
			return b
		}
	}
	return nil
}
