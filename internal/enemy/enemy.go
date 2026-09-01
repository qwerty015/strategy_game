// Package enemy is a minimal, engine-free stand-in for an attacking unit.
// There is no autonomous AI here -- a real attacker (archer/swordsman) is
// a separate, later pass, see AGENTS.md's roadmap notes. This package
// exists so the defensive side of the game (WatchTower + package sentry,
// HP, package combat, the Builder's auto-repair) can be exercised end to
// end in a real game session before real attackers exist -- see
// cmd/game's debug spawn hotkey.
//
// Per the user's explicit request, an Enemy IS player-directed: select it
// and right-click a tile to walk there (see MoveTo/RemainingPath and
// cmd/game's selection + move-order handling). This same "select, then
// right-click a point to walk there" shape is meant to carry over to
// future player-controlled combat units (swordsman/archer) once they
// exist -- it isn't a one-off hack specific to the debug enemy.
package enemy

import (
	"strategy_game/internal/building"
	"strategy_game/internal/combat"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/world"
)

// TicksPerTile is how many simulation ticks it takes an Enemy to cross
// one tile -- the same movement speed every other free-roaming unit in
// this game uses (package builder, lumberjack, ...).
const TicksPerTile = 2

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

// Enemy is a target with health at a world tile position. It never
// chooses an attack target or a destination on its own -- Tick only ever
// damages a building already within AttackRange (see that constant's doc
// comment), and it only walks anywhere because the player issued a move
// order via MoveTo. Something with real AI (a later pass, see
// AGENTS.md's roadmap) would choose both on its own instead.
type Enemy struct {
	X, Y int
	HP   int

	attackCooldown int

	path      []pathfind.Point
	pathIdx   int
	tileTicks int
}

// New creates an enemy at (x, y) with full health.
func New(x, y int) *Enemy {
	return &Enemy{X: x, Y: y, HP: MaxHP}
}

// Alive reports whether the enemy still has health remaining.
func (e *Enemy) Alive() bool {
	return e != nil && e.HP > 0
}

// MoveTo computes a path from e's current position to (x, y) over land --
// buildings and water block it, the same free-roaming movement rule
// package builder/lumberjack already use, not the road network serfs need
// -- and starts walking it. Reports whether a route was found; a false
// result leaves any route e was already following untouched, so clicking
// an unreachable tile doesn't cancel an order that was still valid.
func (e *Enemy) MoveTo(grid *world.Grid, buildings []*building.Building, x, y int) bool {
	path, ok := pathfind.FindLandPath(grid, buildings, pathfind.Point{X: e.X, Y: e.Y}, pathfind.Point{X: x, Y: y})
	if !ok {
		return false
	}
	e.path, e.pathIdx, e.tileTicks = path, 0, 0
	return true
}

// RemainingPath returns the tiles still ahead on e's current route,
// starting from (and including) the tile it's walking toward right now --
// for the same route-line overlay every other unit's selection already
// gets (see ui.DrawSelectedRoute). nil once idle or with no order given.
func (e *Enemy) RemainingPath() []pathfind.Point {
	if e.pathIdx >= len(e.path) {
		return nil
	}
	return e.path[e.pathIdx:]
}

// Tick advances every living enemy's move order (if any) one step, then
// lets it damage the nearest finished building within AttackRange, once
// every AttackCooldownTicks -- see AttackRange's doc comment for why the
// attack half of this exists at all despite "no AI". Call once per
// simulation tick from cmd/game, alongside every other controller's Tick.
func Tick(enemies []*Enemy, buildings []*building.Building) {
	for _, e := range enemies {
		if !e.Alive() {
			continue
		}
		tickMovement(e)
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

// tickMovement advances e one step along its current path every
// TicksPerTile ticks -- the same tile-by-tile walk every other unit in
// this game already does (see e.g. package sentry's tickWalking).
func tickMovement(e *Enemy) {
	if len(e.path) == 0 {
		return
	}
	e.tileTicks++
	if e.tileTicks < TicksPerTile {
		return
	}
	e.tileTicks = 0
	if e.pathIdx < len(e.path)-1 {
		e.pathIdx++
		e.X, e.Y = e.path[e.pathIdx].X, e.path[e.pathIdx].Y
		return
	}
	// Arrived: clear the order instead of endlessly "finishing" the last step.
	e.path = nil
	e.pathIdx = 0
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
