package main

import (
	"fmt"
	"os"
	"testing"

	"strategy_game/internal/advisor"
	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/enemy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/i18n"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/miner"
	"strategy_game/internal/quarry"
	"strategy_game/internal/render"
	"strategy_game/internal/resource"
	"strategy_game/internal/save"
	"strategy_game/internal/sentry"
	"strategy_game/internal/ui"
	"strategy_game/internal/villagers"
	"strategy_game/internal/world"
)

func TestSeedFishLimitsEveryConnectedWaterBody(t *testing.T) {
	grid := world.NewGrid(8, 4)
	// A ten-tile pond and a disconnected one-tile puddle exercise both the
	// 40% cap and the deliberate one-fish minimum for tiny ponds.
	for y := 0; y < 2; y++ {
		for x := 0; x < 5; x++ {
			grid.Set(x, y, world.Tile{Terrain: world.Water})
		}
	}
	grid.Set(7, 3, world.Tile{Terrain: world.Water})

	buildings := seedFish(grid, nil)
	large := waterBodyCells(grid, gridPoint{0, 0})
	small := waterBodyCells(grid, gridPoint{7, 3})
	if got, want := countFishInCells(buildings, large), 4; got != want {
		t.Fatalf("large pond fish = %d, want %d (40%% of 10)", got, want)
	}
	if got, want := countFishInCells(buildings, small), 1; got != want {
		t.Fatalf("small pond fish = %d, want %d (tiny-pond minimum)", got, want)
	}
}

// TestSeedStoneDepositsSplitsAcrossMultipleRegions covers "раздели камень
// на 2-5 областей": deposits must not form one single patch, and their
// total count must land in the stoneMinPercent-stoneMaxPercent range of the
// map's area.
func TestSeedStoneDepositsSplitsAcrossMultipleRegions(t *testing.T) {
	grid := world.NewGrid(40, 30)
	// No keep-away point exercised here -- this test is only about the
	// abundance percentage and region count; TestSeedDepositsKeepAllKindsAwayFromWarehouse
	// covers the distance constraint on its own.
	buildings := seedStoneDeposits(grid, nil, 0x1b873593, gridPoint{}, 0)

	area := grid.Width * grid.Height
	minCells, maxCells := scaledStoneDepositCells(area, stoneMinPercent), scaledStoneDepositCells(area, stoneMaxPercent)
	if len(buildings) < minCells || len(buildings) > maxCells {
		t.Fatalf("placed %d stone-deposit cells, want between %d and %d (%d-%d%% target of %d)", len(buildings), minCells, maxCells, stoneMinPercent, stoneMaxPercent, area)
	}

	regions := countStoneRegions(buildings)
	if regions < 2 || regions > 5 {
		t.Fatalf("stone deposits form %d connected regions, want 2-5", regions)
	}

	for _, b := range buildings {
		if b.Kind != building.StoneDeposit || b.Reserve != building.StoneDepositReserve {
			t.Fatalf("deposit at (%d,%d) has Kind=%v Reserve=%d, want StoneDeposit at full reserve", b.X, b.Y, b.Kind, b.Reserve)
		}
	}
}

// countStoneRegions flood-fills cardinal-adjacent deposit cells to count how
// many disconnected blobs seedStoneDeposits actually produced.
func countStoneRegions(buildings []*building.Building) int {
	cells := make(map[gridPoint]bool, len(buildings))
	for _, b := range buildings {
		cells[gridPoint{b.X, b.Y}] = true
	}
	seen := make(map[gridPoint]bool, len(cells))
	regions := 0
	for start := range cells {
		if seen[start] {
			continue
		}
		regions++
		queue := []gridPoint{start}
		seen[start] = true
		for i := 0; i < len(queue); i++ {
			p := queue[i]
			for _, d := range [...]gridPoint{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				n := gridPoint{p.x + d.x, p.y + d.y}
				if cells[n] && !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
	}
	return regions
}

// TestEnsureStoneDepositsDoesNotReseedAFullyMinedWorld covers the exact bug
// StoneSeeded exists to prevent: a modern save where the player has mined
// every deposit dry (zero StoneDeposit buildings, alreadySeeded true) must
// not have a fresh region conjured back in on load.
func TestEnsureStoneDepositsDoesNotReseedAFullyMinedWorld(t *testing.T) {
	grid := world.NewGrid(40, 30)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	buildings := []*building.Building{warehouse} // no deposits left, world already seeded once

	got := ensureStoneDeposits(grid, buildings, true, 0x1b873593, gridPoint{warehouse.X, warehouse.Y})

	if len(got) != 1 {
		t.Fatalf("ensureStoneDeposits on an already-seeded, fully-mined world returned %d buildings, want 1 (no reseeding)", len(got))
	}
}

// TestEnsureStoneDepositsSeedsAnUnmigratedSave covers the other half: a save
// from before this feature (StoneSeeded false, no deposits) must receive a
// fresh region on load.
func TestEnsureStoneDepositsSeedsAnUnmigratedSave(t *testing.T) {
	grid := world.NewGrid(40, 30)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	buildings := []*building.Building{warehouse}

	got := ensureStoneDeposits(grid, buildings, false, 0x1b873593, gridPoint{warehouse.X, warehouse.Y})

	deposits := 0
	for _, b := range got {
		if b.Kind == building.StoneDeposit {
			deposits++
		}
	}
	if deposits == 0 {
		t.Fatal("ensureStoneDeposits did not seed a region for an unmigrated save")
	}
}

// TestSeedOreDepositsRespectsDesignedAbundance covers "угля должно быть
// больше чем золотой и железной руды": each ore kind's seeded cell count
// must land within its own designed percentage range of the map's area,
// and Coal's range sits strictly above Iron's and Gold's. Iron and Gold
// themselves are equal by design now (both a fixed 1%, see
// ironOreMinPercent's doc comment) -- the user explicitly asked for them
// to match, dropping Iron's old edge over Gold.
func TestSeedOreDepositsRespectsDesignedAbundance(t *testing.T) {
	grid := world.NewGrid(40, 30)
	area := grid.Width * grid.Height

	cases := []struct {
		name                   string
		kind                   building.Kind
		minPercent, maxPercent int
		seed                   uint32
	}{
		{"coal", building.CoalDeposit, coalMinPercent, coalMaxPercent, defaultCoalSeed},
		{"iron ore", building.IronOreDeposit, ironOreMinPercent, ironOreMaxPercent, defaultIronOreSeed},
		{"gold ore", building.GoldOreDeposit, goldOreMinPercent, goldOreMaxPercent, defaultGoldOreSeed},
	}
	for _, c := range cases {
		// No keep-away point exercised here -- this test is only about the
		// abundance percentages; TestSeedDepositsKeepAllKindsAwayFromWarehouse
		// covers the distance constraint on its own.
		buildings := seedOreDeposits(grid, nil, c.kind, c.minPercent, c.maxPercent, c.seed, gridPoint{}, 0)
		minCells, maxCells := scaledDepositCells(area, c.minPercent), scaledDepositCells(area, c.maxPercent)
		if len(buildings) < minCells || len(buildings) > maxCells {
			t.Fatalf("%s: placed %d cells, want between %d and %d (reduced %d-%d%% target of %d)", c.name, len(buildings), minCells, maxCells, c.minPercent, c.maxPercent, area)
		}
		for _, b := range buildings {
			if b.Kind != c.kind || b.Reserve != building.OreDepositReserve {
				t.Fatalf("%s: deposit at (%d,%d) has Kind=%v Reserve=%d, want %v at full reserve", c.name, b.X, b.Y, b.Kind, b.Reserve, c.kind)
			}
		}
	}
	if coalMaxPercent <= ironOreMaxPercent {
		t.Fatalf("designed abundance ordering broken: coal(%d-%d) should exceed iron(%d-%d)",
			coalMinPercent, coalMaxPercent, ironOreMinPercent, ironOreMaxPercent)
	}
	if ironOreMinPercent != goldOreMinPercent || ironOreMaxPercent != goldOreMaxPercent {
		t.Fatalf("iron(%d-%d) and gold(%d-%d) should match by design",
			ironOreMinPercent, ironOreMaxPercent, goldOreMinPercent, goldOreMaxPercent)
	}
}

// TestSeedDepositsKeepAllKindsAwayFromWarehouse covers "уголь, камень,
// руды не спавнились ближе чем на 20 клеток от склада": every finite
// deposit kind -- stone, coal, gold ore, iron ore -- must sit at least
// minDepositDistanceFromWarehouse away from the given point.
func TestSeedDepositsKeepAllKindsAwayFromWarehouse(t *testing.T) {
	grid := world.NewGrid(80, 60)
	warehouse := gridPoint{18, 10}
	minDistSq := float64(minDepositDistanceFromWarehouse * minDepositDistanceFromWarehouse)

	checkDistance := func(kind building.Kind, buildings []*building.Building) {
		if len(buildings) == 0 {
			t.Fatalf("%v: seeding placed no cells at all", kind)
		}
		for _, b := range buildings {
			dx, dy := float64(b.X-warehouse.x), float64(b.Y-warehouse.y)
			if dist := dx*dx + dy*dy; dist < minDistSq {
				t.Fatalf("%v: deposit at (%d,%d) is within %d cells of the warehouse, want >= %d", kind, b.X, b.Y, minDepositDistanceFromWarehouse, minDepositDistanceFromWarehouse)
			}
		}
	}

	checkDistance(building.StoneDeposit, seedStoneDeposits(grid, nil, 0x1b873593, warehouse, minDepositDistanceFromWarehouse))
	for _, kind := range []building.Kind{building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit} {
		checkDistance(kind, seedOreDeposits(grid, nil, kind, 3, 4, 0xabcdef01, warehouse, minDepositDistanceFromWarehouse))
	}
}

// TestEnsureOreDepositsDoesNotReseedAFullyMinedWorld mirrors
// TestEnsureStoneDepositsDoesNotReseedAFullyMinedWorld for OreSeeded: a
// modern save with every ore deposit mined dry must not have fresh regions
// conjured back in on load.
func TestEnsureOreDepositsDoesNotReseedAFullyMinedWorld(t *testing.T) {
	grid := world.NewGrid(40, 30)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	buildings := []*building.Building{warehouse}

	got := ensureOreDeposits(grid, buildings, true, defaultCoalSeed, defaultGoldOreSeed, defaultIronOreSeed, gridPoint{warehouse.X, warehouse.Y})

	if len(got) != 1 {
		t.Fatalf("ensureOreDeposits on an already-seeded, fully-mined world returned %d buildings, want 1 (no reseeding)", len(got))
	}
}

// TestEnsureOreDepositsSeedsAnUnmigratedSave mirrors
// TestEnsureStoneDepositsSeedsAnUnmigratedSave for all three ore kinds.
func TestEnsureOreDepositsSeedsAnUnmigratedSave(t *testing.T) {
	grid := world.NewGrid(40, 30)
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	buildings := []*building.Building{warehouse}

	got := ensureOreDeposits(grid, buildings, false, defaultCoalSeed, defaultGoldOreSeed, defaultIronOreSeed, gridPoint{warehouse.X, warehouse.Y})

	found := map[building.Kind]bool{}
	for _, b := range got {
		found[b.Kind] = true
	}
	for _, kind := range []building.Kind{building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit} {
		if !found[kind] {
			t.Fatalf("ensureOreDeposits did not seed any %v for an unmigrated save", kind)
		}
	}
}

// TestGenerateGridCarvesASeaAlongTheWholeEdgeWithinTargetPercent covers
// "вода должна быть скраю карты единая на границе с краем карты": the sea
// must be one unified strip touching *every* position along whichever map
// edge it grows from, not merely a blob that happens to touch the edge
// somewhere -- plus the usual designed-abundance check (8-14% of the map).
func TestGenerateGridCarvesASeaAlongTheWholeEdgeWithinTargetPercent(t *testing.T) {
	grid := generateGrid(60, 45, 0x1b873593)
	area := grid.Width * grid.Height

	water := 0
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if grid.At(x, y).Terrain == world.Water {
				water++
			}
		}
	}
	minCells, maxCells := area*seaMinPercent/100, area*seaMaxPercent/100
	if water < minCells || water > maxCells {
		t.Fatalf("sea has %d water cells, want between %d and %d (reduced %d-%d%% target of %d)", water, minCells, maxCells, seaMinPercent, seaMaxPercent, area)
	}

	edgeWater := func(x, y int) bool { return grid.At(x, y).Terrain == world.Water }
	fullyWater := func(get func(i int) bool, length int) bool {
		for i := 0; i < length; i++ {
			if !get(i) {
				return false
			}
		}
		return true
	}
	top := fullyWater(func(x int) bool { return edgeWater(x, 0) }, grid.Width)
	bottom := fullyWater(func(x int) bool { return edgeWater(x, grid.Height-1) }, grid.Width)
	left := fullyWater(func(y int) bool { return edgeWater(0, y) }, grid.Height)
	right := fullyWater(func(y int) bool { return edgeWater(grid.Width-1, y) }, grid.Height)
	if !top && !bottom && !left && !right {
		t.Fatal("no map edge is entirely water, want the sea to form one unified strip along a whole edge")
	}
}

// TestGenerateGridProducesDifferentMapsForDifferentSeeds is the closest
// stand-in for "карта генерируется случайно" a deterministic test can make:
// two distinct seeds must not produce an identical tile layout.
func TestGenerateGridProducesDifferentMapsForDifferentSeeds(t *testing.T) {
	a := generateGrid(60, 45, 0x1b873593)
	b := generateGrid(60, 45, 0x9e3779b9)

	identical := true
	for y := 0; y < a.Height && identical; y++ {
		for x := 0; x < a.Width; x++ {
			if a.At(x, y).Terrain != b.At(x, y).Terrain {
				identical = false
				break
			}
		}
	}
	if identical {
		t.Fatal("two different seeds produced an identical map, want the layout to vary")
	}
}

// TestFindWarehouseSpotIsBuildableWithRoomForTheStartingRoad checks the
// procedural replacement for the old fixed warehouseX/Y constants: the
// chosen spot must be plain grass with a buildable tile directly south for
// the starting Road, at least warehouseEdgeMargin from every map edge.
func TestFindWarehouseSpotIsBuildableWithRoomForTheStartingRoad(t *testing.T) {
	grid := generateGrid(60, 45, 0x1b873593)
	spot, ok := findWarehouseSpot(grid, 0xabcdef01)
	if !ok {
		t.Fatal("findWarehouseSpot found no spot on a freshly generated map")
	}
	if grid.At(spot.x, spot.y).Terrain != world.Grass {
		t.Fatalf("warehouse spot (%d,%d) is not plain grass", spot.x, spot.y)
	}
	if !grid.At(spot.x, spot.y+1).Buildable() {
		t.Fatalf("tile south of the warehouse spot (%d,%d) is not buildable, no room for the starting road", spot.x, spot.y)
	}
	if spot.x < warehouseEdgeMargin || spot.x >= grid.Width-warehouseEdgeMargin ||
		spot.y < warehouseEdgeMargin || spot.y >= grid.Height-warehouseEdgeMargin {
		t.Fatalf("warehouse spot (%d,%d) is within %d tiles of a map edge", spot.x, spot.y, warehouseEdgeMargin)
	}
}

// TestFindWarehouseSpotIsWithinMaxDistanceOfWater covers the user's
// explicit request ("склад спавнился недалеко от воды, максимум 20
// клеток"): the chosen spot must have a Water tile within
// maxWarehouseDistanceFromWater straight-line tiles.
func TestFindWarehouseSpotIsWithinMaxDistanceOfWater(t *testing.T) {
	grid := generateGrid(60, 45, 0x1b873593)
	spot, ok := findWarehouseSpot(grid, 0xabcdef01)
	if !ok {
		t.Fatal("findWarehouseSpot found no spot on a freshly generated map")
	}

	limitSq := float64(maxWarehouseDistanceFromWater * maxWarehouseDistanceFromWater)
	within := false
	for y := 0; y < grid.Height && !within; y++ {
		for x := 0; x < grid.Width; x++ {
			if grid.At(x, y).Terrain != world.Water {
				continue
			}
			dx, dy := float64(spot.x-x), float64(spot.y-y)
			if dx*dx+dy*dy <= limitSq {
				within = true
				break
			}
		}
	}
	if !within {
		t.Fatalf("warehouse spot (%d,%d) has no water within %d tiles", spot.x, spot.y, maxWarehouseDistanceFromWater)
	}
}

// TestFindWarehouseSpotVariesWithSeed covers the user's explicit request
// ("перегенерируем расположение респауна... в случайном порядке"): unlike
// the old fixed warehouseX/Y constants, two different seeds on the same map
// must not always land on the same spot.
func TestFindWarehouseSpotVariesWithSeed(t *testing.T) {
	grid := generateGrid(60, 45, 0x1b873593)
	a, ok := findWarehouseSpot(grid, 0xabcdef01)
	if !ok {
		t.Fatal("findWarehouseSpot found no spot for seed a")
	}
	b, ok := findWarehouseSpot(grid, 0x12345678)
	if !ok {
		t.Fatal("findWarehouseSpot found no spot for seed b")
	}
	if a == b {
		t.Fatalf("two different seeds landed on the same warehouse spot (%d,%d), want the pick to vary", a.x, a.y)
	}
}

// TestSeedThicketsAddsTreesBeyondTheUniformScatter covers the roadmap's
// "чащи": thickets must add MORE trees on top of seedTrees' map-wide ~1%
// scatter, not replace or cap it.
func TestSeedThicketsAddsTreesBeyondTheUniformScatter(t *testing.T) {
	grid := generateGrid(60, 45, 0x1b873593)

	var uniformOnly []*building.Building
	uniformOnly = seedTrees(grid, uniformOnly)

	var withThickets []*building.Building
	withThickets = seedTrees(grid, withThickets)
	withThickets = seedThickets(grid, withThickets, 0x27d4eb2f)

	if len(withThickets) <= len(uniformOnly) {
		t.Fatalf("tree count with thickets = %d, want more than the uniform-only count %d", len(withThickets), len(uniformOnly))
	}
}

// TestSeedThicketsNeverStrandsATree is a regression guard for a real bug
// the user reported in-game ("дерево вырастает внутри и к нему невозможно
// подойти"): thicket generation used to only check whether a tree's own
// cell was free (building.CanPlace), never whether a worker could actually
// reach it -- a tree could end up fully boxed in by its own neighbours,
// permanently unreachable to a lumberjack. Every tree in a generated
// thicket must have at least one of its 8 neighbouring cells free of
// anything but a Road, matching pathfind.FindLandPath's exact walkability
// rule.
func TestSeedThicketsNeverStrandsATree(t *testing.T) {
	grid := generateGrid(60, 45, 0x1b873593)
	var buildings []*building.Building
	buildings = seedThickets(grid, buildings, 0x27d4eb2f)

	trees := 0
	for _, b := range buildings {
		if b.Kind != building.Tree {
			continue
		}
		trees++
		if !treeCellHasOpenNeighbor(grid, buildings, b.X, b.Y, -1, -1) {
			t.Fatalf("tree at (%d,%d) has no walkable neighbor -- unreachable", b.X, b.Y)
		}
	}
	if trees == 0 {
		t.Fatal("no trees were planted at all -- test isn't exercising anything")
	}
}

// TestTreePlacementKeepsEveryoneReachable exercises the check function
// directly: a cell surrounded on all 8 sides must be rejected, and a
// placement that would seal off an existing neighbouring tree's only open
// side must also be rejected, even though the new cell itself still has
// other open neighbours.
func TestTreePlacementKeepsEveryoneReachable(t *testing.T) {
	grid := world.NewGrid(5, 5)

	t.Run("fully surrounded candidate is rejected", func(t *testing.T) {
		var buildings []*building.Building
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				buildings = append(buildings, building.NewTree(2+dx, 2+dy))
			}
		}
		if treePlacementKeepsEveryoneReachable(grid, buildings, 2, 2) {
			t.Fatal("a cell surrounded on all 8 sides was accepted")
		}
	})

	t.Run("placement that would seal off a neighbor is rejected", func(t *testing.T) {
		// A tree at (2,2) whose only open side is (3,2). Planting a new
		// tree at (3,2) would strand it, even though (3,2) itself has
		// other open neighbours.
		buildings := []*building.Building{building.NewTree(2, 2)}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 || (dx == 1 && dy == 0) {
					continue
				}
				buildings = append(buildings, building.NewTree(2+dx, 2+dy))
			}
		}
		if treePlacementKeepsEveryoneReachable(grid, buildings, 3, 2) {
			t.Fatal("a placement that strands an existing neighbor tree was accepted")
		}
	})

	t.Run("ordinary open placement is accepted", func(t *testing.T) {
		if !treePlacementKeepsEveryoneReachable(grid, nil, 2, 2) {
			t.Fatal("an ordinary placement with no neighbours at all was rejected")
		}
	})
}

func TestFishRegrowthRespectsBodyCap(t *testing.T) {
	grid := world.NewGrid(3, 1)
	for x := 0; x < grid.Width; x++ {
		grid.Set(x, 0, world.Tile{Terrain: world.Water})
	}
	// Three water tiles may hold one fish (floor(3 * 40 / 100)).
	existing := building.NewFish(0, 0)
	game := &Game{
		grid:      grid,
		buildings: []*building.Building{existing},
		fishRegrowth: []fishRegrowth{{
			waterX: 1,
			waterY: 0,
			ticks:  1,
			target: 1,
			seed:   1,
		}},
	}

	game.tickFishRegrowth()
	pond := waterBodyCells(grid, gridPoint{0, 0})
	if got, want := countFishInCells(game.buildings, pond), 1; got != want {
		t.Fatalf("fish after capped regrowth = %d, want %d", got, want)
	}
	if len(game.fishRegrowth) != 1 {
		t.Fatal("regrowth entry disappeared even though the pond is at its cap")
	}
}

// TestSelectionAt_BuildingWinsOverInvisibleResident covers "нажатие на
// здание открывает NPC, а не здание": a resident stationed at its post
// (e.g. a baker inside its Bakery) sits on the building's own tile but is
// deliberately not drawn there -- see Villager.VisibleOnMap. A click on
// that tile must resolve to the building, not steal the click for an
// invisible worker the player can't even see standing there.
func TestSelectionAt_BuildingWinsOverInvisibleResident(t *testing.T) {
	bakery := &building.Building{Kind: building.Bakery, X: 2, Y: 0}
	game := &Game{
		buildings: []*building.Building{bakery},
		vills:     villagers.NewController(),
		logi:      logistics.NewController(&building.Building{Kind: building.Warehouse}, 0),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
		camera:    render.NewCamera(),
	}
	game.vills.Spawn(villagers.Baker, bakery)
	baker := game.vills.Villagers[0]
	if baker.VisibleOnMap() {
		t.Fatal("test setup: baker unexpectedly visible while working")
	}

	sx, sy := game.camera.TileToScreen(bakery.X, bakery.Y)
	got := game.selectionAt(int(sx)+1, int(sy)+1)
	if got.Kind != ui.SelectionBuilding || got.Building != bakery {
		t.Fatalf("selection = %+v, want the Bakery building", got)
	}
}

// TestSelectionAt_BuildingWinsOverVisibleFarmer verifies that a building
// remains inspectable even when its visible farmer is standing on a field
// cell inside the Farm footprint.
func TestSelectionAt_BuildingWinsOverVisibleFarmer(t *testing.T) {
	farm := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	game := &Game{
		buildings: []*building.Building{farm},
		vills:     villagers.NewController(),
		logi:      logistics.NewController(&building.Building{Kind: building.Warehouse}, 0),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
		camera:    render.NewCamera(),
	}
	game.vills.Spawn(villagers.Farmer, farm)
	farmer := game.vills.Villagers[0]
	farmer.X, farmer.Y = farm.X+1, farm.Y // a field cell, not the farmhouse tile
	if !farmer.VisibleOnMap() {
		t.Fatal("test setup: farmer unexpectedly hidden while tending the field")
	}

	sx, sy := game.camera.TileToScreen(farmer.X, farmer.Y)
	got := game.selectionAt(int(sx)+1, int(sy)+1)
	if got.Kind != ui.SelectionBuilding || got.Building != farm {
		t.Fatalf("selection = %+v, want the Farm under the visible farmer", got)
	}
}

// TestSelectionAt_ConstructionSiteWinsOverBuilder verifies the universal
// click rule for the unit that is always visible: a builder working on a
// construction site must not prevent the player from inspecting that site.
func TestSelectionAt_ConstructionSiteWinsOverBuilder(t *testing.T) {
	site := building.NewConstructionSite(building.LumberjackHut, 2, 0)
	game := &Game{
		buildings: []*building.Building{site},
		vills:     villagers.NewController(),
		logi:      logistics.NewController(&building.Building{Kind: building.Warehouse}, 0),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
		camera:    render.NewCamera(),
	}
	bl := game.builders.Hire(&building.Building{Kind: building.Warehouse})
	bl.X, bl.Y = site.X, site.Y

	sx, sy := game.camera.TileToScreen(site.X, site.Y)
	got := game.selectionAt(int(sx)+1, int(sy)+1)
	if got.Kind != ui.SelectionBuilding || got.Building != site {
		t.Fatalf("selection = %+v, want the construction site under the builder", got)
	}
}

// TestSelectionAt_WarehouseWinsOverSerf verifies that the same rule applies
// to a serf, which is always visible while working: the Warehouse remains
// the selected object on its own tile.
func TestSelectionAt_WarehouseWinsOverSerf(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	game := &Game{
		buildings: []*building.Building{warehouse},
		vills:     villagers.NewController(),
		logi:      logistics.NewController(warehouse, 1),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
		camera:    render.NewCamera(),
	}
	serf := game.logi.Serfs[0]
	serf.X, serf.Y = warehouse.X, warehouse.Y

	sx, sy := game.camera.TileToScreen(warehouse.X, warehouse.Y)
	got := game.selectionAt(int(sx)+1, int(sy)+1)
	if got.Kind != ui.SelectionBuilding || got.Building != warehouse {
		t.Fatalf("selection = %+v, want the Warehouse under the serf", got)
	}
}

// TestSelectionAt_SerfOnRoadWinsOverTheRoad is a regression guard for a
// real bug the user reported directly: after TestSelectionAt_*WinsOver*
// above made a building win over any unit standing on it (so an
// inspector click always reaches a workplace, not the worker passing
// through), that same rule quietly made it impossible to ever select a
// unit standing on a Road tile -- roads have nothing of their own worth
// inspecting, and nearly every walking unit spends nearly all its time
// on one. A serf on a road must still be selectable; the road itself is
// only the fallback when no unit is actually standing there.
func TestSelectionAt_SerfOnRoadWinsOverTheRoad(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	road := &building.Building{Kind: building.Road, X: 3, Y: 0}
	game := &Game{
		buildings: []*building.Building{warehouse, road},
		vills:     villagers.NewController(),
		logi:      logistics.NewController(warehouse, 1),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
		camera:    render.NewCamera(),
	}
	serf := game.logi.Serfs[0]
	serf.X, serf.Y = road.X, road.Y

	sx, sy := game.camera.TileToScreen(road.X, road.Y)
	got := game.selectionAt(int(sx)+1, int(sy)+1)
	if got.Kind != ui.SelectionSerf || got.Serf != serf {
		t.Fatalf("selection = %+v, want the serf standing on the road", got)
	}

	// And with nobody standing there, the same tile still resolves to the
	// road -- the fallback path must actually fire, not just no-op.
	serf.X, serf.Y = -1, -1
	got = game.selectionAt(int(sx)+1, int(sy)+1)
	if got.Kind != ui.SelectionBuilding || got.Building != road {
		t.Fatalf("selection with nobody on the tile = %+v, want the road itself", got)
	}
}

// TestBuildingSelectionAtReturnsRoad ensures continuous demolition targets the
// road itself even though regular selection intentionally gives a traveller
// standing there priority (see TestSelectionAt_SerfOnRoadWinsOverTheRoad).
func TestBuildingSelectionAtReturnsRoad(t *testing.T) {
	road := &building.Building{Kind: building.Road, X: 3, Y: 0}
	game := &Game{buildings: []*building.Building{road}, camera: render.NewCamera()}

	sx, sy := game.camera.TileToScreen(road.X, road.Y)
	got := game.buildingSelectionAt(int(sx)+1, int(sy)+1)
	if got.Kind != ui.SelectionBuilding || got.Building != road {
		t.Fatalf("demolition selection = %+v, want the road", got)
	}
}

// TestUnitsAt_CountsAnyoneOnTheFootprintEvenWithoutADedicatedResident
// covers "показывать сколько внутри людей ... в т.ч. харчевне и складе":
// unitsAt must work on buildings that never get an assigned resident
// (Tavern, Warehouse), counting whoever is physically standing there right
// now, not just a workplace's one dedicated worker.
func TestUnitsAt_CountsAnyoneOnTheFootprintEvenWithoutADedicatedResident(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 10, Y: 0} // 3x3 footprint
	game := &Game{
		buildings: []*building.Building{warehouse, tavern, farm},
		logi:      logistics.NewController(warehouse, 0),
		vills:     villagers.NewController(),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
	}
	game.logi.Hire()
	game.logi.Hire()
	game.logi.Serfs[0].X, game.logi.Serfs[0].Y = warehouse.X, warehouse.Y
	game.logi.Serfs[1].X, game.logi.Serfs[1].Y = tavern.X, tavern.Y

	game.vills.Spawn(villagers.Farmer, farm)
	game.vills.Villagers[0].X, game.vills.Villagers[0].Y = farm.X+1, farm.Y // a field cell, still inside the footprint

	if got := game.unitsAt(warehouse); got != 1 {
		t.Errorf("unitsAt(warehouse) = %d, want 1 (a serf standing there, no dedicated resident)", got)
	}
	if got := game.unitsAt(tavern); got != 1 {
		t.Errorf("unitsAt(tavern) = %d, want 1 (a serf eating there, no dedicated resident)", got)
	}
	if got := game.unitsAt(farm); got != 1 {
		t.Errorf("unitsAt(farm) = %d, want 1 (the farmer, standing on a field cell inside the footprint)", got)
	}
}

func TestSerializeAndRestorePigChainWorkers(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	pigFarm := &building.Building{Kind: building.PigFarm, X: 2, Y: 0}
	meatWorkshop := &building.Building{Kind: building.MeatWorkshop, X: 4, Y: 0}
	buildings := []*building.Building{warehouse, pigFarm, meatWorkshop}

	makeGame := func() *Game {
		return &Game{
			buildings: buildings,
			stock:     resource.NewStockpile(0),
			logi:      logistics.NewController(warehouse, 0),
			vills:     villagers.NewController(),
			jacks:     lumberjack.NewController(),
			fishers:   fishing.NewController(),
			quarry:    quarry.NewController(),
			builders:  builder.NewController(),
			miners:    miner.NewController(),
			sentries:  sentry.NewController(),
		}
	}
	source := makeGame()
	source.vills.Spawn(villagers.Swineherd, pigFarm)
	source.vills.Spawn(villagers.Butcher, meatWorkshop)
	source.vills.Villagers[0].X, source.vills.Villagers[0].Y = 2, 1
	source.vills.Villagers[1].X, source.vills.Villagers[1].Y = 4, 1

	restored := makeGame()
	restored.restoreUnits(source.serializeUnits(), buildings)
	if got := len(restored.vills.Villagers); got != 2 {
		t.Fatalf("restored pig-chain workers = %d, want 2", got)
	}
	if got := restored.vills.Villagers[0]; got.Profession != villagers.Swineherd || got.HomeBuilding() != pigFarm || got.X != 2 || got.Y != 1 {
		t.Fatalf("restored swineherd = profession %v home %v at (%d,%d), want PigFarm worker at (2,1)", got.Profession, got.HomeBuilding(), got.X, got.Y)
	}
	if got := restored.vills.Villagers[1]; got.Profession != villagers.Butcher || got.HomeBuilding() != meatWorkshop || got.X != 4 || got.Y != 1 {
		t.Fatalf("restored butcher = profession %v home %v at (%d,%d), want MeatWorkshop worker at (4,1)", got.Profession, got.HomeBuilding(), got.X, got.Y)
	}
}

func TestSerializeAndRestoreCarpenter(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	carpentry := &building.Building{Kind: building.CarpentryWorkshop, X: 2, Y: 0}
	buildings := []*building.Building{warehouse, carpentry}

	makeGame := func() *Game {
		return &Game{
			buildings: buildings,
			stock:     resource.NewStockpile(0),
			logi:      logistics.NewController(warehouse, 0),
			vills:     villagers.NewController(),
			jacks:     lumberjack.NewController(),
			fishers:   fishing.NewController(),
			quarry:    quarry.NewController(),
			builders:  builder.NewController(),
			miners:    miner.NewController(),
			sentries:  sentry.NewController(),
		}
	}
	source := makeGame()
	source.vills.Spawn(villagers.Carpenter, carpentry)
	source.vills.Villagers[0].X, source.vills.Villagers[0].Y = 2, 1

	restored := makeGame()
	restored.restoreUnits(source.serializeUnits(), buildings)
	if got := len(restored.vills.Villagers); got != 1 {
		t.Fatalf("restored carpentry workers = %d, want 1", got)
	}
	if got := restored.vills.Villagers[0]; got.Profession != villagers.Carpenter || got.HomeBuilding() != carpentry || got.X != 2 || got.Y != 1 {
		t.Fatalf("restored carpenter = profession %v home %v at (%d,%d), want CarpentryWorkshop worker at (2,1)", got.Profession, got.HomeBuilding(), got.X, got.Y)
	}
}

func TestSerializeAndRestoreDismissedSerf(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	buildings := []*building.Building{warehouse}
	source := &Game{
		buildings: buildings,
		stock:     resource.NewStockpile(0),
		logi:      logistics.NewController(warehouse, 1),
		vills:     villagers.NewController(),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
	}
	if !source.logi.RequestDismissal(source.logi.Serfs[0]) {
		t.Fatal("RequestDismissal = false")
	}

	restored := &Game{
		buildings: buildings,
		stock:     resource.NewStockpile(0),
		logi:      logistics.NewController(warehouse, 0),
		vills:     villagers.NewController(),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
	}
	restored.restoreUnits(source.serializeUnits(), buildings)
	if got := len(restored.logi.Serfs); got != 1 {
		t.Fatalf("restored serfs = %d, want 1", got)
	}
	if !restored.logi.Serfs[0].Dismissing() {
		t.Fatal("dismissal flag was lost during save/restore")
	}
}

// TestConfirmedRemovalDismissesSerf checks the inspector confirmation keeps
// the existing safe dismissal rule: a serf leaves after, rather than during,
// a delivery.
func TestConfirmedRemovalDismissesSerf(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	game := &Game{logi: logistics.NewController(warehouse, 1)}
	game.selection = ui.Selection{Kind: ui.SelectionSerf, Serf: game.logi.Serfs[0]}

	game.requestSelectedRemoval()
	if game.dialog != ui.DialogConfirmRemoval {
		t.Fatalf("removal dialog = %v, want confirmation", game.dialog)
	}
	if game.logi.Serfs[0].Dismissing() {
		t.Fatal("serf dismissal was requested before confirmation")
	}
	game.dialog = ui.DialogNone
	game.removeSelected()

	if !game.logi.Serfs[0].Dismissing() {
		t.Fatal("inspector removal did not request serf dismissal")
	}
}

func TestRequestSelectedBuildingRemovalOpensConfirmation(t *testing.T) {
	road := &building.Building{Kind: building.Road, X: 2, Y: 2}
	game := &Game{selection: ui.Selection{Kind: ui.SelectionBuilding, Building: road}}

	game.requestSelectedRemoval()

	if game.dialog != ui.DialogConfirmRemoval {
		t.Fatalf("building removal dialog = %v, want confirmation", game.dialog)
	}
}
func TestDeleteWarehousePromotesRemainingWarehouse(t *testing.T) {
	first := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	second := &building.Building{Kind: building.Warehouse, X: 4, Y: 0}
	game := &Game{
		buildings: []*building.Building{first, second},
		stock:     resource.NewStockpile(0),
		logi:      logistics.NewController(first, 1),
		vills:     villagers.NewController(),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
		selection: ui.Selection{Kind: ui.SelectionBuilding, Building: first},
	}
	game.logi.AddWarehouse(second)

	game.deleteSelectedBuilding()

	if got := len(game.buildings); got != 1 {
		t.Fatalf("buildings after warehouse deletion = %d, want 1", got)
	}
	if game.buildings[0] != second {
		t.Fatal("remaining building is not the second warehouse")
	}
	if game.logi.Warehouse != second {
		t.Fatal("remaining warehouse was not promoted to logistics root")
	}
	if got := len(game.logi.Warehouses); got != 1 || game.logi.Warehouses[0] != second {
		t.Fatalf("registered warehouses = %v, want only remaining warehouse", game.logi.Warehouses)
	}
	if got := game.logi.Serfs[0]; got.X != second.X || got.Y != second.Y {
		t.Fatalf("serf after route reset at (%d,%d), want promoted warehouse (%d,%d)", got.X, got.Y, second.X, second.Y)
	}
}

func TestDeleteLastWarehouseIsRejected(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	game := &Game{
		buildings: []*building.Building{warehouse},
		stock:     resource.NewStockpile(0),
		logi:      logistics.NewController(warehouse, 0),
		vills:     villagers.NewController(),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
		selection: ui.Selection{Kind: ui.SelectionBuilding, Building: warehouse},
	}

	game.deleteSelectedBuilding()

	if got := len(game.buildings); got != 1 {
		t.Fatalf("last warehouse was deleted; buildings = %d, want 1", got)
	}
	if game.logi.Warehouse != warehouse {
		t.Fatal("logistics root changed after rejected last-warehouse deletion")
	}
}

// TestHireOptionsCapsAtOneWorkerPerBuilding covers "количество NPC не
// должно быть больше строений": a profession's hire card must go
// unavailable once every matching building already has a resident, and
// clicking a still-available card must fill exactly one of the empty ones
// -- never more, never a building of the wrong kind. Serfs are the one
// exception and stay available regardless of building count.
// TestServeHaulWorkloadScalesWithDistanceAndRate locks in the shape of the
// serf-recommendation formula: workload is proportional to both the
// building's own output rate and its round-trip distance from the
// Warehouse, and a Mill (no RequiresWorker, but still produces something a
// serf must haul) is not silently skipped.
func TestServeHaulWorkloadScalesWithDistanceAndRate(t *testing.T) {
	near := serfHaulWorkload(building.Farm, 5)
	far := serfHaulWorkload(building.Farm, 50)
	if far <= near*5 {
		t.Fatalf("workload at 10x the distance = %f, want roughly 10x the near workload %f", far, near)
	}
	if got := serfHaulWorkload(building.Warehouse, 10); got != 0 {
		t.Fatalf("Warehouse workload = %f, want 0 (no Recipe, nothing to haul)", got)
	}
	if got := serfHaulWorkload(building.Mill, 10); got <= 0 {
		t.Fatal("Mill workload = 0, want > 0 (Mill produces Flour a serf must haul, even without RequiresWorker)")
	}
}

// TestRecommendedServeCountGrowsWithDistanceAndProducerCount is an
// integration-level check with real road-connected buildings: moving the
// same producer farther from the Warehouse, or adding more producers,
// must never recommend *fewer* serfs.
func TestRecommendedServeCountGrowsWithDistanceAndProducerCount(t *testing.T) {
	buildAt := func(mills int, spacing int) *Game {
		// Road runs along Y=0; every Mill sits one row below (Y=1),
		// cardinally adjacent to its own road tile, so no Road ever
		// shares a coordinate with a Mill or the Warehouse regardless of
		// how many mills are placed along the same corridor.
		warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
		buildings := []*building.Building{warehouse}
		farthest := mills * spacing
		for rx := 1; rx <= farthest; rx++ {
			buildings = append(buildings, &building.Building{Kind: building.Road, X: rx, Y: 0})
		}
		for i := 0; i < mills; i++ {
			x := (i + 1) * spacing
			buildings = append(buildings, &building.Building{Kind: building.Mill, X: x, Y: 1})
		}
		return &Game{buildings: buildings}
	}

	oneMillNear := buildAt(1, 5).recommendedServeCount()
	oneMillFar := buildAt(1, 60).recommendedServeCount()
	if oneMillFar < oneMillNear {
		t.Fatalf("recommendation dropped from %d to %d when the same Mill moved farther away", oneMillNear, oneMillFar)
	}

	twoMillsNear := buildAt(2, 5).recommendedServeCount()
	if twoMillsNear < oneMillNear {
		t.Fatalf("recommendation dropped from %d to %d when a second Mill was added", oneMillNear, twoMillsNear)
	}

	if got := (&Game{buildings: []*building.Building{{Kind: building.Warehouse, X: 0, Y: 0}}}).recommendedServeCount(); got != 1 {
		t.Fatalf("empty settlement (just a Warehouse) recommends %d, want 1 (the flat baseline)", got)
	}
}

// TestTickAdvisorQueuesAndCoolsDownTips exercises the queue/cooldown
// policy tickAdvisor owns (see internal/advisor for the rule logic
// itself, tested separately): a tip appears once its check interval
// elapses, acknowledging it clears the toast and starts its cooldown, it
// does not reappear during that cooldown even though the underlying
// condition is still true, and it becomes eligible again once the
// cooldown elapses.
// TestTrimSerfsToRecommendedDismissesExactlyTheExcess covers the user's
// explicit request: right-clicking the Serf card should dismiss however
// many serfs are above the recommendation in one go, using the same safe
// RequestDismissal mechanism a single dismiss always used (a serf
// finishes its current haul before actually leaving, nothing is dropped).
func TestTrimSerfsToRecommendedDismissesExactlyTheExcess(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}

	t.Run("dismisses exactly the excess", func(t *testing.T) {
		game := &Game{logi: logistics.NewController(warehouse, 10)}
		game.trimSerfsToRecommended(6)

		dismissing := 0
		for _, s := range game.logi.Serfs {
			if s.Dismissing() {
				dismissing++
			}
		}
		if dismissing != 4 {
			t.Fatalf("dismissing = %d, want 4 (10 serfs - 6 recommended)", dismissing)
		}
	})

	t.Run("already at or below recommended is a no-op", func(t *testing.T) {
		game := &Game{logi: logistics.NewController(warehouse, 5)}
		game.trimSerfsToRecommended(5)
		for _, s := range game.logi.Serfs {
			if s.Dismissing() {
				t.Fatal("a serf was dismissed even though the count already matched the recommendation")
			}
		}

		game.trimSerfsToRecommended(20)
		for _, s := range game.logi.Serfs {
			if s.Dismissing() {
				t.Fatal("a serf was dismissed even though the count was already below the recommendation")
			}
		}
	})

	t.Run("a repeated click does not re-mark already-dismissing serfs or over-dismiss", func(t *testing.T) {
		game := &Game{logi: logistics.NewController(warehouse, 10)}
		game.trimSerfsToRecommended(6) // marks 4 dismissing
		game.trimSerfsToRecommended(6) // must be a no-op: 6 active remain, matches recommended

		dismissing := 0
		for _, s := range game.logi.Serfs {
			if s.Dismissing() {
				dismissing++
			}
		}
		if dismissing != 4 {
			t.Fatalf("dismissing = %d, want still 4 after a repeated click at the same recommendation", dismissing)
		}
	})
}

// hireOptionIndex returns kind's position in options -- the slice index
// HireIndexAt resolves clicks to, which is not necessarily the same as
// the HireKind enum's own numeric value.
func hireOptionIndex(t *testing.T, options []ui.HireOption, kind ui.HireKind) int {
	t.Helper()
	for i, o := range options {
		if o.Kind == kind {
			return i
		}
	}
	t.Fatalf("no hire option for kind %v", kind)
	return -1
}

// findHireCardPoint scans for a screen point HireIndexAt resolves to the
// given card index, so tests can simulate a click without hardcoding the
// left panel's internal geometry constants.
func findHireCardPoint(t *testing.T, layout ui.Layout, optionCount, wantIndex int) (int, int) {
	t.Helper()
	for y := 0; y < layout.Height; y++ {
		if index, ok := layout.HireIndexAt(20, y, optionCount); ok && index == wantIndex {
			return 20, y
		}
	}
	t.Fatalf("no point resolved to hire card index %d", wantIndex)
	return 0, 0
}

// TestHireCardServeRightClickOpensDialogOnlyWhenThereIsExcess covers the
// user's explicit refinement: right-clicking the Serf card dismisses just
// one, as before, when the count is already at or below recommended --
// the confirm dialog (trim-all vs. dismiss-one) only appears when there's
// an actual excess to ask about.
func TestHireCardServeRightClickOpensDialogOnlyWhenThereIsExcess(t *testing.T) {
	newGame := func(serfCount int) *Game {
		warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
		return &Game{
			buildings: []*building.Building{warehouse},
			stock:     resource.NewStockpile(0),
			pop:       &economy.Population{},
			logi:      logistics.NewController(warehouse, serfCount),
			vills:     villagers.NewController(),
			jacks:     lumberjack.NewController(),
			fishers:   fishing.NewController(),
			quarry:    quarry.NewController(),
			builders:  builder.NewController(),
			miners:    miner.NewController(),
			sentries:  sentry.NewController(),
			layout:    ui.NewLayout(1024, 768),
			leftTab:   ui.HireTab,
		}
	}

	t.Run("at recommended: dismisses one immediately, no dialog", func(t *testing.T) {
		game := newGame(1) // recommendedServeCount floors at 1 with just a Warehouse
		options := game.hireOptions()
		mx, my := findHireCardPoint(t, game.layout, len(options), hireOptionIndex(t, options, ui.HireSerf))

		if !game.handleHireCardDismissRightClick(mx, my) {
			t.Fatal("right-click on the Serf card was not handled")
		}
		if game.dialog != ui.DialogNone {
			t.Fatalf("dialog = %v, want DialogNone (no excess to ask about)", game.dialog)
		}
		if !game.logi.Serfs[0].Dismissing() {
			t.Fatal("expected the one serf to be dismissed directly, same as before this change")
		}
	})

	t.Run("above recommended: opens the confirm dialog, dismisses no one yet", func(t *testing.T) {
		game := newGame(10)
		options := game.hireOptions()
		mx, my := findHireCardPoint(t, game.layout, len(options), hireOptionIndex(t, options, ui.HireSerf))

		if !game.handleHireCardDismissRightClick(mx, my) {
			t.Fatal("right-click on the Serf card was not handled")
		}
		if game.dialog != ui.DialogConfirmTrimServes {
			t.Fatalf("dialog = %v, want DialogConfirmTrimServes", game.dialog)
		}
		for _, s := range game.logi.Serfs {
			if s.Dismissing() {
				t.Fatal("a serf was dismissed before the dialog was answered")
			}
		}
	})
}

func TestTickAdvisorQueuesAndCoolsDownTips(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	game := &Game{
		buildings: []*building.Building{warehouse},
		stock:     resource.NewStockpile(0),
		pop:       &economy.Population{},
		logi:      logistics.NewController(warehouse, 0), // 0 serfs -> recommendedServeCount's floor of 1 makes this KindServeCountLow
		vills:     villagers.NewController(),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
		grid:      world.NewGrid(4, 4),
	}

	tick := func(n int) {
		for i := 0; i < n; i++ {
			game.worldTicks++
			game.tickAdvisor()
		}
	}

	tick(advisorCheckIntervalTicks)
	if game.advisorVisible == nil {
		t.Fatal("expected a tip to be visible after the first check interval")
	}
	if game.advisorVisible.Kind != advisor.KindServeCountLow {
		t.Fatalf("visible tip kind = %v, want KindServeCountLow", game.advisorVisible.Kind)
	}

	game.acknowledgeAdvisorTip()
	if game.advisorVisible != nil {
		t.Fatal("tip should be cleared right after being acknowledged")
	}

	tick(advisorCheckIntervalTicks)
	if game.advisorVisible != nil {
		t.Fatalf("tip reappeared during its own cooldown: %+v", game.advisorVisible)
	}

	tick(advisorCooldownTicks)
	if game.advisorVisible == nil {
		t.Fatal("expected the tip to reappear once its cooldown elapsed")
	}
}

func TestHireOptionsCapsAtOneWorkerPerBuilding(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	farm := &building.Building{Kind: building.Farm, X: 2, Y: 0}
	game := &Game{
		buildings: []*building.Building{warehouse, farm},
		stock:     resource.NewStockpile(0),
		pop:       &economy.Population{},
		logi:      logistics.NewController(warehouse, 0),
		vills:     villagers.NewController(),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
	}
	game.stock.Add(resource.Gold, 10) // enough to hire a couple of units

	options := game.hireOptions()
	farmer := findHireOption(t, options, ui.HireFarmer)
	if farmer.Current != 0 || farmer.Limit != 1 || !farmer.Available {
		t.Fatalf("farmer option = %+v, want Current=0 Limit=1 Available=true (one empty Farm)", farmer)
	}
	if serf := findHireOption(t, options, ui.HireSerf); serf.Limit != 0 || !serf.Available {
		t.Fatalf("serf option = %+v, want Limit=0 (unlimited) Available=true", serf)
	}

	game.hireFromTab(ui.HireFarmer)
	if got := len(game.vills.Villagers); got != 1 {
		t.Fatalf("villagers after hiring a farmer = %d, want 1", got)
	}
	if game.vills.Villagers[0].HomeBuilding() != farm {
		t.Fatal("hired farmer was not assigned to the vacant Farm")
	}

	options = game.hireOptions()
	farmer = findHireOption(t, options, ui.HireFarmer)
	if farmer.Current != 1 || farmer.Available {
		t.Fatalf("farmer option after hiring = %+v, want Current=1 Available=false (no Farm left empty)", farmer)
	}

	// The only Farm is already staffed -- clicking again must be a no-op,
	// not double-assign a second farmer to the same building.
	game.hireFromTab(ui.HireFarmer)
	if got := len(game.vills.Villagers); got != 1 {
		t.Fatalf("villagers after a second hire attempt = %d, want still 1 (no vacant Farm)", got)
	}
}

// TestHireSerfSpendsGoldAndFailsWhenBroke covers "создание любого юнита -1
// золото": hiring a serf must deduct exactly one Gold, and must refuse
// (leaving the stockpile and roster untouched) once the town is broke.
func TestHireSerfSpendsGoldAndFailsWhenBroke(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	game := &Game{
		buildings: []*building.Building{warehouse},
		stock:     resource.NewStockpile(0),
		pop:       &economy.Population{},
		logi:      logistics.NewController(warehouse, 0),
		vills:     villagers.NewController(),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
	}
	game.stock.Add(resource.Gold, 1)

	game.hireSerf()
	if got := len(game.logi.Serfs); got != 1 {
		t.Fatalf("serfs after hiring with 1 gold = %d, want 1", got)
	}
	if got := game.stock.Amount(resource.Gold); got != 0 {
		t.Fatalf("gold after hiring = %d, want 0", got)
	}

	game.hireSerf()
	if got := len(game.logi.Serfs); got != 1 {
		t.Fatalf("serfs after hiring while broke = %d, want still 1 (hire must fail, not go into debt)", got)
	}
	if game.statusMsg == "" {
		t.Fatal("hiring while broke left statusMsg empty, want a message explaining why it failed")
	}
}

// TestFinishConstructionDoesNotAutoSpawnAWorker covers "юнита юзер создает
// отдельно за золото": a building completing construction must NOT get a
// resident automatically -- it stays empty until the player pays to hire
// someone into it, exactly like a workplace whose resident died.
// TestShowsAccessMarkerExcludesEveryDepositKind is a regression guard for
// a real "high CPU load, game hangs" bug: the per-frame access-marker loop
// in Draw excluded StoneDeposit but was never updated to exclude the three
// ore-family kinds when they shipped, so every ore/coal deposit triggered a
// full pathfind BFS (g.buildingConnected) 60 times a second. On the bigger
// procedural map, with hundreds of individual deposit Buildings, that's
// what actually caused the stall.
func TestShowsAccessMarkerExcludesEveryDepositKind(t *testing.T) {
	for _, kind := range []building.Kind{
		building.Road, building.Tree, building.Fish, building.Warehouse,
		building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit,
	} {
		if showsAccessMarker(kind) {
			t.Errorf("showsAccessMarker(%v) = true, want false", kind)
		}
	}
	for _, kind := range []building.Kind{
		building.Farm, building.LumberjackHut, building.Tavern, building.MinerHut, building.Smeltery,
	} {
		if !showsAccessMarker(kind) {
			t.Errorf("showsAccessMarker(%v) = false, want true", kind)
		}
	}
}

func TestFinishConstructionDoesNotAutoSpawnAWorker(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse, X: 0, Y: 0}
	hut := building.NewConstructionSite(building.LumberjackHut, 5, 0)
	game := &Game{
		buildings: []*building.Building{warehouse, hut},
		stock:     resource.NewStockpile(0),
		pop:       &economy.Population{},
		logi:      logistics.NewController(warehouse, 0),
		vills:     villagers.NewController(),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
		quarry:    quarry.NewController(),
		builders:  builder.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
	}

	game.finishConstruction(hut)

	if got := len(game.jacks.Lumberjacks); got != 0 {
		t.Fatalf("lumberjacks after finishConstruction = %d, want 0 (no auto-spawn -- must be hired separately)", got)
	}
}

// TestResetToNewGameDiscardsProgress covers the Esc pause menu's "New Game"
// button: it must throw away every bit of the running town's mutable state
// -- spent gold, deaths, the active selection, an open dialog -- and land
// back on a game indistinguishable from a fresh NewGame().
func TestResetToNewGameDiscardsProgress(t *testing.T) {
	g := NewGame()
	if !g.stock.Remove(resource.Gold, 50) {
		t.Fatal("setup: could not spend starting gold")
	}
	g.pop.Deaths = 7
	g.selection = ui.Selection{Kind: ui.SelectionBuilding, Building: g.buildings[0]}
	g.dialog = ui.DialogConfirmNewGame

	g.resetToNewGame()

	if got := g.stock.Amount(resource.Gold); got != startingGold {
		t.Fatalf("gold after reset = %d, want %d (fresh NewGame)", got, startingGold)
	}
	if g.pop.Deaths != 0 {
		t.Fatalf("deaths after reset = %d, want 0", g.pop.Deaths)
	}
	if g.dialog != ui.DialogNone {
		t.Fatalf("dialog after reset = %v, want DialogNone", g.dialog)
	}
	if g.selection.Kind != ui.SelectionNone {
		t.Fatalf("selection after reset = %v, want SelectionNone", g.selection.Kind)
	}
	if g.statusMsg == "" {
		t.Fatal("status message after reset is empty, want a confirmation message")
	}
}

// TestToggleAutosaveSlot_TogglesOnAndOff covers the pure state transitions
// behind the Esc pause menu's per-slot autosave toggle: picking a slot turns
// it on, picking the *same* slot again turns it off, and picking a
// *different* slot just switches the target directly without needing to
// be turned off first.
func TestToggleAutosaveSlot_TogglesOnAndOff(t *testing.T) {
	g := NewGame()
	if g.autosaveSlot != 0 {
		t.Fatalf("autosaveSlot on a fresh game = %d, want 0 (off by default)", g.autosaveSlot)
	}

	g.toggleAutosaveSlot(3)
	if g.autosaveSlot != 3 {
		t.Fatalf("autosaveSlot after toggling on slot 3 = %d, want 3", g.autosaveSlot)
	}
	if !g.slotCache[2].Autosave {
		t.Fatal("slotCache[2].Autosave = false after toggling slot 3 on, want true")
	}

	g.toggleAutosaveSlot(5)
	if g.autosaveSlot != 5 {
		t.Fatalf("autosaveSlot after toggling on slot 5 = %d, want 5 (switched, not merely added)", g.autosaveSlot)
	}
	if g.slotCache[2].Autosave {
		t.Fatal("slotCache[2].Autosave still true after switching the target to slot 5")
	}

	g.toggleAutosaveSlot(5)
	if g.autosaveSlot != 0 {
		t.Fatalf("autosaveSlot after toggling slot 5 off = %d, want 0", g.autosaveSlot)
	}
}

// TestTickAutosave_FiresAfterIntervalAndWritesTheSlot covers the actual
// timer: nothing happens before autosaveIntervalTicks calls, and the
// designated slot's file exists on disk right after the tick that crosses
// the threshold, using whatever name the slot already had.
func TestTickAutosave_FiresAfterIntervalAndWritesTheSlot(t *testing.T) {
	g := NewGame()
	const slot = 5
	path := slotPath(slot)
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })

	g.toggleAutosaveSlot(slot)

	for range autosaveIntervalTicks - 1 {
		g.tickAutosave()
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("autosave slot file exists before autosaveIntervalTicks was reached")
	}

	g.tickAutosave()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("autosave slot file missing after autosaveIntervalTicks ticks: %v", err)
	}
	if g.autosaveTicks != 0 {
		t.Fatalf("autosaveTicks after firing = %d, want reset to 0", g.autosaveTicks)
	}

	name, ok := save.PeekName(path)
	if !ok {
		t.Fatal("autosaved file has no readable name")
	}
	if want := fmt.Sprintf("%s %d", i18n.T().SlotDefaultName, slot); name != want {
		t.Fatalf("autosaved name = %q, want %q (default name on a slot's first save)", name, want)
	}
}

func findHireOption(t *testing.T, options []ui.HireOption, kind ui.HireKind) ui.HireOption {
	t.Helper()
	for _, o := range options {
		if o.Kind == kind {
			return o
		}
	}
	t.Fatalf("no hire option for kind %v", kind)
	return ui.HireOption{}
}

// TestScaledDepositCellsApplyPercentDirectly locks in the current
// generation formula per the user's explicit request ("камень я бы сделал
// 3-4%, уголь 2-3%, железная руда 1% и золото 1%"): both stone and ore
// apply their percentage straight to the map area, with no separate
// scaling knob layered underneath it any more (there used to be one --
// oreDepositGenerationPercent/stoneDepositGenerationPercent -- now
// removed as redundant once the percentages themselves were retuned
// directly).
func TestScaledDepositCellsApplyPercentDirectly(t *testing.T) {
	const area, percent = 10000, 10
	if got, want := scaledStoneDepositCells(area, percent), 1000; got != want {
		t.Fatalf("stone cells=%d, want %d (10%% of %d)", got, want, area)
	}
	if got, want := scaledDepositCells(area, percent), 1000; got != want {
		t.Fatalf("ore cells=%d, want %d (10%% of %d)", got, want, area)
	}
}

func TestBuildModePlacesRoadUnderWalkingUnitInsteadOfSelectingIt(t *testing.T) {
	game := NewGame()
	roadIndex := -1
	for index, kind := range game.palette.Kinds {
		if kind == building.Road {
			roadIndex = index
			break
		}
	}
	if roadIndex < 0 || len(game.logi.Serfs) == 0 {
		t.Fatal("test setup lacks a road palette item or serf")
	}

	mapRect := game.layout.MapRect()
	targetX, targetY := -1, -1
	for y := 0; y < game.grid.Height && targetX < 0; y++ {
		for x := 0; x < game.grid.Width; x++ {
			if !building.CanPlace(game.grid, game.buildings, building.Road, x, y) {
				continue
			}
			sx, sy := game.camera.TileToScreen(x, y)
			if sx >= float64(mapRect.Min.X) && sx < float64(mapRect.Max.X) && sy >= float64(mapRect.Min.Y) && sy < float64(mapRect.Max.Y) {
				targetX, targetY = x, y
				break
			}
		}
	}
	if targetX < 0 {
		t.Fatal("no visible buildable road tile found")
	}

	serf := game.logi.Serfs[0]
	serf.X, serf.Y = targetX, targetY
	game.palette.Select(roadIndex)
	game.buildMode = true
	before := len(game.buildings)
	sx, sy := game.camera.TileToScreen(targetX, targetY)
	game.handleLeftClick(int(sx)+1, int(sy)+1)

	if got := len(game.buildings); got != before+1 {
		t.Fatalf("building count after road click = %d, want %d", got, before+1)
	}
	if game.selection.Kind != ui.SelectionNone {
		t.Fatalf("selection after road click = %v, want none", game.selection.Kind)
	}
}

func TestReserveConstructionMaterialsUsesAvailableStockOnly(t *testing.T) {
	stock := resource.NewStockpile(0)
	stock.Add(resource.Plank, 3)
	stock.Add(resource.StoneBlock, 2)
	g := &Game{stock: stock}
	site := building.NewConstructionSite(building.Farm, 2, 3)

	g.reserveConstructionMaterials(site)

	if got := site.InputBuffer[resource.Plank]; got != 3 {
		t.Errorf("reserved planks = %d, want 3", got)
	}
	if got := site.InputBuffer[resource.StoneBlock]; got != 2 {
		t.Errorf("reserved stone blocks = %d, want 2", got)
	}
	if got := stock.Amount(resource.Plank); got != 0 {
		t.Errorf("stock planks after reservation = %d, want 0", got)
	}
	if got := stock.Amount(resource.StoneBlock); got != 0 {
		t.Errorf("stock stone blocks after reservation = %d, want 0", got)
	}
	if tip, short := advisor.ConstructionMaterialShortage(site); !short || tip.Resource != resource.Plank || tip.Missing != site.ConstructionMaterialCost(resource.Plank)-3 {
		t.Fatalf("shortage after partial reserve = %+v, %v; want remaining plank shortage", tip, short)
	}
}

// TestEnclosedGatherWorkerTip turns a closed wall into an actionable advisor
// message only when it affects a resource gatherer. Automatic gates make the
// region reachable again and therefore remove the warning.
func TestEnclosedGatherWorkerTip(t *testing.T) {
	grid := world.NewGrid(10, 10)
	hut := &building.Building{Kind: building.LumberjackHut, X: 4, Y: 4}
	buildings := []*building.Building{hut}
	for x := 2; x <= 7; x++ {
		buildings = append(buildings, &building.Building{Kind: building.StoneWall, X: x, Y: 2})
		buildings = append(buildings, &building.Building{Kind: building.StoneWall, X: x, Y: 7})
	}
	for y := 3; y <= 6; y++ {
		buildings = append(buildings, &building.Building{Kind: building.StoneWall, X: 2, Y: y})
		buildings = append(buildings, &building.Building{Kind: building.StoneWall, X: 7, Y: y})
	}
	game := &Game{
		grid:      grid,
		buildings: buildings,
		jacks:     lumberjack.NewController(),
		quarry:    quarry.NewController(),
		miners:    miner.NewController(),
		sentries:  sentry.NewController(),
	}
	game.jacks.Spawn(hut)
	tip, blocked := game.enclosedGatherWorkerTip()
	if !blocked || tip.Kind != advisor.KindGatherWorkerEnclosed || tip.Count != 1 || tip.Building != hut {
		t.Fatalf("enclosedGatherWorkerTip() = %+v, %v; want one lumberjack at its hut", tip, blocked)
	}
	for _, b := range game.buildings {
		if b.X == 4 && b.Y == 2 {
			b.Kind = building.Gate
			b.GateAuto = true
			break
		}
	}
	if tip, blocked = game.enclosedGatherWorkerTip(); blocked {
		t.Fatalf("automatic gate should clear enclosure tip, got %+v", tip)
	}
}

// TestCommandSelectedEnemyTo_IssuesAMoveOrder exercises the user's
// explicit "выбрал противника, кликнул ПКМ, противник идёт туда" flow
// end to end through the real screen-coordinate path (camera.ScreenToTile
// -> Enemy.MoveTo), the same call chain handleMouse's right-click branch
// uses -- not just internal/enemy's own unit test, which never touches
// cmd/game's plumbing at all.
func TestCommandSelectedEnemyTo_IssuesAMoveOrder(t *testing.T) {
	grid := world.NewGrid(10, 10)
	game := &Game{
		grid:    grid,
		camera:  render.NewCamera(),
		enemies: []*enemy.Enemy{enemy.New(0, 0)},
	}
	e := game.enemies[0]
	game.selection = ui.Selection{Kind: ui.SelectionEnemy, Enemy: e}

	sx, sy := game.camera.TileToScreen(5, 5)
	game.commandSelectedEnemyTo(int(sx), int(sy))

	if len(e.RemainingPath()) == 0 {
		t.Fatal("RemainingPath() is empty after commandSelectedEnemyTo -- no move order was issued")
	}
}
