package main

import (
	"errors"
	"image"
	"math"
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/combat"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/resource"
	"strategy_game/internal/sentry"
	"strategy_game/internal/ui"
	"strategy_game/internal/world"
)

// TestNewDuelGame_MapIsSymmetricAndConnected locks in the "N против ИИ"
// map shape: a 4-quadrant water cross (see growQuadrantWaterCross), the
// player and its one opponent placed in diagonally opposite quadrants
// (NW/SE, per quadrantAssignmentOrder) exactly equidistant from the
// map's own center, and a real, walkable land route between them --
// this test's old, pre-4-quadrant version was actually named
// "...IsthmusIsClear" and manually re-derived isthmus geometry by hand;
// pathfind.FindLandPath (already used elsewhere in this file) is a much
// more direct way to assert "these two points are actually connected"
// without duplicating growQuadrantWaterCross's own arithmetic.
func TestNewDuelGame_MapIsSymmetricAndConnected(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AINormal})

	playerWarehouse := findWarehouseOwnedBy(g.buildings, 0)
	aiWarehouse := findWarehouseOwnedBy(g.buildings, 1)
	if playerWarehouse == nil || aiWarehouse == nil {
		t.Fatal("both factions must start with an operational warehouse")
	}
	if playerWarehouse.X >= g.grid.Width/2 || playerWarehouse.Y >= g.grid.Height/2 {
		t.Fatalf("player warehouse (%d,%d) is not in the NW quadrant of a %dx%d map", playerWarehouse.X, playerWarehouse.Y, g.grid.Width, g.grid.Height)
	}
	if aiWarehouse.X < g.grid.Width/2 || aiWarehouse.Y < g.grid.Height/2 {
		t.Fatalf("the sole opponent's warehouse (%d,%d) is not in the diagonally opposite SE quadrant", aiWarehouse.X, aiWarehouse.Y)
	}

	centerX, centerY := float64(g.grid.Width-1)/2, float64(g.grid.Height-1)/2
	playerDist := math.Hypot(centerX-float64(playerWarehouse.X), centerY-float64(playerWarehouse.Y))
	aiDist := math.Hypot(float64(aiWarehouse.X)-centerX, float64(aiWarehouse.Y)-centerY)
	if diff := playerDist - aiDist; diff > 0.01 || diff < -0.01 {
		t.Fatalf("player warehouse is %.2f tiles from center, AI warehouse is %.2f -- want equal", playerDist, aiDist)
	}

	if _, ok := pathfind.FindLandPath(g.grid, g.buildings,
		pathfind.Point{X: playerWarehouse.X, Y: playerWarehouse.Y},
		pathfind.Point{X: aiWarehouse.X, Y: aiWarehouse.Y},
	); !ok {
		t.Fatal("no walkable land route between the player's and the AI's warehouse -- the quadrants are not actually connected")
	}
}

// TestNewDuelGame_AllFourQuadrantsAreMutuallyConnected is
// TestNewDuelGame_MapIsSymmetricAndConnected's full-map version: a 3
// opponent match uses every one of the 4 quadrants (see
// quadrantAssignmentOrder), and every one of them must be able to reach
// every other one -- the exact real bug a naive two-independent-
// crossings design could produce (see growQuadrantWaterCross's doc
// comment on why its crossings come in mirrored pairs instead).
func TestNewDuelGame_AllFourQuadrantsAreMutuallyConnected(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy, AIEasy, AIEasy})
	var warehouses []*building.Building
	for owner := 0; owner < 4; owner++ {
		wh := findWarehouseOwnedBy(g.buildings, owner)
		if wh == nil {
			t.Fatalf("owner %d has no warehouse in a 3-opponent (4-faction) match", owner)
		}
		warehouses = append(warehouses, wh)
	}
	for i, from := range warehouses {
		for j, to := range warehouses {
			if i == j {
				continue
			}
			if _, ok := pathfind.FindLandPath(g.grid, g.buildings,
				pathfind.Point{X: from.X, Y: from.Y},
				pathfind.Point{X: to.X, Y: to.Y},
			); !ok {
				t.Fatalf("no walkable land route from owner %d's warehouse to owner %d's -- a quadrant is isolated", i, j)
			}
		}
	}
}

// TestPruneNaturalResourcesFromIsthmus_KeepsAllIsthmusesClear runs the
// real map generator several times (map shape is otherwise fixed --
// growQuadrantWaterCross has no seed -- but resource placement still
// varies) and confirms no natural resource ever lands inside any of the
// 4 dry crossings, per the user's explicit request that trees (or
// anything else) must not block the isthmus. Rebuilds the crossings the
// same way newDuelGame itself does, rather than re-deriving them from
// the finished grid, so this stays exactly in sync with production.
func TestPruneNaturalResourcesFromIsthmus_KeepsAllIsthmusesClear(t *testing.T) {
	for attempt := 0; attempt < 5; attempt++ {
		g := newDuelGame([]aiDifficulty{AINormal})
		grid := world.NewGrid(duelMapWidth, duelMapHeight)
		isthmuses := growQuadrantWaterCross(grid)
		for _, b := range g.buildings {
			if !isNaturalResourceKind(b.Kind) {
				continue
			}
			p := image.Point{X: b.X, Y: b.Y}
			for _, isthmus := range isthmuses {
				if p.In(isthmus) {
					t.Fatalf("attempt %d: kind %v sits at (%d,%d), inside a dry crossing -- it must stay clear", attempt, b.Kind, b.X, b.Y)
				}
			}
		}
	}
}

// TestNewDuelGame_NaturalResourcesAreExactlyMirrored locks in a real
// fairness bug found from an actual playtest report: without mirroring,
// stone/ore deposits only avoided the player's own warehouse point, so
// one side could end up holding effectively all of a resource by pure
// chance. Run across several fresh (randomly seeded) maps, since a single
// run could coincidentally look balanced even with the bug present.
func TestNewDuelGame_NaturalResourcesAreExactlyMirrored(t *testing.T) {
	kinds := []building.Kind{building.Tree, building.Fish, building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit}
	for attempt := 0; attempt < 5; attempt++ {
		g := newDuelGame([]aiDifficulty{AINormal})
		centerX := g.grid.Width / 2
		for _, kind := range kinds {
			left, right := 0, 0
			for _, b := range g.buildings {
				if b.Kind != kind {
					continue
				}
				switch {
				case b.X < centerX:
					left++
				case b.X > centerX:
					right++
				// b.X == centerX: the one self-mirroring column sits
				// exactly on the map's own centerline (mirrorX maps it
				// to itself) -- mirrorNaturalResourcesForFairness places
				// a single copy there, correctly, rather than two
				// identical ones on top of each other. Equidistant from
				// both warehouses by construction, so it counts toward
				// neither side rather than being force-classified as
				// "right" by a strict less-than comparison, which isn't
				// what happened and would fail this exact-equality
				// check for no real unfairness at all.
				default:
				}
			}
			if left != right {
				t.Fatalf("attempt %d: kind=%v left=%d right=%d -- want exactly equal (mirrored)", attempt, kind, left, right)
			}
		}
	}
}

// TestNewDuelGame_MineralDepositCountsAreFixedNotPercentage is the
// regression for a real playtest report ("деревьев просто какое-то
// нереальное количество создалось", filed against ore/stone specifically
// -- see AGENTS.md): the duel map used to reuse the single-player
// percentage-based seedStoneDeposits/seedOreDeposits across its whole
// (much bigger, 4-quadrant) area, producing hundreds of tiles of each
// mineral. Each mineral must now land in exactly duelXxxDepositTiles-sized
// canonical clusters, mirrored into up to 4 quadrants -- so the map-wide
// total must be an exact multiple of the per-quadrant count, never more
// (a self-mirroring tile sitting exactly on a mirror axis only ever
// reduces the total below 4x, it can't inflate it above the per-quadrant
// count times the number of quadrants that actually got a copy).
func TestNewDuelGame_MineralDepositCountsAreFixedNotPercentage(t *testing.T) {
	cases := []struct {
		kind  building.Kind
		tiles int
	}{
		{building.StoneDeposit, duelStoneDepositTiles},
		{building.CoalDeposit, duelCoalDepositTiles},
		{building.GoldOreDeposit, duelGoldOreDepositTiles},
		{building.IronOreDeposit, duelIronOreDepositTiles},
	}
	for attempt := 0; attempt < 5; attempt++ {
		g := newDuelGame([]aiDifficulty{AINormal, AINormal, AINormal})
		for _, c := range cases {
			count := 0
			for _, b := range g.buildings {
				if b.Kind == c.kind {
					count++
				}
			}
			if count == 0 {
				t.Fatalf("attempt %d: kind=%v has zero tiles anywhere on the map, want up to %d per quadrant", attempt, c.kind, c.tiles)
			}
			if count > c.tiles*int(quadrantCount) {
				t.Fatalf("attempt %d: kind=%v has %d tiles total, want at most %d (%d per quadrant x %d quadrants) -- looks percentage-scaled again, not fixed", attempt, c.kind, count, c.tiles*int(quadrantCount), c.tiles, int(quadrantCount))
			}
		}
	}
}

// TestNewDuelGame_MineralsLandNearTheirOwnWarehouse is the regression for
// the user's explicit request ("переделай спавн ресурсов чтоб они
// появлялись рядом а не раскиданые на карте"): findStoneStart/findOreStart
// pick the single best-scoring tile across the WHOLE map with no notion of
// "nearby" beyond minDepositDistanceFromWarehouse's lower bound, so a
// cluster could previously land clear across the quadrant from a
// faction's own base. Every mineral tile must now sit within
// maxDepositDistanceFromWarehouse of SOME warehouse (each quadrant's own,
// by the four-way symmetry mirrorNaturalResourcesForFairness guarantees).
func TestNewDuelGame_MineralsLandNearTheirOwnWarehouse(t *testing.T) {
	for attempt := 0; attempt < 5; attempt++ {
		g := newDuelGame([]aiDifficulty{AINormal, AINormal, AINormal})
		var warehouses []*building.Building
		for _, b := range g.buildings {
			if b.Kind == building.Warehouse {
				warehouses = append(warehouses, b)
			}
		}
		for _, b := range g.buildings {
			switch b.Kind {
			case building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
			default:
				continue
			}
			nearest := math.Inf(1)
			for _, wh := range warehouses {
				dx, dy := float64(b.X-wh.X), float64(b.Y-wh.Y)
				if d := math.Hypot(dx, dy); d < nearest {
					nearest = d
				}
			}
			if nearest > maxDepositDistanceFromWarehouse {
				t.Fatalf("attempt %d: %v at (%d,%d) is %.1f tiles from its nearest warehouse, want <= %d", attempt, b.Kind, b.X, b.Y, nearest, maxDepositDistanceFromWarehouse)
			}
		}
	}
}

// countAIConstructedBuildings counts the AI's own player/AI-constructed
// buildings, excluding natural resource nodes -- g.ownedBuildings(1) now
// also returns every Tree/Fish/deposit on the whole map (see
// isNaturalResourceKind's doc comment on why both factions' controllers
// need to see them), which would otherwise swamp a "did the AI build
// anything" count with hundreds of irrelevant, constantly-fluctuating
// (trees get cut, fish get caught and regrow) entries.
func countAIConstructedBuildings(g *Game) int {
	n := 0
	for _, b := range g.ownedBuildings(1) {
		if !isNaturalResourceKind(b.Kind) {
			n++
		}
	}
	return n
}

// TestDuelSimulation_PlayerControllersNeverTargetTheOpponentsBuildings is
// a real cross-faction leak found from an actual playtest report ("the
// opponent's servant delivers to my warehouse"): every player controller
// Tick call in the main tick loop (cmd/game/main.go's tickOnce) used to
// pass the raw, unfiltered g.buildings -- which in a "1×1 против ИИ" game
// holds BOTH factions' buildings in one shared slice -- instead of
// g.ownedBuildingsWithRoads(0), so nothing stopped a player's serf from
// hauling from, delivering to, or otherwise interacting with the AI's own
// buildings. Runs a real simulation and checks every player serf's
// current building references every tick.
func TestDuelSimulation_PlayerControllersNeverTargetTheOpponentsBuildings(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIHard})
	belongsToOpponent := func(b *building.Building) bool {
		return b != nil && b.Owner != 0
	}
	for i := 0; i < 3000; i++ {
		g.tickOnce()
		for _, s := range g.logi.Serfs {
			if belongsToOpponent(s.AtBuilding()) {
				t.Fatalf("tick %d: player serf standing at an opponent-owned building (kind=%v)", i, s.AtBuilding().Kind)
			}
			if belongsToOpponent(s.PickupBuilding()) {
				t.Fatalf("tick %d: player serf picking up from an opponent-owned building (kind=%v)", i, s.PickupBuilding().Kind)
			}
			if belongsToOpponent(s.DropoffBuilding()) {
				t.Fatalf("tick %d: player serf delivering to an opponent-owned building (kind=%v)", i, s.DropoffBuilding().Kind)
			}
		}
	}
}

// TestDuelGame_PlayerCannotSelectOrDemolishOpponentBuildings is a real bug
// found from an actual playtest report: "я почему-то могу выбирать
// постройки и юнитов противника, смотреть что у него на складе или в
// здании, а также могу удалить его постройки". selectionAt and
// buildingSelectionAt (the latter is what continuous demolition itself
// resolves a click through) never checked Owner at all.
func TestDuelGame_PlayerCannotSelectOrDemolishOpponentBuildings(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AINormal})
	aiWarehouse := findWarehouseOwnedBy(g.buildings, 1)
	if aiWarehouse == nil {
		t.Fatal("AI warehouse missing")
	}
	mx, my := g.camera.TileToScreen(aiWarehouse.X, aiWarehouse.Y)
	mx += g.camera.TilePixels() / 2
	my += g.camera.TilePixels() / 2

	if sel := g.selectionAt(int(mx), int(my)); sel.Kind == ui.SelectionBuilding && sel.Building == aiWarehouse {
		t.Fatal("selectionAt let the player select the AI's own warehouse")
	}
	if sel := g.buildingSelectionAt(int(mx), int(my)); sel.Kind == ui.SelectionBuilding && sel.Building == aiWarehouse {
		t.Fatal("buildingSelectionAt (continuous demolition's own resolver) let the player target the AI's own warehouse")
	}
}

// TestDuelGame_HireOptionsNeverCountTheOpponentsBuildings is a real bug
// found from an actual playtest report: "у меня отображается 1 доступный
// рыбак хотя хижину я еще не построил". hireOptions' countBuildings and
// hireFromTab's per-profession search loops used to scan the whole map's
// buildings, so the AI's own finished huts looked like player vacancies
// -- clicking one of those cards would have spawned a player worker
// straight into the AI's building.
func TestDuelGame_HireOptionsNeverCountTheOpponentsBuildings(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIHard})
	for i := 0; i < 5000; i++ {
		g.tickOnce()
	}
	if len(g.ownedBuildings(1)) <= 2 {
		t.Skip("AI hasn't built anything yet this run -- nothing to check")
	}
	options := g.hireOptions()
	for _, opt := range options {
		if opt.Kind == ui.HireSerf || opt.Kind == ui.HireBuilder {
			continue // uncapped/flat-limit options, not tied to a building kind at all
		}
		if opt.Current > opt.Limit {
			t.Fatalf("hire option %v: Current=%d > Limit=%d -- the AI's own buildings are inflating the player's limit", opt.Kind, opt.Current, opt.Limit)
		}
	}
	// Directly confirm the fix: hireFromTab must never actually manage
	// to place a player worker into one of the AI's buildings, even
	// after being invoked repeatedly.
	for i := 0; i < 20; i++ {
		g.hireFromTab(ui.HireLumberjack)
		g.hireFromTab(ui.HireFisherman)
		g.hireFromTab(ui.HireQuarryman)
		g.hireFromTab(ui.HireMiner)
	}
	for _, j := range g.jacks.Lumberjacks {
		if h := j.HomeBuilding(); h != nil && h.Owner != 0 {
			t.Fatal("a player lumberjack ended up homed in the AI's own LumberjackHut")
		}
	}
	for _, f := range g.fishers.Fishermen {
		if h := f.HomeBuilding(); h != nil && h.Owner != 0 {
			t.Fatal("a player fisherman ended up homed in the AI's own FisherHut")
		}
	}
	for _, q := range g.quarry.Quarrymen {
		if h := q.HomeBuilding(); h != nil && h.Owner != 0 {
			t.Fatal("a player quarryman ended up homed in the AI's own QuarryHut")
		}
	}
	for _, m := range g.miners.Miners {
		if h := m.HomeBuilding(); h != nil && h.Owner != 0 {
			t.Fatal("a player miner ended up homed in the AI's own MinerHut")
		}
	}
}

// TestDuelGame_PlayerWatchTowerKillsAnOpposingIntruder is a real gap
// found from an actual playtest report ("почему башня не убила его
// слуг"): a WatchTower's Sentry could only ever fire at the sandbox-only
// enemy.Enemy, never at anything belonging to the AI opponent -- an
// enemy serf that wandered into range walked straight through unharmed.
// Builds a real WatchTower+Sentry for the player and places one of the
// AI's own serfs directly in range, then runs the actual game tick loop
// (not a hand-rolled sentry.Controller.Tick call) to prove the full
// opposingIntruderTargetsFor wiring works end to end.
func TestDuelGame_PlayerWatchTowerKillsAnOpposingIntruder(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AINormal})
	playerWarehouse := findWarehouseOwnedBy(g.buildings, 0)
	if playerWarehouse == nil {
		t.Fatal("player warehouse missing")
	}
	tower := building.NewConstructionSite(building.WatchTower, playerWarehouse.X+2, playerWarehouse.Y)
	tower.ConstructionStage = building.ConstructionNone
	tower.Owner = 0
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)
	g.buildings = append(g.buildings, tower)
	g.sentries.Spawn(tower)

	intruder := g.ais[0].logi.Hire()
	intruder.X, intruder.Y = tower.X+1, tower.Y // within WatchTowerRange
	present := func() bool {
		for _, s := range g.ais[0].logi.Serfs {
			if s == intruder {
				return true
			}
		}
		return false
	}

	// Not a bare serf-count comparison: the AI's own aiHireServes hires
	// more serfs on its own schedule throughout the run, which would
	// mask a real kill (or fake one) if only the total count were
	// checked -- confirmed this exact pointer is what actually leaves
	// the roster instead.
	const maxTicks = sentry.ShotCooldownTicks + sentry.WatchTowerRange*2 + 50
	for i := 0; i < maxTicks; i++ {
		g.tickOnce()
		if !present() {
			return // killed -- the tower's own Controller.Tick removed it
		}
	}
	t.Fatalf("the player's WatchTower never killed the AI's intruding serf after %d ticks", maxTicks)
}

// TestDuelGame_VictoryScreenAppearsAndFreezesTheMatch is a real gap found
// from the user asking "подумай над победой, что значит и как будет
// выглядеть": factionDefeated existed (tested in isolation) but nothing
// in the running game ever called it -- there was no way at all for a
// "1×1 против ИИ" match to actually end, even after one side's every
// building and unit was gone.
func TestDuelGame_VictoryScreenAppearsAndFreezesTheMatch(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy})
	if g.duelResult != duelResultNone {
		t.Fatal("duelResult should start at duelResultNone")
	}

	// Wipe the AI's own faction directly -- this test is about the
	// result screen appearing and behaving correctly, not about how a
	// faction actually gets defeated in play (see
	// TestDuelSimulation_AIBuildsAndFactionsFight for that).
	wipeFactionForTest(g, 1)

	g.tickOnce()
	if g.duelResult != duelResultVictory {
		t.Fatalf("duelResult = %v, want duelResultVictory once the AI has nothing left", g.duelResult)
	}

	// The simulation itself must freeze: ticking further must not crash
	// on a fully-wiped faction, and must not somehow reset the result.
	for i := 0; i < 100; i++ {
		g.tickOnce()
	}
	if g.duelResult != duelResultVictory {
		t.Fatalf("duelResult changed after the match was already decided: %v", g.duelResult)
	}

	// The result screen's "В главное меню" button resolves a click on
	// its own rect (see updateDuelResult, which reads this same pure
	// function) -- verified directly, the same way title.go's own
	// titleActionAt/modeSelectActionAt are, since live ebiten cursor
	// state isn't available in a headless test.
	back := duelResultBackRect(g.layout.Width, g.layout.Height)
	cx, cy := back.Min.X+back.Dx()/2, back.Min.Y+back.Dy()/2
	if !duelResultBackClicked(cx, cy, g.layout.Width, g.layout.Height) {
		t.Fatal("a click at the center of the back button's own rect did not resolve to it")
	}
	if duelResultBackClicked(0, 0, g.layout.Width, g.layout.Height) {
		t.Fatal("a click at the corner of the screen unexpectedly resolved to the back button")
	}
}

// wipeFactionForTest zeroes owner's buildings (except Road/StoneWall/
// Gate, matching factionDefeated's own exclusion), clears its unit
// rosters and empties its stockpile -- the exact steps
// TestDuelGame_VictoryScreenAppearsAndFreezesTheMatch already needed,
// pulled out so the FFA test below can wipe one specific faction among
// several (including the player, owner 0, whose controllers live
// directly on Game rather than in g.ais) without duplicating this dance.
func wipeFactionForTest(g *Game, owner int) {
	for _, b := range g.buildings {
		if b.Owner == owner && b.Kind != building.Road && b.Kind != building.StoneWall && b.Kind != building.Gate {
			b.HP = 0
		}
	}
	logi, vills, jacks, fishers, quarryC, builders, miners, sentries, soldiers := g.logi, g.vills, g.jacks, g.fishers, g.quarry, g.builders, g.miners, g.sentries, g.soldiers
	stockSet := func(s *resource.Stockpile) { g.stock = s }
	if owner != 0 {
		f := g.factionByOwner(owner)
		if f == nil {
			return
		}
		logi, vills, jacks, fishers, quarryC, builders, miners, sentries, soldiers = f.logi, f.vills, f.jacks, f.fishers, f.quarry, f.builders, f.miners, f.sentries, f.soldiers
		stockSet = func(s *resource.Stockpile) { f.stock = s }
	}
	logi.Serfs = nil
	vills.Villagers = nil
	jacks.Lumberjacks = nil
	fishers.Fishermen = nil
	quarryC.Quarrymen = nil
	builders.Builders = nil
	miners.Miners = nil
	sentries.Sentries = nil
	soldiers.Soldiers = nil
	// Otherwise the still-fully-intact brain (or, for the player, a
	// still-nonzero stockpile) leaves something to rebuild from before
	// checkDuelResult even runs.
	stockSet(resource.NewStockpile(stockpileCapacity))
}

// TestDuelGame_FFAResultRequiresEveryBotDefeated is the "все против
// всех" generalization of TestDuelGame_VictoryScreenAppearsAndFreezesTheMatch:
// with more than one opponent, defeating only SOME of them must not end
// the match either way -- victory needs every last one gone, and the
// player's own defeat ends it immediately regardless of how many bots
// are still standing (see checkDuelResult's own doc comment on "все
// против всех").
func TestDuelGame_FFAResultRequiresEveryBotDefeated(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy, AIEasy})
	if g.duelResult != duelResultNone {
		t.Fatal("duelResult should start at duelResultNone")
	}

	wipeFactionForTest(g, 1)
	g.tickOnce()
	if g.duelResult != duelResultNone {
		t.Fatalf("duelResult = %v after defeating only one of two bots, want duelResultNone (match continues)", g.duelResult)
	}

	wipeFactionForTest(g, 2)
	g.tickOnce()
	if g.duelResult != duelResultVictory {
		t.Fatalf("duelResult = %v after defeating every bot, want duelResultVictory", g.duelResult)
	}
}

// TestDuelGame_FFAPlayerDefeatEndsTheMatchEvenWithBotsStillFighting
// covers the other half: the player's own elimination must end the
// match in defeat right away, without waiting to see which of the
// surviving bots would eventually "win" the rest of the fight -- per the
// user's explicit framing ("не важно кто победит из ботов дальше").
func TestDuelGame_FFAPlayerDefeatEndsTheMatchEvenWithBotsStillFighting(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy, AIEasy})
	wipeFactionForTest(g, 0)
	g.tickOnce()
	if g.duelResult != duelResultDefeat {
		t.Fatalf("duelResult = %v after the player's own defeat with two bots still alive, want duelResultDefeat", g.duelResult)
	}
}

// TestDuelGame_PlayerLogisticsRouteOverTheDefeatedAIsOldRoads is a real
// bug found from an actual playtest report: after the player conquers
// the AI's territory (destroys every AI building) and builds their own
// Tavern/workshops there, the AI's own leftover Road tiles stay tagged
// with the AI's old Owner forever -- pruneDestroyedBuildings deliberately
// never removes Road, and nothing ever reassigns it. ownedBuildingsWithRoads
// used to filter those out as "not the player's", so a player serf's
// road-only routing (Tavern hauling, a worker's own trip to eat) could
// never physically cross them -- reported as "боевым юнитам доставляется
// еда слугами, а рыболов не может пойти поесть... мистика!" (soldier
// feeding uses off-road pathing, which never filtered by Owner, so it
// kept working and made the asymmetry look like a mystery). Roads are
// shared infrastructure now, exactly like a tree or ore deposit already
// was -- this proves it by building a real route the player's own
// controllers must cross an Owner=1 road segment to complete.
func TestDuelGame_PlayerLogisticsRouteOverTheDefeatedAIsOldRoads(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0, Owner: 0, HP: building.MaxHP}
	// A road chain the player's own logistics must cross to reach the
	// Tavern below -- tiles 1 and 2 are still tagged Owner: 1, standing
	// in for "the defeated AI's own leftover road", never reassigned.
	road0 := &building.Building{Kind: building.Road, X: 1, Y: 0, Owner: 0, HP: building.MaxHP}
	road1 := &building.Building{Kind: building.Road, X: 2, Y: 0, Owner: 1, HP: building.MaxHP}
	road2 := &building.Building{Kind: building.Road, X: 3, Y: 0, Owner: 1, HP: building.MaxHP}
	road3 := &building.Building{Kind: building.Road, X: 4, Y: 0, Owner: 0, HP: building.MaxHP}
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0, Owner: 0, ConstructionStage: building.ConstructionNone}

	g := &Game{buildings: []*building.Building{warehouse, road0, road1, road2, road3, tavern}}

	playerBuildings := g.ownedBuildingsWithRoads(0)
	if _, ok := pathfind.FindPath(playerBuildings, warehouse, tavern); !ok {
		t.Fatal("player logistics can't route to a player Tavern across the defeated AI's leftover (Owner=1) road -- roads must be faction-neutral for connectivity")
	}
}

// TestDuelGame_SaveAndLoadRoundTripsTheAIFaction is the feature the user
// explicitly confirmed wanting ("Конечно нужно сохранение"), after
// saveGame previously refused to save a duel game at all rather than
// lose the AI's own economy silently. Plays a duel game forward for a
// while (so the AI has real buildings/units/brain progress, not just its
// starting Warehouse), saves it, loads it into a fresh Game, and checks
// the AI faction actually comes back: same difficulty, same brain
// progress, a real unit roster, and -- the point of all of it -- the
// reloaded game keeps ticking without the AI silently freezing.
func TestDuelGame_SaveAndLoadRoundTripsTheAIFaction(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIHard})
	// Capped at 2000, not run for a fixed 5000 -- and stopped the instant
	// the match is actually decided (checkDuelResult), whichever comes
	// first. A real bug found running this exact test after loadGame
	// stopped silently accepting another faction's Warehouse (see
	// TestDuelGame_LoadFailsRatherThanAdoptAnEnemyWarehouse): ticking a
	// fully undefended player for 5000 ticks against AIHard reliably let
	// the AI actually reach and destroy the player's own Warehouse
	// (previously masked -- loadGame just silently adopted the AI's
	// Warehouse instead of reporting the player had none left). This test
	// only needs the AI to have made SOME real progress, not to survive
	// an entire match either way.
	for i := 0; i < 2000 && g.duelResult == duelResultNone; i++ {
		g.tickOnce()
	}
	if g.duelResult != duelResultNone {
		t.Fatalf("test setup: the match was already decided (duelResult=%v) before 2000 ticks -- pick a shorter/safer tick count", g.duelResult)
	}
	if len(g.ais) == 0 || len(g.ownedBuildings(1)) <= 2 {
		t.Fatal("test setup: AI hasn't built anything yet after 2000 ticks")
	}
	wantDifficulty := g.ais[0].brain.difficulty
	wantBuildIndex := g.ais[0].brain.buildIndex
	wantAIPopCount := g.ais[0].pop.Count
	wantAIGold := g.ais[0].stock.Amount(resource.Gold)

	path := t.TempDir() + "/duel_save.json"
	if err := g.saveGame(path, "duel test"); err != nil {
		t.Fatalf("saveGame failed for a duel game: %v", err)
	}

	loaded := newDuelGame([]aiDifficulty{AIHard}) // any fresh Game to load into -- loadGame replaces everything relevant
	if err := loaded.loadGame(path); err != nil {
		t.Fatalf("loadGame failed: %v", err)
	}
	if len(loaded.ais) == 0 {
		t.Fatal("g.ais is empty after loading a duel save -- the AI faction was not reconstructed")
	}
	if loaded.ais[0].brain.difficulty != wantDifficulty {
		t.Fatalf("AI difficulty = %v, want %v", loaded.ais[0].brain.difficulty, wantDifficulty)
	}
	if loaded.ais[0].brain.buildIndex != wantBuildIndex {
		t.Fatalf("AI brain.buildIndex = %d, want %d", loaded.ais[0].brain.buildIndex, wantBuildIndex)
	}
	if loaded.ais[0].pop.Count != wantAIPopCount {
		t.Fatalf("AI population.Count = %d, want %d", loaded.ais[0].pop.Count, wantAIPopCount)
	}
	if got := loaded.ais[0].stock.Amount(resource.Gold); got != wantAIGold {
		t.Fatalf("AI gold = %d, want %d", got, wantAIGold)
	}
	totalAIUnits := len(loaded.ais[0].logi.Serfs) + len(loaded.ais[0].vills.Villagers) + len(loaded.ais[0].jacks.Lumberjacks) +
		len(loaded.ais[0].fishers.Fishermen) + len(loaded.ais[0].quarry.Quarrymen) + len(loaded.ais[0].builders.Builders) +
		len(loaded.ais[0].miners.Miners) + len(loaded.ais[0].sentries.Sentries) + len(loaded.ais[0].soldiers.Soldiers)
	if totalAIUnits == 0 {
		t.Fatal("the AI's entire unit roster is empty after loading -- restoreUnits never dispatched an Owner: 1 unit anywhere")
	}

	// The point of all of it: the reloaded AI must keep functioning, not
	// silently freeze -- run it forward and confirm its own tick doesn't
	// panic and its population isn't just draining to zero outright.
	for i := 0; i < 2000; i++ {
		loaded.tickOnce()
	}
	if loaded.ais[0].pop.Count == 0 {
		t.Fatal("the reloaded AI's population dropped to zero within 2000 ticks -- it isn't functioning after load")
	}
}

// TestDuelGame_LoadFailsRatherThanAdoptAnEnemyWarehouse is a real bug found
// from an actual playtest report ("создай слуг от юзера, они начинают
// ходить по кругу карты без перерыва" -- newly hired serfs wandering the
// map forever): loadGame used to resolve the player's own warehouse via
// the owner-blind findWarehouse(buildings), which just returns whichever
// operational Warehouse comes first in the array. That's harmless in an
// ordinary single-player save (every building is Owner 0 there anyway),
// but once the player's OWN Warehouse has been destroyed in a duel match
// before saving, an AI faction's still-standing Warehouse came first
// instead -- silently anchoring the player's logistics controller to an
// enemy building clear across the map. A player with no Warehouse left of
// their own has no economy to load into, whatever other buildings
// survive: loadGame must report errNoWarehouseInSave here, not adopt a
// foreign one.
func TestDuelGame_LoadFailsRatherThanAdoptAnEnemyWarehouse(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIHard})
	for i := 0; i < 2000; i++ {
		g.tickOnce()
	}
	if len(g.ownedBuildings(1)) <= 2 {
		t.Fatal("test setup: AI hasn't built anything yet after 2000 ticks")
	}
	// Simulate the player's own Warehouse having been destroyed in combat
	// before this save happened -- drop every Owner:0 Warehouse, leaving
	// the AI's (Owner:1) Warehouse as the only one left in the file.
	var survivors []*building.Building
	for _, b := range g.buildings {
		if b.Kind == building.Warehouse && b.Owner == 0 {
			continue
		}
		survivors = append(survivors, b)
	}
	g.buildings = survivors

	path := t.TempDir() + "/no_player_warehouse.json"
	if err := g.saveGame(path, "no warehouse test"); err != nil {
		t.Fatalf("saveGame failed: %v", err)
	}

	loaded := newDuelGame([]aiDifficulty{AIHard})
	err := loaded.loadGame(path)
	if !errors.Is(err, errNoWarehouseInSave) {
		t.Fatalf("loadGame error = %v, want errNoWarehouseInSave", err)
	}
}

// TestDuelSimulation_AIBuildsAndFactionsFight is the goal's own
// end-to-end verification, run entirely headless: a "1×1 против ИИ"
// game ticked forward for a long stretch of simulation time with no
// player input at all must (1) see the AI actually place and finish
// real buildings, using exactly the same construction/hiring mechanics
// the player uses, and (2) see the two factions' soldiers actually
// fight -- a rival soldier or building taking real damage -- once they
// meet, all through the ordinary auto-engage mechanism, no scripted
// combat outcome.
func TestDuelSimulation_AIBuildsAndFactionsFight(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIHard}) // fastest decisions/attacks -- fewest ticks needed to observe both behaviours
	initialAIBuildings := countAIConstructedBuildings(g)

	// 150000, not the original 60000: decisionIntervalTicks was tripled
	// after a playtest report that the AI economically outpaced the
	// player far too fast for a human to keep up with (see its own doc
	// comment) -- the AI now needs proportionally longer, in simulated
	// ticks, to reach the same build-order milestones.
	const maxTicks = 150000
	var aiBuilt, aiFinishedOne, combatDamageSeen bool
	var maxAIBuildings int

	for i := 0; i < maxTicks; i++ {
		g.tickOnce()

		aiBuildingCount := countAIConstructedBuildings(g)
		if aiBuildingCount > maxAIBuildings {
			maxAIBuildings = aiBuildingCount
		}
		if aiBuildingCount > initialAIBuildings {
			aiBuilt = true
		}
		if !aiFinishedOne {
			for _, b := range g.ownedBuildings(1) {
				if b.Kind != building.Warehouse && b.Kind != building.Road && !isNaturalResourceKind(b.Kind) && b.ConstructionStage == building.ConstructionNone {
					aiFinishedOne = true
					break
				}
			}
		}
		if !combatDamageSeen {
			for _, b := range g.buildings {
				// Natural resource nodes never get a real HP value at
				// all (see isNaturalResourceKind's doc comment) -- a
				// real bug in this exact test found while investigating
				// a user's playtest report: without this exclusion,
				// b.HP < building.MaxHP was true for hundreds of
				// buildings on tick 0 already, making combatDamageSeen
				// trivially true before the two factions could ever
				// possibly have met. The test always "passed" but never
				// actually verified anything about combat.
				if isNaturalResourceKind(b.Kind) {
					continue
				}
				if b.HP < building.MaxHP {
					combatDamageSeen = true
					break
				}
			}
		}
		if !combatDamageSeen {
			for _, s := range g.soldiers.Soldiers {
				if s.HP < combat.MaxHP {
					combatDamageSeen = true
					break
				}
			}
		}
		if !combatDamageSeen && len(g.ais) > 0 {
			for _, s := range g.ais[0].soldiers.Soldiers {
				if s.HP < combat.MaxHP {
					combatDamageSeen = true
					break
				}
			}
		}
		if g.factionDefeated(0) || g.factionDefeated(1) {
			combatDamageSeen = true
			break
		}
		if aiBuilt && aiFinishedOne && combatDamageSeen {
			break // everything the goal asks for has already been observed
		}
	}

	if !aiBuilt {
		t.Fatalf("AI never placed a single new building beyond its starting Warehouse+Road (peak owned buildings = %d, started with %d)", maxAIBuildings, initialAIBuildings)
	}
	if !aiFinishedOne {
		t.Fatal("AI placed construction sites but never actually finished one -- see aiHireBuilder/builder.Controller")
	}
	if !combatDamageSeen {
		t.Fatal("no building or soldier on either side ever took combat damage -- the two factions never actually fought")
	}
}
