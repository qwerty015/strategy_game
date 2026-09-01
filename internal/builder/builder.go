// Package builder simulates the worker who turns a placed-but-unfinished
// building or road (see building.ConstructionStage) into a finished one.
// Like a lumberjack, a builder is not restricted to the road network: he
// walks over every non-water tile to reach a site. Construction is a
// two-phase process: the builder starts a free "foundation" pass the
// instant he arrives (no materials needed yet), then either finds the
// site's required Plank/StoneBlock already delivered and moves straight to
// finishing, or waits at the site until a serf brings them (see package
// logistics' construction-material delivery, the one job type serfs may
// carry off the road network).
package builder

import (
	"strategy_game/internal/building"
	"strategy_game/internal/hunger"
	"strategy_game/internal/meal"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

const (
	// HungerInterval is the shared 20% satiety threshold. Hunger is checked
	// between jobs and never interrupts an active construction stage.
	HungerInterval = hunger.MealThresholdTicks
	TicksPerTile   = 2

	// RepairTicks is how long a builder spends at a damaged, finished
	// building before it's back to building.MaxHP -- see StateRepairing.
	// Repair costs no delivered materials (unlike fresh construction):
	// the amounts involved (one combat.DamagePerHit-sized dent, 10% at a
	// time) didn't seem worth reusing InputBuffer[Plank]/[StoneBlock] for,
	// especially since a WatchTower already uses InputBuffer[StoneBlock]
	// for its own ammunition -- mixing the two would be genuinely
	// ambiguous, not just extra bookkeeping.
	RepairTicks = 40
)

// State is the visible activity state of a builder.
type State int

const (
	StateIdle State = iota
	StateToSite
	StateFoundation
	StateWaitingMaterials
	StateFinishing
	StateToTavern

	// StateRepairing is appended last to keep every earlier State's saved
	// int value unchanged, the same reasoning every building.Kind append
	// in this codebase follows. See startRepairJob/repairUsable.
	StateRepairing
)

// EventKind identifies a world change emitted by the controller.
type EventKind int

const (
	// ConstructionComplete fires the moment a site's finishing stage ends.
	// The game layer owns the building slice, so it clears
	// ConstructionStage, resets ProgressTicks for the building's real
	// recipe, and spawns its resident worker (if any) -- the same way a
	// freshly built building always has.
	ConstructionComplete EventKind = iota
	WorkerDied
	WorkerDismissed
)

// Event is returned after a worker finishes a world interaction.
type Event struct {
	Kind     EventKind
	Building *building.Building
}

// Builder is one physical worker. Unlike a lumberjack or quarryman, he has
// no dedicated hut -- Warehouse is his post to return to when idle or to
// eat, mirroring a Serf.
type Builder struct {
	Warehouse *building.Building
	X, Y      int

	state      State
	target     *building.Building // the construction site currently being walked to or worked
	tavern     *building.Building
	meal       resource.Type
	path       []pathfind.Point
	pathIdx    int
	tileTicks  int
	workTicks  int
	hungerTick int

	// Starving is true when the worker needs food but currently cannot reach a
	// stocked Tavern. The worker keeps performing the current work loop so a
	// missing Tavern does not deadlock construction.
	Starving bool

	// dismissing mirrors logistics.Serf's field of the same name: set by
	// RequestDismissal, the builder finishes whatever site he's currently
	// working before leaving, exactly like a serf finishes an in-progress
	// haul first.
	dismissing bool
}

// Dismissing reports whether the builder will leave town after finishing
// his current site (see RequestDismissal).
func (b *Builder) Dismissing() bool {
	return b.dismissing
}

// Controller owns every builder in the settlement.
type Controller struct {
	Builders []*Builder
	meals    meal.Selector
}

// NewController creates an empty builder roster.
func NewController() *Controller {
	return &Controller{meals: meal.NewSelector(0x6a4d7a8c)}
}

// MealSeed returns the persistent pseudo-random state for builder meals.
func (c *Controller) MealSeed() uint32 { return c.meals.Seed() }

// SetMealSeed restores the persistent pseudo-random state for builder meals.
func (c *Controller) SetMealSeed(seed uint32) { c.meals.SetSeed(seed) }

// NewBuilder creates a worker at the warehouse's access point.
func NewBuilder(warehouse *building.Building) *Builder {
	if warehouse == nil {
		return &Builder{}
	}
	p := warehouse.AccessPoint()
	return &Builder{Warehouse: warehouse, X: p.X, Y: p.Y, meal: resource.Bread}
}

// Hire adds one builder at warehouse. Callers (cmd/game) are responsible
// for the town-wide cap of 3 -- the controller itself doesn't enforce a
// limit, the same division of responsibility as logistics.Controller.Hire.
func (c *Controller) Hire(warehouse *building.Building) *Builder {
	b := NewBuilder(warehouse)
	c.Builders = append(c.Builders, b)
	return b
}

// Restore recreates a builder while preserving position, hunger, the
// selected meal and the current work state. Routes are rebuilt from the
// saved tile because transient path slices are intentionally not part of
// the JSON format.
func (c *Controller) Restore(warehouse *building.Building, x, y, hungerTicks int, starving bool, state State, target *building.Building, workTicks int, grid *world.Grid, buildings []*building.Building, savedMeal ...resource.Type) *Builder {
	b := NewBuilder(warehouse)
	b.X, b.Y = x, y
	if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
		b.meal = savedMeal[0]
	}
	if hungerTicks < 0 {
		hungerTicks = 0
	}
	b.hungerTick = hungerTicks
	b.Starving = starving
	b.workTicks = workTicks

	switch state {
	case StateToSite:
		// target is either a fresh construction site or a damaged,
		// already-finished building being walked to for repair -- see
		// startRepairJob/repairUsable.
		validTarget := target != nil && (target.ConstructionStage != building.ConstructionNone || repairUsable(buildings, target))
		if validTarget && b.routeTo(grid, buildings, pathfind.Point{X: target.X, Y: target.Y}) {
			b.target = target
			b.state = StateToSite
		}
	case StateFoundation, StateWaitingMaterials, StateFinishing:
		if target != nil && target.ConstructionStage != building.ConstructionNone {
			b.target = target
			b.state = state
		}
	case StateRepairing:
		if repairUsable(buildings, target) {
			b.target = target
			b.state = state
		}
	case StateToTavern:
		wanted := []resource.Type(nil)
		if len(savedMeal) > 0 && resource.IsFood(savedMeal[0]) {
			wanted = append(wanted, savedMeal[0])
		}
		if tavern, meal, path, ok := nearestTavernWithFood(grid, buildings, pathfind.Point{X: x, Y: y}, nil, &c.meals, wanted...); ok {
			b.setPath(path)
			b.tavern = tavern
			if len(savedMeal) == 0 || !resource.IsFood(savedMeal[0]) {
				b.meal = meal
			}
			b.state = StateToTavern
		}
	}

	c.Builders = append(c.Builders, b)
	return b
}

// RequestDismissal marks a live builder to leave once idle -- construction
// in progress is finished first, mirroring logistics.Controller's method of
// the same name for a Serf. Returns false for a stale pointer, which keeps
// UI actions harmless after a builder was already removed.
func (c *Controller) RequestDismissal(b *Builder) bool {
	for _, candidate := range c.Builders {
		if candidate == b {
			candidate.dismissing = true
			return true
		}
	}
	return false
}

// RemoveWarehouse re-anchors every builder whose Warehouse is about to be
// deleted onto the given replacement, mirroring how logistics re-anchors
// serfs. A builder mid-route keeps walking; only the idle-return point
// changes.
func (c *Controller) RemoveWarehouse(removed, replacement *building.Building) {
	for _, b := range c.Builders {
		if b.Warehouse == removed {
			b.Warehouse = replacement
		}
	}
}

// CancelRouteTo resets any builder currently walking toward target -- a
// Tavern being eaten at, or a construction site about to be cancelled --
// back to idle at the warehouse, instead of leaving it holding a dangling
// pointer to a building that's about to be removed from the world.
func (c *Controller) CancelRouteTo(target *building.Building) {
	for _, b := range c.Builders {
		if b.tavern == target {
			b.tavern = nil
			b.resetToIdle()
			continue
		}
		if b.target == target {
			b.resetToIdle()
		}
	}
}

// State reports what the builder is doing.
func (b *Builder) State() State { return b.state }

// TargetSite returns the construction site currently being walked to or
// worked, if any.
func (b *Builder) TargetSite() *building.Building { return b.target }

// Meal returns the food reserved for the current or next Tavern trip.
func (b *Builder) Meal() resource.Type { return b.meal }

// HungerTicks returns simulation ticks since the last meal.
func (b *Builder) HungerTicks() int { return b.hungerTick }

// SatietyPercent returns the player-facing 0-100 satiety value.
func (b *Builder) SatietyPercent() int { return hunger.Percent(b.hungerTick) }

// RemainingPath returns the tiles still ahead on the builder's current
// route, starting from (and including) the tile it's walking toward right
// now -- for the inspector's route-line overlay (see
// ui.DrawSelectedRoute). nil once idle or with no path assigned.
func (b *Builder) RemainingPath() []pathfind.Point {
	if b.pathIdx >= len(b.path) {
		return nil
	}
	return b.path[b.pathIdx:]
}

// WorkTicks returns progress through the current construction stage.
func (b *Builder) WorkTicks() int { return b.workTicks }

// AtPost reports whether the worker is idle. Unlike a hut-based profession,
// a builder has no interior to hide inside, so "idle" doesn't imply any
// particular position -- he simply stands wherever his last job left him.
func (b *Builder) AtPost() bool {
	return b.state == StateIdle
}

// VisibleOnMap is always true: unlike a hut-based profession, a builder has
// no interior to hide inside while idle -- he simply stands at the
// warehouse.
func (b *Builder) VisibleOnMap() bool { return true }

// Reserve seeds ledger with every builder currently walking to eat, so
// other controllers sharing a Tavern see this claim before making their
// own commitments this tick. Call once per simulation tick, before this or
// any other controller's Tick runs.
func (c *Controller) Reserve(ledger *reservations.Ledger) {
	for _, b := range c.Builders {
		if b.state == StateToTavern && b.tavern != nil {
			ledger.ReservePickup(b.tavern, b.meal, 1)
		}
	}
}

// MaxWaitingHunger returns the highest HungerTicks among builders that will
// actually try to eat this tick (idle, HungerTicks >= HungerInterval), or -1
// if none will. See lumberjack.Controller's method of the same name for why
// this ordering matters when Tavern food is scarce.
func (c *Controller) MaxWaitingHunger() int {
	best := -1
	for _, b := range c.Builders {
		if b.state != StateIdle || b.hungerTick < HungerInterval {
			continue
		}
		if b.hungerTick > best {
			best = b.hungerTick
		}
	}
	return best
}

// Tick advances every builder and returns completed events. Call once per
// simulation tick, after every controller sharing ledger has had a chance
// to Reserve its own pre-existing in-flight units.
func (c *Controller) Tick(grid *world.Grid, buildings []*building.Building, ledger *reservations.Ledger) []Event {
	var events []Event
	remaining := c.Builders[:0]
	for _, b := range c.Builders {
		b.hungerTick++
		if hunger.Dead(b.hungerTick) {
			events = append(events, Event{Kind: WorkerDied})
			continue
		}
		// Checked before hunger/job assignment, same ordering as
		// logistics.Controller.Tick: an idle builder leaves immediately,
		// and one who just finished a site never starts a fresh job first.
		if b.dismissing && b.state == StateIdle {
			events = append(events, Event{Kind: WorkerDismissed})
			continue
		}
		remaining = append(remaining, b)

		switch b.state {
		case StateIdle:
			if b.hungerTick >= HungerInterval && c.tryStartMeal(b, grid, buildings, ledger) {
				continue
			}
			c.startSiteJob(b, grid, buildings)
			if b.state == StateIdle {
				// A fresh building always takes priority over patching an
				// old one -- only tried once startSiteJob found nothing.
				c.startRepairJob(b, grid, buildings)
			}
		case StateToSite:
			if !siteUsable(buildings, b.target) && !repairUsable(buildings, b.target) {
				b.resetToIdle()
				continue
			}
			if b.advancePath() {
				if b.target.ConstructionStage == building.ConstructionNone {
					// Arrived at a damaged, already-finished building, not
					// a fresh site -- see startRepairJob.
					b.state = StateRepairing
				} else {
					b.state = StateFoundation
				}
				b.workTicks = 0
			}
		case StateRepairing:
			if !repairUsable(buildings, b.target) {
				b.resetToIdle()
				continue
			}
			b.workTicks++
			if b.workTicks < RepairTicks {
				continue
			}
			b.target.HP = building.MaxHP
			// No ConstructionComplete event: the resident worker (if any)
			// never left, so cmd/game must not re-spawn one.
			b.resetToIdle()
		case StateFoundation:
			if !siteUsable(buildings, b.target) {
				b.resetToIdle()
				continue
			}
			b.workTicks++
			total := building.Types[b.target.Kind].ConstructionFoundationTicks
			if b.workTicks < total {
				continue
			}
			b.target.ProgressTicks = 0
			if b.target.ConstructionMaterialsReady() {
				b.target.ConstructionStage = building.ConstructionFinishing
				b.state = StateFinishing
				b.workTicks = 0
			} else {
				b.target.ConstructionStage = building.ConstructionWaitingMaterials
				b.state = StateWaitingMaterials
			}
		case StateWaitingMaterials:
			if !siteUsable(buildings, b.target) {
				b.resetToIdle()
				continue
			}
			if b.target.ConstructionMaterialsReady() {
				b.target.ConstructionStage = building.ConstructionFinishing
				b.state = StateFinishing
				b.workTicks = 0
				continue
			}
			// Real bug this fixes: this wait has no upper bound (a serf
			// might take a very long time to reach a distant site with
			// scarce materials), but hunger was never checked here --
			// only at StateIdle. hunger.Dead is checked unconditionally
			// above regardless of state, so a builder stuck waiting simply
			// starved with no chance to eat. b.target is untouched by the
			// Tavern trip (tryStartMeal never clears it), and the
			// StateToTavern arrival handler below resumes StateWaitingMaterials
			// at this same site afterward instead of losing the already
			// finished foundation phase through a fresh startSiteJob.
			if b.hungerTick >= HungerInterval && c.tryStartMeal(b, grid, buildings, ledger) {
				continue
			}
		case StateFinishing:
			if !siteUsable(buildings, b.target) {
				b.resetToIdle()
				continue
			}
			b.workTicks++
			total := building.Types[b.target.Kind].ConstructionBuildTicks
			if b.workTicks < total {
				continue
			}
			site := b.target
			site.ConstructionStage = building.ConstructionNone
			site.ProgressTicks = 0
			// The delivered materials did their job; a finished building
			// starts with a clean InputBuffer for its own real recipe (if
			// any), same as a normally-placed one always has.
			delete(site.InputBuffer, resource.Plank)
			delete(site.InputBuffer, resource.StoneBlock)
			b.resetToIdle()
			events = append(events, Event{Kind: ConstructionComplete, Building: site})
		case StateToTavern:
			if len(b.path) == 0 {
				if b.tavern == nil || !b.routeToBuilding(grid, buildings, b.tavern, StateToTavern) {
					b.resetToIdle()
				}
				continue
			}
			if !b.advancePath() {
				continue
			}
			if b.tavern != nil && b.tavern.TakeInput(b.meal, 1) {
				b.hungerTick = 0
				b.Starving = false
			} else {
				b.Starving = true
			}
			b.tavern = nil
			// A meal interrupted from StateWaitingMaterials (see above)
			// must resume waiting at the same site, not go idle -- idle
			// would run startSiteJob fresh, which unconditionally starts a
			// new site at StateToSite/StateFoundation and would either
			// re-do already-finished foundation work on this exact site or
			// abandon it to a different one entirely. A meal interrupted
			// from ordinary StateIdle leaves b.target nil, so this check
			// correctly falls through to the old behavior for that case.
			if siteUsable(buildings, b.target) && b.target.ConstructionStage == building.ConstructionWaitingMaterials {
				b.state = StateWaitingMaterials
				b.path, b.pathIdx, b.tileTicks = nil, 0, 0
				continue
			}
			// No hut to walk back to (see VisibleOnMap) -- go idle right
			// here at the Tavern, exactly like a Serf does.
			b.resetToIdle()
		}
	}
	c.Builders = remaining
	return events
}

// siteUsable reports whether target is still a live construction site --
// it may have been cancelled (Delete) or already finished by the time this
// builder's next tick runs.
func siteUsable(buildings []*building.Building, target *building.Building) bool {
	if target == nil || target.ConstructionStage == building.ConstructionNone {
		return false
	}
	return containsBuilding(buildings, target)
}

// repairUsable reports whether target is still a live, damaged, finished
// building worth walking to or working on -- it may have been fully
// healed by a second builder, destroyed, or removed (Delete) by the time
// this builder's next tick runs.
func repairUsable(buildings []*building.Building, target *building.Building) bool {
	if target == nil || target.ConstructionStage != building.ConstructionNone {
		return false
	}
	if target.HP <= 0 || target.HP >= building.MaxHP {
		return false
	}
	return containsBuilding(buildings, target)
}

// nearestTavernWithFood mirrors lumberjack's helper of the same name.
func nearestTavernWithFood(grid *world.Grid, buildings []*building.Building, from pathfind.Point, ledger *reservations.Ledger, selector *meal.Selector, wanted ...resource.Type) (tavern *building.Building, selected resource.Type, path []pathfind.Point, ok bool) {
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
		p := b.AccessPoint()
		route, reachable := pathfind.FindLandPath(grid, buildings, from, pathfind.Point{X: p.X, Y: p.Y})
		if !reachable {
			continue
		}
		if bestLen == -1 || len(route) < bestLen {
			tavern, available, path, bestLen = b, foods, route, len(route)
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

func (c *Controller) tryStartMeal(b *Builder, grid *world.Grid, buildings []*building.Building, ledger *reservations.Ledger) bool {
	tavern, meal, path, ok := nearestTavernWithFood(grid, buildings, pathfind.Point{X: b.X, Y: b.Y}, ledger, &c.meals)
	if !ok {
		b.Starving = true
		return false
	}
	b.Starving = false
	b.tavern = tavern
	b.meal = meal
	b.setPath(path)
	b.state = StateToTavern
	ledger.ReservePickup(tavern, meal, 1)
	return true
}

// startSiteJob finds the nearest reachable construction site not already
// claimed by another builder. Unlike a stone deposit (thousands of units,
// several quarrymen welcome), exactly one builder per site makes sense --
// a second pair of hands has nothing productive to do once the first is
// already digging the same foundation.
// startSiteJob deliberately has no lumberjack.MaxWorkRadius-style distance
// cap, unlike the other three "roam free land" professions: a construction
// site is a unique, player-placed target, not an interchangeable resource
// node -- skipping a far one wouldn't send the builder to a nearer
// equivalent, it would just leave that exact building permanently
// unstarted. This is safe without a cap because StateIdle already never
// starts a fresh walk while hungerTick >= HungerInterval (see the Tick
// switch), so the worst case is a walk starting at just under that
// threshold: roughly (hunger.MaxTicks-HungerInterval)/TicksPerTile ~= 130
// tiles of headroom, comfortably more than this map's longest single-axis
// distance -- and StateWaitingMaterials can no longer starve a builder in
// place once there (see its own hunger check), so the site's distance
// itself is the only remaining risk, not how long the wait there runs.
func (c *Controller) startSiteJob(b *Builder, grid *world.Grid, buildings []*building.Building) {
	start := pathfind.Point{X: b.X, Y: b.Y}
	bestLength := int(^uint(0) >> 1)
	var bestSite *building.Building
	var bestPath []pathfind.Point
	for _, candidate := range buildings {
		if candidate == nil || candidate.ConstructionStage == building.ConstructionNone || c.siteReserved(candidate, b) {
			continue
		}
		path, ok := pathfind.FindLandPath(grid, buildings, start, pathfind.Point{X: candidate.X, Y: candidate.Y})
		if !ok || len(path) >= bestLength {
			continue
		}
		bestSite, bestPath, bestLength = candidate, path, len(path)
	}
	if bestSite == nil {
		return
	}
	b.target = bestSite
	b.setPath(bestPath)
	b.state = StateToSite
}

func (c *Controller) siteReserved(site *building.Building, except *Builder) bool {
	for _, b := range c.Builders {
		if b != except && b.target == site {
			return true
		}
	}
	return false
}

// startRepairJob finds the nearest reachable damaged, finished building
// not already claimed by another builder (siteReserved doubles as the
// reservation check here too: b.target means "the site or building this
// builder is currently walking to or working on" either way). Only
// tried once startSiteJob found no fresh construction -- see its call
// site in Tick.
func (c *Controller) startRepairJob(b *Builder, grid *world.Grid, buildings []*building.Building) {
	start := pathfind.Point{X: b.X, Y: b.Y}
	bestLength := int(^uint(0) >> 1)
	var bestSite *building.Building
	var bestPath []pathfind.Point
	for _, candidate := range buildings {
		if candidate == nil || !repairUsable(buildings, candidate) || c.siteReserved(candidate, b) {
			continue
		}
		path, ok := pathfind.FindLandPath(grid, buildings, start, pathfind.Point{X: candidate.X, Y: candidate.Y})
		if !ok || len(path) >= bestLength {
			continue
		}
		bestSite, bestPath, bestLength = candidate, path, len(path)
	}
	if bestSite == nil {
		return
	}
	b.target = bestSite
	b.setPath(bestPath)
	b.state = StateToSite
}

func (b *Builder) routeTo(grid *world.Grid, buildings []*building.Building, goal pathfind.Point) bool {
	path, ok := pathfind.FindLandPath(grid, buildings, pathfind.Point{X: b.X, Y: b.Y}, goal)
	if !ok {
		return false
	}
	b.setPath(path)
	return true
}

func (b *Builder) routeToBuilding(grid *world.Grid, buildings []*building.Building, target *building.Building, state State) bool {
	if target == nil {
		return false
	}
	p := target.AccessPoint()
	if !b.routeTo(grid, buildings, pathfind.Point{X: p.X, Y: p.Y}) {
		return false
	}
	b.state = state
	return true
}

func (b *Builder) setPath(path []pathfind.Point) {
	b.path = path
	b.pathIdx = 0
	b.tileTicks = 0
}

func (b *Builder) advancePath() bool {
	if len(b.path) == 0 {
		return true
	}
	b.tileTicks++
	if b.tileTicks < TicksPerTile {
		return false
	}
	b.tileTicks = 0
	if b.pathIdx < len(b.path)-1 {
		b.pathIdx++
		b.X, b.Y = b.path[b.pathIdx].X, b.path[b.pathIdx].Y
		return false
	}
	return true
}

// resetToIdle clears job state without moving the builder -- unlike
// lumberjack/quarryman/fishing's version of this, there's no hut to
// teleport back into for the "hidden while idle" illusion, since a builder
// is always visible (see VisibleOnMap). He simply stands wherever his last
// job left him, the same way a Serf does after Serf.reset().
func (b *Builder) resetToIdle() {
	b.state = StateIdle
	b.target = nil
	b.path = nil
	b.pathIdx = 0
	b.tileTicks = 0
	b.workTicks = 0
}

func containsBuilding(buildings []*building.Building, target *building.Building) bool {
	if target == nil {
		return false
	}
	for _, b := range buildings {
		if b == target {
			return true
		}
	}
	return false
}
