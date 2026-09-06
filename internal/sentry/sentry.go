// Package sentry simulates the Sentry unit: a stationary resident of one
// WatchTower who fires a stone sling at any enemy within range, the same
// way package villagers' workers live and work at one building. Unlike a
// production worker it doesn't tend a recipe -- see Controller.engage --
// but it eats and walks to a Tavern exactly like every other unit in this
// game ("сытость 0-100% у всех юнитов без исключения").
package sentry

import (
	"strategy_game/internal/building"
	"strategy_game/internal/combat"
	"strategy_game/internal/enemy"
	"strategy_game/internal/hunger"
	"strategy_game/internal/meal"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
)

const (
	// HungerInterval is the 20% satiety threshold at which a Sentry starts
	// walking to the Tavern. Death happens at hunger.MaxTicks.
	HungerInterval = hunger.MealThresholdTicks

	// TicksPerTile is how many simulation ticks it takes a Sentry to cross
	// one tile of road, walking to/from the Tavern.
	TicksPerTile = 2

	// WatchTowerRange is how far (in tiles, a plain square radius -- not
	// line of sight) a Sentry can hit a target from its tower. Per the
	// user's explicit request this ignores walls in between: "можно
	// стрелять через стену". Lowered from the original 5 to 2 -- the
	// user tried 5 in-game and found it too far.
	WatchTowerRange = 2

	// ShotCooldownTicks paces fire so a Sentry can't empty its tower's
	// whole stone buffer in an instant once an enemy is in range.
	ShotCooldownTicks = 20

	// shotVisualLifetime is how many simulation ticks the stone is "in
	// flight" before the kill lands (see shotPendingTarget). Lowered from
	// 6 to 1 -- the user found that over 6 ticks a moving target could
	// take several steps, so the stone visually landed on one tile while
	// the kill (and death animation) happened on another: "камень падает
	// на одну клетку, а противник умирает на другой". At 1 tick the
	// target has essentially no time to move between the shot and the
	// kill, so the two always agree.
	shotVisualLifetime = 1
)

type phase int

const (
	working phase = iota
	toTavern
	toHome
)

// Sentry is the resident of one WatchTower.
type Sentry struct {
	Home *building.Building
	X, Y int

	ph        phase
	path      []pathfind.Point
	pathIdx   int
	tileTicks int

	tavern *building.Building // which Tavern this trip is headed to/from, while ph != working
	meal   resource.Type      // food reserved for the current Tavern trip

	ticksSinceMeal int

	// shotCooldown counts down to zero between shots -- see
	// Controller.engage. Not persisted across save/load: a restored
	// Sentry is simply ready to fire again immediately, the same
	// "restart the visible detail, keep what matters" tradeoff
	// RestoreSentry already makes for the walking path.
	shotCooldown int

	// shotVisualTicks and shotTarget store only enough information for the
	// renderer to show the most recent sling stone in flight. Not persisted
	// across save/load, the same as shotCooldown above.
	//
	// shotPendingTarget is the one exception to "visual-only": the user
	// explicitly asked for the kill itself to land when the stone visually
	// arrives, not the instant it's thrown ("раньше было сперва противник
	// погибает, а потом летит камень в него" -- an observed real bug). See
	// Controller.Tick, which zeroes shotPendingTarget's HP the tick
	// shotVisualTicks reaches 0. A target that's already dead by then (killed
	// by something else in the meantime) is a harmless no-op.
	shotVisualTicks          int
	shotTargetX, shotTargetY int
	shotPendingTarget        *enemy.Enemy

	// shotPendingIntruder is shotPendingTarget's "1×1 против ИИ"
	// counterpart -- see IntruderTarget's doc comment. Exactly one of
	// shotPendingTarget/shotPendingIntruder is set at a time, matching
	// the mutually-exclusive convention soldier.factionTarget already
	// uses for the same "debug enemy vs. real opposing faction"
	// distinction.
	shotPendingIntruder *IntruderTarget

	// Starving mirrors package villagers' field of the same name: true
	// once HungerInterval has passed and there was nowhere to actually go
	// eat.
	Starving bool
}

// IntruderTarget is combat.IntruderTarget -- kept as an alias so every
// existing reference to sentry.IntruderTarget in this package and its
// tests keeps working unchanged now that package soldier also needs the
// exact same shape (see combat.IntruderTarget's doc comment for why it
// moved to the shared, engine-free combat package instead of staying
// sentry-only).
type IntruderTarget = combat.IntruderTarget

// State is the public, read-only activity state used by the inspector and
// render layer.
type State int

const (
	SentryWorking State = iota
	SentryToTavern
	SentryToHome
)

// State reports whether the Sentry is at its post or walking to eat.
func (s *Sentry) State() State { return State(s.ph) }

// HungerTicks returns simulation ticks since the Sentry's last meal.
func (s *Sentry) HungerTicks() int { return s.ticksSinceMeal }

// SatietyPercent returns the player-facing 0-100 satiety value.
func (s *Sentry) SatietyPercent() int { return hunger.Percent(s.ticksSinceMeal) }

// RemainingPath returns the tiles still ahead on the Sentry's current
// route -- for the inspector's route-line overlay. nil while at post.
func (s *Sentry) RemainingPath() []pathfind.Point {
	if s.pathIdx >= len(s.path) {
		return nil
	}
	return s.path[s.pathIdx:]
}

// HomeBuilding returns the WatchTower this Sentry works at.
func (s *Sentry) HomeBuilding() *building.Building { return s.Home }

// Working reports whether the Sentry is at its post right now, as opposed
// to out walking to/from a meal.
func (s *Sentry) Working() bool { return s.ph == working }

// Meal returns the food reserved for the current or next Tavern trip.
func (s *Sentry) Meal() resource.Type { return s.meal }

// ShotVisual reports the most recent sling shot while it is on screen. Its
// coordinates are world tiles; progress is in (0, 1] and advances over a
// small fixed number of simulation ticks. No state is returned once the
// visual has expired, so loading a save never recreates a stale projectile.
func (s *Sentry) ShotVisual() (fromX, fromY, targetX, targetY int, progress float64, ok bool) {
	if s == nil || s.Home == nil || s.shotVisualTicks <= 0 {
		return 0, 0, 0, 0, 0, false
	}
	progress = float64(shotVisualLifetime-s.shotVisualTicks+1) / float64(shotVisualLifetime)
	if progress > 1 {
		progress = 1
	}
	return s.Home.X, s.Home.Y, s.shotTargetX, s.shotTargetY, progress, true
}

// NewSentry creates a Sentry standing at its tower, on duty.
func NewSentry(home *building.Building) *Sentry {
	return &Sentry{Home: home, X: home.X, Y: home.Y, meal: resource.Bread}
}

// Controller owns every Sentry in town.
type Controller struct {
	Sentries []*Sentry
	meals    meal.Selector
}

// NewController creates an empty roster; use Spawn to add a Sentry once
// its tower is hired into (see cmd/game's Barracks hire button).
func NewController() *Controller {
	return &Controller{meals: meal.NewSelector(0x53e47b21)}
}

// MealSeed returns the persistent pseudo-random state for meal choices.
func (c *Controller) MealSeed() uint32 { return c.meals.Seed() }

// SetMealSeed restores the persistent pseudo-random state for meal choices.
func (c *Controller) SetMealSeed(seed uint32) { c.meals.SetSeed(seed) }

// Spawn adds a Sentry on duty at home.
func (c *Controller) Spawn(home *building.Building) *Sentry {
	s := NewSentry(home)
	c.Sentries = append(c.Sentries, s)
	return s
}

// HasHome reports whether a Sentry is already assigned to a tower. One
// WatchTower holds only its matching single resident.
func (c *Controller) HasHome(home *building.Building) bool {
	for _, s := range c.Sentries {
		if s.Home == home {
			return true
		}
	}
	return false
}

// RestoreSentry recreates a Sentry from a save snapshot, the same
// contract as villagers.Controller.RestoreVillager: keeps the saved map
// position and hunger state, rebuilds any in-progress Tavern route from
// that exact position.
func (c *Controller) RestoreSentry(home *building.Building, x, y, hungerTicks int, starving bool, state State, buildings []*building.Building, savedMeal ...resource.Type) *Sentry {
	s := NewSentry(home)
	s.X, s.Y = x, y
	if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
		s.meal = savedMeal[0]
	}
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	s.ticksSinceMeal = hungerTicks
	s.Starving = starving

	switch state {
	case SentryToTavern:
		wanted := []resource.Type(nil)
		if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
			wanted = append(wanted, savedMeal[0])
		}
		if tavern, meal, path, ok := nearestTavernWithFood(buildings, pathfind.Point{X: x, Y: y}, nil, &c.meals, wanted...); ok {
			s.path, s.pathIdx, s.tileTicks = path, 0, 0
			s.tavern = tavern
			if len(savedMeal) == 0 || !resource.IsFood(savedMeal[0]) {
				s.meal = meal
			}
			s.ph = toTavern
		}
	case SentryToHome:
		if path, ok := pathfind.FindPathFromPoint(buildings, pathfind.Point{X: x, Y: y}, home); ok {
			s.path, s.pathIdx, s.tileTicks = path, 0, 0
			s.ph = toHome
		}
	}

	c.Sentries = append(c.Sentries, s)
	return s
}

// RemoveHome removes the Sentry assigned to a tower that was deleted.
func (c *Controller) RemoveHome(home *building.Building) {
	kept := c.Sentries[:0]
	for _, s := range c.Sentries {
		if s.Home != home {
			kept = append(kept, s)
		}
	}
	c.Sentries = kept
}

// CancelRouteTo resets any Sentry currently walking toward target as a
// Tavern back to its post, instead of leaving it holding a dangling
// pointer to a building about to be removed.
func (c *Controller) CancelRouteTo(target *building.Building) {
	for _, s := range c.Sentries {
		if s.ph != toTavern || s.tavern != target {
			continue
		}
		s.tavern = nil
		s.path, s.pathIdx, s.tileTicks = nil, 0, 0
		s.X, s.Y = s.Home.X, s.Home.Y
		s.ph = working
	}
}

// Reserve seeds ledger with every Sentry currently walking to eat, so
// other controllers sharing a Tavern see this claim before making their
// own commitments this tick. Call once per simulation tick, before this
// or any other controller's Tick runs.
func (c *Controller) Reserve(ledger *reservations.Ledger) {
	for _, s := range c.Sentries {
		if s.ph == toTavern && s.tavern != nil {
			ledger.ReservePickup(s.tavern, s.meal, 1)
		}
	}
}

// MaxWaitingHunger returns the highest HungerTicks among Sentries that
// will actually try to eat this tick, or -1 if none will -- see
// villagers.Controller.MaxWaitingHunger's doc comment for why this
// matters for tick ordering across controllers.
func (c *Controller) MaxWaitingHunger() int {
	best := -1
	for _, s := range c.Sentries {
		if s.ph != working || s.ticksSinceMeal < HungerInterval {
			continue
		}
		if s.ticksSinceMeal > best {
			best = s.ticksSinceMeal
		}
	}
	return best
}

// TickResult summarizes one Controller.Tick call. Deaths is this
// controller's own Sentries lost to starvation; Kills is opposing units
// (see IntruderTarget) a WatchTower's stone actually finished off this
// tick -- a real playtest report ("счетчик убито врагов не считает
// юнитов, нужно считать убитых с помощью башни или убитых боевыми
// юнитами") found economy.Population.Kills never counted a WatchTower's
// real cross-faction kills at all, only the sandbox-only debug
// enemy.Enemy (see cmd/game's pruneDeadEnemies) -- this is the WatchTower
// half of that fix (see package soldier's identical TickResult for the
// combat-unit half). No BuildingsDestroyed counter here: a WatchTower's
// intruders list only ever wraps opposing units, never buildings -- see
// cmd/game's intruderTargetsFrom.
type TickResult struct {
	Deaths, Kills int
}

// Tick advances hunger, movement and combat for every Sentry. Call once
// per simulation tick, after every controller sharing ledger has had a
// chance to Reserve its own pre-existing in-flight units. enemies is the
// current debug enemy roster (see cmd/game) -- a Sentry only ever reads
// it, never mutates the slice itself, though it does lower a target's HP
// in place. intruders is this Sentry's "1×1 против ИИ" targets -- see
// IntruderTarget's doc comment; nil/empty outside that mode.
func (c *Controller) Tick(buildings []*building.Building, enemies []*enemy.Enemy, intruders []IntruderTarget, ledger *reservations.Ledger) TickResult {
	var result TickResult
	remaining := c.Sentries[:0]
	for _, s := range c.Sentries {
		if s.shotVisualTicks > 0 {
			s.shotVisualTicks--
			if s.shotVisualTicks == 0 {
				if s.shotPendingTarget != nil {
					// The stone has visually arrived -- this is when it
					// actually kills, not when it was thrown. See
					// shotPendingTarget's doc comment.
					s.shotPendingTarget.HP = 0
					s.shotPendingTarget = nil
				}
				if s.shotPendingIntruder != nil {
					s.shotPendingIntruder.Kill()
					s.shotPendingIntruder = nil
					result.Kills++
				}
			}
		}
		s.ticksSinceMeal++
		if hunger.Dead(s.ticksSinceMeal) {
			result.Deaths++
			continue
		}
		c.tick(s, buildings, enemies, intruders, ledger)
		remaining = append(remaining, s)
	}
	c.Sentries = remaining
	return result
}

func (c *Controller) tick(s *Sentry, buildings []*building.Building, enemies []*enemy.Enemy, intruders []IntruderTarget, ledger *reservations.Ledger) {
	switch s.ph {
	case working:
		c.tickWorking(s, buildings, enemies, intruders, ledger)
	case toTavern, toHome:
		tickWalking(s, buildings)
	}
}

func (c *Controller) tickWorking(s *Sentry, buildings []*building.Building, enemies []*enemy.Enemy, intruders []IntruderTarget, ledger *reservations.Ledger) {
	if hunger.NeedsMeal(s.ticksSinceMeal) {
		if tavern, meal, path, ok := nearestTavernWithFood(buildings, pathfind.Point{X: s.Home.X, Y: s.Home.Y}, ledger, &c.meals); ok {
			s.Starving = false
			s.X, s.Y = s.Home.X, s.Home.Y
			s.tavern = tavern
			s.meal = meal
			s.path, s.pathIdx, s.tileTicks = path, 0, 0
			s.ph = toTavern
			ledger.ReservePickup(tavern, meal, 1)
			return
		}
		s.Starving = true
	}
	c.engage(s, enemies, intruders)
}

// engage fires at the nearest living target (debug enemy or opposing-
// faction intruder, whichever is actually closer) within WatchTowerRange,
// once per ShotCooldownTicks, consuming one stone from the tower's
// InputBuffer per shot -- a silent no-op with nothing in range, no stone
// left, or still on cooldown.
//
// Per the user's explicit request ("1 попадание камня в противника его
// убивает"), a hit is a kill -- unlike a building, which still takes
// combat.DamagePerHit (10%) per hit from the same stone. A stone sling is
// lethal to a person but only chips a wall. The kill itself, though,
// lands only once the stone visually arrives (see shotPendingTarget/
// shotPendingIntruder and Controller.Tick), not the instant it's thrown
// here -- a real bug the user caught in-game ("раньше было сперва
// противник погибает, а потом летит камень в него"): the target must
// stay alive and interactable for the roughly WatchTowerRange*
// TicksPerTile ticks the stone is airborne.
func (c *Controller) engage(s *Sentry, enemies []*enemy.Enemy, intruders []IntruderTarget) {
	if s.shotCooldown > 0 {
		s.shotCooldown--
		return
	}
	if s.Home == nil {
		return
	}
	enemyTarget := nearestEnemyInRange(s.Home.X, s.Home.Y, enemies)
	intruderTarget, intruderDist, intruderOK := nearestIntruderInRange(s.Home.X, s.Home.Y, intruders)

	var targetX, targetY int
	var pendingEnemy *enemy.Enemy
	var pendingIntruder *IntruderTarget
	switch {
	case enemyTarget != nil && (!intruderOK || abs(enemyTarget.X-s.Home.X)+abs(enemyTarget.Y-s.Home.Y) <= intruderDist):
		targetX, targetY, pendingEnemy = enemyTarget.X, enemyTarget.Y, enemyTarget
	case intruderOK:
		targetX, targetY, pendingIntruder = intruderTarget.X, intruderTarget.Y, intruderTarget
	default:
		return
	}
	if !s.Home.TakeInput(resource.StoneBlock, 1) {
		return
	}
	s.shotTargetX, s.shotTargetY = targetX, targetY
	s.shotPendingTarget = pendingEnemy
	s.shotPendingIntruder = pendingIntruder
	s.shotVisualTicks = shotVisualLifetime
	s.shotCooldown = ShotCooldownTicks
}

// nearestIntruderInRange mirrors nearestEnemyInRange for IntruderTarget --
// see its doc comment for the range convention. Returns the candidate's
// own tile distance too, so engage can compare it directly against a
// simultaneously-in-range enemy.Enemy candidate's distance.
func nearestIntruderInRange(cx, cy int, intruders []IntruderTarget) (target *IntruderTarget, dist int, ok bool) {
	bestDist := 0
	for i := range intruders {
		t := &intruders[i]
		if t.Alive == nil || !t.Alive() {
			continue
		}
		dx, dy := abs(t.X-cx), abs(t.Y-cy)
		if dx > WatchTowerRange || dy > WatchTowerRange {
			continue
		}
		d := dx + dy
		if !ok || d < bestDist {
			target, bestDist, ok = t, d, true
		}
	}
	return target, bestDist, ok
}

// nearestEnemyInRange returns the closest living enemy within
// WatchTowerRange tiles of (cx, cy) -- a plain square radius per-axis
// (Chebyshev distance), matching the user's "5 клеток в любую сторону"
// description, not a circular/Euclidean one. nil if none qualify.
func nearestEnemyInRange(cx, cy int, enemies []*enemy.Enemy) *enemy.Enemy {
	var best *enemy.Enemy
	bestDist := 0
	for _, e := range enemies {
		if !e.Alive() {
			continue
		}
		dx, dy := abs(e.X-cx), abs(e.Y-cy)
		if dx > WatchTowerRange || dy > WatchTowerRange {
			continue
		}
		dist := dx + dy
		if best == nil || dist < bestDist {
			best, bestDist = e, dist
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// nearestTavernWithFood mirrors villagers' unexported helper of the same
// name -- see its doc comment there for the full rationale (`from` is a
// Point rather than a building so RestoreSentry can search from the exact
// saved position, not always Home).
func nearestTavernWithFood(buildings []*building.Building, from pathfind.Point, ledger *reservations.Ledger, selector *meal.Selector, wanted ...resource.Type) (tavern *building.Building, selected resource.Type, path []pathfind.Point, ok bool) {
	bestLen := -1
	var available []resource.Type
	for _, b := range buildings {
		if b == nil || b.Kind != building.Tavern {
			continue
		}
		foods := make([]resource.Type, 0, len(resource.FoodTypes()))
		for _, food := range resource.FoodTypes() {
			if len(wanted) > 0 && food != wanted[0] {
				continue
			}
			if ledger != nil && ledger.AvailableInput(b, food) <= 0 {
				continue
			}
			if ledger == nil && b.InputBuffer[food] <= 0 {
				continue
			}
			foods = append(foods, food)
		}
		if len(foods) == 0 {
			continue
		}
		p, reachable := pathfind.FindPathFromPoint(buildings, from, b)
		if !reachable {
			continue
		}
		if bestLen == -1 || len(p) < bestLen {
			tavern, available, path, bestLen = b, foods, p, len(p)
		}
	}
	if tavern == nil {
		return nil, 0, nil, false
	}
	if selector == nil || len(wanted) > 0 {
		selected = available[0]
	} else {
		selected, _ = selector.Pick(available)
	}
	return tavern, selected, path, true
}

func tickWalking(s *Sentry, buildings []*building.Building) {
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

	if s.ph == toTavern {
		tavern := s.tavern
		s.X, s.Y = tavern.X, tavern.Y
		if tavern.TakeInput(s.meal, 1) {
			s.ticksSinceMeal = 0
		}
		if path, ok := pathfind.FindPath(buildings, tavern, s.Home); ok {
			s.path, s.pathIdx, s.tileTicks = path, 0, 0
			s.ph = toHome
		}
		s.tavern = nil
		return
	}

	s.X, s.Y = s.Home.X, s.Home.Y
	s.ph = working
}
