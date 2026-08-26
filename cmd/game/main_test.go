package main

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/render"
	"strategy_game/internal/resource"
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

// TestSelectionAt_VisibleFarmerStillSelectable is the companion case: a
// farmer actively tending a field cell IS drawn there, so clicking that
// exact tile should still select the farmer, not fall through to the
// Farm building underneath.
func TestSelectionAt_VisibleFarmerStillSelectable(t *testing.T) {
	farm := &building.Building{Kind: building.Farm, X: 0, Y: 0}
	game := &Game{
		buildings: []*building.Building{farm},
		vills:     villagers.NewController(),
		logi:      logistics.NewController(&building.Building{Kind: building.Warehouse}, 0),
		jacks:     lumberjack.NewController(),
		fishers:   fishing.NewController(),
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
	if got.Kind != ui.SelectionVillager || got.Villager != farmer {
		t.Fatalf("selection = %+v, want the visible farmer", got)
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
	}
	restored.restoreUnits(source.serializeUnits(), buildings)
	if got := len(restored.logi.Serfs); got != 1 {
		t.Fatalf("restored serfs = %d, want 1", got)
	}
	if !restored.logi.Serfs[0].Dismissing() {
		t.Fatal("dismissal flag was lost during save/restore")
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
	}

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
