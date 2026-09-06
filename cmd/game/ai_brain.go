package main

import (
	"strategy_game/internal/building"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/resource"
	"strategy_game/internal/soldier"
	"strategy_game/internal/villagers"
	"strategy_game/internal/world"
)

// aiDifficulty scales how fast the AI opponent's economy and army grow --
// per the user's explicit request for 3 levels, differing only in AI
// behaviour, never in starting resources (see newFaction, which gives the
// AI exactly the same starting stockpile as the player regardless of
// difficulty).
type aiDifficulty int

const (
	AIEasy aiDifficulty = iota
	AINormal
	AIHard
)

// decisionIntervalTicks is how often (in simulation ticks) the brain
// re-evaluates what to build/hire/attack with -- lower means the AI
// reacts and expands faster.
// decisionIntervalTicks was 40/25/15 originally -- roughly tripled after
// a real playtest report: the AI can do everything a single decision
// tick allows for free in one instant (hire serfs, hire a builder, staff
// every empty building, place its next construction, requeue the
// Armory, hire soldiers, consider an attack), while a human player must
// manually navigate menus and place things one at a time, each costing
// real seconds of deliberate play. The reported outcome at the old
// values: the player had managed a FisherHut, Tavern and Winery by the
// time the AI already had a finished WatchTower and was quietly massing
// soldiers. Tripling the interval doesn't remove that structural
// advantage (a deeper fix would rate-limit the AI to fewer actions per
// tick, real future work), but it does triple how much real time the
// player has to keep pace before the AI's next burst of free actions.
func (d aiDifficulty) decisionIntervalTicks() int {
	switch d {
	case AIEasy:
		return 120
	case AIHard:
		return 45
	default:
		return 75
	}
}

// attackSquadSize is how many idle soldiers the AI gathers at its
// Barracks before marching on the player -- lower means it attacks
// sooner, with a smaller force.
func (d aiDifficulty) attackSquadSize() int {
	switch d {
	case AIEasy:
		return 8
	case AIHard:
		return 3
	default:
		return 5
	}
}

// aiBuildOrder is the AI's fixed construction priority list -- the same
// kind of progression a player would follow by hand (Tavern to keep
// everyone fed, FisherHut for an actually ongoing food supply, then
// gather/process chains, then the equipment chain a Barracks hire
// needs). Still skips Mill/Bakery/Winery/MeatWorkshop -- a shorter,
// combat-focused chain than the player's full self-sufficiency.
//
// FisherHut was NOT in this list originally (relying only on the AI's
// one-time starting food stock instead) -- a real bug found by
// simulating a full 40000-tick match: the AI's population reliably
// starved to death entirely (0 population, 0 soldiers) around tick
// 25000-30000 in every run, well before ever gathering attackSquadSize
// idle soldiers to march -- "нападут ли они друг на друга" was
// unreachable no matter how healthy the rest of the economy was. The
// duel map's water divide runs right along both factions' territory
// (see growCenterWaterStrip), so a FisherHut placement is always
// available nearby -- Fish needs no further processing chain (unlike
// Farm->Mill->Bakery for Bread) and Tavern already accepts it.
var aiBuildOrder = []building.Kind{
	// Tavern first, and deliberately not gated on anything else finishing
	// first: a real bug found by actually simulating this brain (see
	// AGENTS.md's "1×1 против ИИ" notes) -- every AI unit, including the
	// 3 starting serfs, starves to death on schedule regardless of how
	// much food sits in the warehouse if there is nowhere to eat it.
	building.Tavern,
	building.FisherHut,
	building.Farm,
	building.LumberjackHut,
	building.CarpentryWorkshop,
	building.QuarryHut,
	building.MinerHut,
	building.Smeltery,
	building.PigFarm,
	building.WatchTower,
	building.Barracks,
	building.Armory,
}

// aiBrain is one AI faction's decision-maker -- see tickAIFaction, which
// calls brain.tick once per simulation tick. Engine-free like every other
// package in this project would be, but small enough (and specific
// enough to cmd/game's own Game/faction types) to live directly in
// cmd/game rather than as its own internal/ package for this first pass.
type aiBrain struct {
	owner      int
	difficulty aiDifficulty
	cooldown   int
	buildIndex int

	// buildAttempts counts consecutive failed placement attempts for
	// aiBuildOrder[buildIndex] -- see aiBuildNext's doc comment on
	// maxBuildAttemptsPerEntry for the real bug this guards against.
	buildAttempts int
}

func newAIBrain(owner int, difficulty aiDifficulty) *aiBrain {
	return &aiBrain{owner: owner, difficulty: difficulty}
}

// tick throttles actual decisions to once every decisionIntervalTicks
// simulation ticks -- an AI that re-evaluated every single tick would be
// needlessly expensive (building placement search included) for no
// behavioural benefit.
func (b *aiBrain) tick(g *Game, f *faction, grid *world.Grid) {
	if b.cooldown > 0 {
		b.cooldown--
		return
	}
	b.cooldown = b.difficulty.decisionIntervalTicks()
	g.aiRepairConnectivity(f, grid)
	g.aiHireServes(f)
	g.aiHireBuilder(f)
	g.aiStaffBuildings(f)
	b.aiBuildNext(g, f, grid)
	g.aiQueueArmoryProduction(f)
	g.aiHireSoldiers(f)
	b.aiConsiderAttack(g, f)
}

// aiRepairConnectivity re-routes a road to any AI building that has gone
// disconnected -- a real bug found by simulating several duel matches
// back to back: aiPlaceBuilding's hasRoadableOrthogonalNeighbor check
// (see its doc comment) only proves a connection is possible at the
// MOMENT of placement. A tile that was free then can still be taken
// later -- by tree regrowth, or by a subsequent aiPlaceBuilding call
// placing something else on it -- silently cutting a building off for
// the rest of the match with nothing to ever notice or fix it. Confirmed
// via simulation: one run's Barracks sat fully disconnected (empty
// InputBuffer the entire game) while every other building, including the
// Armory two tiles away, worked fine -- zero AI soldiers were ever hired
// as a direct result, even with a fully finished build order and 224
// banked gold. Cheap to run: skipped entirely once nothing is
// disconnected (the common case), and only recomputes a route for
// buildings that actually need one.
func (g *Game) aiRepairConnectivity(f *faction, grid *world.Grid) {
	warehouse := findWarehouseOwnedBy(g.buildings, f.owner)
	if warehouse == nil {
		return
	}
	warehouseAccess := warehouse.AccessPoint()
	for _, b := range g.ownedBuildings(f.owner) {
		if isNaturalResourceKind(b.Kind) || b.Kind == building.Warehouse || b.Kind == building.Road {
			continue
		}
		if g.buildingConnected(b) {
			continue
		}
		access := b.AccessPoint()
		path, ok := pathfind.FindLandPath(grid, g.buildings, pathfind.Point{X: access.X, Y: access.Y}, pathfind.Point{X: warehouseAccess.X, Y: warehouseAccess.Y})
		if !ok {
			continue
		}
		g.aiPaveRoad(f, pathfind.Point{X: access.X, Y: access.Y}, path)
		g.invalidateConnectionCache()
	}
}

// aiTargetServeCount is a simple, fixed serf-count target -- a real bug
// found by actually simulating this brain (see AGENTS.md's "1×1 против
// ИИ" notes): with only its starting 3 serfs, the AI's whole town
// (12+ buildings, several workers each needing feeding) starved to
// death wholesale long before any soldier was ever hired -- there simply
// weren't enough hands to keep the one Tavern stocked with food on top
// of every other haul. Not the player's own recommendedServeCount
// formula (that depends on live road-distance measurements this brain
// has no easy access to outside a decision tick) -- just enough headroom
// for the AI's own fixed, comparatually small build order.
const aiTargetServeCount = 10

// aiHireServes tops up the AI's serf count toward aiTargetServeCount, a
// few at a time per decision tick.
func (g *Game) aiHireServes(f *faction) {
	const maxHiresPerTick = 2
	for i := 0; i < maxHiresPerTick && len(f.logi.Serfs) < aiTargetServeCount; i++ {
		if !f.stock.Remove(resource.Gold, unitHireCost) {
			return
		}
		f.logi.Hire()
	}
}

// aiHireBuilder keeps up to two Builders on staff -- enough to work more
// than one construction site at once without over-spending gold on idle
// hands.
func (g *Game) aiHireBuilder(f *faction) {
	if len(f.builders.Builders) >= 2 {
		return
	}
	warehouse := findWarehouseOwnedBy(g.buildings, f.owner)
	if warehouse == nil || !f.stock.Remove(resource.Gold, unitHireCost) {
		return
	}
	f.builders.Hire(warehouse)
}

// aiHasResident reports whether b already has a worker assigned, across
// whichever controller its profession actually lives in.
func (g *Game) aiHasResident(f *faction, b *building.Building) bool {
	switch b.Kind {
	case building.PigFarm, building.MeatWorkshop, building.CarpentryWorkshop, building.Smeltery, building.Armory, building.Farm, building.Bakery, building.Winery:
		return f.vills.HasHome(b)
	case building.LumberjackHut:
		for _, u := range f.jacks.Lumberjacks {
			if u.HomeBuilding() == b {
				return true
			}
		}
	case building.FisherHut:
		for _, u := range f.fishers.Fishermen {
			if u.HomeBuilding() == b {
				return true
			}
		}
	case building.QuarryHut:
		for _, u := range f.quarry.Quarrymen {
			if u.HomeBuilding() == b {
				return true
			}
		}
	case building.MinerHut:
		for _, u := range f.miners.Miners {
			if u.HomeBuilding() == b {
				return true
			}
		}
	case building.WatchTower:
		for _, u := range f.sentries.Sentries {
			if u.HomeBuilding() == b {
				return true
			}
		}
	}
	return false
}

// aiStaffBuildings hires a worker into every finished, unstaffed
// RequiresWorker building the AI owns -- the same "1 gold per hire" the
// player pays through the Hire tab (see trySpendGold), just spent from
// the AI's own stockpile instead.
func (g *Game) aiStaffBuildings(f *faction) {
	for _, b := range g.ownedBuildings(f.owner) {
		if b.ConstructionStage != building.ConstructionNone || !building.Types[b.Kind].RequiresWorker {
			continue
		}
		if g.aiHasResident(f, b) {
			continue
		}
		switch b.Kind {
		case building.PigFarm:
			g.aiHireVillagerInto(f, villagers.Swineherd, b)
		case building.MeatWorkshop:
			g.aiHireVillagerInto(f, villagers.Butcher, b)
		case building.CarpentryWorkshop:
			g.aiHireVillagerInto(f, villagers.Carpenter, b)
		case building.Smeltery:
			g.aiHireVillagerInto(f, villagers.Smelter, b)
		case building.Armory:
			g.aiHireVillagerInto(f, villagers.Weaponsmith, b)
		case building.Farm:
			g.aiHireVillagerInto(f, villagers.Farmer, b)
		case building.Bakery:
			g.aiHireVillagerInto(f, villagers.Baker, b)
		case building.Winery:
			g.aiHireVillagerInto(f, villagers.Winemaker, b)
		case building.LumberjackHut:
			if f.stock.Remove(resource.Gold, unitHireCost) {
				f.jacks.Spawn(b)
			}
		case building.FisherHut:
			if f.stock.Remove(resource.Gold, unitHireCost) {
				f.fishers.Spawn(b)
			}
		case building.QuarryHut:
			if f.stock.Remove(resource.Gold, unitHireCost) {
				f.quarry.Spawn(b)
			}
		case building.MinerHut:
			if f.stock.Remove(resource.Gold, unitHireCost) {
				f.miners.Spawn(b)
			}
		case building.WatchTower:
			if f.stock.Remove(resource.Gold, unitHireCost) {
				f.sentries.Spawn(b)
			}
		}
	}
}

func (g *Game) aiHireVillagerInto(f *faction, profession villagers.Profession, home *building.Building) {
	if !f.stock.Remove(resource.Gold, unitHireCost) {
		return
	}
	f.vills.Spawn(profession, home)
}

// aiHasBuilding reports whether the AI already has (or is building) one
// of kind.
func (g *Game) aiHasBuilding(f *faction, kind building.Kind) bool {
	for _, b := range g.ownedBuildings(f.owner) {
		if b.Kind == kind {
			return true
		}
	}
	return false
}

// aiBuildNext advances the AI's fixed build order by one entry per
// decision tick at most: skip anything already placed, otherwise try to
// place the next one (and only advance the index once that placement
// actually succeeds -- a failed attempt, e.g. not enough banked material
// yet, just retries on the next decision).
// maxBuildAttemptsPerEntry caps how many decision ticks aiBuildNext will
// keep retrying the same aiBuildOrder entry before giving up on it and
// moving to the next one -- a real bug found by simulating several duel
// matches back to back: aiPlaceBuilding can legitimately fail forever on
// a given map (most commonly FisherHut, which additionally requires a
// cardinal water-adjacent tile within aiPlacementSearchRadius -- not
// every Warehouse spot has one within range). Without a cap, getting
// stuck on one entry blocked the ENTIRE rest of the build order forever,
// including Barracks/Armory -- in a 4-run batch, 3 of 4 duel matches
// never fielded a single AI soldier in 50000 ticks because of exactly
// this (confirmed via brain.buildIndex staying stuck below len(
// aiBuildOrder) the whole run). Retrying isn't free (material cost is
// only spent on success, but the ring search itself is real work), so
// this is generous rather than tight -- most entries succeed within a
// handful of attempts once resources are banked.
const maxBuildAttemptsPerEntry = 40

func (b *aiBrain) aiBuildNext(g *Game, f *faction, grid *world.Grid) {
	for b.buildIndex < len(aiBuildOrder) {
		kind := aiBuildOrder[b.buildIndex]
		if g.aiHasBuilding(f, kind) {
			b.buildIndex++
			b.buildAttempts = 0
			continue
		}
		if g.aiPlaceBuilding(f, grid, kind) {
			b.buildIndex++
			b.buildAttempts = 0
			return
		}
		b.buildAttempts++
		if b.buildAttempts >= maxBuildAttemptsPerEntry {
			b.buildIndex++
			b.buildAttempts = 0
			continue
		}
		return
	}
}

// aiPlacementSearchRadius bounds how far from its own Warehouse the AI
// will look for room to place its next building -- generous enough to
// find room even in a cramped starting area without searching forever.
const aiPlacementSearchRadius = 25

// aiPlaceBuilding finds a legally placeable tile for kind within
// aiPlacementSearchRadius of the AI's Warehouse (expanding ring search),
// requires a real off-road walkable route back to the Warehouse's own
// access point (pathfind.FindLandPath -- the same free-roam pathing
// package builder/soldier already use), and if one exists: places the
// construction site, then paves every tile of that route as a Road so
// the site is immediately reachable by the AI's own serfs/builders, the
// same way a player connects a new building by hand.
func (g *Game) aiPlaceBuilding(f *faction, grid *world.Grid, kind building.Kind) bool {
	warehouse := findWarehouseOwnedBy(g.buildings, f.owner)
	if warehouse == nil {
		return false
	}
	bt := building.Types[kind]
	if f.stock.Amount(resource.Plank) < bt.PlankCost || f.stock.Amount(resource.StoneBlock) < bt.StoneCost {
		return false // not enough material banked yet -- try again next decision
	}
	warehouseAccess := warehouse.AccessPoint()
	for radius := 2; radius <= aiPlacementSearchRadius; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if absInt(dx) != radius && absInt(dy) != radius {
					continue // only this ring -- smaller ones already tried
				}
				x, y := warehouse.X+dx, warehouse.Y+dy
				if !building.CanPlace(grid, g.buildings, kind, x, y) {
					continue
				}
				access := pathfind.Point{X: x + bt.AccessX, Y: y + bt.AccessY}
				path, ok := pathfind.FindLandPath(grid, g.buildings, access, pathfind.Point{X: warehouseAccess.X, Y: warehouseAccess.Y})
				if !ok {
					continue
				}
				// A real bug found by simulation: CanPlace+FindLandPath only
				// prove a WALKING route exists -- buildingConnected demands
				// a Road meet the access tile by a cardinal side, never just
				// a diagonal corner (see aiPaveRoad's doc comment). A site
				// whose access tile has no free/roadable orthogonal neighbour
				// at all (boxed in on all four sides by other buildings, e.g.
				// two AI buildings placed diagonally adjacent by an earlier
				// ring search) can never satisfy that no matter how the road
				// is routed -- skip it and keep searching rather than build
				// a permanently disconnected building.
				if !g.hasRoadableOrthogonalNeighbor(grid, access) {
					continue
				}
				site := building.NewConstructionSite(kind, x, y)
				site.Owner = f.owner
				g.buildings = append(g.buildings, site)
				g.aiPaveRoad(f, access, path)
				g.invalidateConnectionCache()
				return true
			}
		}
	}
	return false
}

// aiEnsureRoad places a finished Road tile at (x, y) if nothing already
// occupies it -- the same "instant, free, already-finished" road
// convention newGameWithSize itself uses for the very first road tile
// beside the starting Warehouse, applied here to every tile of an AI
// building's connecting route.
func (g *Game) aiEnsureRoad(f *faction, x, y int) {
	for _, b := range g.buildings {
		if b.X == x && b.Y == y {
			return
		}
	}
	g.buildings = append(g.buildings, &building.Building{Kind: building.Road, X: x, Y: y, Owner: f.owner, HP: building.MaxHP})
}

// aiPaveRoad lays a Road tile at access and at every point of path, the
// same as a plain aiEnsureRoad loop -- except it also bridges any
// diagonal hop with an orthogonal corner tile first.
//
// A real bug found by simulation: FindLandPath (used to plan path, an
// off-road walking route) freely takes diagonal steps, including the
// very first step off of a building's access tile. But the STRICT
// road-only connectivity check buildingConnected relies on
// (findPathBetween's requireRoad path) refuses to treat a building as
// connected via a diagonal first/last hop -- "a road cannot activate a
// building by just touching the corner of its footprint" (see that
// function's own doc comment). Paving literally along path's diagonal
// steps produced a Road chain that looked connected to the eye (every
// tile between the building and the warehouse had *some* building on
// it) but was invisibly disconnected the moment the AI's new building
// touched a diagonal neighbour -- every single one of a freshly built
// AI CarpentryWorkshop's 60000 simulated ticks saw zero Log delivered,
// tracked down to exactly this. Only ever surfaced once natural
// resource nodes stopped being wiped out by pruneDestroyedBuildings
// (see isNaturalResourceKind) -- before that fix nothing was ever
// around to make the AI's route hug a diagonal in the first place.
func (g *Game) aiPaveRoad(f *faction, access pathfind.Point, path []pathfind.Point) {
	prev := access
	g.aiEnsureRoad(f, access.X, access.Y)
	for _, p := range path {
		if prev.X != p.X && prev.Y != p.Y {
			// Diagonal hop -- bridge it with one of the two orthogonal
			// corner tiles so no two consecutive Road tiles are ever
			// only diagonally adjacent. Try the other corner if the
			// first choice is already occupied by something that isn't
			// a Road (a tree, a deposit, another building).
			if !g.aiEnsureRoadIfPlaceable(f, prev.X, p.Y) {
				g.aiEnsureRoadIfPlaceable(f, p.X, prev.Y)
			}
		}
		g.aiEnsureRoad(f, p.X, p.Y)
		prev = p
	}
}

// aiEnsureRoadIfPlaceable is aiEnsureRoad, but reports whether the tile
// ended up Road-occupied (already was, or was just placed) rather than
// blocked by some other building -- used by aiPaveRoad to pick between
// a diagonal hop's two possible bridging corners.
func (g *Game) aiEnsureRoadIfPlaceable(f *faction, x, y int) bool {
	for _, b := range g.buildings {
		if b.X == x && b.Y == y {
			return b.Kind == building.Road
		}
	}
	g.buildings = append(g.buildings, &building.Building{Kind: building.Road, X: x, Y: y, Owner: f.owner, HP: building.MaxHP})
	return true
}

// hasRoadableOrthogonalNeighbor reports whether p has at least one
// cardinal (non-diagonal) neighbour tile that already carries a Road, or
// could still be paved into one -- see aiPlaceBuilding's doc comment on
// why a candidate site without this can never actually satisfy
// buildingConnected no matter how its route is paved.
func (g *Game) hasRoadableOrthogonalNeighbor(grid *world.Grid, p pathfind.Point) bool {
	for _, d := range [4]pathfind.Point{{X: 0, Y: -1}, {X: 0, Y: 1}, {X: -1, Y: 0}, {X: 1, Y: 0}} {
		nx, ny := p.X+d.X, p.Y+d.Y
		if !grid.InBounds(nx, ny) {
			continue
		}
		occupied := false
		for _, b := range g.buildings {
			if b.X == nx && b.Y == ny {
				if b.Kind == building.Road {
					return true
				}
				occupied = true
				break
			}
		}
		if !occupied && building.CanPlace(grid, g.buildings, building.Road, nx, ny) {
			return true
		}
	}
	return false
}

// aiQueueArmoryProduction keeps a standing production order on every
// finished Armory the AI owns, per the user's explicit weapon/armour
// recipe -- topped up rather than a one-shot batch, so production never
// runs dry once the initial batch is crafted.
func (g *Game) aiQueueArmoryProduction(f *faction) {
	for _, b := range g.ownedBuildings(f.owner) {
		if b.Kind != building.Armory || b.ConstructionStage != building.ConstructionNone {
			continue
		}
		// Bug found by simulation: topping each item back up to its
		// target independently, every decision tick, meant Bow (first
		// in armoryOrder) never actually drained to 0 -- the Armory's
		// activeArmoryItem always works the first non-empty queue in
		// priority order, so LeatherArmor and Sword sat queued forever
		// without ever getting a turn (confirmed: armor=0, sword=0
		// across a full 60000-tick run while bow stayed maxed at the
		// Barracks). Only queue a fresh batch once the WHOLE queue has
		// actually drained -- lets Bow->LeatherArmor->Sword each fully
		// run in turn before refilling.
		empty := true
		for _, n := range b.ProductionQueue {
			if n > 0 {
				empty = false
				break
			}
		}
		if !empty {
			continue
		}
		if b.ProductionQueue == nil {
			b.ProductionQueue = map[resource.Type]int{}
		}
		b.ProductionQueue[resource.Bow] = 3
		b.ProductionQueue[resource.LeatherArmor] = 6
		b.ProductionQueue[resource.Sword] = 3
	}
}

// aiHireSoldiers spends whatever equipment has actually arrived at each
// Barracks the AI owns, exactly like the player's own
// hireArcher/hireSwordsman (see canHireEquippedSoldier) -- just against
// the AI's own faction fields. Capped at a few hires per decision tick
// rather than draining the whole buffer at once, so a sudden equipment
// surplus doesn't dump an entire army on top of the Barracks in one go.
func (g *Game) aiHireSoldiers(f *faction) {
	const maxHiresPerKindPerTick = 3
	for _, b := range g.ownedBuildings(f.owner) {
		if b.Kind != building.Barracks || b.ConstructionStage != building.ConstructionNone {
			continue
		}
		for i := 0; i < maxHiresPerKindPerTick; i++ {
			if b.InputBuffer[resource.Gold] < unitHireCost || b.InputBuffer[resource.Sword] < 1 || b.InputBuffer[resource.LeatherArmor] < 1 {
				break
			}
			x, y, ok := g.freeGroundTileNear(b.X, b.Y)
			if !ok {
				break
			}
			b.TakeInput(resource.Gold, unitHireCost)
			b.TakeInput(resource.Sword, 1)
			b.TakeInput(resource.LeatherArmor, 1)
			f.soldiers.Spawn(soldier.Swordsman, x, y)
		}
		for i := 0; i < maxHiresPerKindPerTick; i++ {
			if b.InputBuffer[resource.Gold] < unitHireCost || b.InputBuffer[resource.Bow] < 1 || b.InputBuffer[resource.LeatherArmor] < 1 {
				break
			}
			x, y, ok := g.freeGroundTileNear(b.X, b.Y)
			if !ok {
				break
			}
			b.TakeInput(resource.Gold, unitHireCost)
			b.TakeInput(resource.Bow, 1)
			b.TakeInput(resource.LeatherArmor, 1)
			f.soldiers.Spawn(soldier.Archer, x, y)
		}
	}
}

// aiConsiderAttack marches every idle soldier at once toward the
// NEAREST opposing faction's Warehouse once the AI has gathered
// attackSquadSize of them -- actual fighting along the way is entirely
// the existing FactionEngageRange auto-engage mechanism (see
// soldier.Controller.Tick), not anything this function does directly.
//
// "Nearest" (not the first opponent found, and not random) matters once
// there can be more than one: per the user's explicit "все против всех",
// a bot in one corner of a 4-quadrant map should march on its actual
// neighbour, not blindly cross the whole map to reach a farther rival
// while ignoring the one next door.
//
// A real bug found from an actual playtest report ("красный уничтожил не
// все постройки других ботов"): this used to target ONLY
// findWarehouseOwnedBy(opponent) -- once an opponent's Warehouse was
// destroyed, that opponent dropped out of consideration entirely, even
// with other real buildings (a FisherHut, an Armory, ...) still standing
// well away from where the Warehouse used to be. factionDefeated needs
// EVERY non-Road/Wall/Gate building gone, not just the Warehouse, so
// those stragglers -- never targeted again -- could survive forever,
// leaving that opponent permanently short of factionDefeated and the
// match unwinnable by elimination. Falls back to
// nearestRealBuildingOwnedBy (any of the opponent's own buildings, same
// exclusions factionDefeated itself uses) once the Warehouse is gone, so
// a fight keeps chasing down the last stragglers instead of stopping the
// moment the "home base" falls.
func (b *aiBrain) aiConsiderAttack(g *Game, f *faction) {
	idle := 0
	for _, s := range f.soldiers.Soldiers {
		if s.Alive() && !s.HasAttackOrder() && !s.HasFactionTarget() && len(s.RemainingPath()) == 0 {
			idle++
		}
	}
	if idle < b.difficulty.attackSquadSize() {
		return
	}
	own := findWarehouseOwnedBy(g.buildings, f.owner)
	if own == nil {
		return
	}
	var target *building.Building
	bestDist := -1
	for _, opponent := range g.opposingOwners(f.owner) {
		candidate := findWarehouseOwnedBy(g.buildings, opponent)
		if candidate == nil {
			candidate = g.nearestRealBuildingOwnedBy(opponent, own.X, own.Y)
		}
		if candidate == nil {
			continue // this opponent has nothing left worth attacking at all
		}
		dist := squaredDistance(own.X, own.Y, candidate.X, candidate.Y)
		if target == nil || dist < bestDist {
			target, bestDist = candidate, dist
		}
	}
	if target == nil {
		return
	}
	for _, s := range f.soldiers.Soldiers {
		if s.Alive() {
			s.MoveTo(g.grid, g.buildings, target.X, target.Y)
		}
	}
}

// nearestRealBuildingOwnedBy finds owner's own closest building to
// (x, y) -- the same Road/StoneWall/Gate/natural-resource exclusion
// factionDefeated itself uses, so this only ever returns a building whose
// destruction actually moves that faction closer to being defeated (a
// leftover Road tile, for instance, wouldn't -- pruneDestroyedBuildings
// never removes it anyway, see its own doc comment). Returns nil once
// nothing like that is left, meaning this owner is functionally already
// defeated (aiConsiderAttack's caller already checked findWarehouseOwnedBy
// came up empty too).
func (g *Game) nearestRealBuildingOwnedBy(owner, x, y int) *building.Building {
	var nearest *building.Building
	bestDist := -1
	for _, b := range g.buildings {
		if b.Owner != owner || isNaturalResourceKind(b.Kind) {
			continue
		}
		switch b.Kind {
		case building.Road, building.StoneWall, building.Gate:
			continue
		}
		dist := squaredDistance(x, y, b.X, b.Y)
		if nearest == nil || dist < bestDist {
			nearest, bestDist = b, dist
		}
	}
	return nearest
}

// squaredDistance is a plain Euclidean-squared distance -- enough to
// compare which of several candidates is nearer without ever needing an
// actual (and much more expensive) pathfinding distance just to rank
// targets.
func squaredDistance(x1, y1, x2, y2 int) int {
	dx, dy := x1-x2, y1-y2
	return dx*dx + dy*dy
}
