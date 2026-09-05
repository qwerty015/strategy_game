package main

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/combat"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/resource"
	"strategy_game/internal/sentry"
	"strategy_game/internal/ui"
	"strategy_game/internal/world"
)

// TestNewDuelGame_MapIsSymmetricAndIsthmusIsClear locks in the "1×1
// против ИИ" map shape the user explicitly asked for: a water divide
// splitting the map into two territories, joined by exactly one dry
// isthmus with no resources on it, and both starting Warehouses exactly
// equidistant from the map's own vertical center line.
func TestNewDuelGame_MapIsSymmetricAndIsthmusIsClear(t *testing.T) {
	g := newDuelGame(AINormal)

	playerWarehouse := findWarehouseOwnedBy(g.buildings, 0)
	aiWarehouse := findWarehouseOwnedBy(g.buildings, 1)
	if playerWarehouse == nil || aiWarehouse == nil {
		t.Fatal("both factions must start with an operational warehouse")
	}
	centerX := float64(g.grid.Width-1) / 2
	playerDist := centerX - float64(playerWarehouse.X)
	aiDist := float64(aiWarehouse.X) - centerX
	if playerDist < 0 {
		playerDist = -playerDist
	}
	if aiDist < 0 {
		aiDist = -aiDist
	}
	if diff := playerDist - aiDist; diff > 0.01 || diff < -0.01 {
		t.Fatalf("player warehouse is %.1f tiles from center, AI warehouse is %.1f -- want equal", playerDist, aiDist)
	}
	if playerWarehouse.X >= g.grid.Width/2 || aiWarehouse.X < g.grid.Width/2 {
		t.Fatalf("warehouses are not on opposite shores: player X=%d, AI X=%d, map width=%d", playerWarehouse.X, aiWarehouse.X, g.grid.Width)
	}

	// The isthmus itself: derive its Y-range straight from the generated
	// grid (whichever rows are dry within the water strip's own X band),
	// then confirm no natural resource landed anywhere in it and that it's
	// actually walkable land the whole way across. A real bug found from
	// an actual playtest report ("деревья... перекрывали проход по
	// перешейку"): this test's own name already promised "IsthmusIsClear"
	// but never once checked it -- growCenterWaterStrip computed the
	// isthmus bounds and the call site discarded them (`_, _ =`), so
	// seedTrees/seedThickets/seedOreDeposits/... were always free to place
	// right on top of the one dry crossing.
	stripCenterX := duelMapWidth / 2
	left := stripCenterX - duelWaterStripWidth/2
	right := duelMapWidth - left
	isthmusMinY, isthmusMaxY := -1, -1
	for y := 0; y < g.grid.Height; y++ {
		if g.grid.At(left, y).Terrain == world.Water {
			continue // a real water row of the strip -- not the crossing
		}
		if isthmusMinY < 0 {
			isthmusMinY = y
		}
		isthmusMaxY = y + 1
		for x := left; x < right; x++ {
			if g.grid.At(x, y).Terrain == world.Water {
				t.Fatalf("isthmus row y=%d is not dry all the way across at x=%d", y, x)
			}
		}
	}
	if isthmusMinY < 0 || isthmusMaxY-isthmusMinY != duelIsthmusWidth {
		t.Fatalf("found isthmus Y-range [%d,%d), want exactly %d rows (duelIsthmusWidth)", isthmusMinY, isthmusMaxY, duelIsthmusWidth)
	}
	for _, b := range g.buildings {
		// Both X (the strip's own band) AND Y (the dry crossing's rows,
		// not the strip's plain water rows) matter here -- a Fish
		// legitimately living in the strip's water, just outside the dry
		// crossing, must NOT trip this check the way a first draft of
		// this test once did (X alone isn't enough: Fish only ever spawns
		// on Water, which every non-isthmus row of this same band already
		// is by construction).
		if isNaturalResourceKind(b.Kind) && b.X >= left && b.X < right && b.Y >= isthmusMinY && b.Y < isthmusMaxY {
			t.Fatalf("kind %v sits at (%d,%d), inside the isthmus's dry crossing -- it must stay clear", b.Kind, b.X, b.Y)
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
		g := newDuelGame(AINormal)
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
	g := newDuelGame(AIHard)
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
	g := newDuelGame(AINormal)
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
	g := newDuelGame(AIHard)
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
	g := newDuelGame(AINormal)
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

	intruder := g.ai.logi.Hire()
	intruder.X, intruder.Y = tower.X+1, tower.Y // within WatchTowerRange
	present := func() bool {
		for _, s := range g.ai.logi.Serfs {
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
	g := newDuelGame(AIEasy)
	if g.duelResult != duelResultNone {
		t.Fatal("duelResult should start at duelResultNone")
	}

	// Wipe the AI's own faction directly -- this test is about the
	// result screen appearing and behaving correctly, not about how a
	// faction actually gets defeated in play (see
	// TestDuelSimulation_AIBuildsAndFactionsFight for that).
	for _, b := range g.buildings {
		if b.Owner == 1 && b.Kind != building.Road && b.Kind != building.StoneWall && b.Kind != building.Gate {
			b.HP = 0
		}
	}
	g.ai.logi.Serfs = nil
	g.ai.vills.Villagers = nil
	g.ai.jacks.Lumberjacks = nil
	g.ai.fishers.Fishermen = nil
	g.ai.quarry.Quarrymen = nil
	g.ai.builders.Builders = nil
	g.ai.miners.Miners = nil
	g.ai.sentries.Sentries = nil
	g.ai.soldiers.Soldiers = nil
	// Also empty the AI's own stockpile -- otherwise its still-fully-
	// intact brain simply places a fresh building this same tick (it
	// still has its starting resources), resurrecting "still has a
	// building" before checkDuelResult ever runs.
	g.ai.stock = resource.NewStockpile(stockpileCapacity)

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
	g := newDuelGame(AIHard)
	for i := 0; i < 5000; i++ {
		g.tickOnce()
	}
	if g.ai == nil || len(g.ownedBuildings(1)) <= 2 {
		t.Fatal("test setup: AI hasn't built anything yet after 5000 ticks")
	}
	wantDifficulty := g.ai.brain.difficulty
	wantBuildIndex := g.ai.brain.buildIndex
	wantAIPopCount := g.ai.pop.Count
	wantAIGold := g.ai.stock.Amount(resource.Gold)

	path := t.TempDir() + "/duel_save.json"
	if err := g.saveGame(path, "duel test"); err != nil {
		t.Fatalf("saveGame failed for a duel game: %v", err)
	}

	loaded := newDuelGame(AIHard) // any fresh Game to load into -- loadGame replaces everything relevant
	if err := loaded.loadGame(path); err != nil {
		t.Fatalf("loadGame failed: %v", err)
	}
	if loaded.ai == nil {
		t.Fatal("g.ai is nil after loading a duel save -- the AI faction was not reconstructed")
	}
	if loaded.ai.brain.difficulty != wantDifficulty {
		t.Fatalf("AI difficulty = %v, want %v", loaded.ai.brain.difficulty, wantDifficulty)
	}
	if loaded.ai.brain.buildIndex != wantBuildIndex {
		t.Fatalf("AI brain.buildIndex = %d, want %d", loaded.ai.brain.buildIndex, wantBuildIndex)
	}
	if loaded.ai.pop.Count != wantAIPopCount {
		t.Fatalf("AI population.Count = %d, want %d", loaded.ai.pop.Count, wantAIPopCount)
	}
	if got := loaded.ai.stock.Amount(resource.Gold); got != wantAIGold {
		t.Fatalf("AI gold = %d, want %d", got, wantAIGold)
	}
	totalAIUnits := len(loaded.ai.logi.Serfs) + len(loaded.ai.vills.Villagers) + len(loaded.ai.jacks.Lumberjacks) +
		len(loaded.ai.fishers.Fishermen) + len(loaded.ai.quarry.Quarrymen) + len(loaded.ai.builders.Builders) +
		len(loaded.ai.miners.Miners) + len(loaded.ai.sentries.Sentries) + len(loaded.ai.soldiers.Soldiers)
	if totalAIUnits == 0 {
		t.Fatal("the AI's entire unit roster is empty after loading -- restoreUnits never dispatched an Owner: 1 unit anywhere")
	}

	// The point of all of it: the reloaded AI must keep functioning, not
	// silently freeze -- run it forward and confirm its own tick doesn't
	// panic and its population isn't just draining to zero outright.
	for i := 0; i < 2000; i++ {
		loaded.tickOnce()
	}
	if loaded.ai.pop.Count == 0 {
		t.Fatal("the reloaded AI's population dropped to zero within 2000 ticks -- it isn't functioning after load")
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
	g := newDuelGame(AIHard) // fastest decisions/attacks -- fewest ticks needed to observe both behaviours
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
		if !combatDamageSeen && g.ai != nil {
			for _, s := range g.ai.soldiers.Soldiers {
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
