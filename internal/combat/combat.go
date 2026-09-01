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

// DamagePerHit is how much HP a single hit removes. Per the user's
// explicit rule ("один удар мечом(луком) - 10%" for structures), this is
// currently only used against buildings and the debug Enemy -- unit HP
// is a deliberately separate, later pass (see AGENTS.md's roadmap notes:
// there's no real attacker that could ever damage a unit yet).
const DamagePerHit = 10

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
