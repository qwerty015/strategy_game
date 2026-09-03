// Package combat is the shared, engine-free math for HP/damage/repair --
// see AGENTS.md. It deliberately knows nothing about *building.Building or
// *enemy.Enemy: every function here works on a plain int (0-100, the same
// scale building.MaxHP/enemy.MaxHP already use), so neither of those
// packages needs to depend on this one just to change a number, and this
// package needs no dependency on either of them.
package combat

// MaxHP is full health, 0-100 scale -- kept here too (duplicating
// building.MaxHP/enemy.MaxHP) so a caller that only imports combat still
// has it, the same reasoning building.BufferCapacity's small duplicated
// constants elsewhere in this codebase already follow.
const MaxHP = 100

// DamagePerHit is how much HP a single hit removes from a *structure*
// (building or wall) -- the user's explicit "один удар мечом(луком) -
// 10%" rule.
const DamagePerHit = 10

// UnitDamagePerHit is how much HP a single hit removes from a *unit* --
// the user's explicit "один удар мечом/стрелой лука - 50% ХП юниту" rule
// (two hits kill), used by package soldier's Archer/Swordsman against the
// debug Enemy. Deliberately not used the other way around yet: the debug
// Enemy still only attacks buildings (DamagePerHit), not soldiers -- see
// AGENTS.md's notes on this round's scope.
const UnitDamagePerHit = 50

// ApplyDamage subtracts amount from hp, clamped to [0, MaxHP].
func ApplyDamage(hp, amount int) int {
	hp -= amount
	if hp < 0 {
		return 0
	}
	return hp
}

// IsDestroyed reports whether hp represents a fully destroyed target.
func IsDestroyed(hp int) bool {
	return hp <= 0
}

// Repair adds amount to hp, clamped to [0, MaxHP].
func Repair(hp, amount int) int {
	hp += amount
	if hp > MaxHP {
		return MaxHP
	}
	return hp
}

// IntruderTarget is any single opposing-faction unit an attacker (a
// Sentry's tower shot, a Soldier's melee/ranged strike) can hit -- shared
// here, not in package sentry or soldier, for the same "engine-free,
// neither side needs to depend on the other" reasoning this whole
// package already follows: cmd/game (which already imports every worker
// package) builds one of these per living opposing unit and hands the
// whole slice to whichever attacker's Tick needs it, without soldier and
// sentry needing to import each other or duplicate the same tiny struct.
// A real gap found from two separate playtest reports ("почему башня не
// убила его слуг", "боевые юниты могут уничтожать любых юнитов
// противника - это враги!"): before this, neither a Sentry nor a Soldier
// could target anything belonging to the "1×1 против ИИ" opponent except
// its other soldiers (and, for a Soldier, its buildings) -- an unarmed
// enemy serf/villager/lumberjack/... was untouchable by either.
type IntruderTarget struct {
	X, Y  int
	Alive func() bool
	Kill  func()
}
