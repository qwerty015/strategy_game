// Package soldier simulates the two mobile combat professions hired
// through the Barracks: Archer and Swordsman. Unlike every other unit in
// this game they never walk to a Tavern themselves when hungry -- food is
// delivered to them by a serf instead (package logistics); see
// NeedsDelivery/Feed. They are entirely player-directed (select, then
// right-click a point to walk there or an enemy to attack), the same
// control model as the debug enemy (package enemy) -- neither profession
// picks its own destination or target.
package soldier

import (
	"strategy_game/internal/building"
	"strategy_game/internal/combat"
	"strategy_game/internal/enemy"
	"strategy_game/internal/hunger"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/world"
)

// Profession distinguishes an Archer from a Swordsman -- everything else
// (movement, hunger/delivery) is identical; only AttackRange and the
// attack cooldown differ.
type Profession int

const (
	Archer Profession = iota
	Swordsman
)

const (
	// TicksPerTile is how many simulation ticks it takes a soldier to
	// cross one tile -- the same movement speed every other free-roaming
	// unit in this game uses.
	TicksPerTile = 2

	// DeliveryThresholdPercent is the satiety at which a soldier needs a
	// serf to bring food -- higher than every other profession's 20% (see
	// hunger.MealThresholdTicks), per the user's explicit "при наступлении
	// голода 30%": a soldier can't walk to the Tavern itself, so it needs
	// more buffer before help is called for.
	DeliveryThresholdPercent = 30

	// ArcherRange/SwordsmanRange are square (Chebyshev) attack radii in
	// tiles -- the user's explicit "лучник — может атаковать за 3 клетки"
	// and "мечник — клетки врага и наша — одинаковые" (adjacent, so 1).
	ArcherRange    = 3
	SwordsmanRange = 1

	// ArcherCooldownTicks/SwordsmanCooldownTicks pace attacks -- zero, per
	// the user's explicit combat-rebalance request ("за 1 тик юнит
	// наносит 1 удар. 2 удара == 2 тика"): a soldier in range lands a hit
	// every single simulation tick, so two hits (combat.UnitDamagePerHit
	// each) kill in exactly two ticks. Previously 20/15 (paced like
	// sentry.ShotCooldownTicks); kept as named constants rather than
	// inlined zeros so cooldownTicks/the doc comments explaining the
	// rule stay in one place if it's ever tuned again.
	ArcherCooldownTicks    = 0
	SwordsmanCooldownTicks = 0

	// attackVisualLifetime keeps a landed blow on screen for exactly one
	// tick before a lethal hit's deferred kill (see pendingKillTarget)
	// resolves -- per the user's explicit "1 тик удар/стрела 1, 2 тик -
	// второй удар/стрела и всё, 3 тик уже анимация смерти юнита": with
	// ArcherCooldownTicks/SwordsmanCooldownTicks == 0, hit 1 lands tick 1,
	// hit 2 (lethal) lands tick 2, and this being 1 means the deferred
	// kill resolves exactly tick 3 -- the whole exchange takes 3 ticks
	// (1.5s at 1x speed), not the 8 ticks a longer lifetime produced.
	// Was 6 (a full three-frame wind-up/impact/recovery animation); at 1
	// the renderer only ever samples AttackVisual() at progress == 1, so
	// an Archer's arrow now appears already at the target rather than
	// visibly flying there -- an accepted trade-off for hitting this
	// exact tick timeline.
	attackVisualLifetime = 1

	// EngageRange is the distance (Chebyshev) at which a soldier with no
	// standing attack order automatically opens fire on the nearest enemy,
	// without waiting for a player right-click -- per the user's explicit
	// "при враге в 2 клетки от группы/юнита - вступать в бой". Deliberately
	// smaller than either AttackRange, so a soldier already has the enemy
	// well within striking distance the instant it engages.
	EngageRange = 2
)

// FactionEngageRange mirrors EngageRange for cross-faction auto-combat --
// see TickFactionCombat/factionTarget. Same value, kept as its own named
// constant since the two mechanisms are otherwise fully independent.
const FactionEngageRange = 2

// factionTarget is a soldier's current cross-faction combat target -- a
// rival building, soldier, or any other opposing unit (see intruder,
// combat.IntruderTarget), used only by "1×1 против ИИ" mode. Exactly one
// field is non-nil, or all nil for "no target". Deliberately separate
// from attackTarget/AttackOrder above, which stays the sandbox-only debug
// enemy.Enemy mechanism, untouched by any of this.
type factionTarget struct {
	building *building.Building
	soldier  *Soldier

	// intruder is any opposing unit that isn't a rival Soldier -- a real
	// gap found from an actual playtest report ("боевые юниты могут
	// уничтожать любых юнитов противника - это враги!"): before this, a
	// Soldier could only ever fight a rival Soldier or a building, never
	// an unarmed enemy serf/villager/lumberjack/.... See
	// combat.IntruderTarget's doc comment -- the exact same shape
	// package sentry's WatchTower already uses for its own identical gap.
	intruder *combat.IntruderTarget
}

func (t factionTarget) alive() bool {
	switch {
	case t.building != nil:
		return t.building.HP > 0
	case t.soldier != nil:
		return t.soldier.Alive()
	case t.intruder != nil:
		return t.intruder.Alive != nil && t.intruder.Alive()
	default:
		return false
	}
}

func (t factionTarget) pos() (int, int) {
	switch {
	case t.building != nil:
		return t.building.X, t.building.Y
	case t.soldier != nil:
		return t.soldier.X, t.soldier.Y
	case t.intruder != nil:
		return t.intruder.X, t.intruder.Y
	default:
		return 0, 0
	}
}

// hit applies one blow: combat.DamagePerHit (10%, the same rate every
// other structure-damaging attack in the game uses) against a building,
// combat.UnitDamagePerHit (50%, two hits kill) against a rival soldier --
// matching the unit-damage rule already established for every other
// soldier-vs-unit fight in this package. An intruder (any other opposing
// unit) is a one-hit kill via its own Kill callback, the same convention
// package sentry's WatchTower already uses against the same kind of
// target -- these units have no HP concept to apply partial damage to at
// all (see package villagers' Kill method and its siblings).
func (t factionTarget) hit() {
	switch {
	case t.building != nil:
		t.building.HP = combat.ApplyDamage(t.building.HP, combat.DamagePerHit)
	case t.soldier != nil:
		t.soldier.HP = combat.ApplyDamage(t.soldier.HP, combat.UnitDamagePerHit)
	case t.intruder != nil:
		t.intruder.Kill()
	}
}

// nearestFactionTarget returns the closest living opposing building,
// soldier, or other unit (intruders) to (x, y) within a square
// (Chebyshev) radius, or ok == false if none qualifies. Every candidate
// kind is compared on equal footing by raw tile distance -- whichever is
// actually closer wins, not "always prefer a building".
func nearestFactionTarget(x, y int, buildings []*building.Building, soldiers []*Soldier, intruders []combat.IntruderTarget, radius int) (t factionTarget, ok bool) {
	bestDist := -1
	consider := func(px, py int, candidate factionTarget) {
		if !inRange(x, y, px, py, radius) {
			return
		}
		d := abs(x-px) + abs(y-py)
		if bestDist == -1 || d < bestDist {
			t, bestDist, ok = candidate, d, true
		}
	}
	for _, b := range buildings {
		if b == nil || b.HP <= 0 {
			continue
		}
		consider(b.X, b.Y, factionTarget{building: b})
	}
	for _, s := range soldiers {
		if s == nil || !s.Alive() {
			continue
		}
		consider(s.X, s.Y, factionTarget{soldier: s})
	}
	for i := range intruders {
		in := &intruders[i]
		if in.Alive == nil || !in.Alive() {
			continue
		}
		consider(in.X, in.Y, factionTarget{intruder: in})
	}
	return t, ok
}

// Soldier is one Archer or Swordsman. HP is exported (0-100, see
// combat.MaxHP) so an attacker can lower it directly, the same convention
// enemy.Enemy already uses.
type Soldier struct {
	Profession Profession
	X, Y       int
	HP         int

	// Owner identifies which faction this soldier belongs to -- see
	// building.Building.Owner's doc comment for the same convention (0 =
	// player, 1 = AI opponent). Meaningless in the ordinary single-player
	// "free map" mode, where it's always the zero value.
	Owner int

	// faction is this soldier's current cross-faction combat target --
	// see factionTarget's doc comment and TickFactionCombat.
	faction factionTarget

	path      []pathfind.Point
	pathIdx   int
	tileTicks int

	// attackTarget is the standing attack order (see AttackOrder): keep
	// approaching and hitting this enemy while it stays alive, even if it
	// moves out of range in the meantime. nil means no attack order --
	// the soldier just executes whatever MoveTo path it has, if any.
	attackTarget   *enemy.Enemy
	attackCooldown int

	// attackVisualTicks and attackTargetX/Y are transient renderer data set
	// only when a hit really lands. They are intentionally not saved.
	attackVisualTicks            int
	attackTargetX, attackTargetY int

	// pendingKillTarget is set only when a landed hit would reduce the
	// target to 0 HP or below: the kill itself is deferred until the
	// attack's own visual (arrow flight for an Archer, sword swing for a
	// Swordsman) actually reaches the target, the tick attackVisualTicks
	// reaches 0 -- the same "don't show a death before its own visual
	// arrives" fix already applied to the sentry's stone throw (see
	// package sentry's shotPendingTarget). A user-reported real bug: with
	// instant HP application an Archer's target could die on-screen
	// several ticks before the arrow visually reached it. A hit that
	// wouldn't be lethal has no such artifact to avoid -- it still
	// applies instantly, so damage feedback stays immediate; only the
	// killing blow needs to wait.
	pendingKillTarget *enemy.Enemy

	ticksSinceMeal int
}

// AttackVisual reports a freshly landed strike for the map renderer. progress
// runs from the wind-up to recovery over a short fixed duration; it is absent
// while merely moving toward a target or waiting on cooldown.
func (s *Soldier) AttackVisual() (targetX, targetY int, progress float64, ok bool) {
	if s == nil || s.attackVisualTicks <= 0 {
		return 0, 0, 0, false
	}
	progress = float64(attackVisualLifetime-s.attackVisualTicks+1) / float64(attackVisualLifetime)
	if progress > 1 {
		progress = 1
	}
	return s.attackTargetX, s.attackTargetY, progress, true
}

// New creates a soldier at (x, y) with full health, no orders.
func New(profession Profession, x, y int) *Soldier {
	return &Soldier{Profession: profession, X: x, Y: y, HP: combat.MaxHP}
}

// Alive reports whether the soldier still has health remaining.
func (s *Soldier) Alive() bool { return s != nil && s.HP > 0 }

// AttackRange returns this soldier's attack radius in tiles.
func (s *Soldier) AttackRange() int {
	if s.Profession == Swordsman {
		return SwordsmanRange
	}
	return ArcherRange
}

func (s *Soldier) cooldownTicks() int {
	if s.Profession == Swordsman {
		return SwordsmanCooldownTicks
	}
	return ArcherCooldownTicks
}

// MoveTo cancels any standing attack order and starts walking toward
// (x, y) over land -- buildings and water block it, the same free-roaming
// movement rule package enemy/builder/lumberjack already use, not the
// road network serfs need. Reports whether a route was found; a false
// result leaves any route already in progress untouched.
func (s *Soldier) MoveTo(grid *world.Grid, buildings []*building.Building, x, y int) bool {
	path, ok := pathfind.FindLandPathForFaction(grid, buildings, pathfind.Point{X: s.X, Y: s.Y}, pathfind.Point{X: x, Y: y}, s.Owner)
	if !ok {
		return false
	}
	s.attackTarget = nil
	s.faction = factionTarget{}
	s.path, s.pathIdx, s.tileTicks = path, 0, 0
	return true
}

// AttackOrder marks target as this soldier's standing order: approach and
// keep attacking while it's alive, re-approaching on its own if the
// target drifts out of range (see Controller.tick). A nil or already-dead
// target simply clears any existing attack order.
func (s *Soldier) AttackOrder(grid *world.Grid, buildings []*building.Building, target *enemy.Enemy) {
	if target == nil || !target.Alive() {
		s.attackTarget = nil
		return
	}
	s.faction = factionTarget{}
	s.attackTarget = target
	s.approach(grid, buildings)
}

// HasAttackOrder reports whether the soldier currently has a standing
// attack order (for the inspector/UI, and so cmd/game knows a red target
// marker is still relevant).
func (s *Soldier) HasAttackOrder() bool { return s.attackTarget != nil }

// HasFactionTarget reports whether the soldier currently has a
// cross-faction combat target ("1×1 против ИИ" mode -- see
// factionTarget), for the same kind of UI/marker use as HasAttackOrder.
func (s *Soldier) HasFactionTarget() bool { return s.faction.alive() }

// AttackFactionOrder is AttackOrder's cross-faction equivalent -- a
// standing order to approach and keep attacking an opposing faction's
// building, re-approaching on its own if it drifts out of range (in
// practice buildings never move, but Controller.tick's dispatch already
// re-checks range every tick regardless, the same loop that already
// drives automatic FactionEngageRange engagement -- see TickFactionCombat
// -- so nothing else has to change here for the approach/attack itself to
// actually happen).
//
// A real bug found from an actual playtest report ("клик боевым юнитом
// на постройку противника - перемещает юнитов, но не уничтожает
// постройку врага"): right-clicking an opposing faction's building used
// to have no attack-order path at all -- only the sandbox debug
// enemy.Enemy had one (AttackOrder above) -- so cmd/game's click handler
// fell through to a plain move order every time, which could walk a
// squad right up to a building without ever actually setting a target to
// fight. A nil or already-destroyed target simply clears any existing
// order, matching AttackOrder's own convention.
func (s *Soldier) AttackFactionOrder(target *building.Building) {
	if target == nil || target.HP <= 0 {
		s.faction = factionTarget{}
		return
	}
	s.attackTarget = nil
	s.faction = factionTarget{building: target}
}

// AttackFactionSoldierOrder is AttackFactionOrder's counterpart for a
// rival Soldier target -- a real gap found alongside that same playtest
// report: right-clicking an opposing soldier had no standing-order path
// either, only a building did. A nil or already-dead target clears any
// existing order, matching AttackFactionOrder's own convention.
func (s *Soldier) AttackFactionSoldierOrder(target *Soldier) {
	if target == nil || !target.Alive() {
		s.faction = factionTarget{}
		return
	}
	s.attackTarget = nil
	s.faction = factionTarget{soldier: target}
}

// AttackFactionIntruderOrder is AttackFactionOrder's counterpart for any
// other opposing unit -- a rival serf/villager/lumberjack/... with no HP
// concept of its own (see combat.IntruderTarget, and factionTarget.hit's
// one-hit-kill handling of this case). A target with no Alive callback,
// or one already dead, clears any existing order.
func (s *Soldier) AttackFactionIntruderOrder(target combat.IntruderTarget) {
	if target.Alive == nil || !target.Alive() {
		s.faction = factionTarget{}
		return
	}
	s.attackTarget = nil
	s.faction = factionTarget{intruder: &target}
}

func (s *Soldier) approach(grid *world.Grid, buildings []*building.Building) {
	if s.attackTarget == nil {
		return
	}
	if inRange(s.X, s.Y, s.attackTarget.X, s.attackTarget.Y, s.AttackRange()) {
		s.path, s.pathIdx, s.tileTicks = nil, 0, 0
		return
	}
	path, ok := pathfind.FindLandPathForFaction(grid, buildings, pathfind.Point{X: s.X, Y: s.Y}, pathfind.Point{X: s.attackTarget.X, Y: s.attackTarget.Y}, s.Owner)
	if ok {
		s.path, s.pathIdx, s.tileTicks = path, 0, 0
	}
}

// inRange reports whether (bx, by) is within a square (Chebyshev) radius
// r of (ax, ay) -- the same "5 клеток в любую сторону" style range check
// package sentry already uses for WatchTowerRange.
func inRange(ax, ay, bx, by, r int) bool {
	return abs(ax-bx) <= r && abs(ay-by) <= r
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// RemainingPath returns the tiles still ahead on the soldier's current
// route -- for the same route-line overlay every other unit's selection
// already gets (see ui.DrawSelectedRoute). nil while idle or in melee/
// firing range of an attack order.
func (s *Soldier) RemainingPath() []pathfind.Point {
	if s.pathIdx >= len(s.path) {
		return nil
	}
	return s.path[s.pathIdx:]
}

// SatietyPercent returns the player-facing 0-100 satiety value.
func (s *Soldier) SatietyPercent() int { return hunger.Percent(s.ticksSinceMeal) }

// HungerTicks returns simulation ticks since the soldier's last meal.
func (s *Soldier) HungerTicks() int { return s.ticksSinceMeal }

// NeedsDelivery reports whether this soldier's satiety has dropped to
// DeliveryThresholdPercent or below and needs a serf to bring food --
// see package logistics' soldier-delivery job.
func (s *Soldier) NeedsDelivery() bool {
	return s.SatietyPercent() <= DeliveryThresholdPercent
}

// Feed resets hunger to zero -- called once a serf's delivery reaches
// this soldier.
func (s *Soldier) Feed() { s.ticksSinceMeal = 0 }

// Controller owns every Archer and Swordsman in town.
type Controller struct {
	Soldiers []*Soldier
}

// NewController creates an empty roster; use Spawn once a Barracks hire
// succeeds.
func NewController() *Controller {
	return &Controller{}
}

// Spawn adds a soldier at (x, y), idle with no orders.
func (c *Controller) Spawn(profession Profession, x, y int) *Soldier {
	s := New(profession, x, y)
	c.Soldiers = append(c.Soldiers, s)
	return s
}

// Restore recreates a soldier from a save snapshot, preserving position,
// health and hunger. An in-progress attack order is deliberately not
// restored (package enemy's own debug roster isn't saved either, see
// AGENTS.md) -- a restored soldier simply starts idle.
func (c *Controller) Restore(profession Profession, x, y, hungerTicks, hp int) *Soldier {
	s := New(profession, x, y)
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	s.ticksSinceMeal = hungerTicks
	if hp > 0 {
		s.HP = hp
	}
	c.Soldiers = append(c.Soldiers, s)
	return s
}

// Tick advances movement, standing-attack-order pursuit, and combat for
// every living soldier; a starved soldier is removed from the roster and
// counted in the returned death total. enemies is used only for
// auto-engage against the sandbox debug enemy (see EngageRange) --
// nil/empty is fine when there's nothing to fight there. opposingBuildings,
// opposingSoldiers and opposingIntruders are this soldier's cross-faction
// targets for "1×1 против ИИ" mode (see TickFactionCombat's doc comment,
// and combat.IntruderTarget for opposingIntruders -- any opposing unit
// that isn't itself a rival Soldier) -- also nil/empty outside that mode.
//
// buildings is this soldier's movement obstacle list, not a target list --
// unlike every other controller's Tick, it must be the WHOLE map's
// buildings (every faction's, see cmd/game's Update/tickAIFaction), not
// just this faction's own. A real bug found from an actual playtest report
// ("юниты противника спокойно проходят через мои ворота"): every other
// unit kind's obstacle list is already scoped to its own faction plus
// shared-neutral objects (Road, natural resources), so it never contains
// another faction's wall/gate to be blocked by in the first place -- if
// this one were scoped the same way, an attacking faction's soldiers would
// simply never see the defender's walls as obstacles at all, gate or no
// gate. See pathfind.FindLandPathForFaction for how a foreign Gate still
// blocks like a solid wall once it IS in the list.
// TickResult summarizes one Controller.Tick call. Deaths is this
// controller's own soldiers lost to starvation. Kills and
// BuildingsDestroyed count opposing-faction targets these soldiers
// actually finished off in cross-faction ("1×1 против ИИ") combat this
// tick -- a real playtest report ("счетчик убито врагов не считает
// юнитов, нужно считать убитых с помощью башни или убитых боевыми
// юнитами") found that economy.Population.Kills only ever incremented
// for the sandbox-only debug enemy.Enemy (see cmd/game's
// pruneDeadEnemies), never for a real opponent's soldier/unit killed in
// duel mode -- this is the soldier half of that fix (see package
// sentry's identical TickResult for the WatchTower half). Kills counts a
// rival Soldier or any other opposing unit (combat.IntruderTarget);
// BuildingsDestroyed is the new, separate counter the same report asked
// for ("введи новый счетчик: 'Разрушено построек'").
type TickResult struct {
	Deaths, Kills, BuildingsDestroyed int
}

// Call once per simulation tick.
func (c *Controller) Tick(grid *world.Grid, buildings []*building.Building, enemies []*enemy.Enemy, opposingBuildings []*building.Building, opposingSoldiers []*Soldier, opposingIntruders []combat.IntruderTarget) TickResult {
	var result TickResult
	remaining := c.Soldiers[:0]
	for _, s := range c.Soldiers {
		if s.attackVisualTicks > 0 {
			s.attackVisualTicks--
			if s.attackVisualTicks == 0 && s.pendingKillTarget != nil {
				s.pendingKillTarget.HP = 0
				s.pendingKillTarget = nil
			}
		}
		s.ticksSinceMeal++
		if hunger.Dead(s.ticksSinceMeal) {
			result.Deaths++
			continue
		}
		// len(s.path) == 0 is the real fix for a bug the user reported:
		// without it, a soldier could never be moved away from an enemy
		// it just fought -- MoveTo clears attackTarget and starts a
		// path, but the very next tick this same check saw attackTarget
		// == nil again (the enemy is almost always still within
		// EngageRange right after a melee exchange) and immediately
		// re-issued an AttackOrder, whose approach() then saw the
		// soldier already in range and cleared the just-started path
		// right back out from under the player. Gating on an empty path
		// too means auto-engage only ever claims a soldier that is
		// truly idle (no standing order AND no move already under way),
		// so an explicit MoveTo/formation order always gets to actually
		// run; only once it finishes (or the soldier was idle to begin
		// with) does auto-engage get another look.
		if s.attackTarget == nil && len(s.path) == 0 {
			if target := nearestEnemyWithin(s.X, s.Y, enemies, EngageRange); target != nil {
				s.AttackOrder(grid, buildings, target)
			}
		}
		if s.attackTarget == nil && !s.faction.alive() && len(s.path) == 0 {
			if t, ok := nearestFactionTarget(s.X, s.Y, opposingBuildings, opposingSoldiers, opposingIntruders, FactionEngageRange); ok {
				s.faction = t
			}
		}
		killedUnit, killedBuilding := c.tick(grid, buildings, s)
		if killedUnit {
			result.Kills++
		}
		if killedBuilding {
			result.BuildingsDestroyed++
		}
		remaining = append(remaining, s)
	}
	c.Soldiers = remaining
	return result
}

// nearestEnemyWithin returns the closest living enemy to (x, y) within a
// square (Chebyshev) radius, or nil if none qualifies.
func nearestEnemyWithin(x, y int, enemies []*enemy.Enemy, radius int) *enemy.Enemy {
	var best *enemy.Enemy
	bestDist := -1
	for _, e := range enemies {
		if e == nil || !e.Alive() || !inRange(x, y, e.X, e.Y, radius) {
			continue
		}
		d := abs(x-e.X) + abs(y-e.Y)
		if bestDist == -1 || d < bestDist {
			best, bestDist = e, d
		}
	}
	return best
}

// tick returns whether this call's combat resolution landed the killing
// blow on an opposing unit or building, respectively -- see TickResult's
// doc comment. Always (false, false) for the sandbox-only
// tickDebugEnemyCombat path, which cmd/game's pruneDeadEnemies counts
// separately.
func (c *Controller) tick(grid *world.Grid, buildings []*building.Building, s *Soldier) (killedUnit, killedBuilding bool) {
	tickMovement(s)
	if s.attackTarget != nil {
		c.tickDebugEnemyCombat(grid, buildings, s)
		return false, false
	}
	if s.faction.alive() {
		return c.tickFactionCombat(grid, buildings, s)
	}
	return false, false
}

// tickDebugEnemyCombat is the original (sandbox-only) attackTarget combat
// resolution, unchanged in behaviour -- split out verbatim so tick can
// also run the parallel cross-faction path below without the two ever
// running in the same call (a soldier only ever has one kind of order at
// a time).
func (c *Controller) tickDebugEnemyCombat(grid *world.Grid, buildings []*building.Building, s *Soldier) {
	if !s.attackTarget.Alive() {
		s.attackTarget = nil
		return
	}
	if !inRange(s.X, s.Y, s.attackTarget.X, s.attackTarget.Y, s.AttackRange()) {
		// The target drifted out of range (or the previous approach path
		// finished short) -- keep chasing on its own, no new player click
		// needed.
		if len(s.path) == 0 {
			s.approach(grid, buildings)
		}
		return
	}
	// In range: stop moving and fight.
	s.path, s.pathIdx, s.tileTicks = nil, 0, 0
	if s.attackCooldown > 0 {
		s.attackCooldown--
		return
	}
	s.attackTargetX, s.attackTargetY = s.attackTarget.X, s.attackTarget.Y
	s.attackVisualTicks = attackVisualLifetime
	s.attackCooldown = s.cooldownTicks()
	if s.attackTarget.HP <= combat.UnitDamagePerHit {
		// Lethal -- defer the actual kill to when the visual lands (see
		// pendingKillTarget's doc comment) instead of applying it here.
		s.pendingKillTarget = s.attackTarget
		s.attackTarget = nil
		return
	}
	s.attackTarget.HP = combat.ApplyDamage(s.attackTarget.HP, combat.UnitDamagePerHit)
}

// tickFactionCombat pursues and fights s.faction -- the "1×1 против ИИ"
// cross-faction target (see factionTarget's doc comment). Mirrors
// tickDebugEnemyCombat's shape (approach if out of range, stop and hit on
// cooldown once in range) but applies damage instantly, with no
// pendingKillTarget-style deferred visual: cross-faction combat has no
// rendered attack animation yet in this first pass (a deliberate,
// documented scope cut -- see AGENTS.md).
func (c *Controller) tickFactionCombat(grid *world.Grid, buildings []*building.Building, s *Soldier) (killedUnit, killedBuilding bool) {
	if !s.faction.alive() {
		s.faction = factionTarget{}
		return false, false
	}
	tx, ty := s.faction.pos()
	if !inRange(s.X, s.Y, tx, ty, s.AttackRange()) {
		if len(s.path) == 0 {
			path, ok := pathfind.FindLandPathForFaction(grid, buildings, pathfind.Point{X: s.X, Y: s.Y}, pathfind.Point{X: tx, Y: ty}, s.Owner)
			if ok {
				s.path, s.pathIdx, s.tileTicks = path, 0, 0
			}
		}
		return false, false
	}
	s.path, s.pathIdx, s.tileTicks = nil, 0, 0
	if s.attackCooldown > 0 {
		s.attackCooldown--
		return false, false
	}
	s.attackCooldown = s.cooldownTicks()
	wasBuilding := s.faction.building != nil
	s.faction.hit()
	if !s.faction.alive() {
		s.faction = factionTarget{}
		if wasBuilding {
			killedBuilding = true
		} else {
			killedUnit = true
		}
	}
	return killedUnit, killedBuilding
}

// tickMovement advances s one step along its current path every
// TicksPerTile ticks -- the same tile-by-tile walk every other unit in
// this game already does (see e.g. package enemy's tickMovement).
func tickMovement(s *Soldier) {
	if len(s.path) == 0 {
		return
	}
	s.tileTicks++
	if s.tileTicks < TicksPerTile {
		return
	}
	s.tileTicks = 0
	if s.pathIdx < len(s.path)-1 {
		s.pathIdx++
		s.X, s.Y = s.path[s.pathIdx].X, s.path[s.pathIdx].Y
		return
	}
	s.path = nil
	s.pathIdx = 0
}
