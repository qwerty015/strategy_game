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
// (building or wall) -- rebalanced from the original 10% to 5% per the
// user's own explicit request ("1 удар юнита в здание снимает... 5%
// хп"), twenty hits to bring one down instead of ten.
const DamagePerHit = 5

// DecayIntervalTicks/DecayAmount are the user's own explicit passive-
// damage rule for a finished building sitting below full health with
// nobody repairing it: "если здание не восстанавливать, ХП уменьшается
// по 1% за 10 тиков". Only ever applied while nothing is actively
// repairing the building (see cmd/game's decayDamagedBuildings) --
// the user's own explicit choice, confirmed when asked directly, so an
// active repair job never has to race a ticking clock.
const (
	DecayIntervalTicks = 10
	DecayAmount        = 1

	// DecayFloor is how low passive decay alone can bring a building --
	// never destroys it outright. The user's own explicit choice,
	// confirmed when asked directly ("не может ли здание само
	// развалиться до 0 без боя" -> нет, есть пол): only real combat
	// damage (DamagePerHit/UnitDamagePerHit) can push a building the
	// rest of the way to destruction.
	DecayFloor = 10
)

// UnitDamagePerHit is how much HP a single hit removes from a *unit* in
// direct soldier-vs-soldier combat -- rebalanced from the original 50%
// (two hits kill) to 20% per the user's own explicit request ("удар
// одним юнитом другого - 20% хп"), five hits to kill instead of two. Not
// used for a WatchTower's own shot against a unit -- see cmd/game's
// intruderTargetsFrom, which always kills outright per the user's
// separate "удар башни по юниту - 100% хп" rule.
const UnitDamagePerHit = 20

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

	// Owner is the faction this unit belongs to -- set by whichever
	// cmd/game helper builds the target list (see intruderTargetsFrom),
	// which already knows which faction it's reading from. Lets a
	// killing blow be credited to the right attacker (see cmd/game's
	// lastAttackerOwner) instead of only ever guessed at by geography.
	Owner int
}
