package main

import (
	"image"
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
	"strategy_game/internal/soldier"
)

// TestNearestRealBuildingOwnedBy_SkipsNeutralAndInfrastructureKinds is
// part of the fix for a real bug found from an actual playtest report
// ("красный уничтожил не все постройки других ботов"): aiConsiderAttack
// used to stop considering an opponent entirely once its Warehouse was
// destroyed, even with other real buildings still standing. This checks
// the fallback target-finder alone: it must skip natural resources (owner
// is meaningless for them) and Road/StoneWall/Gate (factionDefeated
// itself never counts these either), landing on the one real building.
func TestNearestRealBuildingOwnedBy_SkipsNeutralAndInfrastructureKinds(t *testing.T) {
	g := &Game{buildings: []*building.Building{
		{Kind: building.Tree, Owner: 0, X: 1, Y: 0},
		{Kind: building.Road, Owner: 2, X: 2, Y: 0},
		{Kind: building.StoneWall, Owner: 2, X: 3, Y: 0},
		{Kind: building.Gate, Owner: 2, X: 4, Y: 0},
		{Kind: building.FisherHut, Owner: 2, X: 5, Y: 0},
	}}
	got := g.nearestRealBuildingOwnedBy(2, 0, 0)
	if got == nil || got.Kind != building.FisherHut {
		t.Fatalf("nearestRealBuildingOwnedBy = %+v, want the FisherHut", got)
	}
}

// TestNearestRealBuildingOwnedBy_PicksTheClosestOne confirms the "nearest"
// half of the name -- aiConsiderAttack's whole point is marching on the
// closest threat/straggler, not a random or first-found one.
func TestNearestRealBuildingOwnedBy_PicksTheClosestOne(t *testing.T) {
	g := &Game{buildings: []*building.Building{
		{Kind: building.FisherHut, Owner: 1, X: 100, Y: 0},
		{Kind: building.Armory, Owner: 1, X: 5, Y: 0},
	}}
	got := g.nearestRealBuildingOwnedBy(1, 0, 0)
	if got == nil || got.Kind != building.Armory {
		t.Fatalf("nearestRealBuildingOwnedBy = %+v, want the closer Armory", got)
	}
}

// TestNearestRealBuildingOwnedBy_NilWhenNothingRealLeft confirms this
// correctly reports "truly nothing left" (the caller then treats that
// opponent as not worth attacking, same as an already-defeated one)
// rather than latching onto a Road tile that pruneDestroyedBuildings
// would never even remove.
func TestNearestRealBuildingOwnedBy_NilWhenNothingRealLeft(t *testing.T) {
	g := &Game{buildings: []*building.Building{
		{Kind: building.Road, Owner: 1, X: 1, Y: 0},
		{Kind: building.StoneWall, Owner: 1, X: 2, Y: 0},
		{Kind: building.Tree, Owner: 0, X: 3, Y: 0},
	}}
	if got := g.nearestRealBuildingOwnedBy(1, 0, 0); got != nil {
		t.Fatalf("nearestRealBuildingOwnedBy = %+v, want nil (nothing real left)", got)
	}
}

// TestAIConsiderAttack_KeepsHuntingAfterTheWarehouseFalls is the
// end-to-end version: once an opponent's Warehouse is gone but a real
// building of theirs survives elsewhere, an idle attack squad must still
// be sent after it -- the exact scenario the user's real playtest report
// described (a rival's Warehouse destroyed, other buildings left
// standing forever because nothing ever attacked them again).
func TestAIConsiderAttack_KeepsHuntingAfterTheWarehouseFalls(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIHard, AIHard})
	f := g.ais[0]        // owner 1, AIHard -> attackSquadSize()==3, garrisonMinimum()==4
	opponent := g.ais[1] // owner 2

	// garrisonMinimum() idle soldiers stay home by design (see
	// aiDefendBase's own doc comment) -- enough to satisfy that AND
	// still clear attackSquadSize with soldiers actually free to march.
	for i := 0; i < f.brain.difficulty.garrisonMinimum()+f.brain.difficulty.attackSquadSize(); i++ {
		f.soldiers.Spawn(soldier.Archer, f.logi.Warehouse.X, f.logi.Warehouse.Y)
	}

	// A straggler building of the opponent's, placed right next to their
	// own (soon to be destroyed) warehouse -- same quadrant, so reachable
	// the same way the warehouse itself was. Tries a handful of nearby
	// offsets since the exact tiles right beside a freshly generated
	// warehouse can be uneven ground, water, or already occupied.
	oh := opponent.logi.Warehouse
	var straggler *building.Building
	for _, d := range [][2]int{{2, 0}, {-2, 0}, {0, 2}, {0, -2}, {3, 1}, {-3, -1}, {1, 3}, {-1, -3}} {
		x, y := oh.X+d[0], oh.Y+d[1]
		if building.CanPlace(g.grid, g.buildings, building.Armory, x, y) {
			straggler = &building.Building{Kind: building.Armory, X: x, Y: y, Owner: opponent.owner, ConstructionStage: building.ConstructionNone, HP: building.MaxHP}
			break
		}
	}
	if straggler == nil {
		t.Fatal("test setup: no nearby offset was placeable for the straggler building")
	}
	g.buildings = append(g.buildings, straggler)

	oh.HP = 0
	g.pruneDestroyedBuildings()
	if findWarehouseOwnedBy(g.buildings, opponent.owner) != nil {
		t.Fatal("test setup: opponent's warehouse should be gone")
	}

	// aiConsiderAttack directly, not the full f.brain.tick -- tick also
	// runs aiBuildDefenses now, which (correctly) walls off this same
	// faction's own isthmus crossings the instant it places them, even
	// before a Gate exists to let its own soldiers back out. These
	// tests are specifically about aiConsiderAttack's own targeting/
	// garrison logic, not an interaction with a wall built the very
	// same tick.
	f.brain.aiConsiderAttack(g, f)

	for _, s := range f.soldiers.Soldiers {
		path := s.RemainingPath()
		if len(path) == 0 {
			continue
		}
		last := path[len(path)-1]
		if last.X == straggler.X && last.Y == straggler.Y {
			return // found it -- at least one soldier is headed there
		}
	}
	t.Fatal("no soldier was routed toward the opponent's last remaining building once its warehouse was destroyed")
}

// TestAIConsiderAttack_WithholdsGarrisonBeforeMarching is the regression
// test for the user's own report of a completely undefended base ("я
// напал на зелёного с юга, его юнит был на севере... к базе не
// подошёл"): aiConsiderAttack used to march literally every living
// soldier the instant attackSquadSize was reached, leaving nothing
// behind. One soldier short of garrisonMinimum+attackSquadSize must not
// attack at all.
func TestAIConsiderAttack_WithholdsGarrisonBeforeMarching(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIHard, AIHard})
	f := g.ais[0]
	n := f.brain.difficulty.garrisonMinimum() + f.brain.difficulty.attackSquadSize() - 1
	for i := 0; i < n; i++ {
		f.soldiers.Spawn(soldier.Archer, f.logi.Warehouse.X, f.logi.Warehouse.Y)
	}

	// aiConsiderAttack directly, not the full f.brain.tick -- tick also
	// runs aiBuildDefenses now, which (correctly) walls off this same
	// faction's own isthmus crossings the instant it places them, even
	// before a Gate exists to let its own soldiers back out. These
	// tests are specifically about aiConsiderAttack's own targeting/
	// garrison logic, not an interaction with a wall built the very
	// same tick.
	f.brain.aiConsiderAttack(g, f)

	for _, s := range f.soldiers.Soldiers {
		if len(s.RemainingPath()) != 0 || s.HasFactionTarget() {
			t.Fatal("should not have attacked at all -- one soldier short once the garrison is withheld")
		}
	}
}

// TestAIConsiderAttack_MarchesOnlyTheSurplusAboveGarrison confirms the
// exact count: with garrisonMinimum+attackSquadSize idle soldiers on
// hand, exactly attackSquadSize of them march, not every last one.
func TestAIConsiderAttack_MarchesOnlyTheSurplusAboveGarrison(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIHard, AIHard})
	f := g.ais[0]
	garrison := f.brain.difficulty.garrisonMinimum()
	squad := f.brain.difficulty.attackSquadSize()
	for i := 0; i < garrison+squad; i++ {
		f.soldiers.Spawn(soldier.Archer, f.logi.Warehouse.X, f.logi.Warehouse.Y)
	}

	// aiConsiderAttack directly, not the full f.brain.tick -- tick also
	// runs aiBuildDefenses now, which (correctly) walls off this same
	// faction's own isthmus crossings the instant it places them, even
	// before a Gate exists to let its own soldiers back out. These
	// tests are specifically about aiConsiderAttack's own targeting/
	// garrison logic, not an interaction with a wall built the very
	// same tick.
	f.brain.aiConsiderAttack(g, f)

	marching := 0
	for _, s := range f.soldiers.Soldiers {
		if len(s.RemainingPath()) != 0 {
			marching++
		}
	}
	if marching != squad {
		t.Fatalf("marching = %d, want exactly attackSquadSize (%d) -- garrisonMinimum (%d) should stay home", marching, squad, garrison)
	}
}

// TestAiDefendBase_SendsAGarrisonSoldierAtANearbyThreat is the
// end-to-end regression test for the user's own report: an idle
// defender within defenseAlertRadius of an intruding opposing soldier
// must actually be ordered to engage it.
func TestAiDefendBase_SendsAGarrisonSoldierAtANearbyThreat(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy, AIEasy})
	f := g.ais[0]
	enemy := g.ais[1]

	wh := f.logi.Warehouse
	threat := enemy.soldiers.Spawn(soldier.Swordsman, wh.X+3, wh.Y)
	threat.Owner = enemy.owner
	defender := f.soldiers.Spawn(soldier.Archer, wh.X, wh.Y)
	defender.Owner = f.owner

	g.aiDefendBase(f)

	if !defender.HasFactionTarget() {
		t.Fatal("aiDefendBase should have ordered the idle defender to engage the nearby threat")
	}
}

// TestAiDefendBase_IgnoresAThreatBeyondTheAlertRadius confirms the
// radius is actually enforced, not "any threat anywhere on the map".
func TestAiDefendBase_IgnoresAThreatBeyondTheAlertRadius(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy, AIEasy})
	f := g.ais[0]
	enemy := g.ais[1]

	wh := f.logi.Warehouse
	radius := f.brain.difficulty.defenseAlertRadius()
	threat := enemy.soldiers.Spawn(soldier.Swordsman, wh.X+radius+20, wh.Y)
	threat.Owner = enemy.owner
	defender := f.soldiers.Spawn(soldier.Archer, wh.X, wh.Y)
	defender.Owner = f.owner

	g.aiDefendBase(f)

	if defender.HasFactionTarget() {
		t.Fatal("a threat well beyond defenseAlertRadius should not have triggered a defense order")
	}
}

// TestAiBuildDefenses_FortifiesBothOfItsOwnCrossings is the regression
// test for the user's own report that the AI "не выстраивает защиту":
// a fresh faction, given enough banked material, must build a StoneWall
// line across both of its own bordering isthmus crossings.
func TestAiBuildDefenses_FortifiesBothOfItsOwnCrossings(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy})
	f := g.ais[0]
	f.stock.Add(resource.Plank, 500)
	f.stock.Add(resource.StoneBlock, 500)

	g.aiBuildDefenses(f, g.grid)

	q := quadrantAssignmentOrder[f.owner]
	for _, idx := range duelIsthmusIndicesFor(q) {
		rect := g.duelIsthmuses[idx]
		found := false
		for _, b := range g.buildings {
			if b.Owner == f.owner && b.Kind == building.StoneWall && (image.Point{X: b.X, Y: b.Y}).In(rect) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("crossing %v was not fortified with a wall", rect)
		}
	}
}

// TestAiFortifyIsthmus_WallSpansTheCrossingsShortAxis is the regression
// test for a real playtest report ("боты строят стену вертикально там
// где нужно горизонтально и наоборот"): the wall must fully span the
// crossing's SHORT axis (duelIsthmusWidth's own dry-row/column count) at
// one fixed point along the LONG axis (duelWaterStripWidth, the
// direction someone actually walks through it) -- a wall running the
// other way instead sits PARALLEL to the direction of travel and blocks
// nothing at all, since every other row/column of the short axis stays
// wide open right beside it. TestAiBuildDefenses_FortifiesBothOfItsOwnCrossings
// above only checked "a wall exists somewhere in the rect", which the
// original, backwards orientation also satisfied -- too weak to have
// caught this.
func TestAiFortifyIsthmus_WallSpansTheCrossingsShortAxis(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy})
	f := g.ais[0]
	q := quadrantAssignmentOrder[f.owner]
	for _, idx := range duelIsthmusIndicesFor(q) {
		rect := g.duelIsthmuses[idx]
		g.aiFortifyIsthmus(f, rect)

		xs, ys := map[int]bool{}, map[int]bool{}
		for _, b := range g.buildings {
			if b.Owner == f.owner && b.Kind == building.StoneWall && (image.Point{X: b.X, Y: b.Y}).In(rect) {
				xs[b.X] = true
				ys[b.Y] = true
			}
		}
		wantSpan, gotSpan, axis := rect.Dy(), len(ys), "Y"
		if rect.Dx() < rect.Dy() {
			wantSpan, gotSpan, axis = rect.Dx(), len(xs), "X"
		}
		if gotSpan != wantSpan {
			t.Fatalf("crossing %v: wall covers %d distinct %s positions, want %d (the full short axis) -- it must block every row/column of the crossing, not run parallel to the direction of travel through it", rect, gotSpan, axis, wantSpan)
		}
	}
}

// TestAiFortifyIsthmus_PromotesTheMiddleSegmentToAGateOnceFinished
// confirms the two-step wall-then-gate flow: a fresh line of StoneWall
// segments must never contain a Gate yet, and once every segment is
// (simulated as) finished, a second call upgrades exactly the middle one
// -- so the faction's own soldiers can still use this crossing while
// every other faction is blocked, same as GatePassableTo already
// guarantees for a player-built gate.
func TestAiFortifyIsthmus_PromotesTheMiddleSegmentToAGateOnceFinished(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy})
	f := g.ais[0]
	q := quadrantAssignmentOrder[f.owner]
	rect := g.duelIsthmuses[duelIsthmusIndicesFor(q)[0]]

	g.aiFortifyIsthmus(f, rect)
	for _, b := range g.buildings {
		if b.Owner == f.owner && b.Kind == building.Gate {
			t.Fatal("a Gate should not appear before any wall segment has finished construction")
		}
	}

	for _, b := range g.buildings {
		if b.Owner == f.owner && b.Kind == building.StoneWall && (image.Point{X: b.X, Y: b.Y}).In(rect) {
			b.ConstructionStage = building.ConstructionNone
		}
	}
	g.aiFortifyIsthmus(f, rect)

	gates := 0
	for _, b := range g.buildings {
		if b.Owner == f.owner && b.Kind == building.Gate {
			gates++
		}
	}
	if gates != 1 {
		t.Fatalf("gates = %d, want exactly 1 once the wall line is finished", gates)
	}
}

// TestAiBuildVariantFor_IsDeterministicAndVariesByOwner is the
// regression test for "базы все как под копирку": the same owner must
// always get the same profile (replaying a save/seed must not change
// what got built), but different bot owners must not all collapse onto
// the same one.
func TestAiBuildVariantFor_IsDeterministicAndVariesByOwner(t *testing.T) {
	first := aiBuildVariantFor(1)
	second := aiBuildVariantFor(1) // a fresh call, same owner -- must reproduce the same answer
	if first != second {
		t.Fatal("aiBuildVariantFor must be a pure, deterministic function of owner")
	}
	seen := map[int]bool{}
	for owner := 1; owner <= 3; owner++ {
		if v := aiBuildVariantFor(owner); v < 0 || v >= len(aiBuildVariants) {
			t.Fatalf("aiBuildVariantFor(%d) = %d, out of range [0,%d)", owner, v, len(aiBuildVariants))
		} else {
			seen[v] = true
		}
	}
	if len(seen) < 2 {
		t.Fatal("expected at least two distinct build-order profiles across 3 bot owners")
	}
}

// TestAiExpandEconomy_KeepsBuildingPastTheEndOfTheCuratedList is the
// regression test for "не развивается": aiBuildNext used to stop
// permanently once its fixed list was fully placed, no matter how much
// material and gold kept piling up unused.
func TestAiExpandEconomy_KeepsBuildingPastTheEndOfTheCuratedList(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIHard})
	f := g.ais[0]
	order := aiBuildVariants[f.brain.buildVariant]
	f.brain.buildIndex = len(order) // pretend the curated list is already fully placed
	f.stock.Add(resource.Plank, 500)
	f.stock.Add(resource.StoneBlock, 500)

	f.brain.aiBuildNext(g, f, g.grid)

	if f.brain.buildIndex != len(order)+1 {
		t.Fatalf("buildIndex = %d, want %d -- aiBuildNext should have placed one aiExpansionOrder building instead of stopping cold", f.brain.buildIndex, len(order)+1)
	}
}

// TestAiFortifyIsthmus_ReplacesAnExistingRoadTile is the regression test
// for a real playtest report ("разреши строительство стены поверх
// участка дороги... дорога не является чем-то запрещенным"):
// aiBuildDefenses could permanently fail to fortify a crossing whenever
// an earlier-built Road tile (the faction's own logistics network, or a
// guard tower's own access road) already happened to sit on the wall's
// own line -- CanPlace rejected the WHOLE straight line the instant any
// one tile overlapped it, and nothing ever removed that road to retry.
func TestAiFortifyIsthmus_ReplacesAnExistingRoadTile(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy})
	f := g.ais[0]
	q := quadrantAssignmentOrder[f.owner]
	rect := g.duelIsthmuses[duelIsthmusIndicesFor(q)[0]]

	// The first tile of the wall's own line, matching aiFortifyIsthmus'
	// own axis choice exactly.
	roadX, roadY := rect.Min.X, rect.Min.Y
	if rect.Dx() >= rect.Dy() {
		roadX = rect.Min.X + rect.Dx()/2
	} else {
		roadY = rect.Min.Y + rect.Dy()/2
	}
	road := &building.Building{Kind: building.Road, X: roadX, Y: roadY, Owner: f.owner, HP: building.MaxHP, ConstructionStage: building.ConstructionNone}
	g.buildings = append(g.buildings, road)

	g.aiFortifyIsthmus(f, rect)

	var wallHere, roadHere bool
	for _, b := range g.buildings {
		if b.X != roadX || b.Y != roadY {
			continue
		}
		if b.Kind == building.StoneWall {
			wallHere = true
		}
		if b.Kind == building.Road {
			roadHere = true
		}
	}
	if !wallHere {
		t.Fatal("the wall segment over the pre-existing road was never placed -- the crossing stayed unfortified")
	}
	if roadHere {
		t.Fatal("the road tile should have been replaced by the wall, not left alongside it")
	}
}

// TestDuelGame_ReloadKeepsAWallsOnlyStragglerFactionAttackable is the
// regression test for a real playtest report ("почему ПКМ на вражеской
// стене не уничтожает постройку?"): a faction reduced to nothing but its
// own defensive perimeter (aiBuildDefenses' walls/gates -- no Warehouse,
// no other real building, no units) used to be dropped from g.ais
// entirely on reload, since loadGame's straggler-anchor fallback only
// ever tried nearestRealBuildingOwnedBy (which deliberately excludes
// Road/StoneWall/Gate). Its walls then became permanently unattackable:
// opposingBuildingsFor only ever gathers candidates from factions
// actually present in g.ais.
func TestDuelGame_ReloadKeepsAWallsOnlyStragglerFactionAttackable(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy})
	f := g.ais[0]
	f.stock.Add(resource.Plank, 500)
	f.stock.Add(resource.StoneBlock, 500)
	g.aiBuildDefenses(f, g.grid)

	for _, b := range g.buildings {
		if b.Owner == f.owner && b.Kind != building.StoneWall && b.Kind != building.Gate && b.Kind != building.Road {
			b.HP = 0
		}
	}
	g.pruneDestroyedBuildings()
	crippleFactionUnits(f)
	f.stock = resource.NewStockpile(stockpileCapacity)

	var wall *building.Building
	for _, b := range g.buildings {
		if b.Owner == f.owner && b.Kind == building.StoneWall {
			wall = b
			break
		}
	}
	if wall == nil {
		t.Fatal("test setup: expected at least one StoneWall to survive the wipe")
	}

	path := t.TempDir() + "/straggler.json"
	if err := g.saveGame(path, "straggler test"); err != nil {
		t.Fatalf("saveGame: %v", err)
	}

	loaded := newDuelGame([]aiDifficulty{AIEasy})
	if err := loaded.loadGame(path); err != nil {
		t.Fatalf("loadGame: %v", err)
	}
	if len(loaded.ais) == 0 {
		t.Fatal("g.ais is empty after reload -- the walls-only straggler faction was dropped entirely")
	}
	if target := loaded.opposingBuildingAt(wall.X, wall.Y); target == nil {
		t.Fatal("the surviving wall is no longer a valid attack target after reload")
	}
}
