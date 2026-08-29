// Package advisor is a heuristic gameplay assistant: it reads the current
// town state and reports what's worth the player's attention right now
// (food running low, an idle workplace, a building the road network
// doesn't reach, too few or too many serfs). Deliberately not a call to an
// external AI/LLM -- for a local, single-player, deterministic game that
// would be a needless network dependency, latency and cost. Like
// economy/logistics/builder, this package knows nothing about ebiten or
// the UI: cmd/game turns a Tip into localized text and a toast, and owns
// every policy question (how often to check, cooldowns, the display
// queue) -- see AGENTS.md for that reasoning.
package advisor

import (
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/hunger"
	"strategy_game/internal/resource"
)

// FoodRunwayWarningTicks is how many ticks of food reserve remaining
// triggers KindFoodRunningOut -- the midpoint of the user's requested
// "300, может быть, 400" range.
const FoodRunwayWarningTicks = 350

// IdleBuildingWarningTicks is how long a workplace must have sat unstaffed
// before it's worth flagging -- long enough that a building the player
// just finished isn't immediately nagged about while they're still on
// their way to the Hire tab.
const IdleBuildingWarningTicks = 200

// GatherWorkerStuckWarningTicks is how long a lumberjack/quarryman/miner
// must have sat idle -- not unstaffed, staffed but unable to find any
// tree/stone deposit/ore deposit to walk to -- before it's worth
// flagging. The midpoint of the user's requested "600 или 700" range.
// Under normal operation a gathering worker's idle state lasts at most a
// tick or two before it finds a new target, so a long, continuous stretch
// of it means the reachable resource near that hut is genuinely gone, not
// that the worker merely hasn't been assigned yet.
const GatherWorkerStuckWarningTicks = 650

// serveCountLowRatio/serveCountHighRatio bound how far the current serf
// count may drift from recommendedServeCount before it's worth mentioning.
// Deliberately asymmetric: too few serfs is a real production problem, so
// the low threshold sits close to the recommendation itself; too many is
// mostly just spent gold, so the high threshold gives a lot of room before
// it's worth interrupting the player over it.
const (
	serveCountLowRatio  = 70  // percent of recommended
	serveCountHighRatio = 200 // percent of recommended
)

// Kind identifies which situation a Tip is reporting.
type Kind int

const (
	KindFoodRunningOut Kind = iota
	KindIdleBuilding
	KindDisconnectedBuilding
	KindServeCountLow
	KindServeCountHigh
	KindGatherWorkerStuck
)

// Tip is one actionable observation. Only the fields relevant to its Kind
// are populated; the rest stay zero.
type Tip struct {
	Kind Kind

	// Building is one representative instance for KindIdleBuilding/
	// KindDisconnectedBuilding/KindGatherWorkerStuck -- not every
	// offending building, just an example the player can jump to. Count
	// is how many there are in total.
	Building *building.Building
	Count    int

	// TicksLeft is how long the current food reserve would last at the
	// current population's consumption rate if nothing changes -- see
	// Evaluate's doc comment for why this ignores ongoing production.
	TicksLeft int

	// Recommended/Current are the serf counts KindServeCountLow/High
	// compared.
	Recommended, Current int
}

// Evaluate returns every situation currently worth a tip, in a fixed
// order (food, idle, disconnected, serf count, gather workers stuck) so
// callers get a stable ranking without needing their own tie-breaking. It
// has no memory of what was already shown or when -- that policy
// (cooldowns, the display queue, how often to even call this) belongs to
// the caller (cmd/game), not here; this function only ever answers "what's
// true right now".
//
// idleSince maps a RequiresWorker building to the simulation tick it most
// recently became unstaffed (see cmd/game's unstaffedWorkerBuildings), and
// gatherStuckSince maps a LumberjackHut/QuarryHut/MinerHut to the tick its
// worker most recently entered its own package's StateIdle (see cmd/game's
// trackAdvisorGatherWorkers) -- staffed and searching, but not finding any
// tree/stone deposit/ore deposit to walk to. The caller owns both trackers
// (Evaluate is a pure function, no state of its own) and passes the
// current tick so each duration can be measured.
func Evaluate(buildings []*building.Building, stock *resource.Stockpile, pop *economy.Population, disconnected map[*building.Building]bool, idleSince map[*building.Building]int, gatherStuckSince map[*building.Building]int, currentTick int, recommendedServe, currentServe int) []Tip {
	var tips []Tip

	if tip, ok := evaluateFoodRunway(buildings, stock, pop); ok {
		tips = append(tips, tip)
	}
	if tip, ok := evaluateSinceMap(KindIdleBuilding, idleSince, currentTick, IdleBuildingWarningTicks); ok {
		tips = append(tips, tip)
	}
	if tip, ok := evaluateDisconnectedBuildings(buildings, disconnected); ok {
		tips = append(tips, tip)
	}
	if tip, ok := evaluateServeCount(recommendedServe, currentServe); ok {
		tips = append(tips, tip)
	}
	if tip, ok := evaluateSinceMap(KindGatherWorkerStuck, gatherStuckSince, currentTick, GatherWorkerStuckWarningTicks); ok {
		tips = append(tips, tip)
	}
	return tips
}

// evaluateFoodRunway reports how long the current food reserve (Warehouse
// stockpile plus every Tavern's InputBuffer, summed across every food
// type) would last at the population's current consumption rate.
//
// Deliberately ignores ongoing production: this is "how much runway you
// have if nothing changes", not a forecast that assumes the farm/winery/
// etc. keep delivering -- a low reserve is worth flagging even if
// production looks healthy right now, since production can stop (a
// worker dies, a road breaks) and the whole point of a warning is to
// catch that before it becomes a crisis, not after.
func evaluateFoodRunway(buildings []*building.Building, stock *resource.Stockpile, pop *economy.Population) (Tip, bool) {
	if pop == nil || pop.Count <= 0 {
		return Tip{}, false
	}
	foodStock := 0
	for _, rt := range resource.FoodTypes() {
		foodStock += stock.Amount(rt)
	}
	for _, b := range buildings {
		if b == nil || b.Kind != building.Tavern {
			continue
		}
		for _, rt := range resource.FoodTypes() {
			foodStock += b.InputBuffer[rt]
		}
	}
	// consumptionRate is in food units per tick: one resident eats once
	// roughly every hunger.MealThresholdTicks (the 20%-satiety point a
	// hungry unit heads to a Tavern at), so pop.Count residents together
	// consume pop.Count meals every MealThresholdTicks ticks.
	ticksLeft := foodStock * hunger.MealThresholdTicks / pop.Count
	if ticksLeft > FoodRunwayWarningTicks {
		return Tip{}, false
	}
	return Tip{Kind: KindFoodRunningOut, TicksLeft: ticksLeft}, true
}

// evaluateSinceMap is the shared shape behind both KindIdleBuilding and
// KindGatherWorkerStuck: a building has been in some undesirable state
// continuously since a given tick, and if that's lasted at least
// warningTicks, it's reported aggregated into a single tip (Count total,
// Building one example) rather than one per building.
func evaluateSinceMap(kind Kind, since map[*building.Building]int, currentTick, warningTicks int) (Tip, bool) {
	count := 0
	var example *building.Building
	for b, startedAt := range since {
		if currentTick-startedAt < warningTicks {
			continue
		}
		count++
		if example == nil {
			example = b
		}
	}
	if count == 0 {
		return Tip{}, false
	}
	return Tip{Kind: kind, Building: example, Count: count}, true
}

// evaluateDisconnectedBuildings reports production buildings the road
// network doesn't currently reach, aggregated the same way as idle ones.
func evaluateDisconnectedBuildings(buildings []*building.Building, disconnected map[*building.Building]bool) (Tip, bool) {
	count := 0
	var example *building.Building
	for _, b := range buildings {
		if b != nil && disconnected[b] {
			count++
			if example == nil {
				example = b
			}
		}
	}
	if count == 0 {
		return Tip{}, false
	}
	return Tip{Kind: KindDisconnectedBuilding, Building: example, Count: count}, true
}

// evaluateServeCount compares the current serf count against
// recommendedServe and reports if it's far enough off in either
// direction to be worth mentioning.
func evaluateServeCount(recommended, current int) (Tip, bool) {
	if recommended <= 0 {
		return Tip{}, false
	}
	switch {
	case current*100 < recommended*serveCountLowRatio:
		return Tip{Kind: KindServeCountLow, Recommended: recommended, Current: current}, true
	case current*100 > recommended*serveCountHighRatio:
		return Tip{Kind: KindServeCountHigh, Recommended: recommended, Current: current}, true
	default:
		return Tip{}, false
	}
}
