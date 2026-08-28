// Command game is the entry point for the strategy game.
package main

import (
	"errors"
	"fmt"
	"image"
	"log"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/i18n"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/miner"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/quarry"
	"strategy_game/internal/render"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/save"
	"strategy_game/internal/ui"
	"strategy_game/internal/villagers"
	"strategy_game/internal/world"
)

const (
	screenWidth  = 1024
	screenHeight = 768
	panSpeed     = 8 // pixels per frame while a pan key is held

	framesPerSimTick = 30 // simulation ticks run at 2/sec on a 60fps display

	// A warehouse is intentionally unlimited. A positive capacity is still
	// supported by resource.Stockpile for isolated tests and future stores.
	stockpileCapacity = 0
	startingSerfs     = 3

	// mapWidth/mapHeight is the fixed size of every procedurally generated
	// map (see generateGrid) -- bigger than the original 40x30 hand-built
	// test map, per the roadmap's "карта большего размера". Only the
	// terrain layout within this fixed size is randomized per game, not
	// the dimensions themselves.
	mapWidth  = 100
	mapHeight = 75

	// Named save-panel slots (side panel, settings tab) live in their own
	// files. Saving/loading is mouse-only through the Settings tab -- there
	// is deliberately no keyboard-shortcut quicksave file alongside them.
	slotPathFormat = "saves/panel_slot_%d.json"
	slotCount      = 5
	maxSlotNameLen = 10 // runes, not bytes -- a Cyrillic name still counts as 10 letters

	defaultTreeSeed            uint32 = 0x4d595df4
	treeRegrowthMinTicks       int    = 180 // 90 seconds at normal speed
	treeRegrowthVariationTicks int    = 180 // total wait is about 90–180 seconds
	treeRegrowthRetryTicks     int    = 30  // retry every 15 seconds if the map is full

	defaultFishSeed            uint32 = 0x6a09e667
	fishRegrowthMinTicks       int    = 180 // fry appears after 90-180 seconds
	fishRegrowthVariationTicks int    = 180
	fishRegrowthRetryTicks     int    = 30

	// defaultStoneSeed picks the one-time shape of the stone region. Unlike
	// the tree/fish seeds it is never advanced or persisted: deposits don't
	// regrow, so once they're placed as ordinary Buildings, the exact seed
	// that produced them no longer matters -- see save.GameState.StoneSeeded.
	defaultStoneSeed uint32 = 0x1b873593

	// defaultCoalSeed/defaultGoldOreSeed/defaultIronOreSeed are the
	// one-time shapes of the three ore-family regions, for the same reason
	// defaultStoneSeed is never advanced or persisted -- see
	// save.GameState.OreSeeded.
	defaultCoalSeed    uint32 = 0x9e3779b9
	defaultGoldOreSeed uint32 = 0x85ebca6b
	defaultIronOreSeed uint32 = 0xc2b2ae35

	// Ore-family abundance ranges (percent of map area), per the game
	// design: Coal is the most common -- both Smeltery recipes consume it
	// -- Gold ore the rarest, since it smelts directly into the hiring
	// currency.
	coalMinPercent, coalMaxPercent       = 4, 8
	ironOreMinPercent, ironOreMaxPercent = 2, 4
	goldOreMinPercent, goldOreMaxPercent = 1, 2

	// seaMinPercent/seaMaxPercent bound the one sea's share of the map's
	// area. Per the roadmap ("водоёмы генерируются у края карты, а не где
	// придётся") the sea always grows inward from a random map edge rather
	// than sitting anywhere, so reaching the coast is always a deliberate
	// road, never something the town happens to already border. The user
	// asked to specifically think through its size: big enough to read as
	// a real coastline and comfortably support several Fisher Huts along
	// it (at 8-14% of a 7500-cell map that's 600-1050 water cells, the
	// same order of magnitude as the stone/ore abundance ranges below),
	// small enough that most of the map stays dry, buildable land.
	seaMinPercent, seaMaxPercent = 8, 14

	// fertileMinPercent/fertileMaxPercent size the map's cosmetic tilled-soil
	// patches. Purely visual -- building.Types[Farm].AllowedTerrain is empty,
	// so a Farm can be placed on any dry tile regardless of terrain -- kept
	// only so a procedural map still has the "here's good farmland" visual
	// cue the original hand-built map had.
	fertileMinPercent, fertileMaxPercent = 6, 10
	fertileRegionCount                   = 2

	// Thickets ("чащи" in the roadmap) are a handful of deliberately dense
	// tree zones, layered on top of -- not instead of -- seedTrees' existing
	// map-wide ~1% uniform scatter. thicketDensityPercent is the chance any
	// one cell inside a zone actually gets a tree, so a thicket reads as a
	// dense grove rather than an unbroken wall of trunks.
	thicketZoneCount                             = 3
	thicketZoneMinPercent, thicketZoneMaxPercent = 3, 5
	thicketDensityPercent                        = 45

	// minDepositDistanceFromWarehouse keeps every finite deposit kind --
	// stone, coal, gold ore, iron ore -- away from the town's starting
	// Warehouse, per the game design ("уголь, камень, руды... должны быть
	// удалены от первоначального склада, минимум 20 клеток").
	minDepositDistanceFromWarehouse = 20

	// maxBuilders is a flat town-wide cap, unlike every other profession
	// (which is capped by matching building count instead) -- a Builder has
	// no dedicated hut to be limited by.
	maxBuilders = 3

	// Starting stockpile: enough construction material for several ordinary
	// buildings or one fenced one (see building.Type's PlankCost/StoneCost),
	// plus a first batch of every food so an early Tavern isn't immediately
	// empty while its own supply chains are still being built.
	startingPlanks  = 200
	startingStone   = 100
	startingBread   = 100
	startingFish    = 100
	startingSausage = 100
	startingWine    = 100
	startingGold    = 100

	// unitHireCost is spent from the shared stockpile every time the player
	// creates a unit -- serf, any profession, or builder -- through the
	// Hire tab or the H shortcut. A building finishing construction no
	// longer spawns its resident automatically (see finishConstruction);
	// staffing it is always this same paid action.
	unitHireCost = 1
)

type treeRegrowth struct {
	ticks, target int
	seed          uint32
}

type fishRegrowth struct {
	waterX, waterY int
	ticks, target  int
	seed           uint32
}

// Game implements ebiten.Game and wires the logic packages (world,
// building, resource, economy, logistics) to rendering and input.
type Game struct {
	grid      *world.Grid
	buildings []*building.Building
	stock     *resource.Stockpile
	pop       *economy.Population
	sim       *economy.Simulator
	logi      *logistics.Controller
	vills     *villagers.Controller
	jacks     *lumberjack.Controller
	fishers   *fishing.Controller
	quarry    *quarry.Controller
	builders  *builder.Controller
	miners    *miner.Controller

	treeRegrowth []treeRegrowth
	treeSeed     uint32
	fishRegrowth []fishRegrowth
	fishSeed     uint32
	// stoneSeeded mirrors save.GameState.StoneSeeded: true once this world
	// has a stone-deposit region, so loading a save never regenerates one
	// over a legitimately fully-mined town. Always true after NewGame.
	stoneSeeded bool
	// oreSeeded mirrors save.GameState.OreSeeded the same way, for the
	// Coal/GoldOre/IronOre regions together.
	oreSeeded bool

	camera        *render.Camera
	palette       *ui.Palette
	layout        ui.Layout
	leftTab       ui.LeftTab
	buildMode     bool
	selection     ui.Selection
	middlePanning bool
	lastMouseX    int
	lastMouseY    int

	// Settings tab save/load modal: dialog is DialogNone outside of the
	// naming/overwrite flow, in which case dialogSlot/dialogText are unused.
	// slotCache holds the five save-panel slots' names and occupancy, kept
	// current so the settings tab can redraw it every frame without paying
	// the cost of reading and parsing five files every frame -- see
	// refreshSlotCache.
	dialog     ui.DialogKind
	dialogSlot int
	dialogText string
	slotCache  []ui.SaveSlotInfo

	statusMsg string
}

func NewGame() *Game {
	mapSeed := newMapSeed()
	grid := generateGrid(mapWidth, mapHeight, mapSeed)

	warehousePoint, ok := findWarehouseSpot(grid)
	if !ok {
		// Should be unreachable given seaMaxPercent+fertileMaxPercent leaves
		// most of the map as plain grass -- but a game must still start
		// rather than panic if generation ever produces a map this crowded.
		warehousePoint = gridPoint{grid.Width / 2, grid.Height / 2}
	}
	warehouse := &building.Building{Kind: building.Warehouse, X: warehousePoint.x, Y: warehousePoint.y}
	initialRoad := &building.Building{Kind: building.Road, X: warehousePoint.x, Y: warehousePoint.y + 1}

	buildings := []*building.Building{warehouse, initialRoad}
	// Trees are sparse persistent world objects, scattered across all free
	// dry cells rather than confined to a special forest area.
	buildings = seedTrees(grid, buildings)
	buildings = seedThickets(grid, buildings, mapSeed^0x27d4eb2f)
	buildings = seedFish(grid, buildings)
	buildings = seedStoneDeposits(grid, buildings, defaultStoneSeed, warehousePoint, minDepositDistanceFromWarehouse)
	buildings = seedOreDeposits(grid, buildings, building.CoalDeposit, coalMinPercent, coalMaxPercent, defaultCoalSeed, warehousePoint, minDepositDistanceFromWarehouse)
	buildings = seedOreDeposits(grid, buildings, building.GoldOreDeposit, goldOreMinPercent, goldOreMaxPercent, defaultGoldOreSeed, warehousePoint, minDepositDistanceFromWarehouse)
	buildings = seedOreDeposits(grid, buildings, building.IronOreDeposit, ironOreMinPercent, ironOreMaxPercent, defaultIronOreSeed, warehousePoint, minDepositDistanceFromWarehouse)

	stock := resource.NewStockpile(stockpileCapacity)
	stock.Add(resource.Plank, startingPlanks)
	stock.Add(resource.StoneBlock, startingStone)
	stock.Add(resource.Bread, startingBread)
	stock.Add(resource.Fish, startingFish)
	stock.Add(resource.Sausage, startingSausage)
	stock.Add(resource.Wine, startingWine)
	stock.Add(resource.Gold, startingGold)

	layout := ui.NewLayout(screenWidth, screenHeight)
	camera := render.NewCamera()
	mapRect := layout.MapRect()
	camera.SetViewport(mapRect.Min.X, mapRect.Min.Y, mapRect.Dx(), mapRect.Dy())
	// Shift the initial view slightly so the town and nearby scattered trees
	// are visible without requiring the player to discover camera panning first.
	camera.Pan(float64(4*render.TileSize), 0, grid.Width, grid.Height, mapRect.Dx(), mapRect.Dy())

	game := &Game{
		grid:        grid,
		buildings:   buildings,
		stock:       stock,
		pop:         &economy.Population{},
		sim:         economy.NewSimulator(framesPerSimTick),
		logi:        logistics.NewController(warehouse, startingSerfs),
		vills:       villagers.NewController(),
		jacks:       lumberjack.NewController(),
		fishers:     fishing.NewController(),
		quarry:      quarry.NewController(),
		builders:    builder.NewController(),
		miners:      miner.NewController(),
		treeSeed:    defaultTreeSeed,
		fishSeed:    defaultFishSeed,
		stoneSeeded: true,
		oreSeeded:   true,
		camera:      camera,
		palette:     ui.NewPalette(),
		layout:      layout,
	}
	game.refreshPopulation()
	game.refreshSlotCache()
	return game
}

func (g *Game) Update() error {
	if width, height := ebiten.WindowSize(); width > 0 && height > 0 {
		g.resizeLayout(width, height)
	}
	g.handleCameraPan()
	g.handleCameraZoom()

	// While the settings tab's save/load modal is open, it owns every key
	// and click: typing a name must not also select a build-palette item
	// (digit keys), hire a serf (H), or delete the current selection.
	if g.dialog != ui.DialogNone {
		g.handleDialogInput()
	} else {
		g.handleMouse()
		g.handleUnitActions()
	}

	for range g.sim.Advance() {
		for _, b := range g.buildings {
			b.TickGrowth()
		}
		g.tickTreeRegrowth()
		g.tickFishRegrowth()
		economy.TickWithConnectivity(g.buildings, g.inactiveWorkerBuildings(), g.disconnectedBuildings())

		// One shared reservation ledger per simulation tick: every
		// controller first reports its own pre-existing in-flight units
		// (Reserve), then all three consume the same ledger in Tick, so
		// a serf and a hungry farmer never both set off for the
		// Tavern's last meal at once. See package reservations.
		ledger := reservations.New()
		g.logi.Reserve(ledger)
		g.vills.Reserve(ledger)
		g.jacks.Reserve(ledger)
		g.fishers.Reserve(ledger)
		g.quarry.Reserve(ledger)
		g.builders.Reserve(ledger)
		g.miners.Reserve(ledger)

		// Whichever controller's Tick runs first this simulation tick
		// effectively wins any contention over shared Tavern food: its
		// claims land in the ledger before the next controller even
		// looks. Ordering by MaxWaitingHunger means the unit that's
		// actually been waiting longest gets first claim -- not just
		// whichever unit type happens to be ticked first every time.
		var jackEvents []lumberjack.Event
		var fishEvents []fishing.Event
		var quarryEvents []quarry.Event
		var builderEvents []builder.Event
		var minerEvents []miner.Event
		var serfResult logistics.TickResult
		var villagerDeaths int
		type unitStep struct {
			hunger int
			run    func()
		}
		steps := []unitStep{
			{g.logi.MaxWaitingHunger(), func() { serfResult = g.logi.Tick(g.grid, g.buildings, g.stock, ledger) }},
			{g.vills.MaxWaitingHunger(), func() { villagerDeaths = g.vills.Tick(g.buildings, ledger) }},
			{g.jacks.MaxWaitingHunger(), func() { jackEvents = g.jacks.Tick(g.grid, g.buildings, ledger) }},
			{g.fishers.MaxWaitingHunger(), func() { fishEvents = g.fishers.Tick(g.grid, g.buildings, ledger) }},
			{g.quarry.MaxWaitingHunger(), func() { quarryEvents = g.quarry.Tick(g.grid, g.buildings, ledger) }},
			{g.builders.MaxWaitingHunger(), func() { builderEvents = g.builders.Tick(g.grid, g.buildings, ledger) }},
			{g.miners.MaxWaitingHunger(), func() { minerEvents = g.miners.Tick(g.grid, g.buildings, ledger) }},
		}
		sort.SliceStable(steps, func(i, j int) bool { return steps[i].hunger > steps[j].hunger })
		for _, step := range steps {
			step.run()
		}
		g.pop.Deaths += serfResult.Deaths + villagerDeaths
		g.pop.Removed += serfResult.Dismissed
		for _, event := range jackEvents {
			switch event.Kind {
			case lumberjack.TreeCut:
				g.cutTree(event.Tree)
			case lumberjack.WorkerDied:
				g.pop.Deaths++
				if event.Cargo > 0 {
					g.stock.Add(resource.Log, event.Cargo)
				}
			}
		}
		for _, event := range fishEvents {
			switch event.Kind {
			case fishing.FishCaught:
				g.catchFish(event.Fish)
			case fishing.WorkerDied:
				g.pop.Deaths++
				if event.Cargo > 0 {
					g.stock.Add(resource.Fish, event.Cargo)
				}
			}
		}
		for _, event := range quarryEvents {
			switch event.Kind {
			case quarry.DepositExhausted:
				g.removeDeposit(event.Deposit)
			case quarry.WorkerDied:
				g.pop.Deaths++
				if event.Cargo > 0 {
					// event.Cargo is the worker's own accessor, already
					// reported in finished Stone Blocks -- see
					// quarry.Quarryman.Cargo's doc comment.
					g.stock.Add(resource.StoneBlock, event.Cargo)
				}
			}
		}
		for _, event := range builderEvents {
			switch event.Kind {
			case builder.ConstructionComplete:
				g.finishConstruction(event.Building)
			case builder.WorkerDied:
				g.pop.Deaths++
			}
		}
		for _, event := range minerEvents {
			switch event.Kind {
			case miner.DepositExhausted:
				g.removeDeposit(event.Deposit)
			case miner.WorkerDied:
				g.pop.Deaths++
				if event.Cargo > 0 {
					// A miner carries raw ore/coal as-is -- no conversion
					// step like a quarryman's stone-to-blocks, and unlike
					// every other gathering profession he can be carrying
					// any of three different resources, so which one
					// travels with the event (CargoResource).
					g.stock.Add(event.CargoResource, event.Cargo)
				}
			}
		}
		g.clearMissingUnitSelection()
		g.refreshPopulation()
	}

	if g.dialog == ui.DialogNone && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	return nil
}

func (g *Game) resizeLayout(width, height int) {
	if width <= 0 || height <= 0 || (g.layout.Width == width && g.layout.Height == height) {
		return
	}
	g.layout = ui.NewLayout(width, height)
	mapRect := g.layout.MapRect()
	g.camera.SetViewport(mapRect.Min.X, mapRect.Min.Y, mapRect.Dx(), mapRect.Dy())
	g.camera.Pan(0, 0, g.grid.Width, g.grid.Height, mapRect.Dx(), mapRect.Dy())
}

// inactiveWorkerBuildings marks every worker building whose resident is
// missing or temporarily away from its post. This is deliberately NOT the
// same as Starving: a worker who is hungry but still at home keeps producing
// until they leave for the Tavern. Starting with all RequiresWorker buildings
// inactive is important after a death: otherwise an empty bakery or farm
// would continue its production cycle invisibly.
func (g *Game) inactiveWorkerBuildings() map[*building.Building]bool {
	m := make(map[*building.Building]bool)
	for _, b := range g.buildings {
		if building.Types[b.Kind].RequiresWorker {
			m[b] = true
		}
	}
	for _, v := range g.vills.Villagers {
		if v.Home != nil {
			m[v.Home] = !v.Working()
		}
	}
	for _, j := range g.jacks.Lumberjacks {
		if j.HomeBuilding() != nil {
			m[j.HomeBuilding()] = !j.AtPost()
		}
	}
	for _, f := range g.fishers.Fishermen {
		if f.HomeBuilding() != nil {
			m[f.HomeBuilding()] = !f.AtPost()
		}
	}
	for _, q := range g.quarry.Quarrymen {
		if q.HomeBuilding() != nil {
			m[q.HomeBuilding()] = !q.AtPost()
		}
	}
	for _, mn := range g.miners.Miners {
		if mn.HomeBuilding() != nil {
			m[mn.HomeBuilding()] = !mn.AtPost()
		}
	}
	return m
}

// unstaffedWorkerBuildings distinguishes a permanently empty workplace from
// a resident who only stepped out to eat. The renderer uses it for the red
// building tint; gameplay pausing still uses inactiveWorkerBuildings above.
func (g *Game) unstaffedWorkerBuildings() map[*building.Building]bool {
	m := make(map[*building.Building]bool)
	for _, b := range g.buildings {
		if building.Types[b.Kind].RequiresWorker {
			m[b] = true
		}
	}
	for _, v := range g.vills.Villagers {
		if v.Home != nil {
			m[v.Home] = false
		}
	}
	for _, j := range g.jacks.Lumberjacks {
		if j.HomeBuilding() != nil {
			m[j.HomeBuilding()] = false
		}
	}
	for _, f := range g.fishers.Fishermen {
		if f.HomeBuilding() != nil {
			m[f.HomeBuilding()] = false
		}
	}
	for _, q := range g.quarry.Quarrymen {
		if q.HomeBuilding() != nil {
			m[q.HomeBuilding()] = false
		}
	}
	for _, mn := range g.miners.Miners {
		if mn.HomeBuilding() != nil {
			m[mn.HomeBuilding()] = false
		}
	}
	return m
}

// hireOptions reports the current headcount, building-based limit, and
// availability for every hireable unit kind, for the left panel's Hire tab.
// Serfs are the only unlimited option; every profession is capped at one
// worker per matching *finished* building (Spawn/HasHome already enforce
// the one-to-one rule -- this just surfaces it to the player before they
// click). A building still under construction doesn't count: it has no
// resident to hire into yet, see finishConstruction. Every card is also
// gated on canAffordHire, so a card the player can't currently pay for
// reads as unavailable even when a vacancy exists.
func (g *Game) hireOptions() []ui.HireOption {
	countBuildings := func(kind building.Kind) int {
		n := 0
		for _, b := range g.buildings {
			if b.Kind == kind && b.ConstructionStage == building.ConstructionNone {
				n++
			}
		}
		return n
	}
	countProfession := func(p villagers.Profession) int {
		n := 0
		for _, v := range g.vills.Villagers {
			if v.Profession == p {
				n++
			}
		}
		return n
	}
	afford := g.canAffordHire()
	limited := func(kind ui.HireKind, bKind building.Kind, current int) ui.HireOption {
		limit := countBuildings(bKind)
		return ui.HireOption{Kind: kind, Current: current, Limit: limit, Available: current < limit && afford}
	}
	return []ui.HireOption{
		{Kind: ui.HireSerf, Current: len(g.logi.Serfs), Limit: 0, Available: afford},
		limited(ui.HireFarmer, building.Farm, countProfession(villagers.Farmer)),
		limited(ui.HireBaker, building.Bakery, countProfession(villagers.Baker)),
		limited(ui.HireWinemaker, building.Winery, countProfession(villagers.Winemaker)),
		limited(ui.HireLumberjack, building.LumberjackHut, len(g.jacks.Lumberjacks)),
		limited(ui.HireFisherman, building.FisherHut, len(g.fishers.Fishermen)),
		limited(ui.HireSwineherd, building.PigFarm, countProfession(villagers.Swineherd)),
		limited(ui.HireButcher, building.MeatWorkshop, countProfession(villagers.Butcher)),
		limited(ui.HireCarpenter, building.CarpentryWorkshop, countProfession(villagers.Carpenter)),
		limited(ui.HireQuarryman, building.QuarryHut, len(g.quarry.Quarrymen)),
		{Kind: ui.HireBuilder, Current: len(g.builders.Builders), Limit: maxBuilders, Available: len(g.builders.Builders) < maxBuilders && afford},
		limited(ui.HireMiner, building.MinerHut, len(g.miners.Miners)),
		limited(ui.HireSmelter, building.Smeltery, countProfession(villagers.Smelter)),
	}
}

// hireFromTab executes a click on an available Hire-tab card. It finds the
// first matching finished building without a resident, spends unitHireCost,
// and assigns a fresh worker to it -- the only way a building ever gets its
// resident now (see finishConstruction), so a freshly built empty workplace
// and one whose worker died look identical to this search.
func (g *Game) hireFromTab(kind ui.HireKind) {
	switch kind {
	case ui.HireSerf:
		g.hireSerf()
	case ui.HireFarmer:
		g.hireVillagerInto(villagers.Farmer, building.Farm)
	case ui.HireBaker:
		g.hireVillagerInto(villagers.Baker, building.Bakery)
	case ui.HireWinemaker:
		g.hireVillagerInto(villagers.Winemaker, building.Winery)
	case ui.HireSwineherd:
		g.hireVillagerInto(villagers.Swineherd, building.PigFarm)
	case ui.HireButcher:
		g.hireVillagerInto(villagers.Butcher, building.MeatWorkshop)
	case ui.HireCarpenter:
		g.hireVillagerInto(villagers.Carpenter, building.CarpentryWorkshop)
	case ui.HireSmelter:
		g.hireVillagerInto(villagers.Smelter, building.Smeltery)
	case ui.HireLumberjack:
		for _, b := range g.buildings {
			if b.Kind == building.LumberjackHut && b.ConstructionStage == building.ConstructionNone && !g.jacks.HasHome(b) {
				if !g.trySpendGold() {
					return
				}
				g.jacks.Spawn(b)
				g.refreshPopulation()
				g.statusMsg = ""
				return
			}
		}
	case ui.HireFisherman:
		for _, b := range g.buildings {
			if b.Kind == building.FisherHut && b.ConstructionStage == building.ConstructionNone && !g.fishers.HasHome(b) {
				if !g.trySpendGold() {
					return
				}
				g.fishers.Spawn(b)
				g.refreshPopulation()
				g.statusMsg = ""
				return
			}
		}
	case ui.HireQuarryman:
		for _, b := range g.buildings {
			if b.Kind == building.QuarryHut && b.ConstructionStage == building.ConstructionNone && !g.quarry.HasHome(b) {
				if !g.trySpendGold() {
					return
				}
				g.quarry.Spawn(b)
				g.refreshPopulation()
				g.statusMsg = ""
				return
			}
		}
	case ui.HireBuilder:
		if len(g.builders.Builders) < maxBuilders {
			if !g.trySpendGold() {
				return
			}
			g.builders.Hire(g.logi.Warehouse)
			g.refreshPopulation()
			g.statusMsg = ""
		}
	case ui.HireMiner:
		for _, b := range g.buildings {
			if b.Kind == building.MinerHut && b.ConstructionStage == building.ConstructionNone && !g.miners.HasHome(b) {
				if !g.trySpendGold() {
					return
				}
				g.miners.Spawn(b)
				g.refreshPopulation()
				g.statusMsg = ""
				return
			}
		}
	}
}

func (g *Game) hireVillagerInto(profession villagers.Profession, kind building.Kind) {
	for _, b := range g.buildings {
		if b.Kind == kind && b.ConstructionStage == building.ConstructionNone && !g.vills.HasHome(b) {
			if !g.trySpendGold() {
				return
			}
			g.vills.Spawn(profession, b)
			g.refreshPopulation()
			g.statusMsg = ""
			return
		}
	}
}

// disconnectedBuildings reports production buildings whose access tile is
// not connected to the Warehouse by a continuous road network. The map is
// recalculated at the simulation boundary, so a newly completed road starts
// the next production tick without requiring any extra state in building.
func (g *Game) disconnectedBuildings() map[*building.Building]bool {
	m := make(map[*building.Building]bool)
	for _, b := range g.buildings {
		if building.Types[b.Kind].Recipe.TicksToProduce > 0 && !g.buildingConnected(b) {
			m[b] = true
		}
	}
	return m
}

func (g *Game) handleCameraPan() {
	var dx, dy float64
	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		dx -= panSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) {
		dx += panSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		dy -= panSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		dy += panSpeed
	}
	if dx != 0 || dy != 0 {
		mapRect := g.layout.MapRect()
		g.camera.Pan(dx, dy, g.grid.Width, g.grid.Height, mapRect.Dx(), mapRect.Dy())
	}

	mx, my := ebiten.CursorPosition()
	mapPoint := image.Pt(mx, my)
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		if !g.middlePanning {
			if !mapPoint.In(g.layout.MapRect()) {
				return
			}
			g.middlePanning = true
			g.lastMouseX, g.lastMouseY = mx, my
		} else {
			mapRect := g.layout.MapRect()
			g.camera.Pan(float64(g.lastMouseX-mx), float64(g.lastMouseY-my), g.grid.Width, g.grid.Height, mapRect.Dx(), mapRect.Dy())
			g.lastMouseX, g.lastMouseY = mx, my
		}
	} else {
		g.middlePanning = false
	}
}

// handleCameraZoom accepts both the mouse wheel and keyboard shortcuts.
// Zooming is limited to the map viewport so a wheel gesture over a side
// panel never changes the inspector's apparent scale.
func (g *Game) handleCameraZoom() {
	mx, my := ebiten.CursorPosition()
	if !image.Pt(mx, my).In(g.layout.MapRect()) {
		return
	}
	_, wheelY := ebiten.Wheel()
	delta := wheelY
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) {
		delta = 1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) {
		delta = -1
	}
	if delta != 0 {
		mapRect := g.layout.MapRect()
		g.camera.ZoomAt(delta, mx, my, g.grid.Width, g.grid.Height)
		g.camera.SetViewport(mapRect.Min.X, mapRect.Min.Y, mapRect.Dx(), mapRect.Dy())
	}
}

func (g *Game) handleMouse() {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		g.buildMode = false
		g.selection.Clear()
		g.statusMsg = ""
		return
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	mx, my := ebiten.CursorPosition()
	if tab, ok := g.layout.MenuTabAt(mx, my); ok {
		g.leftTab = tab
		g.statusMsg = ""
		return
	}
	switch g.leftTab {
	case ui.HireTab:
		options := g.hireOptions()
		if index, ok := g.layout.HireIndexAt(mx, my, len(options)); ok {
			if index < len(options) && options[index].Available {
				g.hireFromTab(options[index].Kind)
			}
			return
		}
	case ui.SettingsTab:
		if lang, ok := g.layout.SettingsLangAt(mx, my); ok {
			i18n.SetLang(lang)
			return
		}
		if speed, ok := g.layout.SettingsSpeedAt(mx, my); ok {
			g.sim.SetSpeed(speed)
			return
		}
		if g.layout.SettingsNewGameAt(mx, my) {
			g.dialog = ui.DialogConfirmNewGame
			return
		}
		if slot, action, ok := g.layout.SettingsSlotActionAt(mx, my); ok {
			g.handleSettingsSlotAction(slot, action)
			return
		}
	default:
		if index, ok := g.layout.BuildIndexAt(mx, my, len(g.palette.Kinds)); ok {
			g.palette.Select(index)
			g.buildMode = true
			g.statusMsg = ""
			return
		}
	}
	if speed, ok := g.layout.SpeedAt(mx, my); ok {
		g.sim.SetSpeed(speed)
		return
	}
	if g.layout.HireAt(mx, my) {
		g.hireSerf()
		return
	}
	if g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil && priorityEligible(g.selection.Building.Kind) {
		if level, ok := g.layout.PriorityLevelAt(mx, my); ok {
			g.logi.SetPriority(g.selection.Building.Kind, level)
			return
		}
	}
	point := image.Pt(mx, my)
	if point.In(g.layout.LeftPanel()) ||
		point.In(g.layout.RightPanel()) ||
		point.In(g.layout.BottomPanel()) {
		return
	}
	if selected := g.selectionAt(mx, my); selected.Kind != ui.SelectionNone {
		g.selection = selected
		g.buildMode = false
		return
	}
	g.selection.Clear()
	if !g.buildMode {
		return
	}

	tx, ty := g.camera.ScreenToTile(mx, my)

	kind := g.palette.SelectedKind()
	if !building.CanPlace(g.grid, g.buildings, kind, tx, ty) {
		g.statusMsg = i18n.T().CantBuildHere
		return
	}
	// Placement only reserves the footprint and starts a construction site
	// (see package builder) -- it neither produces nor, for a Warehouse,
	// acts as a logistics endpoint until a Builder actually finishes it;
	// see finishConstruction.
	placed := building.NewConstructionSite(kind, tx, ty)
	g.buildings = append(g.buildings, placed)
	g.statusMsg = ""
}

func (g *Game) handleUnitActions() {
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		g.hireSerf()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) {
		if g.selection.Kind == ui.SelectionSerf && g.selection.Serf != nil {
			if g.logi.RequestDismissal(g.selection.Serf) {
				g.statusMsg = i18n.T().SerfDismissRequested
			}
			return
		}
		g.deleteSelectedBuilding()
	}
}

// canAffordHire reports whether the stockpile can currently cover one more
// unitHireCost. Used both to gate the hire actions themselves and to grey
// out a Hire-tab card before the player even clicks it (see hireOptions).
func (g *Game) canAffordHire() bool {
	return g.stock.Amount(resource.Gold) >= unitHireCost
}

// trySpendGold deducts unitHireCost from the stockpile and reports success.
// On failure it leaves the stockpile untouched and sets a status message,
// so every hire call site can just `if !g.trySpendGold() { return }` before
// actually creating the unit.
func (g *Game) trySpendGold() bool {
	if !g.stock.Remove(resource.Gold, unitHireCost) {
		g.statusMsg = i18n.T().NotEnoughGold
		return false
	}
	return true
}

func (g *Game) hireSerf() {
	if !g.trySpendGold() {
		return
	}
	g.logi.Hire()
	g.refreshPopulation()
	g.statusMsg = ""
}

// deleteSelectedBuilding removes the selected building and invalidates all
// active routes before the slice is changed. Every building can be removed
// except the final Warehouse: with several Warehouses, the logistics
// controller first promotes one of the survivors to be the new root.
func (g *Game) deleteSelectedBuilding() {
	if g.selection.Kind != ui.SelectionBuilding || g.selection.Building == nil {
		return
	}
	b := g.selection.Building
	if b.ConstructionStage != building.ConstructionNone {
		// A cancelled construction site was never registered as a
		// Warehouse or given a resident (see finishConstruction), so none
		// of the kind-specific protections below apply. Any materials
		// already delivered go back to the stockpile, exactly like a
		// cancelled haul returns carried cargo elsewhere.
		g.stock.Add(resource.Plank, b.InputBuffer[resource.Plank])
		g.stock.Add(resource.StoneBlock, b.InputBuffer[resource.StoneBlock])
		g.builders.CancelRouteTo(b)
		g.logi.CancelAllJobs(g.stock)
		for i, candidate := range g.buildings {
			if candidate != b {
				continue
			}
			g.buildings = append(g.buildings[:i], g.buildings[i+1:]...)
			g.selection.Clear()
			g.statusMsg = i18n.T().Deleted
			return
		}
		return
	}
	if b.Kind == building.Tree {
		g.statusMsg = i18n.T().CannotDeleteTree
		return
	}
	if b.Kind == building.Fish {
		g.statusMsg = i18n.T().CannotDeleteFish
		return
	}
	switch b.Kind {
	case building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
		g.statusMsg = i18n.T().CannotDeleteStoneDeposit
		return
	}

	if b.Kind == building.Warehouse {
		if !g.logi.RemoveWarehouse(b) {
			g.statusMsg = i18n.T().CannotDeleteWarehouse
			return
		}
		// Builders have no per-building Home the way every other worker
		// does -- Warehouse is their only anchor -- so deleting one they're
		// anchored to needs its own re-pointing, not RemoveHome.
		g.builders.RemoveWarehouse(b, g.logi.Warehouse)
	}
	g.logi.CancelAllJobs(g.stock)
	g.vills.RemoveHome(b)
	g.vills.CancelRouteTo(b) // in case b is a Tavern someone is mid-trip to eat at
	g.jacks.RemoveHome(b, g.stock)
	g.jacks.CancelRouteTo(b) // same, for lumberjacks
	g.fishers.RemoveHome(b, g.stock)
	g.fishers.CancelRouteTo(b) // same, for fishermen
	g.quarry.RemoveHome(b, g.stock)
	g.quarry.CancelRouteTo(b)   // same, for quarrymen
	g.builders.CancelRouteTo(b) // in case b is a Tavern a builder is mid-trip to eat at
	g.miners.RemoveHome(b, g.stock)
	g.miners.CancelRouteTo(b) // same, for miners
	for i, candidate := range g.buildings {
		if candidate != b {
			continue
		}
		g.buildings = append(g.buildings[:i], g.buildings[i+1:]...)
		if g.pop != nil {
			g.pop.Removed++
		}
		g.selection.Clear()
		g.statusMsg = i18n.T().Deleted
		return
	}
}

// clearMissingUnitSelection prevents the inspector from retaining a pointer
// to a dismissed or starved unit that left on this simulation tick.
func (g *Game) clearMissingUnitSelection() {
	switch g.selection.Kind {
	case ui.SelectionSerf:
		for _, s := range g.logi.Serfs {
			if s == g.selection.Serf {
				return
			}
		}
	case ui.SelectionVillager:
		for _, v := range g.vills.Villagers {
			if v == g.selection.Villager {
				return
			}
		}
	case ui.SelectionLumberjack:
		for _, j := range g.jacks.Lumberjacks {
			if j == g.selection.Lumberjack {
				return
			}
		}
	case ui.SelectionFisherman:
		for _, f := range g.fishers.Fishermen {
			if f == g.selection.Fisherman {
				return
			}
		}
	case ui.SelectionQuarryman:
		for _, q := range g.quarry.Quarrymen {
			if q == g.selection.Quarryman {
				return
			}
		}
	case ui.SelectionBuilder:
		for _, bl := range g.builders.Builders {
			if bl == g.selection.Builder {
				return
			}
		}
	case ui.SelectionMiner:
		for _, m := range g.miners.Miners {
			if m == g.selection.Miner {
				return
			}
		}
	default:
		return
	}
	g.selection.Clear()
}

// refreshPopulation rebuilds the live headcount while retaining the
// persistent death/removal history shown in the HUD.
func (g *Game) refreshPopulation() {
	g.pop.Count = len(g.logi.Serfs) + len(g.vills.Villagers) + len(g.jacks.Lumberjacks) + len(g.fishers.Fishermen) + len(g.quarry.Quarrymen) + len(g.builders.Builders) + len(g.miners.Miners)
}

// selectionAt resolves map coordinates to a live game object. Units have
// priority over buildings because a worker standing beside a building is the
// more useful thing to inspect on a click.
// selectionAt resolves map coordinates to a live game object. A unit only
// takes priority over a building at the same tile while it's actually
// drawn there (VisibleOnMap): a worker standing beside a building is the
// more useful thing to inspect on a click, but a resident merely stationed
// inside its workplace (invisible, per the same rule the renderer uses)
// must not steal a click meant for the building itself. Serfs have no
// building to hide inside, so they're always eligible.
func (g *Game) selectionAt(mx, my int) ui.Selection {
	tx, ty := g.camera.ScreenToTile(mx, my)
	for i := len(g.vills.Villagers) - 1; i >= 0; i-- {
		v := g.vills.Villagers[i]
		if v.X == tx && v.Y == ty && v.VisibleOnMap() {
			return ui.Selection{Kind: ui.SelectionVillager, Villager: v}
		}
	}
	for i := len(g.logi.Serfs) - 1; i >= 0; i-- {
		s := g.logi.Serfs[i]
		if s.X == tx && s.Y == ty {
			return ui.Selection{Kind: ui.SelectionSerf, Serf: s}
		}
	}
	for i := len(g.jacks.Lumberjacks) - 1; i >= 0; i-- {
		j := g.jacks.Lumberjacks[i]
		if j.X == tx && j.Y == ty && j.VisibleOnMap() {
			return ui.Selection{Kind: ui.SelectionLumberjack, Lumberjack: j}
		}
	}
	for i := len(g.fishers.Fishermen) - 1; i >= 0; i-- {
		f := g.fishers.Fishermen[i]
		if f.X == tx && f.Y == ty && f.VisibleOnMap() {
			return ui.Selection{Kind: ui.SelectionFisherman, Fisherman: f}
		}
	}
	for i := len(g.quarry.Quarrymen) - 1; i >= 0; i-- {
		q := g.quarry.Quarrymen[i]
		if q.X == tx && q.Y == ty && q.VisibleOnMap() {
			return ui.Selection{Kind: ui.SelectionQuarryman, Quarryman: q}
		}
	}
	for i := len(g.builders.Builders) - 1; i >= 0; i-- {
		bl := g.builders.Builders[i]
		if bl.X == tx && bl.Y == ty {
			return ui.Selection{Kind: ui.SelectionBuilder, Builder: bl}
		}
	}
	for i := len(g.miners.Miners) - 1; i >= 0; i-- {
		m := g.miners.Miners[i]
		if m.X == tx && m.Y == ty && m.VisibleOnMap() {
			return ui.Selection{Kind: ui.SelectionMiner, Miner: m}
		}
	}
	for i := len(g.buildings) - 1; i >= 0; i-- {
		b := g.buildings[i]
		footprint := building.Types[b.Kind].Footprint
		if tx >= b.X && tx < b.X+footprint && ty >= b.Y && ty < b.Y+footprint {
			return ui.Selection{Kind: ui.SelectionBuilding, Building: b}
		}
	}
	return ui.Selection{}
}

// unitsAt counts every unit (serf, villager, lumberjack, fisherman)
// currently standing somewhere on b's footprint, for the inspector's
// "people inside" line. This is a raw position count, not just an
// assigned resident: it works the same way for a Tavern or Warehouse
// (neither has a dedicated worker) as it does for a workplace, and also
// counts a serf momentarily standing there mid-delivery.
func (g *Game) unitsAt(b *building.Building) int {
	if b == nil {
		return 0
	}
	footprint := building.Types[b.Kind].Footprint
	within := func(x, y int) bool {
		return x >= b.X && x < b.X+footprint && y >= b.Y && y < b.Y+footprint
	}
	count := 0
	for _, s := range g.logi.Serfs {
		if within(s.X, s.Y) {
			count++
		}
	}
	for _, v := range g.vills.Villagers {
		if within(v.X, v.Y) {
			count++
		}
	}
	for _, j := range g.jacks.Lumberjacks {
		if within(j.X, j.Y) {
			count++
		}
	}
	for _, f := range g.fishers.Fishermen {
		if within(f.X, f.Y) {
			count++
		}
	}
	for _, q := range g.quarry.Quarrymen {
		if within(q.X, q.Y) {
			count++
		}
	}
	for _, bl := range g.builders.Builders {
		if within(bl.X, bl.Y) {
			count++
		}
	}
	for _, m := range g.miners.Miners {
		if within(m.X, m.Y) {
			count++
		}
	}
	return count
}

func (g *Game) buildingConnected(b *building.Building) bool {
	if b == nil || b.Kind == building.Warehouse {
		return true
	}
	warehouse := findWarehouse(g.buildings)
	if warehouse == nil {
		return false
	}
	_, ok := pathfind.FindPath(g.buildings, warehouse, b)
	return ok
}

// spawnWorkersFor gives each worker building its physical resident. Other
// building kinds don't get a resident: serfs remain a town-wide logistics
// pool, while a lumberjack is tied to one Lumberjack Hut. The only caller
// left is the pre-unit-persistence save migration path in loadGame: a
// building finishing construction no longer auto-spawns its worker (see
// finishConstruction), and neither does NewGame's starting Warehouse/Road
// (neither is RequiresWorker) -- staffing is always a separate, paid hire.
func (g *Game) spawnWorkersFor(b *building.Building) {
	switch b.Kind {
	case building.Farm:
		g.vills.Spawn(villagers.Farmer, b)
	case building.Bakery:
		g.vills.Spawn(villagers.Baker, b)
	case building.Winery:
		g.vills.Spawn(villagers.Winemaker, b)
	case building.PigFarm:
		g.vills.Spawn(villagers.Swineherd, b)
	case building.MeatWorkshop:
		g.vills.Spawn(villagers.Butcher, b)
	case building.CarpentryWorkshop:
		g.vills.Spawn(villagers.Carpenter, b)
	case building.LumberjackHut:
		g.jacks.Spawn(b)
	case building.FisherHut:
		g.fishers.Spawn(b)
	case building.QuarryHut:
		g.quarry.Spawn(b)
	case building.MinerHut:
		g.miners.Spawn(b)
	case building.Smeltery:
		g.vills.Spawn(villagers.Smelter, b)
	}
}

// finishConstruction reacts to builder.ConstructionComplete: a Warehouse
// specifically is registered as an additional logistics endpoint, deferred
// from placement time to this point (see handleMouse) so an unfinished
// Warehouse never accepts deliveries before a Builder has actually finished
// it. It deliberately does NOT spawn a resident worker: staffing a
// finished building is always a separate, paid action through the Hire tab
// (see hireVillagerInto and hireFromTab's per-profession cases) -- a
// building completing construction leaves it staffed exactly like a
// resident who just died, empty and waiting to be hired into.
func (g *Game) finishConstruction(b *building.Building) {
	if b.Kind == building.Warehouse {
		g.logi.AddWarehouse(b)
	}
	g.statusMsg = ""
}

// slotPath returns the file path for save-panel slot n (1-slotCount).
func slotPath(n int) string {
	return fmt.Sprintf(slotPathFormat, n)
}

// refreshSlotCache re-reads every save-panel slot's name and occupancy. It
// is not called every frame -- only on startup and right after a save-panel
// save -- so drawing the settings tab never has to touch disk.
func (g *Game) refreshSlotCache() {
	infos := make([]ui.SaveSlotInfo, slotCount)
	for i := 0; i < slotCount; i++ {
		name, ok := save.PeekName(slotPath(i + 1))
		infos[i] = ui.SaveSlotInfo{Name: name, Occupied: ok}
	}
	g.slotCache = infos
}

// handleSettingsSlotAction executes a click on a save-panel slot's Save or
// Load button. Saving into an occupied slot opens a confirmation dialog
// instead of overwriting immediately; saving into an empty slot goes
// straight to naming. Loading an empty slot does nothing -- its Load button
// is drawn muted for the same reason.
func (g *Game) handleSettingsSlotAction(slot int, action ui.SettingsSlotAction) {
	if slot < 1 || slot > slotCount {
		return
	}
	info := g.slotCache[slot-1]
	switch action {
	case ui.SettingsSlotSave:
		g.dialogSlot = slot
		if info.Occupied {
			g.dialog = ui.DialogConfirmOverwrite
			g.dialogText = info.Name
		} else {
			g.dialog = ui.DialogNaming
			g.dialogText = ""
		}
	case ui.SettingsSlotLoad:
		if !info.Occupied {
			return
		}
		g.loadAndReport(slotPath(slot))
	}
}

// handleDialogInput runs instead of the normal input handlers while a
// settings-tab save/load modal is open (see Update).
func (g *Game) handleDialogInput() {
	switch g.dialog {
	case ui.DialogConfirmOverwrite:
		g.handleConfirmOverwriteInput()
	case ui.DialogNaming:
		g.handleNamingInput()
	case ui.DialogConfirmNewGame:
		g.handleConfirmNewGameInput()
	}
}

// handleConfirmNewGameInput runs while the Settings tab's "New Game" confirm
// dialog is open. Unlike the save-slot overwrite dialog (whose Enter/left
// button just advances to a second, naming step), this one is a plain
// confirm/cancel: Enter or the left button commit the reset immediately.
func (g *Game) handleConfirmNewGameInput() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.dialog = ui.DialogNone
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.resetToNewGame()
		return
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	mx, my := ebiten.CursorPosition()
	if left, ok := g.layout.SettingsDialogButtonAt(mx, my); ok {
		if left {
			g.resetToNewGame()
		} else {
			g.dialog = ui.DialogNone
		}
	}
}

// resetToNewGame discards the running town and replaces it with a fresh one
// -- the Settings tab's "New Game" button. The active language is a
// package-level display preference (internal/i18n), not Game state, so it
// survives the reset untouched. The current window layout is carried over
// (and the fresh camera's viewport re-fitted to it) so the reset doesn't
// visibly snap back to the default 1024x768 layout in a resized window; the
// camera's own position keeps NewGame's usual initial pan.
func (g *Game) resetToNewGame() {
	layout := g.layout
	fresh := NewGame()
	fresh.layout = layout
	mapRect := layout.MapRect()
	fresh.camera.SetViewport(mapRect.Min.X, mapRect.Min.Y, mapRect.Dx(), mapRect.Dy())
	*g = *fresh
	g.statusMsg = i18n.T().NewGameStarted
}

func (g *Game) handleConfirmOverwriteInput() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.dialog = ui.DialogNone
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.dialog = ui.DialogNaming
		return
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	mx, my := ebiten.CursorPosition()
	if left, ok := g.layout.SettingsDialogButtonAt(mx, my); ok {
		if left {
			g.dialog = ui.DialogNaming
		} else {
			g.dialog = ui.DialogNone
		}
	}
}

// handleNamingInput builds up g.dialogText from typed characters (capped at
// maxSlotNameLen runes, so a Cyrillic name still counts letters and not
// UTF-8 bytes), then commits or cancels the save on Enter/Escape or a click
// on the dialog's buttons.
func (g *Game) handleNamingInput() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.dialog = ui.DialogNone
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		if r := []rune(g.dialogText); len(r) > 0 {
			g.dialogText = string(r[:len(r)-1])
		}
	}
	for _, ch := range ebiten.AppendInputChars(nil) {
		if ch < ' ' {
			continue
		}
		if len([]rune(g.dialogText)) >= maxSlotNameLen {
			break
		}
		g.dialogText += string(ch)
	}

	commit := inpututil.IsKeyJustPressed(ebiten.KeyEnter)
	cancel := false
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		if left, ok := g.layout.SettingsDialogButtonAt(mx, my); ok {
			if left {
				commit = true
			} else {
				cancel = true
			}
		}
	}
	if cancel {
		g.dialog = ui.DialogNone
		return
	}
	if commit {
		g.commitDialogSave()
	}
}

// commitDialogSave saves the current game into g.dialogSlot under the typed
// name, falling back to "Слот N" when the player accepted an empty name.
func (g *Game) commitDialogSave() {
	name := g.dialogText
	if name == "" {
		name = fmt.Sprintf("%s %d", i18n.T().SlotDefaultName, g.dialogSlot)
	}
	if err := g.saveGame(slotPath(g.dialogSlot), name); err != nil {
		g.statusMsg = i18n.T().SaveFailedPrefix + err.Error()
	} else {
		g.statusMsg = i18n.T().Saved
		g.refreshSlotCache()
	}
	g.dialog = ui.DialogNone
}

// buildSaveState assembles the full serializable snapshot of the running
// game, shared by every save path (quicksave and every named slot).
func (g *Game) buildSaveState(name string) save.GameState {
	return save.GameState{
		Name:               name,
		GridWidth:          g.grid.Width,
		GridHeight:         g.grid.Height,
		Tiles:              g.grid.Tiles(),
		Buildings:          dereferenceBuildings(g.buildings),
		Stockpile:          *g.stock,
		Population:         *g.pop,
		Units:              g.serializeUnits(),
		BuildingPriority:   g.serializeBuildingPriorities(),
		TreeRegrowth:       g.serializeTreeRegrowth(),
		TreeSeed:           g.treeSeed,
		FishRegrowth:       g.serializeFishRegrowth(),
		FishSeed:           g.fishSeed,
		StoneSeeded:        g.stoneSeeded,
		OreSeeded:          g.oreSeeded,
		SerfMealSeed:       g.logi.MealSeed(),
		VillagerMealSeed:   g.vills.MealSeed(),
		LumberjackMealSeed: g.jacks.MealSeed(),
		FishermanMealSeed:  g.fishers.MealSeed(),
		QuarrymanMealSeed:  g.quarry.MealSeed(),
		BuilderMealSeed:    g.builders.MealSeed(),
		MinerMealSeed:      g.miners.MealSeed(),
		CameraX:            g.camera.X,
		CameraY:            g.camera.Y,
		CameraZoom:         g.camera.Scale,
	}
}

func (g *Game) saveGame(path, name string) error {
	return save.Save(path, g.buildSaveState(name))
}

// loadGame reads and restores a full game snapshot from path, replacing
// every controller, the grid, and the stockpile in place. Used by both the
// S/L quicksave keys and the settings tab's per-slot Load button.
func (g *Game) loadGame(path string) error {
	state, err := save.Load(path)
	if err != nil {
		return err
	}
	grid, err := world.NewGridFromTiles(state.GridWidth, state.GridHeight, state.Tiles)
	if err != nil {
		return err
	}
	buildings := referenceBuildings(state.Buildings)
	buildings, hadTrees := ensureTrees(grid, buildings, len(state.TreeRegrowth) > 0)
	buildings, _ = ensureFish(grid, buildings, len(state.FishRegrowth) > 0)
	warehouse := findWarehouse(buildings)
	if warehouse == nil {
		return errNoWarehouseInSave
	}
	buildings = ensureStoneDeposits(grid, buildings, state.StoneSeeded, defaultStoneSeed, gridPoint{warehouse.X, warehouse.Y})
	buildings = ensureOreDeposits(grid, buildings, state.OreSeeded, defaultCoalSeed, defaultGoldOreSeed, defaultIronOreSeed, gridPoint{warehouse.X, warehouse.Y})

	g.grid = grid
	g.buildings = buildings
	stock := state.Stockpile
	// Older saves carried the temporary 200-unit limit. The town rule is
	// now explicit: every warehouse shares an unlimited stockpile.
	stock.Capacity = 0
	g.stock = &stock
	pop := state.Population
	g.pop = &pop
	g.treeRegrowth = restoreTreeRegrowth(state.TreeRegrowth)
	g.treeSeed = state.TreeSeed
	if g.treeSeed == 0 {
		g.treeSeed = defaultTreeSeed
	}
	g.fishRegrowth = restoreFishRegrowth(state.FishRegrowth)
	g.fishSeed = state.FishSeed
	if g.fishSeed == 0 {
		g.fishSeed = defaultFishSeed
	}
	// ensureStoneDeposits/ensureOreDeposits above guarantee a region exists
	// one way or another (freshly generated, or already present in the
	// save), so from here on this world always counts as seeded.
	g.stoneSeeded = true
	g.oreSeeded = true
	g.camera.X, g.camera.Y = state.CameraX, state.CameraY
	if !hadTrees && g.camera.X == 0 {
		// Old saves were usually made from the original left-aligned view;
		// keep the newly migrated grove visible after the first load too.
		g.camera.X = float64(4 * render.TileSize)
	}
	g.camera.Scale = state.CameraZoom
	if g.camera.Scale <= 0 {
		g.camera.Scale = 1
	}
	mapRect := g.layout.MapRect()
	g.camera.SetViewport(mapRect.Min.X, mapRect.Min.Y, mapRect.Dx(), mapRect.Dy())
	g.camera.Pan(0, 0, g.grid.Width, g.grid.Height, mapRect.Dx(), mapRect.Dy())
	g.selection.Clear()

	// Jobs are rebuilt from the saved positions. The roster itself is
	// restored, so hiring extra serfs or saving a worker halfway to the
	// Tavern no longer silently resets the town.
	g.logi = logistics.NewController(warehouse, 0)
	for _, b := range buildings {
		if b.Kind == building.Warehouse && b != warehouse {
			g.logi.AddWarehouse(b)
		}
	}
	g.vills = villagers.NewController()
	g.jacks = lumberjack.NewController()
	g.fishers = fishing.NewController()
	g.quarry = quarry.NewController()
	g.builders = builder.NewController()
	g.miners = miner.NewController()
	if len(state.Units) == 0 {
		// Saves from before unit persistence did not contain a roster.
		// Keep those saves playable with the old sensible defaults.
		g.logi = logistics.NewController(warehouse, startingSerfs)
		for _, b := range buildings {
			if b.Kind == building.Warehouse && b != warehouse {
				g.logi.AddWarehouse(b)
			}
		}
		for _, b := range buildings {
			g.spawnWorkersFor(b)
		}
	} else {
		g.restoreUnits(state.Units, buildings)
	}
	if state.SerfMealSeed != 0 {
		g.logi.SetMealSeed(state.SerfMealSeed)
	}
	if state.VillagerMealSeed != 0 {
		g.vills.SetMealSeed(state.VillagerMealSeed)
	}
	if state.LumberjackMealSeed != 0 {
		g.jacks.SetMealSeed(state.LumberjackMealSeed)
	}
	if state.FishermanMealSeed != 0 {
		g.fishers.SetMealSeed(state.FishermanMealSeed)
	}
	if state.QuarrymanMealSeed != 0 {
		g.quarry.SetMealSeed(state.QuarrymanMealSeed)
	}
	if state.BuilderMealSeed != 0 {
		g.builders.SetMealSeed(state.BuilderMealSeed)
	}
	if state.MinerMealSeed != 0 {
		g.miners.SetMealSeed(state.MinerMealSeed)
	}
	for _, p := range state.BuildingPriority {
		g.logi.SetPriority(p.Kind, p.Level)
	}
	g.refreshPopulation()

	return nil
}

// errNoWarehouseInSave marks the one load failure with its own dedicated,
// already-fully-worded i18n string (LoadFailedNoWarehouse) instead of the
// generic "load failed: <err>" composition -- see loadAndReport.
var errNoWarehouseInSave = errors.New("no warehouse in save")

// loadAndReport calls loadGame and sets statusMsg to describe the outcome,
// exactly like the S/L quicksave keys and the settings tab's per-slot Load
// button both need.
func (g *Game) loadAndReport(path string) {
	switch err := g.loadGame(path); {
	case err == nil:
		g.statusMsg = i18n.T().Loaded
	case errors.Is(err, errNoWarehouseInSave):
		g.statusMsg = i18n.T().LoadFailedNoWarehouse
	default:
		g.statusMsg = i18n.T().LoadFailedPrefix + err.Error()
	}
}

func findWarehouse(buildings []*building.Building) *building.Building {
	for _, b := range buildings {
		if b.Kind == building.Warehouse {
			return b
		}
	}
	return nil
}

func dereferenceBuildings(in []*building.Building) []building.Building {
	out := make([]building.Building, len(in))
	for i, b := range in {
		out[i] = *b
	}
	return out
}

// serializeBuildingPriorities converts the controller's priority map into
// a stable, sorted slice for the save file. Go map iteration order is
// randomized; sorting by Kind keeps the saved file (and therefore
// TestSaveLoadRoundTrip-style comparisons) reproducible from run to run.
func (g *Game) serializeBuildingPriorities() []save.BuildingPriorityState {
	priorities := g.logi.Priorities()
	kinds := make([]building.Kind, 0, len(priorities))
	for kind := range priorities {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	out := make([]save.BuildingPriorityState, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, save.BuildingPriorityState{Kind: kind, Level: priorities[kind]})
	}
	return out
}

func (g *Game) serializeUnits() []save.UnitState {
	units := make([]save.UnitState, 0, len(g.logi.Serfs)+len(g.vills.Villagers)+len(g.jacks.Lumberjacks)+len(g.fishers.Fishermen)+len(g.quarry.Quarrymen)+len(g.builders.Builders)+len(g.miners.Miners))
	for _, s := range g.logi.Serfs {
		units = append(units, save.UnitState{
			Kind:        save.UnitSerf,
			X:           s.X,
			Y:           s.Y,
			HomeIndex:   -1,
			HungerTicks: s.HungerTicks(),
			Starving:    s.Starving,
			Dismissing:  s.Dismissing(),
		})
	}
	for _, v := range g.vills.Villagers {
		var kind save.UnitKind
		switch v.Profession {
		case villagers.Baker:
			kind = save.UnitBaker
		case villagers.Winemaker:
			kind = save.UnitWinemaker
		case villagers.Swineherd:
			kind = save.UnitSwineherd
		case villagers.Butcher:
			kind = save.UnitButcher
		case villagers.Carpenter:
			kind = save.UnitCarpenter
		case villagers.Smelter:
			kind = save.UnitSmelter
		default:
			kind = save.UnitFarmer
		}
		units = append(units, save.UnitState{
			Kind:        kind,
			X:           v.X,
			Y:           v.Y,
			HomeIndex:   indexOfBuilding(g.buildings, v.HomeBuilding()),
			HungerTicks: v.HungerTicks(),
			Starving:    v.Starving,
			State:       int(v.State()),
			Meal:        v.Meal(),
		})
	}
	for _, j := range g.jacks.Lumberjacks {
		_, cargoAmount := j.Cargo()
		targetIndex := indexOfBuilding(g.buildings, j.TargetTree())
		units = append(units, save.UnitState{
			Kind:        save.UnitLumberjack,
			X:           j.X,
			Y:           j.Y,
			HomeIndex:   indexOfBuilding(g.buildings, j.HomeBuilding()),
			HungerTicks: j.HungerTicks(),
			Starving:    j.Starving,
			State:       int(j.State()),
			TargetIndex: targetIndex,
			WorkTicks:   j.WorkTicks(),
			Cargo:       resource.Log,
			CargoAmount: cargoAmount,
			Meal:        j.Meal(),
		})
	}
	for _, f := range g.fishers.Fishermen {
		_, cargoAmount := f.Cargo()
		targetIndex := indexOfBuilding(g.buildings, f.TargetFish())
		units = append(units, save.UnitState{
			Kind:        save.UnitFisherman,
			X:           f.X,
			Y:           f.Y,
			HomeIndex:   indexOfBuilding(g.buildings, f.HomeBuilding()),
			HungerTicks: f.HungerTicks(),
			Starving:    f.Starving,
			State:       int(f.State()),
			TargetIndex: targetIndex,
			WorkTicks:   f.WorkTicks(),
			Cargo:       resource.Fish,
			CargoAmount: cargoAmount,
			Meal:        f.Meal(),
		})
	}
	for _, q := range g.quarry.Quarrymen {
		targetIndex := indexOfBuilding(g.buildings, q.TargetDeposit())
		units = append(units, save.UnitState{
			Kind:        save.UnitQuarryman,
			X:           q.X,
			Y:           q.Y,
			HomeIndex:   indexOfBuilding(g.buildings, q.HomeBuilding()),
			HungerTicks: q.HungerTicks(),
			Starving:    q.Starving,
			State:       int(q.State()),
			TargetIndex: targetIndex,
			WorkTicks:   q.WorkTicks(),
			Cargo:       resource.StoneBlock,
			CargoAmount: q.RawCargo(),
			Meal:        q.Meal(),
		})
	}
	for _, bl := range g.builders.Builders {
		targetIndex := indexOfBuilding(g.buildings, bl.TargetSite())
		units = append(units, save.UnitState{
			Kind:        save.UnitBuilder,
			X:           bl.X,
			Y:           bl.Y,
			HomeIndex:   indexOfBuilding(g.buildings, bl.Warehouse),
			HungerTicks: bl.HungerTicks(),
			Starving:    bl.Starving,
			State:       int(bl.State()),
			TargetIndex: targetIndex,
			WorkTicks:   bl.WorkTicks(),
			Meal:        bl.Meal(),
		})
	}
	for _, mn := range g.miners.Miners {
		cargoResource, cargoAmount := mn.Cargo()
		targetIndex := indexOfBuilding(g.buildings, mn.TargetDeposit())
		quotaIndex, quotaProgress := mn.QuotaProgress()
		units = append(units, save.UnitState{
			Kind:          save.UnitMiner,
			X:             mn.X,
			Y:             mn.Y,
			HomeIndex:     indexOfBuilding(g.buildings, mn.HomeBuilding()),
			HungerTicks:   mn.HungerTicks(),
			Starving:      mn.Starving,
			State:         int(mn.State()),
			TargetIndex:   targetIndex,
			WorkTicks:     mn.WorkTicks(),
			Cargo:         cargoResource,
			CargoAmount:   cargoAmount,
			Meal:          mn.Meal(),
			QuotaIndex:    quotaIndex,
			QuotaProgress: quotaProgress,
		})
	}
	return units
}

func (g *Game) restoreUnits(states []save.UnitState, buildings []*building.Building) {
	for _, state := range states {
		switch state.Kind {
		case save.UnitSerf:
			g.logi.RestoreSerf(state.X, state.Y, state.HungerTicks, state.Starving, state.Dismissing)
		case save.UnitFarmer, save.UnitBaker, save.UnitWinemaker, save.UnitSwineherd, save.UnitButcher, save.UnitCarpenter, save.UnitSmelter:
			if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) {
				continue
			}
			home := buildings[state.HomeIndex]
			var profession villagers.Profession
			switch state.Kind {
			case save.UnitBaker:
				profession = villagers.Baker
			case save.UnitWinemaker:
				profession = villagers.Winemaker
			case save.UnitSwineherd:
				profession = villagers.Swineherd
			case save.UnitButcher:
				profession = villagers.Butcher
			case save.UnitCarpenter:
				profession = villagers.Carpenter
			case save.UnitSmelter:
				profession = villagers.Smelter
			default:
				profession = villagers.Farmer
			}
			if (profession == villagers.Farmer && home.Kind != building.Farm) ||
				(profession == villagers.Baker && home.Kind != building.Bakery) ||
				(profession == villagers.Winemaker && home.Kind != building.Winery) ||
				(profession == villagers.Swineherd && home.Kind != building.PigFarm) ||
				(profession == villagers.Butcher && home.Kind != building.MeatWorkshop) ||
				(profession == villagers.Carpenter && home.Kind != building.CarpentryWorkshop) ||
				(profession == villagers.Smelter && home.Kind != building.Smeltery) {
				continue
			}
			g.vills.RestoreVillager(profession, home, state.X, state.Y, state.HungerTicks, state.Starving, villagers.State(state.State), buildings, state.Meal)
		case save.UnitLumberjack:
			if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.LumberjackHut {
				continue
			}
			var target *building.Building
			if state.TargetIndex >= 0 && state.TargetIndex < len(buildings) && buildings[state.TargetIndex].Kind == building.Tree {
				target = buildings[state.TargetIndex]
			}
			g.jacks.Restore(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, lumberjack.State(state.State), target, state.WorkTicks, state.CargoAmount, g.grid, buildings, state.Meal)
		case save.UnitFisherman:
			if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.FisherHut {
				continue
			}
			var target *building.Building
			if state.TargetIndex >= 0 && state.TargetIndex < len(buildings) && buildings[state.TargetIndex].Kind == building.Fish {
				target = buildings[state.TargetIndex]
			}
			g.fishers.Restore(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, fishing.State(state.State), target, state.WorkTicks, state.CargoAmount, g.grid, buildings, state.Meal)
		case save.UnitQuarryman:
			if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.QuarryHut {
				continue
			}
			var target *building.Building
			if state.TargetIndex >= 0 && state.TargetIndex < len(buildings) && buildings[state.TargetIndex].Kind == building.StoneDeposit {
				target = buildings[state.TargetIndex]
			}
			g.quarry.Restore(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, quarry.State(state.State), target, state.WorkTicks, state.CargoAmount, g.grid, buildings, state.Meal)
		case save.UnitBuilder:
			if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.Warehouse {
				continue
			}
			var target *building.Building
			if state.TargetIndex >= 0 && state.TargetIndex < len(buildings) && buildings[state.TargetIndex].ConstructionStage != building.ConstructionNone {
				target = buildings[state.TargetIndex]
			}
			g.builders.Restore(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, builder.State(state.State), target, state.WorkTicks, g.grid, buildings, state.Meal)
		case save.UnitMiner:
			if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.MinerHut {
				continue
			}
			var target *building.Building
			if state.TargetIndex >= 0 && state.TargetIndex < len(buildings) {
				switch buildings[state.TargetIndex].Kind {
				case building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
					target = buildings[state.TargetIndex]
				}
			}
			g.miners.Restore(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, miner.State(state.State), target, state.WorkTicks, state.CargoAmount, state.Cargo, state.QuotaIndex, state.QuotaProgress, g.grid, buildings, state.Meal)
		}
	}
}

func indexOfBuilding(buildings []*building.Building, target *building.Building) int {
	for i, b := range buildings {
		if b == target {
			return i
		}
	}
	return -1
}

func referenceBuildings(in []building.Building) []*building.Building {
	out := make([]*building.Building, len(in))
	for i := range in {
		out[i] = &in[i]
	}
	return out
}

// newMapSeed returns a fresh, non-persisted seed for one game's procedural
// terrain -- per the user's explicit request ("карта генерируется случайно
// при 'Новой игре'"), unlike every other *Seed constant in this file (the
// tree/fish regrowth seeds, and the stone/ore region seeds), the map layout
// itself is meant to be different every time NewGame runs, not a fixed
// default. Nothing about this seed is ever saved: the terrain doesn't need
// re-seeding the way regrowth does, since the whole tile grid is saved and
// loaded verbatim (save.GameState.Tiles).
func newMapSeed() uint32 {
	return uint32(time.Now().UnixNano())
}

// generateGrid builds one fresh procedural map: a sea hugging a random edge
// (see seaMinPercent), a couple of cosmetic fertile patches, and otherwise
// plain grass for seedTrees/seedThickets/seedStoneDeposits/seedOreDeposits
// to scatter their objects across afterward.
func generateGrid(width, height int, seed uint32) *world.Grid {
	g := world.NewGrid(width, height)
	growSeaRegion(g, seed^0x9e3779b9)
	growFertileRegions(g, seed^0x85ebca6b)
	return g
}

// growSeaRegion carves one sea out of the map, starting from a randomly
// chosen point on a randomly chosen edge and growing inward the same
// blob-region way seedStoneDeposits does -- see seaMinPercent's doc comment
// for why it starts on the edge rather than anywhere.
func growSeaRegion(g *world.Grid, seed uint32) {
	area := g.Width * g.Height
	span := seaMaxPercent - seaMinPercent + 1
	percent := seaMinPercent + int(seed%uint32(span))
	target := area * percent / 100
	if target <= 0 {
		return
	}

	start := seaEdgeStart(g, seed)
	claimed := map[gridPoint]bool{start: true}
	frontier := []gridPoint{start}
	placed := 0
	for len(frontier) > 0 && placed < target {
		bestIdx := 0
		bestScore := seaScatterScore(frontier[0].x, frontier[0].y) ^ seed
		for i := 1; i < len(frontier); i++ {
			score := seaScatterScore(frontier[i].x, frontier[i].y) ^ seed
			if score < bestScore {
				bestScore, bestIdx = score, i
			}
		}
		p := frontier[bestIdx]
		frontier = append(frontier[:bestIdx], frontier[bestIdx+1:]...)

		g.Set(p.x, p.y, world.Tile{Terrain: world.Water})
		placed++

		for _, d := range [...]gridPoint{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := gridPoint{p.x + d.x, p.y + d.y}
			if claimed[n] || !g.InBounds(n.x, n.y) {
				continue
			}
			claimed[n] = true
			frontier = append(frontier, n)
		}
	}
}

// seaEdgeStart picks a random point on a randomly chosen map edge as the
// sea's growth origin.
func seaEdgeStart(g *world.Grid, seed uint32) gridPoint {
	switch (seed >> 16) % 4 {
	case 0: // top
		return gridPoint{int(seed % uint32(g.Width)), 0}
	case 1: // right
		return gridPoint{g.Width - 1, int(seed % uint32(g.Height))}
	case 2: // bottom
		return gridPoint{int(seed % uint32(g.Width)), g.Height - 1}
	default: // left
		return gridPoint{0, int(seed % uint32(g.Height))}
	}
}

func seaScatterScore(x, y int) uint32 {
	return uint32(x)*668265263 ^ uint32(y)*374761393 ^ 0x6a09e667
}

// growFertileRegions splits a random 6-10% of the map's area across a
// couple of separate patches, mirroring seedStoneDeposits' region-count
// approach. Purely cosmetic -- see fertileMinPercent's doc comment.
func growFertileRegions(g *world.Grid, seed uint32) {
	area := g.Width * g.Height
	span := fertileMaxPercent - fertileMinPercent + 1
	percent := fertileMinPercent + int(seed%uint32(span))
	total := area * percent / 100
	if total <= 0 {
		return
	}
	base := total / fertileRegionCount
	for i := 0; i < fertileRegionCount; i++ {
		regionSeed := seed ^ uint32(i)*0x9e3779b9
		growFertileRegion(g, base, regionSeed)
	}
}

// growFertileRegion places one blob of Fertile tiles, the same way
// growStoneRegion places a stone-deposit blob, but painting terrain
// directly instead of adding a Building. Growth halts wherever it meets
// non-grass ground (water, an earlier patch) instead of growing through it,
// same as a deposit region halts at an unplaceable cell.
func growFertileRegion(g *world.Grid, target int, seed uint32) {
	if target <= 0 {
		return
	}
	start, ok := findFertileStart(g, seed)
	if !ok {
		return
	}

	claimed := map[gridPoint]bool{start: true}
	frontier := []gridPoint{start}
	placed := 0
	for len(frontier) > 0 && placed < target {
		bestIdx := 0
		bestScore := fertileScatterScore(frontier[0].x, frontier[0].y) ^ seed
		for i := 1; i < len(frontier); i++ {
			score := fertileScatterScore(frontier[i].x, frontier[i].y) ^ seed
			if score < bestScore {
				bestScore, bestIdx = score, i
			}
		}
		p := frontier[bestIdx]
		frontier = append(frontier[:bestIdx], frontier[bestIdx+1:]...)

		if g.At(p.x, p.y).Terrain != world.Grass {
			continue
		}
		g.Set(p.x, p.y, world.Tile{Terrain: world.Fertile})
		placed++

		for _, d := range [...]gridPoint{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := gridPoint{p.x + d.x, p.y + d.y}
			if claimed[n] || !g.InBounds(n.x, n.y) {
				continue
			}
			claimed[n] = true
			frontier = append(frontier, n)
		}
	}
}

func findFertileStart(g *world.Grid, seed uint32) (gridPoint, bool) {
	bestScore := ^uint32(0)
	var best gridPoint
	found := false
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			if g.At(x, y).Terrain != world.Grass {
				continue
			}
			score := fertileScatterScore(x, y) ^ seed
			if !found || score < bestScore {
				bestScore, best, found = score, gridPoint{x, y}, true
			}
		}
	}
	return best, found
}

func fertileScatterScore(x, y int) uint32 {
	return uint32(x)*3266489917 ^ uint32(y)*2246822519 ^ 0x1b873593
}

// findWarehouseSpot picks the plain-grass tile closest to the map's center
// that also has a buildable tile directly south for the starting Road --
// the procedural-generation replacement for the old fixed warehouseX/Y
// constants. Centering it keeps the starting position roughly equidistant
// from whatever the sea/deposit generation ends up placing around the
// edges, on a map whose layout is different every game.
func findWarehouseSpot(g *world.Grid) (gridPoint, bool) {
	cx, cy := g.Width/2, g.Height/2
	bestDist := 0
	var best gridPoint
	found := false
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			if g.At(x, y).Terrain != world.Grass || !g.InBounds(x, y+1) || !g.At(x, y+1).Buildable() {
				continue
			}
			dx, dy := x-cx, y-cy
			dist := dx*dx + dy*dy
			if !found || dist < bestDist {
				bestDist, best, found = dist, gridPoint{x, y}, true
			}
		}
	}
	return best, found
}

// seedThickets plants extra trees inside a handful of deliberately dense
// zones, on top of (not instead of) seedTrees' map-wide uniform ~1%
// scatter -- the roadmap's "чащи". Each zone is grown the same blob-region
// way as a stone/ore deposit; a visited cell gets a tree only
// thicketDensityPercent of the time, so a thicket reads as a dense grove
// rather than an unbroken wall of trunks.
func seedThickets(grid *world.Grid, buildings []*building.Building, seed uint32) []*building.Building {
	area := grid.Width * grid.Height
	span := thicketZoneMaxPercent - thicketZoneMinPercent + 1
	for i := 0; i < thicketZoneCount; i++ {
		zoneSeed := seed ^ uint32(i)*0x85ebca6b
		percent := thicketZoneMinPercent + int(zoneSeed%uint32(span))
		target := area * percent / 100
		buildings = growThicketZone(grid, buildings, target, zoneSeed)
	}
	return buildings
}

func growThicketZone(grid *world.Grid, buildings []*building.Building, target int, seed uint32) []*building.Building {
	if target <= 0 {
		return buildings
	}
	start, ok := findThicketStart(grid, buildings, seed)
	if !ok {
		return buildings
	}

	claimed := map[gridPoint]bool{start: true}
	frontier := []gridPoint{start}
	visited := 0
	for len(frontier) > 0 && visited < target {
		bestIdx := 0
		bestScore := thicketScatterScore(frontier[0].x, frontier[0].y) ^ seed
		for i := 1; i < len(frontier); i++ {
			score := thicketScatterScore(frontier[i].x, frontier[i].y) ^ seed
			if score < bestScore {
				bestScore, bestIdx = score, i
			}
		}
		p := frontier[bestIdx]
		frontier = append(frontier[:bestIdx], frontier[bestIdx+1:]...)
		visited++

		if (thicketScatterScore(p.x, p.y)^seed)%100 < thicketDensityPercent && building.CanPlace(grid, buildings, building.Tree, p.x, p.y) {
			buildings = append(buildings, building.NewTree(p.x, p.y))
		}

		for _, d := range [...]gridPoint{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := gridPoint{p.x + d.x, p.y + d.y}
			if claimed[n] || !grid.InBounds(n.x, n.y) || !grid.At(n.x, n.y).Buildable() {
				continue
			}
			claimed[n] = true
			frontier = append(frontier, n)
		}
	}
	return buildings
}

func findThicketStart(grid *world.Grid, buildings []*building.Building, seed uint32) (gridPoint, bool) {
	bestScore := ^uint32(0)
	var best gridPoint
	found := false
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if !building.CanPlace(grid, buildings, building.Tree, x, y) {
				continue
			}
			score := thicketScatterScore(x, y) ^ seed
			if !found || score < bestScore {
				bestScore, best, found = score, gridPoint{x, y}, true
			}
		}
	}
	return best, found
}

func thicketScatterScore(x, y int) uint32 {
	return uint32(x)*374761393 ^ uint32(y)*668265263 ^ 0x27d4eb2f
}

// seedTrees adds no more than one percent of the map area in trees. Candidate
// cells are ordered by a stable coordinate hash, so the selection looks
// scattered while old-save migration remains reproducible. CanPlace also
// protects against roads, buildings, and trees already occupying a cell.
func seedTrees(grid *world.Grid, buildings []*building.Building) []*building.Building {
	maxTrees := grid.Width * grid.Height / 100
	if maxTrees == 0 {
		return buildings
	}

	type candidate struct {
		x, y  int
		score uint32
	}
	candidates := make([]candidate, 0, maxTrees*4)
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if !grid.At(x, y).Buildable() {
				continue
			}
			candidates = append(candidates, candidate{x: x, y: y, score: treeScatterScore(x, y)})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score < candidates[j].score
	})

	planted := 0
	for _, c := range candidates {
		if planted >= maxTrees {
			break
		}
		if !building.CanPlace(grid, buildings, building.Tree, c.x, c.y) {
			continue
		}
		buildings = append(buildings, building.NewTree(c.x, c.y))
		planted++
	}
	return buildings
}

func treeScatterScore(x, y int) uint32 {
	// The hash makes the first maxTrees cells look randomly scattered without
	// relying on runtime randomness or storing a random generator in a save.
	return uint32(x)*73856093 ^ uint32(y)*19349663 ^ 0x85ebca6b
}

// seedStoneDeposits splits a random 5-10% of the map's area (per the game
// design) across 2-5 separate regions, rather than one single patch, so the
// player has more than one spot worth building a Quarry Hut near. Each
// deposit cell holds a full building.StoneDepositReserve. Unlike trees and
// fish, this only ever runs once per world -- see save.GameState.StoneSeeded
// and ensureStoneDeposits. avoid/minDistance keep the region away from a
// fixed point -- every call site passes the starting Warehouse and
// minDepositDistanceFromWarehouse; minDistance <= 0 disables the
// constraint, kept for callers (tests) that want to isolate the abundance
// percentage from the distance rule.
func seedStoneDeposits(grid *world.Grid, buildings []*building.Building, seed uint32, avoid gridPoint, minDistance int) []*building.Building {
	area := grid.Width * grid.Height
	percent := 5 + int(seed%6) // 5..10 inclusive, total share of the map
	total := area * percent / 100
	if total <= 0 {
		return buildings
	}

	regionCount := 2 + int((seed>>8)%4) // 2..5 inclusive separate regions
	base := total / regionCount
	extra := total % regionCount
	for i := 0; i < regionCount; i++ {
		target := base
		if i < extra {
			target++
		}
		regionSeed := seed ^ uint32(i)*0x9e3779b9
		buildings = growStoneRegion(grid, buildings, target, regionSeed, avoid, minDistance)
	}
	return buildings
}

// growStoneRegion places one contiguous blob of up to target stone-deposit
// cells, starting from a deterministically chosen free tile and expanding
// outward. Called once per region by seedStoneDeposits.
func growStoneRegion(grid *world.Grid, buildings []*building.Building, target int, seed uint32, avoid gridPoint, minDistance int) []*building.Building {
	if target <= 0 {
		return buildings
	}
	start, ok := findStoneStart(grid, buildings, seed, avoid, minDistance)
	if !ok {
		return buildings
	}

	claimed := map[gridPoint]bool{start: true}
	frontier := []gridPoint{start}
	placed := 0
	for len(frontier) > 0 && placed < target {
		// Grow from whichever frontier cell has the lowest scatter score,
		// rather than always index 0, so the region fills out into a blob
		// instead of a single-file line hugging one edge.
		bestIdx := 0
		bestScore := stoneScatterScore(frontier[0].x, frontier[0].y) ^ seed
		for i := 1; i < len(frontier); i++ {
			score := stoneScatterScore(frontier[i].x, frontier[i].y) ^ seed
			if score < bestScore {
				bestScore, bestIdx = score, i
			}
		}
		p := frontier[bestIdx]
		frontier = append(frontier[:bestIdx], frontier[bestIdx+1:]...)

		if tooCloseToPoint(p.x, p.y, avoid, minDistance) || !building.CanPlace(grid, buildings, building.StoneDeposit, p.x, p.y) {
			continue
		}
		buildings = append(buildings, building.NewStoneDeposit(p.x, p.y))
		placed++

		for _, d := range [...]gridPoint{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := gridPoint{p.x + d.x, p.y + d.y}
			if claimed[n] || !grid.InBounds(n.x, n.y) || !grid.At(n.x, n.y).Buildable() {
				continue
			}
			claimed[n] = true
			frontier = append(frontier, n)
		}
	}
	return buildings
}

func findStoneStart(grid *world.Grid, buildings []*building.Building, seed uint32, avoid gridPoint, minDistance int) (gridPoint, bool) {
	bestScore := ^uint32(0)
	var best gridPoint
	found := false
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if tooCloseToPoint(x, y, avoid, minDistance) || !building.CanPlace(grid, buildings, building.StoneDeposit, x, y) {
				continue
			}
			score := stoneScatterScore(x, y) ^ seed
			if !found || score < bestScore {
				bestScore, best, found = score, gridPoint{x, y}, true
			}
		}
	}
	return best, found
}

func stoneScatterScore(x, y int) uint32 {
	return uint32(x)*2246822519 ^ uint32(y)*3266489917 ^ 0x27d4eb2f
}

// ensureStoneDeposits places a fresh stone region only for a world that has
// genuinely never had one. alreadySeeded (see save.GameState.StoneSeeded)
// is what tells that apart from a modern save where the player has
// legitimately mined every deposit dry -- there is no regrowth queue to
// infer it from, unlike trees or fish. warehouse is the point the region
// must stay minDepositDistanceFromWarehouse away from -- for a migrated
// save this is the save's own original Warehouse, not the fixed NewGame
// coordinates, since an old town may not sit at the default spot.
func ensureStoneDeposits(grid *world.Grid, buildings []*building.Building, alreadySeeded bool, seed uint32, warehouse gridPoint) []*building.Building {
	if alreadySeeded {
		return buildings
	}
	return seedStoneDeposits(grid, buildings, seed, warehouse, minDepositDistanceFromWarehouse)
}

// removeDeposit deletes an exhausted deposit from the world once its
// Reserve reaches zero -- stone or any of the three ore kinds. The tile
// underneath needs no separate change: it was always ordinary buildable
// ground, the same way felling a tree reveals the grass it always stood on.
func (g *Game) removeDeposit(deposit *building.Building) {
	if deposit == nil {
		return
	}
	switch deposit.Kind {
	case building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
	default:
		return
	}
	for i, candidate := range g.buildings {
		if candidate != deposit {
			continue
		}
		g.buildings = append(g.buildings[:i], g.buildings[i+1:]...)
		return
	}
}

// seedOreDeposits is seedStoneDeposits generalized to any of the three
// finite ore-family kinds (Coal, GoldOre, IronOre), each with its own
// abundance range -- per the game design, Coal is deliberately the most
// common (it's needed by both Smeltery recipes), Gold ore the rarest (it
// smelts directly into the hiring currency). avoid/minDistance keep a
// region away from a fixed point -- every call site passes the starting
// Warehouse and minDepositDistanceFromWarehouse; minDistance <= 0 disables
// the constraint entirely, kept for callers (tests) that want to isolate
// the abundance percentages from the distance rule.
func seedOreDeposits(grid *world.Grid, buildings []*building.Building, kind building.Kind, minPercent, maxPercent int, seed uint32, avoid gridPoint, minDistance int) []*building.Building {
	area := grid.Width * grid.Height
	span := maxPercent - minPercent + 1
	percent := minPercent
	if span > 0 {
		percent += int(seed % uint32(span))
	}
	total := area * percent / 100
	if total <= 0 {
		return buildings
	}

	regionCount := 2 + int((seed>>8)%4) // 2..5 inclusive separate regions
	base := total / regionCount
	extra := total % regionCount
	for i := 0; i < regionCount; i++ {
		target := base
		if i < extra {
			target++
		}
		regionSeed := seed ^ uint32(i)*0x9e3779b9
		buildings = growOreRegion(grid, buildings, kind, target, regionSeed, avoid, minDistance)
	}
	return buildings
}

// tooCloseToPoint reports whether (x,y) sits within minDistance grid cells
// (Euclidean) of avoid. minDistance <= 0 means "no constraint" -- always false.
func tooCloseToPoint(x, y int, avoid gridPoint, minDistance int) bool {
	if minDistance <= 0 {
		return false
	}
	dx, dy := float64(x-avoid.x), float64(y-avoid.y)
	return dx*dx+dy*dy < float64(minDistance*minDistance)
}

// growOreRegion is growStoneRegion generalized to kind, with the same
// avoid/minDistance keep-away as seedOreDeposits.
func growOreRegion(grid *world.Grid, buildings []*building.Building, kind building.Kind, target int, seed uint32, avoid gridPoint, minDistance int) []*building.Building {
	if target <= 0 {
		return buildings
	}
	start, ok := findOreStart(grid, buildings, kind, seed, avoid, minDistance)
	if !ok {
		return buildings
	}

	claimed := map[gridPoint]bool{start: true}
	frontier := []gridPoint{start}
	placed := 0
	for len(frontier) > 0 && placed < target {
		bestIdx := 0
		bestScore := oreScatterScore(frontier[0].x, frontier[0].y, kind) ^ seed
		for i := 1; i < len(frontier); i++ {
			score := oreScatterScore(frontier[i].x, frontier[i].y, kind) ^ seed
			if score < bestScore {
				bestScore, bestIdx = score, i
			}
		}
		p := frontier[bestIdx]
		frontier = append(frontier[:bestIdx], frontier[bestIdx+1:]...)

		if tooCloseToPoint(p.x, p.y, avoid, minDistance) || !building.CanPlace(grid, buildings, kind, p.x, p.y) {
			continue
		}
		buildings = append(buildings, building.NewOreDeposit(kind, p.x, p.y))
		placed++

		for _, d := range [...]gridPoint{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := gridPoint{p.x + d.x, p.y + d.y}
			if claimed[n] || !grid.InBounds(n.x, n.y) || !grid.At(n.x, n.y).Buildable() {
				continue
			}
			claimed[n] = true
			frontier = append(frontier, n)
		}
	}
	return buildings
}

func findOreStart(grid *world.Grid, buildings []*building.Building, kind building.Kind, seed uint32, avoid gridPoint, minDistance int) (gridPoint, bool) {
	bestScore := ^uint32(0)
	var best gridPoint
	found := false
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if tooCloseToPoint(x, y, avoid, minDistance) || !building.CanPlace(grid, buildings, kind, x, y) {
				continue
			}
			score := oreScatterScore(x, y, kind) ^ seed
			if !found || score < bestScore {
				bestScore, best, found = score, gridPoint{x, y}, true
			}
		}
	}
	return best, found
}

// oreScatterScore folds kind into the hash (unlike stoneScatterScore) so
// Coal/GoldOre/IronOre don't all pick the exact same starting cells and
// region shapes when seeded with related seeds.
func oreScatterScore(x, y int, kind building.Kind) uint32 {
	return uint32(x)*2246822519 ^ uint32(y)*3266489917 ^ uint32(kind)*2654435761 ^ 0x27d4eb2f
}

// ensureOreDeposits is ensureStoneDeposits generalized to the ore-family
// trio, seeded together and guarded by the same single flag
// (save.GameState.OreSeeded) since they're always generated in the same
// NewGame call -- there's no scenario where the game would want to seed
// only one of the three without the others. warehouse is the point
// GoldOre/IronOre must stay minOreDistanceFromWarehouse away from -- for a
// migrated save this is the save's own original Warehouse, not the fixed
// NewGame coordinates, since an old town may not sit at the default spot.
func ensureOreDeposits(grid *world.Grid, buildings []*building.Building, alreadySeeded bool, coalSeed, goldOreSeed, ironOreSeed uint32, warehouse gridPoint) []*building.Building {
	if alreadySeeded {
		return buildings
	}
	buildings = seedOreDeposits(grid, buildings, building.CoalDeposit, coalMinPercent, coalMaxPercent, coalSeed, warehouse, minDepositDistanceFromWarehouse)
	buildings = seedOreDeposits(grid, buildings, building.GoldOreDeposit, goldOreMinPercent, goldOreMaxPercent, goldOreSeed, warehouse, minDepositDistanceFromWarehouse)
	buildings = seedOreDeposits(grid, buildings, building.IronOreDeposit, ironOreMinPercent, ironOreMaxPercent, ironOreSeed, warehouse, minDepositDistanceFromWarehouse)
	return buildings
}

// seedFish fills each connected water body to at most 40% of its cells. A
// one-cell minimum is deliberate: the approved small-pond exception keeps a
// tiny usable pond from becoming permanently sterile through integer rounding.
func seedFish(grid *world.Grid, buildings []*building.Building) []*building.Building {
	seen := make(map[gridPoint]bool)
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			start := gridPoint{x, y}
			if seen[start] || grid.At(x, y).Terrain != world.Water {
				continue
			}
			cells := waterBodyCells(grid, start)
			for _, cell := range cells {
				seen[cell] = true
			}
			sort.Slice(cells, func(i, j int) bool {
				return fishScatterScore(cells[i].x, cells[i].y) < fishScatterScore(cells[j].x, cells[j].y)
			})
			for i, cell := range cells[:fishBodyLimit(cells)] {
				fish := building.NewFish(cell.x, cell.y)
				// A stable mixture of young, growing and mature fish makes an
				// initial pond immediately useful without skipping its growth loop.
				if i == 0 || fishScatterScore(cell.x, cell.y)&3 == 0 {
					fish.GrowthTicks = fish.GrowthTargetTicks
				} else {
					fish.GrowthTicks = fish.GrowthTargetTicks * int(20+fishScatterScore(cell.x, cell.y)%60) / 100
				}
				buildings = append(buildings, fish)
			}
		}
	}
	return buildings
}

type gridPoint struct{ x, y int }

func waterBodyCells(grid *world.Grid, start gridPoint) []gridPoint {
	if grid == nil || !grid.InBounds(start.x, start.y) || grid.At(start.x, start.y).Terrain != world.Water {
		return nil
	}
	seen := map[gridPoint]bool{start: true}
	queue := []gridPoint{start}
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		for _, d := range [...]gridPoint{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := gridPoint{p.x + d.x, p.y + d.y}
			if seen[n] || !grid.InBounds(n.x, n.y) || grid.At(n.x, n.y).Terrain != world.Water {
				continue
			}
			seen[n] = true
			queue = append(queue, n)
		}
	}
	return queue
}

func fishBodyLimit(cells []gridPoint) int {
	if len(cells) == 0 {
		return 0
	}
	limit := len(cells) * 40 / 100
	if limit < 1 {
		return 1
	}
	return limit
}

func fishScatterScore(x, y int) uint32 {
	return uint32(x)*83492791 ^ uint32(y)*2654435761 ^ 0xc2b2ae35
}

// ensureFish migrates saves made before fish became persistent water objects.
// A genuine all-caught pond still has delayed FishRegrowth entries, so it is
// not mistaken for an old save and refilled immediately.
func ensureFish(grid *world.Grid, buildings []*building.Building, hasRegrowth bool) ([]*building.Building, bool) {
	for _, b := range buildings {
		if b.Kind == building.Fish {
			return buildings, true
		}
	}
	if hasRegrowth {
		return buildings, false
	}
	return seedFish(grid, buildings), false
}

func (g *Game) catchFish(fish *building.Building) {
	if fish == nil || fish.Kind != building.Fish {
		return
	}
	for i, candidate := range g.buildings {
		if candidate != fish {
			continue
		}
		g.buildings = append(g.buildings[:i], g.buildings[i+1:]...)
		g.scheduleFishRegrowth(fish.X, fish.Y)
		return
	}
}

func (g *Game) scheduleFishRegrowth(waterX, waterY int) {
	seed := g.nextFishSeed()
	target := fishRegrowthMinTicks + int(seed%uint32(fishRegrowthVariationTicks))
	g.fishRegrowth = append(g.fishRegrowth, fishRegrowth{waterX: waterX, waterY: waterY, target: target, seed: seed})
}

func (g *Game) nextFishSeed() uint32 {
	if g.fishSeed == 0 {
		g.fishSeed = defaultFishSeed
	}
	g.fishSeed ^= g.fishSeed << 13
	g.fishSeed ^= g.fishSeed >> 17
	g.fishSeed ^= g.fishSeed << 5
	return g.fishSeed
}

func (g *Game) tickFishRegrowth() {
	for i := 0; i < len(g.fishRegrowth); {
		regrowth := &g.fishRegrowth[i]
		if regrowth.target <= 0 {
			regrowth.target = fishRegrowthMinTicks
		}
		if regrowth.ticks < regrowth.target {
			regrowth.ticks++
			i++
			continue
		}
		cells := waterBodyCells(g.grid, gridPoint{regrowth.waterX, regrowth.waterY})
		if len(cells) == 0 || countFishInCells(g.buildings, cells) >= fishBodyLimit(cells) {
			regrowth.ticks = regrowth.target - fishRegrowthRetryTicks
			if regrowth.ticks < 0 {
				regrowth.ticks = 0
			}
			i++
			continue
		}
		if x, y, ok := g.findFishSpawnCell(cells, regrowth.seed); ok {
			g.buildings = append(g.buildings, building.NewFish(x, y))
			g.fishRegrowth = append(g.fishRegrowth[:i], g.fishRegrowth[i+1:]...)
			continue
		}
		regrowth.ticks = regrowth.target - fishRegrowthRetryTicks
		if regrowth.ticks < 0 {
			regrowth.ticks = 0
		}
		i++
	}
}

func countFishInCells(buildings []*building.Building, cells []gridPoint) int {
	inBody := make(map[gridPoint]bool, len(cells))
	for _, cell := range cells {
		inBody[cell] = true
	}
	count := 0
	for _, b := range buildings {
		if b != nil && b.Kind == building.Fish && inBody[gridPoint{b.X, b.Y}] {
			count++
		}
	}
	return count
}

func (g *Game) findFishSpawnCell(cells []gridPoint, seed uint32) (int, int, bool) {
	bestScore := ^uint32(0)
	best := gridPoint{}
	found := false
	for _, cell := range cells {
		if !building.CanPlace(g.grid, g.buildings, building.Fish, cell.x, cell.y) {
			continue
		}
		score := fishScatterScore(cell.x, cell.y) ^ seed
		if !found || score < bestScore {
			bestScore, best, found = score, cell, true
		}
	}
	return best.x, best.y, found
}

func (g *Game) serializeFishRegrowth() []save.FishRegrowthState {
	states := make([]save.FishRegrowthState, 0, len(g.fishRegrowth))
	for _, r := range g.fishRegrowth {
		states = append(states, save.FishRegrowthState{WaterX: r.waterX, WaterY: r.waterY, Ticks: r.ticks, TargetTicks: r.target, Seed: r.seed})
	}
	return states
}

func restoreFishRegrowth(states []save.FishRegrowthState) []fishRegrowth {
	regrowth := make([]fishRegrowth, 0, len(states))
	for _, state := range states {
		ticks, target := state.Ticks, state.TargetTicks
		if ticks < 0 {
			ticks = 0
		}
		if target <= 0 {
			target = fishRegrowthMinTicks
		}
		regrowth = append(regrowth, fishRegrowth{waterX: state.WaterX, waterY: state.WaterY, ticks: ticks, target: target, seed: state.Seed})
	}
	return regrowth
}

// ensureTrees migrates saves created before persistent tree objects existed.
// Current saves already contain at least one Tree and are left untouched so
// each tree's individual growth timer remains authoritative.
func ensureTrees(grid *world.Grid, buildings []*building.Building, hasRegrowth bool) ([]*building.Building, bool) {
	for _, b := range buildings {
		if b.Kind == building.Tree {
			return buildings, true
		}
	}
	if hasRegrowth {
		return buildings, false
	}

	// The pre-tree prototype represented a forest as a 10x10 terrain patch.
	// Convert that legacy decoration back to ordinary grass before scattering
	// real tree objects, so loading an old save does not preserve a fake forest.
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if grid.At(x, y).Terrain != world.Forest {
				continue
			}
			grid.Set(x, y, world.Tile{Terrain: world.Grass})
		}
	}

	return seedTrees(grid, buildings), false
}

func (g *Game) cutTree(tree *building.Building) {
	if tree == nil || tree.Kind != building.Tree {
		return
	}
	for i, candidate := range g.buildings {
		if candidate != tree {
			continue
		}
		g.buildings = append(g.buildings[:i], g.buildings[i+1:]...)
		g.scheduleTreeRegrowth()
		return
	}
}

func (g *Game) scheduleTreeRegrowth() {
	seed := g.nextTreeSeed()
	target := treeRegrowthMinTicks + int(seed%uint32(treeRegrowthVariationTicks))
	g.treeRegrowth = append(g.treeRegrowth, treeRegrowth{target: target, seed: seed})
}

func (g *Game) nextTreeSeed() uint32 {
	if g.treeSeed == 0 {
		g.treeSeed = defaultTreeSeed
	}
	// xorshift32 is small, deterministic, and saves as one ordinary integer.
	g.treeSeed ^= g.treeSeed << 13
	g.treeSeed ^= g.treeSeed >> 17
	g.treeSeed ^= g.treeSeed << 5
	return g.treeSeed
}

func (g *Game) tickTreeRegrowth() {
	maxTrees := g.grid.Width * g.grid.Height / 100
	if maxTrees <= 0 {
		return
	}
	currentTrees := countTrees(g.buildings)
	for i := 0; i < len(g.treeRegrowth); {
		regrowth := &g.treeRegrowth[i]
		if regrowth.target <= 0 {
			regrowth.target = treeRegrowthMinTicks
		}
		if regrowth.ticks < regrowth.target {
			regrowth.ticks++
			i++
			continue
		}
		if currentTrees >= maxTrees {
			regrowth.ticks = regrowth.target
			i++
			continue
		}

		x, y, ok := g.findTreeSpawnCell(regrowth.seed)
		if !ok {
			// The settlement may have filled the available land. Retry later
			// without discarding the delayed respawn.
			regrowth.ticks = regrowth.target - treeRegrowthRetryTicks
			if regrowth.ticks < 0 {
				regrowth.ticks = 0
			}
			i++
			continue
		}

		g.buildings = append(g.buildings, building.NewTree(x, y))
		currentTrees++
		g.treeRegrowth = append(g.treeRegrowth[:i], g.treeRegrowth[i+1:]...)
	}
}

func (g *Game) findTreeSpawnCell(seed uint32) (int, int, bool) {
	bestScore := ^uint32(0)
	bestX, bestY := 0, 0
	found := false
	for y := 0; y < g.grid.Height; y++ {
		for x := 0; x < g.grid.Width; x++ {
			if !g.grid.At(x, y).Buildable() || !building.CanPlace(g.grid, g.buildings, building.Tree, x, y) {
				continue
			}
			score := treeScatterScore(x, y) ^ seed
			if !found || score < bestScore {
				bestScore = score
				bestX, bestY = x, y
				found = true
			}
		}
	}
	return bestX, bestY, found
}

// priorityEligible reports whether a building kind actually competes for a
// limited input, and so gets the supply-priority control in its
// inspector. A building with no Recipe.Inputs (a Farm gathering from the
// land, a Warehouse, ...) has nothing to prioritize against.
func priorityEligible(kind building.Kind) bool {
	return len(building.Types[kind].Recipe.Inputs) > 0
}

func countTrees(buildings []*building.Building) int {
	count := 0
	for _, b := range buildings {
		if b != nil && b.Kind == building.Tree {
			count++
		}
	}
	return count
}

func (g *Game) serializeTreeRegrowth() []save.TreeRegrowthState {
	regrowth := make([]save.TreeRegrowthState, 0, len(g.treeRegrowth))
	for _, r := range g.treeRegrowth {
		regrowth = append(regrowth, save.TreeRegrowthState{Ticks: r.ticks, TargetTicks: r.target, Seed: r.seed})
	}
	return regrowth
}

func restoreTreeRegrowth(states []save.TreeRegrowthState) []treeRegrowth {
	regrowth := make([]treeRegrowth, 0, len(states))
	for _, state := range states {
		ticks, target := state.Ticks, state.TargetTicks
		if ticks < 0 {
			ticks = 0
		}
		if target <= 0 {
			target = treeRegrowthMinTicks
		}
		regrowth = append(regrowth, treeRegrowth{ticks: ticks, target: target, seed: state.Seed})
	}
	return regrowth
}

func (g *Game) Draw(screen *ebiten.Image) {
	render.Tick()
	render.DrawGrid(screen, g.grid, g.camera)
	render.DrawBuildings(screen, g.grid, g.buildings, g.camera, g.unstaffedWorkerBuildings())
	render.DrawSerfs(screen, g.logi.Serfs, g.camera)
	render.DrawVillagers(screen, g.vills.Villagers, g.camera)
	render.DrawLumberjacks(screen, g.jacks.Lumberjacks, g.camera)
	render.DrawFishermen(screen, g.fishers.Fishermen, g.camera)
	render.DrawQuarrymen(screen, g.quarry.Quarrymen, g.camera)
	render.DrawBuilders(screen, g.builders.Builders, g.camera)
	render.DrawMiners(screen, g.miners.Miners, g.camera)

	mx, my := ebiten.CursorPosition()
	tx, ty := g.camera.ScreenToTile(mx, my)
	kind := g.palette.SelectedKind()
	if g.buildMode {
		valid := building.CanPlace(g.grid, g.buildings, kind, tx, ty)
		ui.DrawPlacementPreview(screen, g.camera, kind, tx, ty, valid)
	}

	ui.DrawBufferLevels(screen, g.buildings, g.camera)
	// Every non-road building exposes its access tile. This keeps the road
	// connection rule visible without requiring the player to click buildings
	// one by one; the inspector still explains the selected building in detail.
	for _, b := range g.buildings {
		if b.Kind != building.Road && b.Kind != building.Tree && b.Kind != building.Fish && b.Kind != building.Warehouse && b.Kind != building.StoneDeposit {
			ui.DrawAccessMarker(screen, g.camera, b, g.buildingConnected(b))
		}
	}
	ui.DrawSelectionMarker(screen, g.camera, g.selection)
	connected := false
	occupants := 0
	showPriority := false
	priorityLevel := 0
	if g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil {
		connected = g.buildingConnected(g.selection.Building)
		occupants = g.unitsAt(g.selection.Building)
		if priorityEligible(g.selection.Building.Kind) {
			showPriority = true
			priorityLevel = g.logi.Priority(g.selection.Building.Kind)
		}
	}
	ui.DrawResourceBarAt(screen, g.stock, g.pop, float64(g.layout.LeftWidth+16), 10)
	ui.DrawBuildPanel(screen, g.layout, g.palette, g.leftTab, g.hireOptions(), g.sim.Speed(), g.slotCache, g.dialog, g.dialogSlot, g.dialogText)
	ui.DrawInspectorPanel(screen, g.layout, g.selection, connected, g.stock, occupants, showPriority, priorityLevel)
	ui.DrawUnitControls(screen, g.layout, len(g.logi.Serfs))
	ui.DrawSpeedPanel(screen, g.layout, g.sim.Speed())

	ui.DrawText(screen, i18n.T().Help, float64(g.layout.LeftWidth+16), float64(g.layout.Height-20))
	if g.statusMsg != "" {
		ui.DrawText(screen, g.statusMsg, 8, float64(g.layout.Height-36))
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	if outsideWidth <= 0 || outsideHeight <= 0 {
		return screenWidth, screenHeight
	}
	g.resizeLayout(outsideWidth, outsideHeight)
	return outsideWidth, outsideHeight
}

func main() {
	i18n.SetLang(i18n.RU)

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle(i18n.T().WindowTitle)
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
