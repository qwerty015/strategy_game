package main

import (
	"image"

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

// garrisonMinimum is how many idle soldiers aiConsiderAttack always
// holds back at home, never sent along with an outgoing attack --
// per the user's report of a completely undefended base ("я напал на
// зелёного с юга, его юнит был на севере... к базе не подошёл"):
// aiConsiderAttack used to march literally every living soldier the
// instant attackSquadSize was reached, leaving nothing behind to react
// to a threat arriving from a different direction. Higher difficulty
// keeps a slightly larger home guard, the same direction as its faster
// decisionIntervalTicks/smaller attackSquadSize -- a "harder" AI is
// meant to feel more competent end to end, not just more reckless.
func (d aiDifficulty) garrisonMinimum() int {
	switch d {
	case AIEasy:
		return 2
	case AIHard:
		return 4
	default:
		return 3
	}
}

// defenseAlertRadius is how far (Chebyshev tiles) an opposing soldier
// can be from one of this faction's own real buildings before
// aiDefendBase notices and reacts -- deliberately well beyond
// soldier.FactionEngageRange (2), which only ever fires once an enemy
// is already adjacent to whichever specific unit happens to be
// standing there. Kept the same across every difficulty on purpose:
// this is about the AI noticing a threat at a fair, human-comparable
// distance (a player scouting a base can see this many tiles ahead
// too), not about rewarding a higher difficulty with superhuman
// detection range -- attackSquadSize/garrisonMinimum/
// decisionIntervalTicks are where difficulty already lives.
func (d aiDifficulty) defenseAlertRadius() int {
	return 7
}

// expansionIntervalTicks paces how many decision ticks aiBuildNext waits
// between two consecutive placements once it has moved on to the
// repeating aiExpansionOrder (see that list's own doc comment) -- unlike
// the original fixed aiBuildOrder, which places as fast as materials
// allow, ongoing expansion deliberately paces itself so a long match
// doesn't carpet an entire quadrant in duplicate buildings within the
// first few thousand ticks. Hard expands faster than Easy, matching the
// same difficulty axis as everywhere else in this type.
func (d aiDifficulty) expansionIntervalTicks() int {
	switch d {
	case AIEasy:
		return 12
	case AIHard:
		return 4
	default:
		return 8
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

// aiBuildOrderMilitary is aiBuildOrder's alternate profile -- Barracks/
// Armory/a second WatchTower pulled to the very front (right after the
// Tavern/FisherHut a starving-to-death AI genuinely cannot skip, see
// aiBuildOrder's own doc comment), everything else pushed later. Per the
// user's own report ("базы все как под копирку"): every AI faction ran
// the exact same aiBuildOrder in the exact same sequence, so three bots
// on one map were only ever distinguishable by their chosen difficulty.
// aiBuildVariantFor picks one of these two lists deterministically per
// faction -- see its own doc comment for why deterministic, not random.
var aiBuildOrderMilitary = []building.Kind{
	building.Tavern,
	building.FisherHut,
	building.WatchTower,
	building.Barracks,
	building.LumberjackHut,
	building.CarpentryWorkshop,
	building.QuarryHut,
	building.MinerHut,
	building.Smeltery,
	building.Armory,
	building.PigFarm,
	building.Farm,
}

// aiBuildVariants indexes every curated aiBuildOrder profile;
// aiBuildVariantFor's return value is always a valid index into this.
var aiBuildVariants = [][]building.Kind{aiBuildOrder, aiBuildOrderMilitary}

// aiBuildVariantFor picks owner's build-order profile deterministically
// -- not math/rand without a shared seed (the same pitfall already
// documented for this codebase's regrowth functions, see AGENTS.md):
// replaying the same save/seed must keep producing the same base, and a
// networked match (see this repo's own planning notes) will eventually
// need every peer to derive the identical choice independently, which a
// pure function of owner already guarantees for free.
func aiBuildVariantFor(owner int) int {
	return owner % len(aiBuildVariants)
}

// aiExpansionOrder is what aiBuildNext falls back to, cyclically, once
// buildIndex has walked past the end of its faction's chosen
// aiBuildVariants entry -- per the user's own report ("не развивается"):
// aiBuildNext's own loop used to be bounded by len(aiBuildOrder), so
// once the AI's initial dozen buildings were up it permanently stopped
// building anything else for the rest of the match, no matter how much
// material and gold kept piling up unused. A second Farm/FisherHut/
// LumberjackHut/CarpentryWorkshop/WatchTower/Barracks round keeps
// growing both the economy (more gatherers/processors) and the army's
// ceiling (a second Barracks can train alongside the first) instead of
// flatlining -- paced by aiDifficulty.expansionIntervalTicks rather than
// placed as fast as aiBuildOrder's own one-time list is.
var aiExpansionOrder = []building.Kind{
	building.Farm,
	building.FisherHut,
	building.LumberjackHut,
	building.CarpentryWorkshop,
	building.WatchTower,
	building.Barracks,
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
	// aiBuildVariants[buildVariant][buildIndex] -- see aiBuildNext's doc
	// comment on maxBuildAttemptsPerEntry for the real bug this guards
	// against.
	buildAttempts int

	// buildVariant selects which of aiBuildVariants this faction follows
	// -- see aiBuildVariantFor's own doc comment. Deliberately NOT
	// persisted to the save: it's a pure function of owner, so a reload
	// recomputes the exact same value newAIBrain already used to build
	// this faction's town in the first place.
	buildVariant int

	// expansionCooldown paces aiExpandEconomy once buildIndex has walked
	// past the curated aiBuildVariants entry -- see that function's own
	// doc comment. Also deliberately not persisted (harmless to reset on
	// reload, see the same function).
	expansionCooldown int
}

func newAIBrain(owner int, difficulty aiDifficulty) *aiBrain {
	return &aiBrain{owner: owner, difficulty: difficulty, buildVariant: aiBuildVariantFor(owner)}
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
	g.aiBuildDefenses(f, grid)
	g.aiQueueArmoryProduction(f)
	g.aiHireSoldiers(f)
	g.aiDefendBase(f)
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
	order := aiBuildVariants[b.buildVariant]
	for b.buildIndex < len(order) {
		kind := order[b.buildIndex]
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
	b.aiExpandEconomy(g, f, grid, order)
}

// aiExpandEconomy is aiBuildNext's fallback once buildIndex has walked
// past the end of this faction's curated aiBuildVariants entry -- see
// aiExpansionOrder's own doc comment ("не развивается"). Paced by
// expansionCooldown/expansionIntervalTicks (decision ticks between
// attempts -- this only ever runs once per decision tick to begin with,
// same as everything else in aiBrain.tick) instead of placed as fast as
// materials allow, so a long match doesn't carpet a quadrant in
// duplicates within its first few thousand ticks. A failed placement
// just retries the same kind next window rather than advancing --
// aiExpansionOrder cycles forever anyway, so there's no "give up and
// move on" the way maxBuildAttemptsPerEntry provides for the one-shot
// list above; a temporarily unplaceable kind (no room yet, not enough
// material yet) simply gets tried again a little later.
func (b *aiBrain) aiExpandEconomy(g *Game, f *faction, grid *world.Grid, order []building.Kind) {
	if b.expansionCooldown > 0 {
		b.expansionCooldown--
		return
	}
	b.expansionCooldown = b.difficulty.expansionIntervalTicks()
	kind := aiExpansionOrder[(b.buildIndex-len(order))%len(aiExpansionOrder)]
	if g.aiPlaceBuilding(f, grid, kind) {
		b.buildIndex++
	}
}

// aiPlacementSearchRadius bounds how far from its own Warehouse the AI
// will look for room to place its next building -- generous enough to
// find room even in a cramped starting area without searching forever.
const aiPlacementSearchRadius = 25

// aiPlaceBuilding finds a legally placeable tile for kind within
// aiPlacementSearchRadius of the AI's Warehouse (expanding ring search)
// -- a thin wrapper over aiPlaceBuildingNear, anchored at the Warehouse
// itself, for every caller that doesn't care where specifically a
// building lands (the original, and still by far the most common, case).
// See aiPlaceBuildingNear's own doc comment for the anchored search
// itself, added so aiBuildDefenses can anchor a defensive WatchTower at
// an isthmus crossing instead.
func (g *Game) aiPlaceBuilding(f *faction, grid *world.Grid, kind building.Kind) bool {
	warehouse := findWarehouseOwnedBy(g.buildings, f.owner)
	if warehouse == nil {
		return false
	}
	return g.aiPlaceBuildingNear(f, grid, kind, warehouse.X, warehouse.Y, aiPlacementSearchRadius)
}

// aiPlaceBuildingNear finds a legally placeable tile for kind within
// maxRadius of (anchorX, anchorY) (expanding ring search), requires a
// real off-road walkable route back to the AI's own Warehouse access
// point (pathfind.FindLandPath -- the same free-roam pathing package
// builder/soldier already use -- the connectivity TARGET is always the
// faction's own Warehouse, regardless of where the search itself is
// anchored), and if one exists: places the construction site, then
// paves every tile of that route as a Road so the site is immediately
// reachable by the AI's own serfs/builders, the same way a player
// connects a new building by hand.
func (g *Game) aiPlaceBuildingNear(f *faction, grid *world.Grid, kind building.Kind, anchorX, anchorY, maxRadius int) bool {
	warehouse := findWarehouseOwnedBy(g.buildings, f.owner)
	if warehouse == nil {
		return false
	}
	bt := building.Types[kind]
	if f.stock.Amount(resource.Plank) < bt.PlankCost || f.stock.Amount(resource.StoneBlock) < bt.StoneCost {
		return false // not enough material banked yet -- try again next decision
	}
	warehouseAccess := warehouse.AccessPoint()
	for radius := 2; radius <= maxRadius; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if absInt(dx) != radius && absInt(dy) != radius {
					continue // only this ring -- smaller ones already tried
				}
				x, y := anchorX+dx, anchorY+dy
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
//
// .Owner = f.owner on each Spawn call is a real, severe playtest bug fix
// found from an actual siege ("как прошёл противник... я ещё и выключил
// автооткрывание"): soldier.Controller.Spawn/Restore never set the new
// Soldier's Owner field at all, leaving it at Go's zero value (0) for
// EVERY soldier ever created, AI-owned ones included. Soldier.Owner is
// what pathfind.FindLandPathForFaction uses to decide which faction's
// gates a soldier may cross (building.GatePassableTo) -- with every AI
// soldier silently misidentifying itself as Owner 0 (the player), it
// walked through the player's own Auto/Open gates as if it were the
// player's own unit, while (the inverse of the same bug) being wrongly
// blocked by its own faction's gates. Explicitly setting it here at the
// two spots (this file) and in soldier.Restore's call site (cmd/game's
// restoreUnitStateInto) closes every place a soldier is actually
// created, without changing soldier.Controller's own exported API (or
// the many existing tests that call Spawn/Restore directly for the
// single-player/free-map case, where Owner 0 is already correct).
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
			f.soldiers.Spawn(soldier.Swordsman, x, y).Owner = f.owner
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
			f.soldiers.Spawn(soldier.Archer, x, y).Owner = f.owner
		}
	}
}

// duelIsthmusIndicesFor names which two of Game.duelIsthmuses (in
// growQuadrantWaterCross's own fixed return order -- [0]=north/NW-NE,
// [1]=south/SW-SE, [2]=west/NW-SW, [3]=east/NE-SE) actually border
// quadrant q. Every quadrant borders exactly its two orthogonal
// neighbours (the 4-cycle NW-NE-SE-SW-NW growQuadrantWaterCross's own
// doc comment describes), never the diagonally opposite one.
func duelIsthmusIndicesFor(q quadrant) [2]int {
	switch q {
	case quadrantNW:
		return [2]int{0, 2} // north + west
	case quadrantNE:
		return [2]int{0, 3} // north + east
	case quadrantSW:
		return [2]int{1, 2} // south + west
	default: // quadrantSE
		return [2]int{1, 3} // south + east
	}
}

// aiIsthmusGuardPoint returns a point just inside q's own territory,
// offset from one of q's two bordering crossing rectangles toward q's
// own side -- where aiBuildDefenses anchors a defensive WatchTower's
// placement search. A north/south crossing (wide, short) borders its
// west quadrant (NW/SW) on the low-X side and its east quadrant (NE/SE)
// on the high-X side; a west/east crossing (narrow, tall) borders its
// north quadrant (NW/NE) above and south quadrant (SW/SE) below.
func aiIsthmusGuardPoint(q quadrant, rect image.Rectangle) (x, y int) {
	const guardOffset = 3
	midX := rect.Min.X + rect.Dx()/2
	midY := rect.Min.Y + rect.Dy()/2
	if rect.Dx() >= rect.Dy() { // north/south crossing
		if q == quadrantNW || q == quadrantSW {
			return rect.Min.X - guardOffset, midY
		}
		return rect.Max.X + guardOffset, midY
	}
	// west/east crossing
	if q == quadrantNW || q == quadrantNE {
		return midX, rect.Min.Y - guardOffset
	}
	return midX, rect.Max.Y + guardOffset
}

// aiBuildDefenses fortifies f's own two bordering isthmus crossings with
// a straight StoneWall line, one Gate, and a nearby WatchTower -- per
// the user's own report that the AI "не выстраивает защиту":
// aiBuildVariants never contained StoneWall/Gate at all (only
// aiPlaceBuilding's generic warehouse-anchored ring search ever placed a
// single, undirected WatchTower). A duel map's crossing geometry is
// fixed and already known from map generation (Game.duelIsthmuses,
// growQuadrantWaterCross's own returned rectangles) -- the same two
// chokepoints a player already fortifies by hand, see release notes
// ("южный перешеек весь перекрыт башнями и имеет глухую стену, без
// ворот") -- so this is a deterministic straight line, not a search.
// A no-op outside a duel map (g.duelIsthmuses unset) or once an owner
// index falls outside quadrantAssignmentOrder's own range.
func (g *Game) aiBuildDefenses(f *faction, grid *world.Grid) {
	if len(g.duelIsthmuses) != int(quadrantCount) || f.owner < 0 || f.owner >= int(quadrantCount) {
		return
	}
	q := quadrantAssignmentOrder[f.owner]
	for _, idx := range duelIsthmusIndicesFor(q) {
		rect := g.duelIsthmuses[idx]
		g.aiFortifyIsthmus(f, rect)
		gx, gy := aiIsthmusGuardPoint(q, rect)
		if !g.aiHasBuildingNear(f, building.WatchTower, gx, gy, aiIsthmusGuardSearchRadius) {
			g.aiPlaceBuildingNear(f, grid, building.WatchTower, gx, gy, aiIsthmusGuardSearchRadius)
		}
	}
}

// aiIsthmusGuardSearchRadius bounds both aiHasBuildingNear's "already
// got one here" check and aiPlaceBuildingNear's own search when placing
// a defensive WatchTower next to a crossing -- small on purpose (unlike
// aiPlacementSearchRadius): a tower guarding a specific chokepoint that
// wandered many tiles away from it because the immediate area was full
// wouldn't actually be guarding it any more.
const aiIsthmusGuardSearchRadius = 8

// aiHasBuildingNear reports whether f already owns a finished-or-being-
// built kind within radius (Chebyshev) of (x, y) -- guards aiBuildDefenses
// against placing a second WatchTower next to a crossing it already
// fortified on an earlier decision tick (aiPlaceBuilding's own
// aiHasBuilding check is map-wide, which would wrongly consider a
// faction's very first, unrelated WatchTower from aiBuildOrder as
// "already handled" for every crossing).
func (g *Game) aiHasBuildingNear(f *faction, kind building.Kind, x, y, radius int) bool {
	for _, b := range g.buildings {
		if b == nil || b.Owner != f.owner || b.Kind != kind {
			continue
		}
		if absInt(b.X-x) <= radius && absInt(b.Y-y) <= radius {
			return true
		}
	}
	return false
}

// aiFortifyIsthmus places a straight StoneWall line across rect (one of
// f's two bordering crossings) and, once that line finishes, upgrades
// its middle segment to a Gate -- see aiPromoteIsthmusGate. Skips
// entirely once ANY wall or gate (any owner -- a crossing is shared
// between exactly two quadrants, and only needs fortifying once; whoever
// gets there first wins the tile race, the same as two players trying to
// build on the same free tile would) already sits on the line.
func (g *Game) aiFortifyIsthmus(f *faction, rect image.Rectangle) {
	horizontal := rect.Dx() >= rect.Dy()
	var from, to building.Point
	if horizontal {
		midY := rect.Min.Y + rect.Dy()/2
		from = building.Point{X: rect.Min.X, Y: midY}
		to = building.Point{X: rect.Max.X - 1, Y: midY}
	} else {
		midX := rect.Min.X + rect.Dx()/2
		from = building.Point{X: midX, Y: rect.Min.Y}
		to = building.Point{X: midX, Y: rect.Max.Y - 1}
	}
	path := wallPath(from, to, horizontal)
	if len(path) == 0 {
		return
	}
	if g.wallPieceAt(path[0].X, path[0].Y) {
		g.aiPromoteIsthmusGate(f, path)
		return
	}
	for _, p := range path {
		if !building.CanPlace(g.grid, g.buildings, building.StoneWall, p.X, p.Y) {
			return // isthmuses are kept clear of resources by construction, so
			// this should be rare -- something else (another faction's
			// building, this same faction's own earlier expansion) got there
			// first; simply don't force it.
		}
	}
	if !building.CanCreateWallTopology(g.buildings, path) {
		return
	}
	// No reserveConstructionMaterials call here, matching
	// aiPlaceBuilding[Near]'s own convention, not commitWallPath's
	// (player-only) one -- an AI faction's builder/logistics controllers
	// already deliver to ANY of their own owned, unfinished buildings
	// generically, with no separate "reservation" step required.
	for _, p := range path {
		site := building.NewConstructionSite(building.StoneWall, p.X, p.Y)
		site.Owner = f.owner
		g.buildings = append(g.buildings, site)
	}
	g.invalidateConnectionCache()
}

// aiPromoteIsthmusGate upgrades the finished middle StoneWall segment of
// path to a Gate, mirroring placeGateAt's own field-setting exactly
// (cmd/game/main.go) -- kept as a separate, AI-owned copy rather than a
// shared call, since placeGateAt reports failure through g.statusMsg (a
// player-facing field this brain has no business touching) and doesn't
// check ownership (irrelevant for a single player, essential here: this
// must only ever touch a wall THIS faction just built, never a rival's
// sharing the same crossing). A no-op while the wall segment is still
// under construction -- retried automatically on the next decision tick.
func (g *Game) aiPromoteIsthmusGate(f *faction, path []building.Point) {
	mid := path[len(path)/2]
	for _, b := range g.buildings {
		if b == nil || b.X != mid.X || b.Y != mid.Y {
			continue
		}
		if b.Kind != building.StoneWall || b.Owner != f.owner || b.ConstructionStage != building.ConstructionNone {
			return
		}
		axis, valid := building.WallAxisAt(g.buildings, mid.X, mid.Y)
		if !valid {
			return
		}
		b.Kind = building.Gate
		b.ConstructionStage = building.ConstructionFoundation
		b.ProgressTicks = 0
		b.InputBuffer = nil
		b.OutputBuffer = nil
		b.GateOpen = false
		b.GateAuto = true
		b.GateAxis = axis
		b.GateReplacesWall = true
		g.invalidateConnectionCache()
		return
	}
}

// aiDefendBase reacts to an opposing soldier within
// difficulty.defenseAlertRadius of any of f's own real buildings by
// peeling off idle garrison soldiers to intercept -- per the user's own
// report ("я напал на зелёного с юга, его юнит был на севере... к базе
// не подошёл"): aiConsiderAttack is the only place this brain ever gives
// a soldier an order, and it only ever marches out, never reacts to a
// threat near home -- the sole existing combat trigger for an idle
// soldier is soldier.FactionEngageRange (2 tiles), which only fires once
// an enemy is already adjacent to whichever specific unit happens to be
// standing there. Picks the single most urgent intrusion (closest to any
// owned building, not just the first one found) and sends garrisonMinimum
// idle soldiers after it with AttackFactionSoldierOrder -- the same
// cross-faction order API the player's own right-click already uses.
// Runs every decision tick, same cadence as everything else in
// aiBrain.tick (see decisionIntervalTicks' own doc comment on why
// defense deliberately isn't faster than that).
func (g *Game) aiDefendBase(f *faction) {
	threats := g.opposingSoldiersFor(f.soldiers)
	if len(threats) == 0 {
		return
	}
	radius := f.brain.difficulty.defenseAlertRadius()
	var target *soldier.Soldier
	bestDist := -1
	for _, b := range g.ownedBuildings(f.owner) {
		if isNaturalResourceKind(b.Kind) {
			continue
		}
		switch b.Kind {
		case building.Road, building.StoneWall, building.Gate:
			continue
		}
		threat, dist := nearestAliveSoldierWithin(threats, b.X, b.Y, radius)
		if threat == nil {
			continue
		}
		if target == nil || dist < bestDist {
			target, bestDist = threat, dist
		}
	}
	if target == nil {
		return
	}
	limit := f.brain.difficulty.garrisonMinimum()
	sent := 0
	for _, s := range f.soldiers.Soldiers {
		if sent >= limit {
			return
		}
		if !s.Alive() || s.HasFactionTarget() || len(s.RemainingPath()) != 0 {
			continue
		}
		s.AttackFactionSoldierOrder(target)
		sent++
	}
}

// nearestAliveSoldierWithin finds the living soldier in soldiers closest
// to (x, y), no farther than radius (Chebyshev) away -- shared by
// aiDefendBase's own per-building scan.
func nearestAliveSoldierWithin(soldiers []*soldier.Soldier, x, y, radius int) (*soldier.Soldier, int) {
	var nearest *soldier.Soldier
	best := -1
	for _, s := range soldiers {
		if s == nil || !s.Alive() {
			continue
		}
		dx, dy := s.X-x, s.Y-y
		if absInt(dx) > radius || absInt(dy) > radius {
			continue
		}
		d := dx*dx + dy*dy
		if nearest == nil || d < best {
			nearest, best = s, d
		}
	}
	return nearest, best
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
// aiDefenderScoutRadius is how far around a prospective target
// aiConsiderAttack counts living opposing soldiers as "defenders" before
// deciding whether attacking is actually worth it -- see the force
// comparison below. Wide enough to catch a garrison stationed at the
// target's own Warehouse/Barracks, not just a soldier standing exactly
// on top of it.
const aiDefenderScoutRadius = 6

func (b *aiBrain) aiConsiderAttack(g *Game, f *faction) {
	idle := 0
	for _, s := range f.soldiers.Soldiers {
		if s.Alive() && !s.HasFactionTarget() && len(s.RemainingPath()) == 0 {
			idle++
		}
	}
	// garrisonMinimum idle soldiers never count toward the attack --
	// see aiDefendBase's own doc comment ("я напал на зелёного с юга,
	// его юнит был на севере... к базе не подошёл"): a faction that
	// commits every last soldier to an attack has nothing left to
	// answer aiDefendBase's own call if a different threat shows up
	// while they're gone.
	attackers := idle - b.difficulty.garrisonMinimum()
	if attackers < b.difficulty.attackSquadSize() {
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
	// A real "decide from the current situation, not just a fixed
	// threshold" check, per the user's own request ("чтобы он принимал
	// решения исходя из текущей обстановки, не просто робот"): don't
	// commit to a march that's already a clearly losing fight. Counting
	// only nearby living defenders (not the target faction's whole
	// army, which may be off attacking someone else entirely) keeps
	// this cheap and keyed to what actually matters for THIS attack.
	defenders := 0
	for _, s := range g.opposingSoldiersFor(f.soldiers) {
		if s != nil && s.Alive() && squaredDistance(s.X, s.Y, target.X, target.Y) <= aiDefenderScoutRadius*aiDefenderScoutRadius {
			defenders++
		}
	}
	if attackers < defenders {
		return // wait for a bigger army rather than feed it in piecemeal
	}
	// Exactly garrisonMinimum idle soldiers stay home; everyone already
	// mid-march toward a previous target gets redirected to the fresh
	// one, same as the original design always did. A soldier that
	// already has a live faction target (HasFactionTarget) is skipped
	// entirely, not redirected -- most importantly, one aiDefendBase
	// just sent after a threat THIS SAME decision tick (aiDefendBase
	// runs right before this in aiBrain.tick): overriding that order
	// here would silently cancel the very defense this function's own
	// garrison-holdback exists to make possible.
	held := 0
	for _, s := range f.soldiers.Soldiers {
		if !s.Alive() || s.HasFactionTarget() {
			continue
		}
		if len(s.RemainingPath()) == 0 && held < b.difficulty.garrisonMinimum() {
			held++
			continue
		}
		s.MoveTo(g.grid, g.buildings, target.X, target.Y)
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
	return nearestRealBuildingIn(g.buildings, owner, x, y)
}

// nearestRealBuildingIn is nearestRealBuildingOwnedBy's underlying scan,
// taking an explicit buildings slice -- needed by loadGame, which must
// run this same search on its own local, not-yet-assigned-to-g.buildings
// slice while deciding whether the player's own warehouse-less save can
// still be reconstructed as a straggler (see errNoWarehouseInSave's own
// call site).
func nearestRealBuildingIn(buildings []*building.Building, owner, x, y int) *building.Building {
	var nearest *building.Building
	bestDist := -1
	for _, b := range buildings {
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
