// Package soldier simulates the two mobile combat professions hired
// through the Barracks: Archer and Swordsman. Unlike every other unit in
// this game they never walk to a Tavern themselves when hungry -- food is
// delivered to them by a serf instead (package logistics); see
// NeedsDelivery/Feed. They are entirely player-directed (select, then
// right-click a point to walk there, or an opposing faction's building/
// unit to attack) -- neither profession picks its own destination or
// target.
package soldier

import (
	"strategy_game/internal/building"
	"strategy_game/internal/combat"
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

	// ArcherCooldownTicks/SwordsmanCooldownTicks pace attacks -- 1, per
	// the user's own explicit combat-rebalance request ("интенсивность
	// ударов юнитов - 1 удар в 2 тика"). tickFactionCombat's own cooldown
	// bookkeeping (set right after a hit, counted down before the next
	// one's allowed) makes the actual gap between hits cooldownTicks+1
	// simulation ticks -- 1 here means a hit, then one skipped tick,
	// then the next hit, exactly the requested 2-tick interval. Was 0
	// (a hit every single tick, no gap at all) before this.
	ArcherCooldownTicks    = 1
	SwordsmanCooldownTicks = 1
)

// FactionEngageRange mirrors EngageRange for cross-faction auto-combat --
// see TickFactionCombat/factionTarget. Same value, kept as its own named
// constant since the two mechanisms are otherwise fully independent.
const FactionEngageRange = 2

// factionTarget is a soldier's current cross-faction combat target -- a
// rival building, soldier, or any other opposing unit (see intruder,
// combat.IntruderTarget), used only by "1×1 против ИИ" mode. Exactly one
// field is non-nil, or all nil for "no target".
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

// owner reports which faction the current target belongs to -- used to
// credit a killing blow to the right attacker (see cmd/game's
// lastAttackerOwner) instead of only ever guessed at by geography.
func (t factionTarget) owner() int {
	switch {
	case t.building != nil:
		return t.building.Owner
	case t.soldier != nil:
		return t.soldier.Owner
	case t.intruder != nil:
		return t.intruder.Owner
	default:
		return 0
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

// hit applies one blow: combat.DamagePerHit (5%, the same rate every
// other structure-damaging attack in the game uses) against a building,
// combat.UnitDamagePerHit (20%, five hits kill) against a rival soldier --
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
// combat.MaxHP) so an attacker can lower it directly.
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

	attackCooldown int

	ticksSinceMeal int
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
// movement rule package builder/lumberjack already use, not the road
// network serfs need. Reports whether a route was found; a false result
// leaves any route already in progress untouched.
func (s *Soldier) MoveTo(grid *world.Grid, buildings []*building.Building, x, y int) bool {
	path, ok := pathfind.FindLandPathForFaction(grid, buildings, pathfind.Point{X: s.X, Y: s.Y}, pathfind.Point{X: x, Y: y}, s.Owner)
	if !ok {
		return false
	}
	s.faction = factionTarget{}
	s.path, s.pathIdx, s.tileTicks = path, 0, 0
	return true
}

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
// to have no attack-order path at all, so cmd/game's click handler fell
// through to a plain move order every time, which could walk a squad
// right up to a building without ever actually setting a target to
// fight. A nil or already-destroyed target simply clears any existing
// order.
func (s *Soldier) AttackFactionOrder(target *building.Building) {
	if target == nil || target.HP <= 0 {
		s.faction = factionTarget{}
		return
	}
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
	s.faction = factionTarget{intruder: &target}
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
// health and hunger -- a restored soldier simply starts idle, with no
// standing order restored.
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

// TickResult summarizes one Controller.Tick call. Deaths is this
// controller's own soldiers lost to starvation or a rival's hit landed
// earlier in the same overall simulation tick (see the roster-removal
// check at the top of the loop below). Kills and BuildingsDestroyed
// count opposing-faction targets these soldiers actually finished off in
// cross-faction ("1×1 против ИИ") combat this tick (see package sentry's
// identical TickResult for the WatchTower half) -- Kills counts a rival
// Soldier or any other opposing unit (combat.IntruderTarget);
// BuildingsDestroyed is a separate counter for a destroyed building.
// DeathPositions/KillPositions carry the tile of each such event, for
// cmd/game's addDeathEffect -- the map's one shared, profession-free
// death animation (see internal/render's DeathEffect). KillOwners is
// KillPositions' own parallel slice (same order, same length) naming
// which faction owned whatever just died -- cmd/game folds every entry
// into lastAttackerOwner, crediting a faction's eventual defeat to
// whoever actually landed the last real blow on it instead of only ever
// guessing by geography (see nearestSurvivingFactionTo's own doc
// comment on the playtest report this replaces).
type TickResult struct {
	Deaths, Kills, BuildingsDestroyed int
	DeathPositions, KillPositions     []pathfind.Point
	KillOwners                        []int
}

// Call once per simulation tick. opposingBuildings, opposingSoldiers and
// opposingIntruders are this soldier's cross-faction targets for "1×1
// против ИИ" mode (see TickFactionCombat's doc comment, and
// combat.IntruderTarget for opposingIntruders -- any opposing unit that
// isn't itself a rival Soldier) -- nil/empty outside that mode.
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
func (c *Controller) Tick(grid *world.Grid, buildings []*building.Building, opposingBuildings []*building.Building, opposingSoldiers []*Soldier, opposingIntruders []combat.IntruderTarget) TickResult {
	var result TickResult
	remaining := c.Soldiers[:0]
	for _, s := range c.Soldiers {
		// A real bug found from an actual playtest report ("куда они все
		// смотрели?", traced to a soldier the player's own archers/
		// WatchTower had genuinely reduced to 0 HP -- confirmed via
		// combat.ApplyDamage -- that then kept marching on and destroyed
		// the player's warehouse anyway): nothing here ever checked
		// whether the soldier THIS loop is about to move/fight with is
		// still alive. A rival hit (factionTarget.hit(), a WatchTower's
		// stone via the intruder-wrapping in cmd/game's
		// intruderTargetsFrom) always lands on some OTHER controller's
		// Tick call, earlier in the same overall simulation tick -- so by
		// the time this soldier's own Tick runs, s.HP already reflects
		// the hit, and checking it right here, before any movement or
		// combat resolution, catches a "zombie" the very first chance
		// this controller gets, exactly like the hunger.Dead check just
		// below already does for a starved soldier.
		if !s.Alive() {
			result.Deaths++
			result.DeathPositions = append(result.DeathPositions, pathfind.Point{X: s.X, Y: s.Y})
			continue
		}
		s.ticksSinceMeal++
		if hunger.Dead(s.ticksSinceMeal) {
			result.Deaths++
			result.DeathPositions = append(result.DeathPositions, pathfind.Point{X: s.X, Y: s.Y})
			continue
		}
		// len(s.path) == 0 gates auto-engage on a truly idle soldier (no
		// standing order AND no move already under way), so an explicit
		// MoveTo/formation order always gets to actually run; only once
		// it finishes (or the soldier was idle to begin with) does
		// auto-engage get another look.
		if !s.faction.alive() && len(s.path) == 0 {
			if t, ok := nearestFactionTarget(s.X, s.Y, opposingBuildings, opposingSoldiers, opposingIntruders, FactionEngageRange); ok {
				s.faction = t
			}
		}
		killedUnit, killedBuilding, killX, killY, killedOwner := c.tick(grid, buildings, s)
		if killedUnit || killedBuilding {
			result.KillPositions = append(result.KillPositions, pathfind.Point{X: killX, Y: killY})
			result.KillOwners = append(result.KillOwners, killedOwner)
		}
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

// tick advances movement and runs cross-faction combat resolution for s,
// if it currently has a live faction target -- see factionTarget's doc
// comment. killX/killY/killedOwner are only meaningful when killedUnit
// or killedBuilding is true (see TickResult.KillPositions/KillOwners).
func (c *Controller) tick(grid *world.Grid, buildings []*building.Building, s *Soldier) (killedUnit, killedBuilding bool, killX, killY, killedOwner int) {
	tickMovement(s)
	if s.faction.alive() {
		return c.tickFactionCombat(grid, buildings, s)
	}
	return false, false, 0, 0, 0
}

// tickFactionCombat pursues and fights s.faction -- the "1×1 против ИИ"
// cross-faction target (see factionTarget's doc comment): approach if
// out of range, stop and hit once in range and off cooldown. Applies
// damage instantly, with no deferred-visual step: cross-faction combat
// has no rendered attack animation yet in this first pass (a deliberate,
// documented scope cut -- see AGENTS.md).
func (c *Controller) tickFactionCombat(grid *world.Grid, buildings []*building.Building, s *Soldier) (killedUnit, killedBuilding bool, killX, killY, killedOwner int) {
	if !s.faction.alive() {
		s.faction = factionTarget{}
		return false, false, 0, 0, 0
	}
	tx, ty := s.faction.pos()
	if !inRange(s.X, s.Y, tx, ty, s.AttackRange()) {
		if len(s.path) == 0 {
			path, ok := pathfind.FindLandPathForFaction(grid, buildings, pathfind.Point{X: s.X, Y: s.Y}, pathfind.Point{X: tx, Y: ty}, s.Owner)
			if ok {
				s.path, s.pathIdx, s.tileTicks = path, 0, 0
			}
		}
		return false, false, 0, 0, 0
	}
	s.path, s.pathIdx, s.tileTicks = nil, 0, 0
	if s.attackCooldown > 0 {
		s.attackCooldown--
		return false, false, 0, 0, 0
	}
	s.attackCooldown = s.cooldownTicks()
	wasBuilding := s.faction.building != nil
	s.faction.hit()
	if !s.faction.alive() {
		killedOwner = s.faction.owner()
		s.faction = factionTarget{}
		killX, killY = tx, ty
		if wasBuilding {
			killedBuilding = true
		} else {
			killedUnit = true
		}
	}
	return killedUnit, killedBuilding, killX, killY, killedOwner
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
