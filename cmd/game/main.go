// Command game is the entry point for the strategy game.
package main

import (
	"errors"
	"fmt"
	"image"
	"log"
	"math"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"strategy_game/internal/advisor"
	"strategy_game/internal/audio"
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
	"strategy_game/internal/worldclock"
)

const (
	screenWidth  = 1024
	screenHeight = 768
	panSpeed     = 8 // pixels per frame while a pan key is held
	edgePanSize  = 18
	edgePanSpeed = 6

	framesPerSimTick = 30 // simulation ticks run at 2/sec on a 60fps display

	// A warehouse is intentionally unlimited. A positive capacity is still
	// supported by resource.Stockpile for isolated tests and future stores.
	stockpileCapacity = 0
	startingSerfs     = 3

	// mapWidth/mapHeight is the fixed size of every procedurally generated
	// map (see generateGrid). Halved from an earlier 100x75 per the user's
	// direct request ("уменьши карту в 2 раза") -- every other spatial
	// constant below that measures a distance in tiles rather than a
	// percentage of map area (minDepositDistanceFromWarehouse,
	// maxWarehouseDistanceFromWater, warehouseEdgeMargin,
	// treeRegrowthRadius, fishRegrowthRadius) is halved right along with
	// it, so a "20 tiles from the coast" rule stays the same fraction of
	// the map instead of suddenly covering most of it. Generation
	// abundance itself (sea/ore/thicket percent, tree/fish density) is
	// already expressed as a percentage of map area, so it scales down
	// automatically with no separate change needed. Only the terrain
	// layout within this fixed size is randomized per game, not the
	// dimensions themselves.
	mapWidth  = 50
	mapHeight = 38

	// Named save-panel slots (pause menu) live in their own
	// files. Saving/loading is mouse-only from the Esc pause menu -- there
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
	// currency. Iron and Gold are deliberately fixed at a single value
	// (min == max), not a range: with each deposit already holding far
	// more Reserve than any realistic session can exhaust (see
	// building.OreDepositReserve's doc comment), the range's only real
	// effect was cosmetic seed-to-seed variety, and the user asked for a
	// specific, predictable density instead.
	//
	// These replace the old two-layer scheme (a wider raw percent times a
	// separate oreDepositGenerationPercent/stoneDepositGenerationPercent
	// scaling knob) with one direct number per the user's explicit request
	// ("камень я бы сделал 3-4%, уголь 2-3%, железная руда 1% и золото
	// 1%") -- see scaledDepositCells/scaledStoneDepositCells below, which
	// now apply a percentage straight to the map area with no hidden
	// second multiplier.
	coalMinPercent, coalMaxPercent       = 2, 3
	ironOreMinPercent, ironOreMaxPercent = 1, 1
	goldOreMinPercent, goldOreMaxPercent = 1, 1

	// stoneMinPercent/stoneMaxPercent: see the ore ranges' doc comment
	// above for why these are a direct percent of map area now, with no
	// separate scaling layer on top. Fixed at a single 2% (min == max,
	// same convention as Iron/Gold above), per the user's follow-up
	// ("камень 2%") lowering it from the initial 3-4% range.
	stoneMinPercent, stoneMaxPercent = 2, 2

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

	// No cosmetic Fertile terrain is generated -- the user asked explicitly
	// not to ("пахотные поля не генерируем вообще! коричневые области"). A
	// Farm builds on any dry tile regardless of terrain
	// (building.Types[Farm].AllowedTerrain is empty), so this loses no
	// gameplay; world.Fertile itself stays defined for old saves that
	// already have it painted from before this change.

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
	// удалены от первоначального склада, минимум 20 клеток" -- halved
	// along with mapWidth/mapHeight, see that constant's doc comment).
	minDepositDistanceFromWarehouse = 10

	// maxBuilders is a flat town-wide cap, unlike every other profession
	// (which is capped by matching building count instead) -- a Builder has
	// no dedicated hut to be limited by.
	maxBuilders = 3

	// Starting stockpile: enough construction material for several ordinary
	// buildings (see building.Type's PlankCost/StoneCost, a flat 5+5 each),
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
	// originX/originY is where the tree that made room for this regrowth
	// was actually cut -- see treeRegrowthRadius's doc comment.
	originX, originY int
	ticks, target    int
	seed             uint32
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

	// connectionCache memoizes the expensive road-only BFS used by the
	// connection marker and inspector. It is cleared whenever a player
	// changes the road/building topology, never every render frame.
	connectionCache map[*building.Building]bool

	// recommendedServeCountCache memoizes recommendedServeCount, which runs
	// one pathfind.FindPath BFS per production building -- exactly the cost
	// pattern that was a real "hundreds of full BFS calls every frame" bug
	// elsewhere in this file (see showsAccessMarker's doc comment), so this
	// must never be recomputed on every Draw. Invalidated by the same event
	// as connectionCache (see invalidateConnectionCache): the recommendation
	// can only change when the road/building layout does. nil means "not
	// computed yet for the current layout".
	recommendedServeCountCache *int

	// Advisor state (see internal/advisor and tickAdvisor/advisorTipText).
	// Deliberately not part of save.GameState -- the same reasoning as
	// weather/lighting (see AGENTS.md): not world state, so it just
	// starts fresh after a load rather than adding save-format surface.
	advisorCheckTicks int
	// advisorIdleSince tracks, per unstaffed RequiresWorker building, the
	// worldTicks value it first became unstaffed -- rebuilt fresh every
	// tick from the current unstaffedWorkerBuildings() (see
	// trackAdvisorIdleBuildings), so a restaffed or removed building's
	// entry disappears on its own without separate pruning.
	advisorIdleSince map[*building.Building]int
	// advisorGatherStuckSince is advisorIdleSince's counterpart for a
	// lumberjack/quarryman/miner that's staffed and searching but can't
	// find any tree/stone deposit/ore deposit -- keyed by the worker's
	// home hut, the tick it most recently entered its own package's
	// StateIdle (see trackAdvisorGatherWorkers).
	advisorGatherStuckSince map[*building.Building]int
	// advisorCooldowns maps a tip Kind to the worldTicks value it may next
	// be queued at, set when the player acknowledges it -- an ignored tip
	// (never acknowledged) stays queued indefinitely instead of vanishing
	// on its own.
	advisorCooldowns map[advisor.Kind]int
	advisorQueue     []advisor.Tip
	advisorVisible   *advisor.Tip

	camera         *render.Camera
	palette        *ui.Palette
	layout         ui.Layout
	leftTab        ui.LeftTab
	buildMode      bool
	demolitionMode bool
	// wallAnchor is the first/end point of the current chained wall draw.
	// Nothing is reserved until a second click commits an orthogonal run.
	wallAnchor    building.Point
	wallAnchored  bool
	selection     ui.Selection
	middlePanning bool
	leftPanning   bool
	leftPanMoved  bool
	lastMouseX    int
	lastMouseY    int

	// Esc pause menu save/load modal: dialog is DialogNone outside of the
	// naming/overwrite flow, in which case dialogSlot/dialogText are unused.
	// slotCache holds the five save-panel slots' names and occupancy, kept
	// current so the Esc pause menu can redraw it every frame without paying
	// the cost of reading and parsing five files every frame -- see
	// refreshSlotCache.
	dialog     ui.DialogKind
	dialogSlot int
	dialogText string
	slotCache  []ui.SaveSlotInfo

	// autosaveSlot is which save-panel slot (1-slotCount) periodic
	// autosave silently re-saves into every autosaveIntervalTicks
	// simulation ticks; 0 means autosave is off. A live, session-only
	// preference like language/speed -- not persisted across launches,
	// and not reset by New Game or Load, see toggleAutosaveSlot's doc
	// comment. autosaveTicks counts simulation ticks since the last
	// autosave (or since it was turned on), reset to 0 on every trigger.
	autosaveSlot  int
	autosaveTicks int

	statusMsg string

	// frontWidth/frontHeight are the actual draw-buffer size of the title UI.
	// Fullscreen input must use these, not WindowSize's stale logical size.
	frontWidth  int
	frontHeight int

	// paused freezes every simulation tick and routes all input to the Esc
	// menu. pauseHelp is the menu's embedded Markdown manual subpage.
	paused    bool
	pauseHelp bool

	// screen is screenPlay during a running settlement. The other values use
	// the same renderer for the title presentation, save picker and manual,
	// but deliberately do not advance the simulation.
	screen     appScreen
	titleFrame int
	helpPages  []helpPage
	helpPage   int

	// worldTicks is the game clock (see internal/worldclock): total
	// simulation ticks elapsed, incremented once per tick inside the
	// g.sim.Advance() loop in Update. It drives visual ambient effects only;
	// playedFrames below is the player-facing, persisted play-time counter.
	worldTicks int

	// playedFrames counts active play updates at Ebiten's fixed 60 TPS. It is
	// deliberately independent of simulation speed: playing at 16x advances
	// the settlement faster, but does not claim the player spent sixteen times
	// longer in the game. Pause screens and title/help screens do not add time.
	playedFrames int
}

// autosaveIntervalTicks is how often (in simulation ticks, not render
// frames or wall-clock time) a designated autosave slot is silently
// re-saved -- about 16-17 minutes of played time at Normal speed
// (framesPerSimTick=30, 60fps => 2 ticks/sec => 2000 ticks ~= 1000s), the
// same order of magnitude as the roadmap's other tick-scale constants
// (hunger.MaxTicks etc.), not wall-clock: a paused or slow game
// legitimately autosaves less often in real time, matching how nothing
// else in the simulation is clocked to the wall either.
const autosaveIntervalTicks = 2000

// NewGame creates a standard playable settlement. The separate sized factory
// below is also used by the non-simulating title presentation map.
func NewGame() *Game {
	return newGameWithSize(mapWidth, mapHeight)
}

func newGameWithSize(width, height int) *Game {
	mapSeed := newMapSeed()
	grid := generateGrid(width, height, mapSeed)

	warehousePoint, ok := findWarehouseSpot(grid, mapSeed^0xc2b2ae35)
	if !ok {
		// Should be unreachable given seaMaxPercent leaves most of the map
		// as plain grass -- but a game must still start rather than panic
		// if generation ever produces a map this crowded.
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
	// No resize call here on purpose. ebiten.WindowSize() reports a stale
	// logical size in fullscreen (see frontWidth/frontHeight's comment
	// below) -- calling resizeLayout from it clobbered the correct
	// g.layout with wrong dimensions right before every click below gets
	// hit-tested against it, which is exactly what made buttons register
	// clicks at the wrong screen position ("клик происходит в другой
	// части"). Layout() and Draw() already call resizeLayout every frame
	// with the real negotiated size -- the same coordinate space
	// ebiten.CursorPosition() reports in -- so g.layout is already correct
	// by the time Update runs; this call was redundant and harmful.
	if g.screen != screenPlay {
		return g.updateFrontScreen()
	}
	if g.paused {
		return g.updatePauseMenu()
	}
	// Esc always opens the pause menu before map input or the simulation can
	// advance. Object-confirmation dialogs still own Esc and use it to cancel.
	if g.dialog == ui.DialogNone && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.openPauseMenu()
		return nil
	}
	g.playedFrames++
	g.handleCameraPan()
	g.handleCameraZoom()

	// A modal owns every key and click, so its destructive confirmation or
	// save-name input cannot accidentally also change the map behind it.
	if g.dialog != ui.DialogNone {
		g.handleDialogInput()
	} else {
		g.handleMouse()
	}

	for range g.sim.Advance() {
		g.worldTicks++
		for _, b := range g.buildings {
			b.TickGrowth()
		}
		g.tickTreeRegrowth()
		g.tickFishRegrowth()
		g.updateAutomaticGates()
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
		g.pop.UnitsDismissed += serfResult.Dismissed
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
			case builder.WorkerDismissed:
				g.pop.UnitsDismissed++
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
		g.tickAutosave()
		g.tickAdvisor()
		g.tickAudioCues()
		g.tickBuildingAmbientSounds()
		g.tickWeatherAmbientSounds()
		g.tickDayNightAmbientSounds()
		g.tickWindAmbientSounds()
	}

	return nil
}

func (g *Game) resizeLayout(width, height int) {
	if width <= 0 || height <= 0 || (g.layout.Width == width && g.layout.Height == height) {
		return
	}
	g.layout = ui.NewLayout(width, height)
	if g.screen != screenPlay {
		g.camera.SetViewport(0, 0, width, height)
		return
	}
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

// finishedBuildingCounts counts already-built (not under construction)
// buildings of each kind, for the left panel's Build tab -- a "(N)" next
// to every card's name, by direct request ("по аналогии с количеством
// юнитов в скобках"), so the player can see how many of a building they
// already have without opening the inspector for each one. A site still
// under construction doesn't count yet, same convention hireOptions'
// own countBuildings below already uses.
func (g *Game) finishedBuildingCounts() map[building.Kind]int {
	counts := make(map[building.Kind]int)
	for _, b := range g.buildings {
		if b.ConstructionStage == building.ConstructionNone {
			counts[b.Kind]++
		}
	}
	return counts
}

// completedTownBuildingCount reports finished player structures for the town
// summary. Roads and naturally generated objects are deliberately excluded:
// the number answers "how many buildings does my town have?", not "how many
// occupied map cells exist?".
func (g *Game) completedTownBuildingCount() int {
	count := 0
	for _, b := range g.buildings {
		if b.ConstructionStage != building.ConstructionNone {
			continue
		}
		switch b.Kind {
		case building.Road, building.StoneWall, building.Gate, building.Tree, building.Fish, building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
			continue
		}
		count++
	}
	return count
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
		return ui.HireOption{Kind: kind, Current: current, Limit: limit, Available: current < limit && afford, GoldCost: unitHireCost}
	}
	return []ui.HireOption{
		{Kind: ui.HireSerf, Current: len(g.logi.Serfs), Limit: 0, Available: afford, Recommended: g.recommendedServeCount(), GoldCost: unitHireCost},
		limited(ui.HireFarmer, building.Farm, countProfession(villagers.Farmer)),
		limited(ui.HireBaker, building.Bakery, countProfession(villagers.Baker)),
		limited(ui.HireWinemaker, building.Winery, countProfession(villagers.Winemaker)),
		limited(ui.HireLumberjack, building.LumberjackHut, len(g.jacks.Lumberjacks)),
		limited(ui.HireFisherman, building.FisherHut, len(g.fishers.Fishermen)),
		limited(ui.HireSwineherd, building.PigFarm, countProfession(villagers.Swineherd)),
		limited(ui.HireButcher, building.MeatWorkshop, countProfession(villagers.Butcher)),
		limited(ui.HireCarpenter, building.CarpentryWorkshop, countProfession(villagers.Carpenter)),
		limited(ui.HireQuarryman, building.QuarryHut, len(g.quarry.Quarrymen)),
		{Kind: ui.HireBuilder, Current: len(g.builders.Builders), Limit: maxBuilders, Available: len(g.builders.Builders) < maxBuilders && afford, GoldCost: unitHireCost},
		limited(ui.HireMiner, building.MinerHut, len(g.miners.Miners)),
		limited(ui.HireSmelter, building.Smeltery, countProfession(villagers.Smelter)),
	}
}

// serfHaulOutputRate returns the (amount, ticks) of one production cycle for
// kind, the same shape as building.Recipe's OutputAmount/TicksToProduce but
// covering the four "gather hut" kinds too (LumberjackHut, QuarryHut,
// MinerHut, FisherHut), which have no Recipe at all -- their worker walks
// out, gathers, and deposits directly into OutputBuffer (see each
// profession package's doc comment). All four share the same 12-tick gather
// cycle (ChopTicks/MineTicks/CatchTicks); QuarryHut is the one exception
// that converts on unload (1 mined stone -> 2 Stone Blocks, see package
// quarry), hence amount 2 instead of 1. This ignores the hut worker's own
// walk time to the resource, which recommendedServeCount's doc comment
// explains is a deliberate, safe simplification.
func serfHaulOutputRate(kind building.Kind) (amount, ticks int) {
	switch kind {
	case building.LumberjackHut, building.MinerHut, building.FisherHut:
		return 1, 12
	case building.QuarryHut:
		return 2, 12
	}
	recipe := building.Types[kind].Recipe
	return recipe.OutputAmount, recipe.TicksToProduce
}

// serfHaulWorkload estimates how much of one serf's continuous attention a
// single production building demands, as a fraction (1.0 = fully occupies
// one serf): production rate (units/tick) times one round trip to the
// Warehouse (in ticks), divided by how much a serf carries per trip. A
// building whose demand exceeds what a single serf can deliver in the time
// it takes to produce another full load needs more than 1.0 of a serf's
// time to keep its buffer from backing up.
func serfHaulWorkload(kind building.Kind, roadTilesOneWay int) float64 {
	amount, ticks := serfHaulOutputRate(kind)
	if ticks <= 0 || amount <= 0 {
		return 0
	}
	rate := float64(amount) / float64(ticks)
	roundTripTicks := float64(2 * roadTilesOneWay * logistics.TicksPerTile)
	return rate * roundTripTicks / float64(logistics.CarryCapacity)
}

// recommendedServeCount sums serfHaulWorkload across every finished
// production building reachable from the Warehouse, rounds up to a whole
// serf count, and adds one flat serf for duties this estimate doesn't
// otherwise cover at all (delivering construction materials, carrying
// finished food specifically to the Tavern rather than the Warehouse).
// Shown next to the Serf hire card per the user's explicit request ("в меню
// 'Юниты' рядом со слугами показывать рекомендацию сколько рекомендуется
// слуг").
//
// Deliberately counts every finished building regardless of whether it
// currently has a resident worker: an unstaffed building produces nothing
// *right now*, but the player will presumably staff it, and the
// recommendation is meant to answer "how many serfs does this settlement
// need once it's running at the capacity I've already built", not just
// "right now this instant". Memoized in recommendedServeCountCache -- see
// its doc comment for why this must never run on every Draw.
func (g *Game) recommendedServeCount() int {
	if g.recommendedServeCountCache != nil {
		return *g.recommendedServeCountCache
	}
	warehouse := findWarehouse(g.buildings)
	total := 0.0
	if warehouse != nil {
		for _, b := range g.buildings {
			if b == nil || b == warehouse || b.ConstructionStage != building.ConstructionNone {
				continue
			}
			amount, ticks := serfHaulOutputRate(b.Kind)
			if ticks <= 0 || amount <= 0 {
				continue
			}
			path, ok := pathfind.FindPath(g.buildings, warehouse, b)
			if !ok {
				continue // not yet road-connected, no hauling demand to plan for
			}
			total += serfHaulWorkload(b.Kind, len(path))
		}
	}
	result := int(math.Ceil(total)) + 1
	g.recommendedServeCountCache = &result
	return result
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

	mx, my := ebiten.CursorPosition()
	mapRect := g.layout.MapRect()
	windowRect := image.Rect(0, 0, g.layout.Width, g.layout.Height)
	mapPoint := image.Pt(mx, my)
	// Moving the cursor to a screen edge scrolls the map, including into a
	// corner for diagonal panning. This used to gate on mapRect (the map
	// strip between the side panels), which matched the vertical edges --
	// there's no top/bottom panel, so mapRect's Y bounds are already the
	// window's -- but made the horizontal edges nearly unreachable: the
	// trigger band sat just inside the map's own border, tens to hundreds
	// of pixels short of the window edge a player's cursor actually stops
	// at, and the side panels swallowed a corner entirely, killing
	// diagonal panning near any of the four corners too. Every left/right
	// panel button stays a comfortable 12px+ inset from the panel's own
	// outer edge (see e.g. layout.go's card rects), so the outer
	// edgePanSize band at the literal window border is safe to use here.
	// A fresh left click is still excluded so selecting a border tile
	// cannot nudge the target out from under the cursor.
	if mapPoint.In(windowRect) && !ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) && !g.leftPanning && !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if mx < edgePanSize {
			dx -= edgePanSpeed
		} else if g.layout.Width-mx <= edgePanSize {
			dx += edgePanSpeed
		}
		if my < edgePanSize {
			dy -= edgePanSpeed
		} else if g.layout.Height-my <= edgePanSize {
			dy += edgePanSpeed
		}
	}
	if dx != 0 || dy != 0 {
		g.camera.Pan(dx, dy, g.grid.Width, g.grid.Height, mapRect.Dx(), mapRect.Dy())
	}

	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		if !g.middlePanning {
			if !mapPoint.In(mapRect) {
				return
			}
			g.middlePanning = true
			g.lastMouseX, g.lastMouseY = mx, my
			return
		}
		g.camera.Pan(float64(g.lastMouseX-mx), float64(g.lastMouseY-my), g.grid.Width, g.grid.Height, mapRect.Dx(), mapRect.Dy())
		g.lastMouseX, g.lastMouseY = mx, my
		return
	}
	g.middlePanning = false
}

// handleCameraZoom restores cursor-anchored wheel zoom for the map itself.
// The pause menu intentionally has no duplicate zoom setting.
func (g *Game) handleCameraZoom() {
	mx, my := ebiten.CursorPosition()
	if !image.Pt(mx, my).In(g.layout.MapRect()) {
		return
	}
	_, wheelY := ebiten.Wheel()
	if wheelY != 0 {
		g.camera.ZoomAt(wheelY, mx, my, g.grid.Width, g.grid.Height)
	}
}
func (g *Game) handleMouse() {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		mx, my := ebiten.CursorPosition()
		if g.handleHireCardDismissRightClick(mx, my) {
			return
		}
		g.buildMode = false
		g.demolitionMode = false
		g.clearWallAnchor()
		g.leftPanning = false
		g.selection.Clear()
		g.statusMsg = ""
		return
	}

	mx, my := ebiten.CursorPosition()
	if g.handleLeftMapDrag(mx, my) {
		return
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		g.handleLeftClick(mx, my)
	}
}

// handleHireCardDismissRightClick lets the player right-click the Serf or
// Builder card in the Hire tab to dismiss some, the reverse of a
// left-click there hiring one. Per the user's explicit requests ("ПКМ по
// слуге в левом меню во вкладке Юниты сокращает 1 слугу, ПКМ по
// строителю - сокращает строителя"; later, for Serf specifically, "чтобы
// уменьшал автоматически до рекомендованного значения" -- see
// trimSerfsToRecommended). Only Serf and Builder support this: every
// other profession is tied 1:1 to a specific workplace building (see
// hireOptions), so dismissing one of those from a flat headcount card
// wouldn't have an unambiguous building to vacate -- removing the
// workplace itself, via the inspector, is how those are let go.
func (g *Game) handleHireCardDismissRightClick(mx, my int) bool {
	if g.leftTab != ui.HireTab {
		return false
	}
	options := g.hireOptions()
	index, ok := g.layout.HireIndexAt(mx, my, len(options))
	if !ok || index >= len(options) {
		return false
	}
	switch options[index].Kind {
	case ui.HireSerf:
		// Per the user's follow-up: only ask when there's actually an
		// excess to trim ("если количество слуг меньше чем рекомендовано
		// то ПКМ так же как и раньше уменьшает на единицу") -- otherwise
		// this behaves exactly like before, dismissing just one.
		active := 0
		for _, s := range g.logi.Serfs {
			if !s.Dismissing() {
				active++
			}
		}
		if active <= options[index].Recommended {
			g.dismissOneSerf()
			return true
		}
		g.dialog = ui.DialogConfirmTrimServes
		return true
	case ui.HireBuilder:
		if len(g.builders.Builders) == 0 {
			return true
		}
		if g.builders.RequestDismissal(g.builders.Builders[0]) {
			g.statusMsg = i18n.T().BuilderDismissRequested
		}
		return true
	}
	return false
}

// trimSerfsToRecommended dismisses however many serfs are above
// recommended, all in one click, per the user's explicit request ("клик
// по слуге правой кнопкой мышки во вкладке юниты, чтобы уменьшал
// автоматически до рекомендованного значения. При этом слуги заканчивают
// свои задания и удаляются"). Dismissal itself still goes through
// logistics.Controller.RequestDismissal, the same safe mechanism a single
// right-click always used -- a dismissed serf finishes whatever haul it's
// currently on before actually leaving, nothing is dropped or lost.
//
// Counts against serfs not already dismissing (Serf.Dismissing()), not
// the raw headcount: a repeated click before earlier dismissals have
// actually left must not re-mark the same serfs and undercount, and must
// not dismiss more than the excess actually still active.
func (g *Game) trimSerfsToRecommended(recommended int) {
	active := 0
	for _, s := range g.logi.Serfs {
		if !s.Dismissing() {
			active++
		}
	}
	if active <= recommended {
		return
	}
	toDismiss := active - recommended
	dismissed := 0
	for _, s := range g.logi.Serfs {
		if dismissed >= toDismiss {
			break
		}
		if s.Dismissing() {
			continue
		}
		if g.logi.RequestDismissal(s) {
			dismissed++
		}
	}
	if dismissed > 0 {
		g.statusMsg = fmt.Sprintf(i18n.T().SerfsTrimmedToRecommended, dismissed, recommended)
	}
}

// dismissOneSerf is the original, single-serf right-click behavior --
// still used when there's no excess to trim (the current count is already
// at or below recommended) and as the "Нет" answer in
// handleConfirmTrimServesInput when the player would rather dismiss just
// one instead of the whole excess at once.
func (g *Game) dismissOneSerf() {
	if len(g.logi.Serfs) == 0 {
		return
	}
	if g.logi.RequestDismissal(g.logi.Serfs[0]) {
		g.statusMsg = i18n.T().SerfDismissRequested
	}
}

// handleLeftMapDrag delays a map click until release. A short press remains a
// normal select/build click; once the pointer travels beyond the dead zone,
// holding the left button pans the camera and never changes selection.
func (g *Game) handleLeftMapDrag(mx, my int) bool {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if !image.Pt(mx, my).In(g.layout.MapRect()) {
			return false
		}
		g.leftPanning = true
		g.leftPanMoved = false
		g.lastMouseX, g.lastMouseY = mx, my
		return true
	}
	if !g.leftPanning {
		return false
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		dx, dy := g.lastMouseX-mx, g.lastMouseY-my
		if !g.leftPanMoved && dx*dx+dy*dy < 25 {
			return true
		}
		if dx != 0 || dy != 0 {
			g.leftPanMoved = true
			mapRect := g.layout.MapRect()
			g.camera.Pan(float64(dx), float64(dy), g.grid.Width, g.grid.Height, mapRect.Dx(), mapRect.Dy())
			g.lastMouseX, g.lastMouseY = mx, my
		}
		return true
	}

	wasDrag := g.leftPanMoved
	g.leftPanning = false
	g.leftPanMoved = false
	if !wasDrag {
		g.handleLeftClick(mx, my)
	}
	return true
}
func (g *Game) handleLeftClick(mx, my int) {
	// The advisor toast floats over the map itself (see AGENTS.md) rather
	// than blocking input like a modal dialog, so its button is checked
	// first, ahead of even the minimap: it's drawn on top of everything
	// else, so a click there should never fall through to whatever
	// happens to be underneath it on the map.
	if g.advisorVisible != nil {
		if g.advisorVisible.Building != nil && g.layout.AdvisorGoToAt(mx, my) {
			g.focusAdvisorBuilding()
			return
		}
		if g.layout.AdvisorAcknowledgeAt(mx, my) {
			g.acknowledgeAdvisorTip()
			return
		}
	}
	// The minimap centers the camera on the clicked point instead of
	// selecting anything -- checked first so it always wins over whatever
	// the inspector happens to show underneath it.
	if worldX, worldY, ok := render.MinimapWorldPoint(g.grid, g.layout.MinimapRect(), mx, my); ok {
		g.camera.CenterOn(worldX, worldY, g.grid.Width, g.grid.Height)
		return
	}
	if tab, ok := g.layout.MenuTabAt(mx, my); ok {
		g.leftTab = tab
		g.buildMode = false
		g.demolitionMode = false
		g.clearWallAnchor()
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
	default:
		if g.layout.DemolitionModeAt(mx, my) {
			g.buildMode = false
			g.clearWallAnchor()
			g.selection.Clear()
			if g.demolitionMode {
				g.demolitionMode = false
			} else {
				g.dialog = ui.DialogConfirmDemolitionMode
			}
			g.statusMsg = ""
			return
		}
		if index, ok := g.layout.BuildIndexAt(mx, my, len(g.palette.Kinds)); ok {
			g.palette.Select(index)
			g.buildMode = true
			g.clearWallAnchor()
			g.statusMsg = ""
			return
		}
	}

	showPriority := g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil && priorityEligible(g.selection.Building.Kind)
	if ui.CanRemoveSelection(g.selection) && g.layout.InspectorRemoveAt(mx, my, showPriority) {
		g.requestSelectedRemoval()
		return
	}
	if showPriority {
		if level, ok := g.layout.PriorityLevelAt(mx, my); ok {
			g.logi.SetPriority(g.selection.Building.Kind, level)
			return
		}
	}
	if g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil &&
		g.selection.Building.Kind == building.Gate && g.selection.Building.ConstructionStage == building.ConstructionNone {
		if g.layout.GateToggleAt(mx, my) {
			g.selection.Building.GateAuto = false
			g.selection.Building.GateOpen = !g.selection.Building.GateOpen
			g.invalidateConnectionCache()
			return
		}
		if g.layout.GateAutoAt(mx, my) {
			g.selection.Building.GateAuto = !g.selection.Building.GateAuto
			if g.selection.Building.GateAuto {
				g.updateAutomaticGates()
			}
			g.invalidateConnectionCache()
			return
		}
	}
	point := image.Pt(mx, my)
	if point.In(g.layout.LeftPanel()) ||
		point.In(g.layout.RightPanel()) {
		return
	}
	if g.demolitionMode {
		// In continuous demolition, a road must win over a unit standing on it:
		// the mode only removes map construction and never dismisses citizens.
		g.selection = g.buildingSelectionAt(mx, my)
		if g.selection.Kind == ui.SelectionBuilding {
			g.deleteSelectedBuilding()
		}
		return
	}
	if g.buildMode {
		// Construction owns map clicks while a palette item is selected. A unit
		// walking across the target tile must never steal the click from a road
		// or other planned building.
		g.selection.Clear()
	} else if selected := g.selectionAt(mx, my); selected.Kind != ui.SelectionNone {
		g.selection = selected
		return
	} else {
		g.selection.Clear()
		return
	}
	tx, ty := g.camera.ScreenToTile(mx, my)

	kind := g.palette.SelectedKind()
	if kind == building.StoneWall {
		g.placeWallPoint(tx, ty)
		return
	}
	if kind == building.Gate {
		g.placeGateAt(tx, ty)
		return
	}
	if !building.CanPlace(g.grid, g.buildings, kind, tx, ty) {
		g.statusMsg = i18n.T().CantBuildHere
		return
	}

	// Every newly placed building starts with one completed road tile at its
	// marked entrance. It makes the connection point unambiguous while the
	// foundation is still under construction; the player then extends this
	// visible anchor to the rest of the road network. Roads themselves remain
	// individually constructed and do not receive another starter road.
	var starterRoad *building.Building
	if kind != building.Road && kind != building.StoneWall && kind != building.Gate {
		var roadOK bool
		starterRoad, roadOK = building.FoundationRoad(g.grid, g.buildings, kind, tx, ty)
		if !roadOK {
			g.statusMsg = i18n.T().CantBuildHere
			return
		}
	}

	// Placement only reserves the footprint and starts a construction site
	// (see package builder) -- it neither produces nor, for a Warehouse,
	// acts as a logistics endpoint until a Builder actually finishes it;
	// see finishConstruction.
	placed := building.NewConstructionSite(kind, tx, ty)
	g.reserveConstructionMaterials(placed)
	g.buildings = append(g.buildings, placed)
	if starterRoad != nil {
		g.buildings = append(g.buildings, starterRoad)
	}
	g.invalidateConnectionCache()
	g.statusMsg = ""
	if tip, missing := advisor.ConstructionMaterialShortage(placed); missing {
		g.queueAdvisorTip(tip)
	}
}

// reserveConstructionMaterials moves everything currently affordable out of
// the shared warehouse stockpile into a new site's construction buffer. The
// site therefore owns those materials immediately, even before a Builder has
// reached its foundation; cancellation already returns that buffer to stock.
// Anything unavailable stays short and ordinary logistics supplies it later.
// clearWallAnchor cancels only the in-progress drag chain; already committed
// wall cells are ordinary independent construction sites and remain intact.
func (g *Game) clearWallAnchor() {
	g.wallAnchored = false
}

// placeWallPoint creates a chained orthogonal run. The first click only marks
// an anchor; every following click commits a shortest horizontal-then-vertical
// or vertical-then-horizontal route and makes that endpoint the new anchor.
// Right click exits build mode and clears the chain (handleMouse).
func (g *Game) placeWallPoint(x, y int) {
	point := building.Point{X: x, Y: y}
	if !g.wallAnchored {
		if !g.wallPieceAt(x, y) && !building.CanPlace(g.grid, g.buildings, building.StoneWall, x, y) {
			g.statusMsg = i18n.T().CantBuildHere
			return
		}
		g.wallAnchor, g.wallAnchored = point, true
		g.statusMsg = ""
		return
	}

	paths := [][]building.Point{wallPath(g.wallAnchor, point, true)}
	if g.wallAnchor.X != point.X && g.wallAnchor.Y != point.Y {
		paths = append(paths, wallPath(g.wallAnchor, point, false))
	}
	for _, path := range paths {
		if g.commitWallPath(path) {
			g.wallAnchor = point
			g.statusMsg = ""
			return
		}
	}
	g.statusMsg = i18n.T().CantBuildHere
}

// wallPath returns an inclusive Manhattan route. horizontalFirst determines
// the bend when the two clicks differ on both axes; no diagonal wall segments
// are created.
func wallPath(from, to building.Point, horizontalFirst bool) []building.Point {
	out := []building.Point{{X: from.X, Y: from.Y}}
	appendLine := func(x0, y0, x1, y1 int) {
		dx, dy := 0, 0
		if x1 > x0 {
			dx = 1
		} else if x1 < x0 {
			dx = -1
		}
		if y1 > y0 {
			dy = 1
		} else if y1 < y0 {
			dy = -1
		}
		for x, y := x0+dx, y0+dy; x != x1+dx || y != y1+dy; x, y = x+dx, y+dy {
			out = append(out, building.Point{X: x, Y: y})
		}
	}
	if horizontalFirst {
		appendLine(from.X, from.Y, to.X, from.Y)
		appendLine(to.X, from.Y, to.X, to.Y)
	} else {
		appendLine(from.X, from.Y, from.X, to.Y)
		appendLine(from.X, to.Y, to.X, to.Y)
	}
	return out
}

// commitWallPath validates the entire run before adding its first foundation,
// so a failed bend never leaves a partial accidental wall. Existing wall/gate
// pieces are intentionally reusable: this is how a later segment closes a
// loop or meets a gate without replacing it.
func (g *Game) commitWallPath(path []building.Point) bool {
	for _, point := range path {
		if g.wallPieceAt(point.X, point.Y) {
			continue
		}
		if !building.CanPlace(g.grid, g.buildings, building.StoneWall, point.X, point.Y) {
			return false
		}
	}
	var shortage *advisor.Tip
	for _, point := range path {
		if g.wallPieceAt(point.X, point.Y) {
			continue
		}
		site := building.NewConstructionSite(building.StoneWall, point.X, point.Y)
		g.reserveConstructionMaterials(site)
		g.buildings = append(g.buildings, site)
		if shortage == nil {
			if tip, missing := advisor.ConstructionMaterialShortage(site); missing {
				shortage = &tip
			}
		}
	}
	g.invalidateConnectionCache()
	if shortage != nil {
		g.queueAdvisorTip(*shortage)
	}
	return true
}

func (g *Game) wallPieceAt(x, y int) bool {
	for _, b := range g.buildings {
		if b != nil && building.IsWallKind(b.Kind) && b.X == x && b.Y == y {
			return true
		}
	}
	return false
}

// placeGateAt upgrades an already finished, straight wall piece in place. The
// underlying segment survives a cancelled construction so no one gets an
// unintentional breach after reserving a gate by mistake.
func (g *Game) placeGateAt(x, y int) {
	var wall *building.Building
	for _, b := range g.buildings {
		if b != nil && b.Kind == building.StoneWall && b.ConstructionStage == building.ConstructionNone && b.X == x && b.Y == y {
			wall = b
			break
		}
	}
	axis, valid := building.WallAxisAt(g.buildings, x, y)
	if wall == nil || !valid {
		g.statusMsg = i18n.T().CantBuildHere
		return
	}
	wall.Kind = building.Gate
	wall.ConstructionStage = building.ConstructionFoundation
	wall.ProgressTicks = 0
	wall.InputBuffer = nil
	wall.OutputBuffer = nil
	wall.GateOpen = false
	wall.GateAuto = true
	wall.GateAxis = axis
	wall.GateReplacesWall = true
	g.reserveConstructionMaterials(wall)
	g.invalidateConnectionCache()
	g.statusMsg = ""
	if tip, missing := advisor.ConstructionMaterialShortage(wall); missing {
		g.queueAdvisorTip(tip)
	}
}

func (g *Game) reserveConstructionMaterials(site *building.Building) {
	if site == nil || g.stock == nil {
		return
	}
	for _, kind := range building.ConstructionMaterialTypes() {
		need := site.ConstructionMaterialCost(kind)
		available := g.stock.Amount(kind)
		reserved := min(need, available)
		if reserved <= 0 || !g.stock.Remove(kind, reserved) {
			continue
		}
		site.AddConstructionMaterial(kind, reserved)
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

// requestSelectedRemoval opens the shared confirmation before the inspector
// changes any building or serf. The selection stays intact beneath the modal,
// so the prompt always describes the exact object the player clicked.
func (g *Game) requestSelectedRemoval() {
	if ui.CanRemoveSelection(g.selection) {
		g.dialog = ui.DialogConfirmRemoval
	}
}

// removeSelected performs a confirmed inspector removal. A serf finishes an
// already assigned delivery before leaving; buildings use the established
// removal flow with its warehouse and natural-resource safeguards.
func (g *Game) removeSelected() {
	switch g.selection.Kind {
	case ui.SelectionSerf:
		if g.selection.Serf != nil && g.logi.RequestDismissal(g.selection.Serf) {
			g.statusMsg = i18n.T().SerfDismissRequested
		}
	case ui.SelectionBuilding:
		g.deleteSelectedBuilding()
	}
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
		// A gate has replaced an existing wall in place. Cancelling before it
		// completes gives the delivered materials back and restores that wall;
		// deleting a completed gate remains an intentional opening.
		if b.Kind == building.Gate && b.GateReplacesWall {
			for _, material := range building.ConstructionMaterialTypes() {
				g.stock.Add(material, b.InputBuffer[material])
			}
			g.builders.CancelRouteTo(b)
			g.logi.CancelAllJobs(g.stock)
			b.Kind = building.StoneWall
			b.ConstructionStage = building.ConstructionNone
			b.ProgressTicks = 0
			b.InputBuffer = nil
			b.OutputBuffer = nil
			b.GateOpen = false
			b.GateAuto = false
			b.GateReplacesWall = false
			g.invalidateConnectionCache()
			g.selection.Clear()
			g.statusMsg = i18n.T().Deleted
			return
		}
		// A cancelled construction site was never registered as a
		// Warehouse or given a resident (see finishConstruction), so none
		// of the kind-specific protections below apply. Any materials
		// already delivered go back to the stockpile, exactly like a
		// cancelled haul returns carried cargo elsewhere.
		for _, material := range building.ConstructionMaterialTypes() {
			g.stock.Add(material, b.InputBuffer[material])
		}
		g.builders.CancelRouteTo(b)
		g.logi.CancelAllJobs(g.stock)
		for i, candidate := range g.buildings {
			if candidate != b {
				continue
			}
			g.buildings = append(g.buildings[:i], g.buildings[i+1:]...)
			g.invalidateConnectionCache()
			if g.pop != nil {
				g.pop.BuildingsRemoved++
			}
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
		g.invalidateConnectionCache()
		if g.pop != nil {
			g.pop.BuildingsRemoved++
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
// persistent death/removal history shown in the empty inspector panel.
func (g *Game) refreshPopulation() {
	g.pop.Count = len(g.logi.Serfs) + len(g.vills.Villagers) + len(g.jacks.Lumberjacks) + len(g.fishers.Fishermen) + len(g.quarry.Quarrymen) + len(g.builders.Builders) + len(g.miners.Miners)
}

// buildingSelectionAt resolves only a building, including a Road. It is used
// by continuous demolition so a passer-by never intercepts a road click.
func (g *Game) buildingSelectionAt(mx, my int) ui.Selection {
	tx, ty := g.camera.ScreenToTile(mx, my)
	for i := len(g.buildings) - 1; i >= 0; i-- {
		b := g.buildings[i]
		if b == nil {
			continue
		}
		footprint := building.Types[b.Kind].Footprint
		if tx >= b.X && tx < b.X+footprint && ty >= b.Y && ty < b.Y+footprint {
			return ui.Selection{Kind: ui.SelectionBuilding, Building: b}
		}
	}
	return ui.Selection{}
}

// selectionAt resolves map coordinates to a live game object. A building
// always wins a click on any tile of its footprint, even when a visible unit
// currently stands there. This keeps a warehouse, construction site, or
// workplace inspectable while workers pass through it; units remain selected
// normally on every unoccupied map tile.
func (g *Game) selectionAt(mx, my int) ui.Selection {
	tx, ty := g.camera.ScreenToTile(mx, my)
	// A building wins over a unit standing on it, so a click always
	// reaches the workplace being inspected (a farm, a construction
	// site, a warehouse) rather than a worker or serf merely passing
	// through -- see the TestSelectionAt_* tests below.
	//
	// Road is the one deliberate exception: it has nothing of its own
	// worth inspecting, and it's also where nearly every walking unit in
	// the game spends nearly all of its time -- letting it win here made
	// it all but impossible to ever click a unit at all, a real
	// regression the user reported directly ("не могу выбрать юнита
	// если он на дороге"). So a road tile is remembered but not
	// returned immediately: the unit loops below get first refusal, and
	// the road is only the fallback if no unit is actually standing
	// there.
	var roadHit *building.Building
	for i := len(g.buildings) - 1; i >= 0; i-- {
		b := g.buildings[i]
		footprint := building.Types[b.Kind].Footprint
		if tx >= b.X && tx < b.X+footprint && ty >= b.Y && ty < b.Y+footprint {
			if b.Kind == building.Road {
				roadHit = b
				continue
			}
			return ui.Selection{Kind: ui.SelectionBuilding, Building: b}
		}
	}
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
	if roadHit != nil {
		return ui.Selection{Kind: ui.SelectionBuilding, Building: roadHit}
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
	if !showsAccessMarker(b.Kind) {
		return false
	}
	if g.connectionCache == nil {
		g.connectionCache = make(map[*building.Building]bool)
	}
	if connected, known := g.connectionCache[b]; known {
		return connected
	}
	warehouse := findWarehouse(g.buildings)
	if warehouse == nil {
		return false
	}
	_, connected := pathfind.FindPath(g.buildings, warehouse, b)
	g.connectionCache[b] = connected
	return connected
}

// updateAutomaticGates changes only the visible/open state of auto gates.
// Pathfinding treats an automatic gate as routeable even while it is visually
// closed, so a unit can approach it and make this pass open it on the next
// simulation tick. Manual closed gates are excluded from routes.
func (g *Game) updateAutomaticGates() {
	for _, gate := range g.buildings {
		if gate == nil || gate.Kind != building.Gate || gate.ConstructionStage != building.ConstructionNone || !gate.GateAuto {
			continue
		}
		gate.GateOpen = g.alliedUnitNear(gate.X, gate.Y, 2)
	}
}

func (g *Game) alliedUnitNear(x, y, radius int) bool {
	near := func(unitX, unitY int) bool {
		if unitX < x-radius || unitX > x+radius || unitY < y-radius || unitY > y+radius {
			return false
		}
		return true
	}
	for _, v := range g.vills.Villagers {
		if v.VisibleOnMap() && near(v.X, v.Y) {
			return true
		}
	}
	for _, s := range g.logi.Serfs {
		if near(s.X, s.Y) {
			return true
		}
	}
	for _, j := range g.jacks.Lumberjacks {
		if near(j.X, j.Y) {
			return true
		}
	}
	for _, f := range g.fishers.Fishermen {
		if f.VisibleOnMap() && near(f.X, f.Y) {
			return true
		}
	}
	for _, q := range g.quarry.Quarrymen {
		if near(q.X, q.Y) {
			return true
		}
	}
	for _, b := range g.builders.Builders {
		if near(b.X, b.Y) {
			return true
		}
	}
	for _, m := range g.miners.Miners {
		if near(m.X, m.Y) {
			return true
		}
	}
	return false
}

func (g *Game) invalidateConnectionCache() {
	g.connectionCache = nil
	g.recommendedServeCountCache = nil
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
	g.invalidateConnectionCache()
	g.statusMsg = ""
}

// slotPath returns the file path for save-panel slot n (1-slotCount).
func slotPath(n int) string {
	return fmt.Sprintf(slotPathFormat, n)
}

// refreshSlotCache re-reads every save-panel slot's name and occupancy. It
// is not called every frame -- only on startup and right after a save-panel
// save -- so drawing the Esc pause menu never has to touch disk.
func (g *Game) refreshSlotCache() {
	infos := make([]ui.SaveSlotInfo, slotCount)
	for i := 0; i < slotCount; i++ {
		name, ok := save.PeekName(slotPath(i + 1))
		infos[i] = ui.SaveSlotInfo{Name: name, Occupied: ok, Autosave: i+1 == g.autosaveSlot}
	}
	g.slotCache = infos
}

// toggleAutosaveSlot designates slot as the periodic-autosave target, or
// turns autosave off if that slot is already the target -- a pause-menu
// preference like language/speed, so it's deliberately not reset by New
// Game or Load, and not written to any save file (a fresh launch always
// starts with autosave off, same as every other live-only preference).
// At most one slot can be the target at a time: picking a new one simply
// replaces the old choice, it does not need to be turned off first.
func (g *Game) toggleAutosaveSlot(slot int) {
	if slot < 1 || slot > slotCount {
		return
	}
	if g.autosaveSlot == slot {
		g.autosaveSlot = 0
	} else {
		g.autosaveSlot = slot
	}
	g.autosaveTicks = 0
	g.refreshSlotCache()
}

// advisorCheckIntervalTicks/advisorCooldownTicks pace the advisor (see
// internal/advisor): a full check every 300 ticks (2.5 minutes of played
// time at Normal speed) is cheap enough not to matter -- the underlying
// data is either already-cached (recommendedServeCount,
// disconnectedBuildings) or a single cheap pass over buildings/units
// (unstaffedWorkerBuildings, already paid every render frame for the
// map's red tint) -- and frequent enough that a genuine crisis (food
// running out) is caught with real warning, not discovered by hindsight.
// The cooldown (10 real minutes of played time) is long enough that an
// acknowledged tip doesn't immediately reappear the moment its condition
// is still barely true, short enough that a real, ongoing problem gets
// mentioned again rather than being silently forgotten forever.
const (
	advisorCheckIntervalTicks = 300
	advisorCooldownTicks      = 1500
)

// trackAdvisorIdleBuildings updates advisorIdleSince from the current
// unstaffedWorkerBuildings() every simulation tick (not just at the
// advisor's own check interval, so idle duration is measured
// continuously): rebuilt fresh each call rather than incrementally
// patched, so a building that got staffed, or was removed entirely,
// simply stops appearing -- no separate pruning pass needed.
func (g *Game) trackAdvisorIdleBuildings() {
	unstaffed := g.unstaffedWorkerBuildings()
	next := make(map[*building.Building]int, len(g.advisorIdleSince))
	for b, isUnstaffed := range unstaffed {
		if !isUnstaffed {
			continue
		}
		if since, ok := g.advisorIdleSince[b]; ok {
			next[b] = since
		} else {
			next[b] = g.worldTicks
		}
	}
	g.advisorIdleSince = next
}

// trackAdvisorGatherWorkers updates advisorGatherStuckSince from every
// lumberjack/quarryman/miner's own State() == StateIdle -- keyed by home
// hut, same "rebuilt fresh every tick" pattern as
// trackAdvisorIdleBuildings and for the same reason: a worker that starts
// moving again, or is removed, simply stops appearing, no separate
// pruning needed. Each profession's StateIdle is its own package-level
// type/constant (lumberjack.StateIdle, quarry.StateIdle, miner.StateIdle
// -- unrelated types that happen to share a name), so they're compared
// separately rather than through one shared interface.
func (g *Game) trackAdvisorGatherWorkers() {
	next := make(map[*building.Building]int, len(g.advisorGatherStuckSince))
	keep := func(home *building.Building, idle bool) {
		if !idle || home == nil {
			return
		}
		if since, ok := g.advisorGatherStuckSince[home]; ok {
			next[home] = since
		} else {
			next[home] = g.worldTicks
		}
	}
	for _, j := range g.jacks.Lumberjacks {
		keep(j.HomeBuilding(), j.State() == lumberjack.StateIdle)
	}
	for _, q := range g.quarry.Quarrymen {
		keep(q.HomeBuilding(), q.State() == quarry.StateIdle)
	}
	for _, m := range g.miners.Miners {
		keep(m.HomeBuilding(), m.State() == miner.StateIdle)
	}
	g.advisorGatherStuckSince = next
}

// tickAdvisor runs the heuristic advisor (see internal/advisor) once per
// simulation tick: idle-building and gather-worker tracking every tick
// (cheap, and duration needs continuous measurement), the full rule check
// only every advisorCheckIntervalTicks. New tips join the display queue
// unless their Kind is already queued/shown or still on cooldown from a
// previous acknowledgement.
func (g *Game) tickAdvisor() {
	g.trackAdvisorIdleBuildings()
	g.trackAdvisorGatherWorkers()

	g.advisorCheckTicks++
	if g.advisorCheckTicks < advisorCheckIntervalTicks {
		return
	}
	g.advisorCheckTicks = 0

	tips := advisor.Evaluate(g.buildings, g.stock, g.pop, g.disconnectedBuildings(), g.advisorIdleSince, g.advisorGatherStuckSince, g.worldTicks, g.recommendedServeCount(), len(g.logi.Serfs))
	for _, tip := range tips {
		g.queueAdvisorTip(tip)
	}
}

// queueAdvisorTip keeps one visible/queued tip of each kind and respects the
// acknowledgement cooldown. Placement calls it immediately for a newly
// unaffordable construction site; periodic evaluation uses the same path.
func (g *Game) queueAdvisorTip(tip advisor.Tip) {
	if g.advisorVisible != nil && g.advisorVisible.Kind == tip.Kind {
		return
	}
	for _, queued := range g.advisorQueue {
		if queued.Kind == tip.Kind {
			return
		}
	}
	if readyAt, onCooldown := g.advisorCooldowns[tip.Kind]; onCooldown && g.worldTicks < readyAt {
		return
	}
	g.advisorQueue = append(g.advisorQueue, tip)
	g.advisorPumpQueue()
}

// advisorPumpQueue promotes the next queued tip to advisorVisible once the
// slot is free. A no-op if something is already showing or the queue is
// empty.
func (g *Game) advisorPumpQueue() {
	if g.advisorVisible != nil || len(g.advisorQueue) == 0 {
		return
	}
	tip := g.advisorQueue[0]
	g.advisorQueue = g.advisorQueue[1:]
	g.advisorVisible = &tip
}

// acknowledgeAdvisorTip handles a click on the toast's "Ознакомлен"
// button: puts the just-shown tip's Kind on cooldown and shows the next
// queued one, if any.
// focusAdvisorBuilding selects the representative building from the active
// advisor tip and centers it in the map viewport. Ebiten does not move the OS
// pointer, so the visible selection marker becomes the in-world focus cursor.
func (g *Game) focusAdvisorBuilding() {
	if g.advisorVisible == nil || g.advisorVisible.Building == nil {
		return
	}
	b := g.advisorVisible.Building
	present := false
	for _, candidate := range g.buildings {
		if candidate == b {
			present = true
			break
		}
	}
	if !present {
		return
	}
	g.buildMode = false
	g.demolitionMode = false
	g.selection = ui.Selection{Kind: ui.SelectionBuilding, Building: b}
	footprint := building.Types[b.Kind].Footprint
	centerX := (float64(b.X) + float64(footprint)/2) * render.TileSize
	centerY := (float64(b.Y) + float64(footprint)/2) * render.TileSize
	g.camera.CenterOn(centerX, centerY, g.grid.Width, g.grid.Height)
}

func (g *Game) acknowledgeAdvisorTip() {
	if g.advisorVisible == nil {
		return
	}
	if g.advisorCooldowns == nil {
		g.advisorCooldowns = make(map[advisor.Kind]int)
	}
	g.advisorCooldowns[g.advisorVisible.Kind] = g.worldTicks + advisorCooldownTicks
	g.advisorVisible = nil
	g.advisorPumpQueue()
}

// advisorTipText turns a Tip into its already-localized, single-line
// display string. Kept in cmd/game (not internal/advisor or internal/ui)
// deliberately: advisor.Tip carries only raw numbers, so neither package
// needs to know about i18n or about each other -- this is the one place
// that bridges "what's true" (advisor) to "what it says" (ui).
func advisorTipText(tip advisor.Tip) string {
	t := i18n.T()
	exampleX, exampleY := 0, 0
	if tip.Building != nil {
		exampleX, exampleY = tip.Building.X, tip.Building.Y
	}
	switch tip.Kind {
	case advisor.KindFoodRunningOut:
		return fmt.Sprintf(t.AdvisorTipFoodRunningOut, tip.TicksLeft)
	case advisor.KindIdleBuilding:
		return fmt.Sprintf(t.AdvisorTipIdleBuilding, tip.Count, exampleX, exampleY)
	case advisor.KindDisconnectedBuilding:
		return fmt.Sprintf(t.AdvisorTipDisconnectedBuilding, tip.Count, exampleX, exampleY)
	case advisor.KindServeCountLow:
		return fmt.Sprintf(t.AdvisorTipServeCountLow, tip.Current, tip.Recommended)
	case advisor.KindServeCountHigh:
		return fmt.Sprintf(t.AdvisorTipServeCountHigh, tip.Current, tip.Recommended)
	case advisor.KindGatherWorkerStuck:
		return fmt.Sprintf(t.AdvisorTipGatherWorkerStuck, tip.Count, exampleX, exampleY)
	case advisor.KindConstructionMaterialsMissing:
		return fmt.Sprintf(t.AdvisorTipConstructionMaterialsMissing, t.ResourceName[tip.Resource], tip.Missing, exampleX, exampleY)
	default:
		return ""
	}
}

// audioChopPeriod/audioMinePeriod/audioHammerPeriod pace tickAudioCues.
// Deliberately not one sound per worker: internal/render/ambient.go
// already solves the same problem for wildlife ("Two candidates at a
// time are enough to make the land feel inhabited without turning
// wildlife into a repeated visual effect on every screen") -- with many
// simultaneous lumberjacks/quarrymen/miners/builders, playing one
// overlapping "thwack" per worker every tick would be a wall of noise,
// not ambiance. Instead: at most one instance of each sound, on a fixed
// tick period, whenever *any* worker of that kind is actively working.
const (
	audioChopPeriod   = lumberjack.ChopTicks // one hit per felling cycle
	audioMinePeriod   = quarry.MineTicks     // quarry and miner share this value
	audioHammerPeriod = 20                   // construction has no natural cycle length; picked by ear
)

// onAudioPeriod reports whether this tick is due for a periodic cue with
// the given period. g.worldTicks is pre-incremented at the top of the
// tick loop (see Update), so it's 1 on the very first simulated tick and
// never actually 0 -- checking "== 0" would make every cue's first
// occurrence wait a full period after the game starts (e.g. two real
// minutes for windAmbientPeriod=240 at 2 ticks/sec), which read as
// "sound is off" to the user until something else coincidentally
// happened to line up. Checking "== 1" instead keeps the exact same
// cadence but lines up the very first hit with the first simulated tick,
// so an unconditional cue like wind is heard within a fraction of a
// second of a fresh game starting -- immediate, audible proof that sound
// works, per the user's explicit request.
func (g *Game) onAudioPeriod(period int) bool {
	return g.worldTicks%period == 1
}

// tickAudioCues plays occasional flavor sound effects for whichever
// professions are actively working right now -- see the constants above
// for why this isn't one sound per worker. Called once per simulation
// tick (see the loop in Update), so pausing already silences it for free:
// no new calls happen while the world isn't ticking.
//
// Per the user's explicit request, every cue here only plays for a worker
// currently on screen -- buildingVisible checks the worker's own X,Y
// (where they're actively chopping/mining/building, not their home hut)
// against the camera's viewport, the same TileBounds.Intersects check
// render already uses to decide what's worth drawing.
func (g *Game) tickAudioCues() {
	if g.onAudioPeriod(audioChopPeriod) && anyMatch(g.jacks.Lumberjacks, func(j *lumberjack.Lumberjack) bool {
		return j.State() == lumberjack.StateChopping && g.buildingVisible(j.X, j.Y, 1)
	}) {
		audio.PlayChop()
	}
	if g.onAudioPeriod(audioMinePeriod) &&
		(anyMatch(g.quarry.Quarrymen, func(q *quarry.Quarryman) bool {
			return q.State() == quarry.StateMining && g.buildingVisible(q.X, q.Y, 1)
		}) ||
			anyMatch(g.miners.Miners, func(m *miner.Miner) bool {
				return m.State() == miner.StateMining && g.buildingVisible(m.X, m.Y, 1)
			})) {
		audio.PlayMining()
	}
	if g.onAudioPeriod(audioHammerPeriod) && anyMatch(g.builders.Builders, func(b *builder.Builder) bool {
		return (b.State() == builder.StateFoundation || b.State() == builder.StateFinishing) && g.buildingVisible(b.X, b.Y, 1)
	}) {
		audio.PlayHammer()
	}
}

// buildingVisible reports whether any tile of a size×size footprint at
// (x, y) is currently inside the camera's viewport. Shared by every new
// audio cue in this file -- see tickAudioCues/tickBuildingAmbientSounds --
// so sound only plays for what's actually on screen, per the user's
// explicit request. Reuses the same TileBounds.Intersects check render
// already relies on (see e.g. internal/render/lumberjacks.go), just from
// cmd/game instead of the render package.
func (g *Game) buildingVisible(x, y, size int) bool {
	return g.camera.VisibleTileBounds(1).Intersects(x, y, size)
}

// buildingAmbientPeriod paces each building kind's ambient work cue --
// same "not too frequent" reasoning as audioChopPeriod/audioMinePeriod/
// audioHammerPeriod above (see their doc comment), picked by ear per
// building rather than tied to its real production cycle length (which,
// per internal/economy/simulator.go, keeps "spinning" even when a
// building is starved of inputs -- not a reliable "is it actually
// working" signal, so this package doesn't try to check that; a
// periodic cue while the building is finished and visible is the same
// "feels alive" approximation internal/render/ambient.go already uses
// for wildlife). PigFarm's period is deliberately long: the user asked
// for its oink specifically not to be too frequent.
var buildingAmbientPeriod = map[building.Kind]int{
	building.Farm:              150,
	building.Mill:              100,
	building.Bakery:            150,
	building.Winery:            150,
	building.PigFarm:           400,
	building.MeatWorkshop:      150,
	building.CarpentryWorkshop: 120,
	building.Smeltery:          120,
	building.FisherHut:         200,
	building.Warehouse:         250,
	building.Tavern:            250,
}

// buildingAmbientSound is buildingAmbientPeriod's matching Play function
// per Kind.
var buildingAmbientSound = map[building.Kind]func(){
	building.Farm:              audio.PlayScythe,
	building.Mill:              audio.PlayMillWork,
	building.Bakery:            audio.PlayBakery,
	building.Winery:            audio.PlaySquish,
	building.PigFarm:           audio.PlayPigOink,
	building.MeatWorkshop:      audio.PlayMeatChop,
	building.CarpentryWorkshop: audio.PlaySaw,
	building.Smeltery:          audio.PlayForge,
	building.FisherHut:         audio.PlayOarSplash,
	building.Warehouse:         audio.PlayCartCreak,
	building.Tavern:            audio.PlayTavernChatter,
}

// tickBuildingAmbientSounds plays each finished, on-screen building's
// flavor cue on its own period (see buildingAmbientPeriod). Buildings
// without an entry (LumberjackHut/QuarryHut/MinerHut already play through
// tickAudioCues above, tied to the worker's actual state; Road/Tree/Fish/
// deposits aren't player structures at all) are silently skipped.
func (g *Game) tickBuildingAmbientSounds() {
	for _, b := range g.buildings {
		if b.ConstructionStage != building.ConstructionNone {
			continue
		}
		period, ok := buildingAmbientPeriod[b.Kind]
		if !ok || !g.onAudioPeriod(period) {
			continue
		}
		footprint := building.Types[b.Kind].Footprint
		if !g.buildingVisible(b.X, b.Y, footprint) {
			continue
		}
		buildingAmbientSound[b.Kind]()
	}
}

// weatherAmbientPeriod/dayNightAmbientPeriod pace the two ambient layers
// below -- not per-building, so buildingVisible doesn't apply to them
// directly (tickWeatherAmbientSounds scans the camera's own viewport
// instead; tickDayNightAmbientSounds is global, like rain/fireflies
// already are).
const (
	weatherAmbientPeriod  = 90
	dayNightAmbientPeriod = 200
	windAmbientPeriod     = 240 // its own period so it doesn't always land on the same tick as day/night
)

// tickWeatherAmbientSounds plays a seagull cue plus a gentle water/wave
// cue whenever the camera's current viewport contains at least one water
// tile -- the user's "если камера около воды" / "звуки мира: шум воды"
// requests. Scanning is limited to the on-screen tiles (not the whole
// map) and only runs once every weatherAmbientPeriod ticks, so this stays
// cheap even on a large map.
func (g *Game) tickWeatherAmbientSounds() {
	if !g.onAudioPeriod(weatherAmbientPeriod) {
		return
	}
	bounds := g.camera.VisibleTileBounds(0)
	minX, minY := max(bounds.MinX, 0), max(bounds.MinY, 0)
	maxX, maxY := min(bounds.MaxX, g.grid.Width-1), min(bounds.MaxY, g.grid.Height-1)
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if g.grid.At(x, y).Terrain == world.Water {
				audio.PlaySeagull()
				audio.PlayWaterWaves()
				return
			}
		}
	}
}

// tickDayNightAmbientSounds plays crickets at night and a lighter day
// ambience during the day -- a global atmospheric layer independent of
// camera position, the same way rain/fireflies already are (see
// internal/render/atmosphere.go, internal/render/ambient.go). Sunrise and
// Sunset stay silent here: they're short transitional phases and nothing
// was specifically requested for them.
func (g *Game) tickDayNightAmbientSounds() {
	if !g.onAudioPeriod(dayNightAmbientPeriod) {
		return
	}
	switch render.CurrentDayPhase() {
	case worldclock.Night:
		audio.PlayCricket()
	case worldclock.Day:
		audio.PlayDayAmbience()
	}
}

// tickWindAmbientSounds plays a general ambient wind cue -- the user's
// "звуки мира: ветер" request. Unlike every other ambient cue in this
// file it isn't gated on camera position or time of day: wind is a
// constant, map-wide presence.
func (g *Game) tickWindAmbientSounds() {
	if g.onAudioPeriod(windAmbientPeriod) {
		audio.PlayWind()
	}
}

// anyMatch reports whether any item in items satisfies pred. The four
// worker professions each have their own unrelated State type (not a
// shared interface), so this stays a small generic helper rather than
// something more specific to one of them.
func anyMatch[T any](items []T, pred func(T) bool) bool {
	for _, item := range items {
		if pred(item) {
			return true
		}
	}
	return false
}

// tickAutosave advances the autosave countdown by one simulation tick
// (see autosaveIntervalTicks) and triggers a silent save once it fires.
// Called once per simulation tick, not once per render frame, so autosave
// cadence is paced by played time, not wall-clock time -- consistent with
// every other tick-scale constant in the simulation (hunger, growth, ...).
func (g *Game) tickAutosave() {
	if g.autosaveSlot == 0 {
		return
	}
	g.autosaveTicks++
	if g.autosaveTicks < autosaveIntervalTicks {
		return
	}
	g.autosaveTicks = 0
	g.autosave()
}

// autosave silently re-saves into g.autosaveSlot, bypassing the naming/
// overwrite-confirmation dialogs a manual Save-button click goes through
// (see commitDialogSave) -- the whole point of autosave is that it never
// interrupts play. It keeps the slot's existing name if it already has
// one, or falls back to the same default a manual save would use if this
// is that slot's very first save.
func (g *Game) autosave() {
	if g.autosaveSlot < 1 || g.autosaveSlot > slotCount {
		return
	}
	name := g.slotCache[g.autosaveSlot-1].Name
	if name == "" {
		name = fmt.Sprintf("%s %d", i18n.T().SlotDefaultName, g.autosaveSlot)
	}
	if err := g.saveGame(slotPath(g.autosaveSlot), name); err != nil {
		g.statusMsg = i18n.T().SaveFailedPrefix + err.Error()
		return
	}
	g.statusMsg = i18n.T().Autosaved
	g.refreshSlotCache()
}

// handleSaveSlotAction executes a click on a save-panel slot's Save or
// Load button. Saving into an occupied slot opens a confirmation dialog
// instead of overwriting immediately; saving into an empty slot goes
// straight to naming. Loading an empty slot does nothing -- its Load button
// is drawn muted for the same reason.
func (g *Game) handleSaveSlotAction(slot int, action ui.SaveSlotAction) {
	if slot < 1 || slot > slotCount {
		return
	}
	info := g.slotCache[slot-1]
	switch action {
	case ui.SaveSlotSave:
		g.dialogSlot = slot
		if info.Occupied {
			g.dialog = ui.DialogConfirmOverwrite
			g.dialogText = info.Name
		} else {
			g.dialog = ui.DialogNaming
			g.dialogText = ""
		}
	case ui.SaveSlotLoad:
		if !info.Occupied {
			return
		}
		g.loadAndReport(slotPath(slot))
	}
}

// handleDialogInput runs instead of the normal input handlers while a
// pause-menu save/load modal is open (see Update).
func (g *Game) handleDialogInput() {
	switch g.dialog {
	case ui.DialogConfirmRemoval:
		g.handleConfirmRemovalInput()
	case ui.DialogConfirmDemolitionMode:
		g.handleConfirmDemolitionModeInput()
	case ui.DialogConfirmTrimServes:
		g.handleConfirmTrimServesInput()
	}
}

// handleConfirmRemovalInput confirms or cancels the inspector's generic
// destructive action. Enter confirms and Escape cancels, matching the other
// modal dialogs without reintroducing a keyboard shortcut for deletion.
func (g *Game) handleConfirmRemovalInput() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.dialog = ui.DialogNone
		return
	}
	confirm := inpututil.IsKeyJustPressed(ebiten.KeyEnter)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		if selected, ok := g.layout.InspectorConfirmRemoveAt(mx, my); ok {
			if !selected {
				g.dialog = ui.DialogNone
				return
			}
			confirm = true
		}
	}
	if confirm {
		g.dialog = ui.DialogNone
		g.removeSelected()
	}
}

// handleConfirmDemolitionModeInput is the one confirmation before a sequence
// of destructive map clicks. Escape/cancel leaves the map unchanged; once
// enabled, Escape or the same left-panel button exits the mode.
func (g *Game) handleConfirmDemolitionModeInput() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.dialog = ui.DialogNone
		return
	}
	confirm := inpututil.IsKeyJustPressed(ebiten.KeyEnter)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		if selected, ok := g.layout.InspectorConfirmRemoveAt(mx, my); ok {
			if !selected {
				g.dialog = ui.DialogNone
				return
			}
			confirm = true
		}
	}
	if confirm {
		g.dialog = ui.DialogNone
		g.demolitionMode = true
		g.buildMode = false
		g.selection.Clear()
		g.statusMsg = ""
	}
}

// handleConfirmTrimServesInput answers the "trim all the way to
// recommended, or just one?" question a right-click on the Serf card asks
// when there's an excess (see trimSerfsToRecommended's doc comment). Both
// answers are real actions, not a cancel: Enter or clicking "Да" trims the
// whole excess in one go, clicking "Нет" dismisses just one (the original
// single-serf behavior). Escape alone leaves both untouched, matching
// every other dialog's cancel behavior.
func (g *Game) handleConfirmTrimServesInput() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.dialog = ui.DialogNone
		return
	}
	yes := inpututil.IsKeyJustPressed(ebiten.KeyEnter)
	answered := yes
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		if selected, ok := g.layout.InspectorConfirmRemoveAt(mx, my); ok {
			yes = selected
			answered = true
		}
	}
	if !answered {
		return
	}
	g.dialog = ui.DialogNone
	if yes {
		g.trimSerfsToRecommended(g.recommendedServeCount())
	} else {
		g.dismissOneSerf()
	}
}

// resetToNewGame discards the running town and replaces it with a fresh one
// -- the Esc pause menu's "New Game" button. The active language is a
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
		PlayedFrames:       g.playedFrames,
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
// S/L quicksave keys and the Esc pause menu's per-slot Load button.
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
	g.invalidateConnectionCache()
	stock := state.Stockpile
	// Older saves carried the temporary 200-unit limit. The town rule is
	// now explicit: every warehouse shares an unlimited stockpile.
	stock.Capacity = 0
	g.stock = &stock
	pop := state.Population
	g.pop = &pop
	g.playedFrames = state.PlayedFrames
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
	// Preserve the camera scale from a save. Zoom is a player-view preference,
	// not simulation state, but restoring it keeps the saved camera position
	// meaningful and makes wheel zoom feel consistent after loading.
	scale := state.CameraZoom
	if scale <= 0 {
		scale = 1
	}
	if scale < 0.60 {
		scale = 0.60
	} else if scale > 2.50 {
		scale = 2.50
	}
	g.camera.Scale = scale
	mapRect := g.layout.MapRect()
	g.camera.SetViewport(mapRect.Min.X, mapRect.Min.Y, mapRect.Dx(), mapRect.Dy())
	g.camera.Pan(0, 0, g.grid.Width, g.grid.Height, mapRect.Dx(), mapRect.Dy())
	g.selection.Clear()

	// Jobs are rebuilt from the saved positions. The roster itself is
	// restored, so hiring extra serfs or saving a worker halfway to the
	// Tavern no longer silently resets the town.
	g.logi = logistics.NewController(warehouse, 0)
	for _, b := range buildings {
		if b.IsOperationalWarehouse() && b != warehouse {
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
			if b.IsOperationalWarehouse() && b != warehouse {
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
// exactly like the S/L quicksave keys and the Esc pause menu's per-slot Load
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
		if b.IsOperationalWarehouse() {
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

// generateGrid builds one fresh procedural map: one sea forming a
// continuous coastal strip along a random map edge (see seaMinPercent),
// and otherwise plain grass for seedTrees/seedThickets/seedStoneDeposits/
// seedOreDeposits to scatter their objects across afterward. There is no
// separate fertile terrain painted -- the user asked explicitly not to
// generate the cosmetic tilled-soil patches at all ("пахотные поля не
// генерируем вообще"); a Farm builds on any dry tile regardless of terrain
// (building.Types[Farm].AllowedTerrain is empty) so this loses no gameplay.
func generateGrid(width, height int, seed uint32) *world.Grid {
	g := world.NewGrid(width, height)
	growSeaRegion(g, seed^0x9e3779b9)
	return g
}

// growSeaRegion paints one continuous sea along the full length of a
// randomly chosen map edge. Per the user's explicit request ("вода должна
// быть скраю карты единая на границе с краем карты") the water must be one
// unified mass touching the *entire* edge, not a blob that merely happens
// to touch it at one point -- so unlike every other region-growth function
// in this file (stone/ore/fertile), this doesn't BFS-expand from a single
// seed cell. Instead, every position along the edge gets its own inland
// depth, and the depths form a smoothed, mean-reverting random walk (a
// small xorshift32-driven step per position, pulled back toward the
// average depth implied by seaMinPercent/seaMaxPercent) so the coastline
// stays close to the target area while still having a natural, uneven
// inner edge rather than a straight ruler-line.
func growSeaRegion(g *world.Grid, seed uint32) {
	area := g.Width * g.Height
	span := seaMaxPercent - seaMinPercent + 1
	percent := seaMinPercent + int(seed%uint32(span))
	target := area * percent / 100
	if target <= 0 {
		return
	}

	horizontal := (seed>>16)%2 == 0 // true: top/bottom edge, coastline runs along X
	far := (seed>>17)%2 == 0        // which of the two edges on that axis

	edgeLength, perpendicular := g.Height, g.Width
	if horizontal {
		edgeLength, perpendicular = g.Width, g.Height
	}

	avgDepth := target / edgeLength
	if avgDepth < 2 {
		avgDepth = 2
	}
	minDepth := avgDepth / 2
	if minDepth < 1 {
		minDepth = 1
	}
	maxDepth := avgDepth * 3 / 2
	if maxDepth > perpendicular-2 {
		maxDepth = perpendicular - 2
	}
	if maxDepth < minDepth {
		maxDepth = minDepth
	}

	rng := seed ^ 0x2545f491
	depth := avgDepth
	for i := 0; i < edgeLength; i++ {
		rng ^= rng << 13
		rng ^= rng >> 17
		rng ^= rng << 5
		step := int(rng%3) - 1 // -1, 0, or +1
		if depth > avgDepth {
			step--
		} else if depth < avgDepth {
			step++
		}
		depth += step
		if depth < minDepth {
			depth = minDepth
		}
		if depth > maxDepth {
			depth = maxDepth
		}

		for d := 0; d < depth; d++ {
			x, y := i, d
			if horizontal {
				if far {
					y = g.Height - 1 - d
				}
			} else {
				x, y = d, i
				if far {
					x = g.Width - 1 - d
				}
			}
			g.Set(x, y, world.Tile{Terrain: world.Water})
		}
	}
}

// warehouseEdgeMargin keeps the starting Warehouse away from the map's
// outer boundary, so the player always has physical room to build around
// their starting point regardless of where the random pick (see
// findWarehouseSpot) lands. Halved along with mapWidth/mapHeight, see that
// constant's doc comment.
const warehouseEdgeMargin = 3

// maxWarehouseDistanceFromWater keeps the starting Warehouse close enough
// to the sea that a Fisher Hut is a realistic early build, per the user's
// explicit request ("склад спавнился недалеко от воды, максимум 20
// клеток" -- halved along with mapWidth/mapHeight, see that constant's doc
// comment). Straight-line distance, the same convention
// minDepositDistanceFromWarehouse/tooCloseToPoint already use elsewhere in
// this file, not a walked path -- the Warehouse doesn't need a road to the
// coast, just to not have generated impractically far from it.
const maxWarehouseDistanceFromWater = 10

// findWarehouseSpot picks a plain-grass tile with a buildable tile
// directly south for the starting Road, at least warehouseEdgeMargin from
// every map edge and within maxWarehouseDistanceFromWater of the sea,
// chosen at random among every valid candidate -- per the user's explicit
// request, replaying "New Game" moves the town around the map, not just
// the terrain around a fixed spot. Falls back to relaxing the edge margin,
// then the water distance, if nothing qualifies (should be unreachable
// given the sea always spans a whole map edge -- see growSeaRegion -- but
// the game must still start rather than fail on an extreme map).
func findWarehouseSpot(g *world.Grid, seed uint32) (gridPoint, bool) {
	near := nearWaterCells(g)
	if p, ok := pickWarehouseCandidate(g, near, seed, warehouseEdgeMargin); ok {
		return p, ok
	}
	if p, ok := pickWarehouseCandidate(g, near, seed, 0); ok {
		return p, ok
	}
	return pickWarehouseCandidate(g, nil, seed, 0)
}

// nearWaterCells returns the set of land tiles within
// maxWarehouseDistanceFromWater of at least one Water tile. Built once by
// scanning a bounded box around every Water tile, rather than a
// nearest-water search per candidate: unlike the sparse deposit-avoidance
// check (tooCloseToPoint tests one candidate against one fixed point --
// the Warehouse), a coastline can be hundreds of tiles long, so inverting
// the loop keeps this to O(water tiles * box area) instead of O(candidates
// * water tiles).
func nearWaterCells(g *world.Grid) map[gridPoint]bool {
	near := make(map[gridPoint]bool)
	limitSq := float64(maxWarehouseDistanceFromWater * maxWarehouseDistanceFromWater)
	for wy := 0; wy < g.Height; wy++ {
		for wx := 0; wx < g.Width; wx++ {
			if g.At(wx, wy).Terrain != world.Water {
				continue
			}
			for y := wy - maxWarehouseDistanceFromWater; y <= wy+maxWarehouseDistanceFromWater; y++ {
				if y < 0 || y >= g.Height {
					continue
				}
				for x := wx - maxWarehouseDistanceFromWater; x <= wx+maxWarehouseDistanceFromWater; x++ {
					if x < 0 || x >= g.Width {
						continue
					}
					dx, dy := float64(x-wx), float64(y-wy)
					if dx*dx+dy*dy <= limitSq {
						near[gridPoint{x, y}] = true
					}
				}
			}
		}
	}
	return near
}

// pickWarehouseCandidate is findWarehouseSpot's inner search over one
// (margin, water-constraint) combination. near is the set built by
// nearWaterCells; pass nil to skip the water-distance check entirely (the
// last-resort fallback).
func pickWarehouseCandidate(g *world.Grid, near map[gridPoint]bool, seed uint32, margin int) (gridPoint, bool) {
	bestScore := ^uint32(0)
	var best gridPoint
	found := false
	for y := margin; y < g.Height-margin; y++ {
		for x := margin; x < g.Width-margin; x++ {
			if g.At(x, y).Terrain != world.Grass || !g.InBounds(x, y+1) || !g.At(x, y+1).Buildable() {
				continue
			}
			if near != nil && !near[gridPoint{x, y}] {
				continue
			}
			score := warehouseScatterScore(x, y) ^ seed
			if !found || score < bestScore {
				bestScore, best, found = score, gridPoint{x, y}, true
			}
		}
	}
	return best, found
}

func warehouseScatterScore(x, y int) uint32 {
	return uint32(x)*2654435761 ^ uint32(y)*40503 ^ 0x9e3779b9
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

		if (thicketScatterScore(p.x, p.y)^seed)%100 < thicketDensityPercent &&
			building.CanPlace(grid, buildings, building.Tree, p.x, p.y) &&
			treePlacementKeepsEveryoneReachable(grid, buildings, p.x, p.y) {
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

// treeCellHasOpenNeighbor reports whether at least one of the 8 tiles
// around (x, y) is land a worker could actually stand on -- matching
// pathfind.FindLandPath's occupancy rules exactly: a Road tile is walkable
// (not an obstacle), every other building (including another Tree) blocks.
// ignoreX/ignoreY lets a caller ask "would this still hold if that other
// cell were also occupied" -- pass an out-of-bounds point (e.g. -1, -1) to
// skip the exclusion.
func treeCellHasOpenNeighbor(g *world.Grid, existing []*building.Building, x, y, ignoreX, ignoreY int) bool {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx, ny := x+dx, y+dy
			if nx == ignoreX && ny == ignoreY {
				continue
			}
			if !g.InBounds(nx, ny) || !g.At(nx, ny).Buildable() {
				continue
			}
			if !nonRoadOccupied(existing, nx, ny) {
				return true
			}
		}
	}
	return false
}

func nonRoadOccupied(existing []*building.Building, x, y int) bool {
	for _, b := range existing {
		if b == nil || b.Kind == building.Road {
			continue
		}
		size := building.Types[b.Kind].Footprint
		if x >= b.X && x < b.X+size && y >= b.Y && y < b.Y+size {
			return true
		}
	}
	return false
}

func treeAt(existing []*building.Building, x, y int) bool {
	for _, b := range existing {
		if b != nil && b.Kind == building.Tree && b.X == x && b.Y == y {
			return true
		}
	}
	return false
}

// treePlacementKeepsEveryoneReachable is the full check used before
// planting a new tree at (x, y). Real bug the user reported ("дерево
// вырастает внутри и к нему невозможно подойти"): thicket generation and
// regrowth previously only checked whether the tree's own cell was free
// (via building.CanPlace), never whether anything could actually reach it.
// pathfind.FindLandPath does allow walking onto the goal tile itself even
// if it's normally an obstacle -- that's how a lumberjack reaches a tree at
// all -- but every tile leading up to it still has to be ordinary walkable
// land, and a tree fully boxed in by its neighbours (routine inside a dense
// thicket) has none. This checks both that the new tree itself has an open
// neighbor, AND that planting it doesn't seal off an existing tree already
// standing next to it -- occupying a neighbour's last open side would just
// move this exact bug onto that other tree instead of preventing it.
func treePlacementKeepsEveryoneReachable(g *world.Grid, existing []*building.Building, x, y int) bool {
	if !treeCellHasOpenNeighbor(g, existing, x, y, -1, -1) {
		return false
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx, ny := x+dx, y+dy
			if treeAt(existing, nx, ny) && !treeCellHasOpenNeighbor(g, existing, nx, ny, x, y) {
				return false
			}
		}
	}
	return true
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
		if !treePlacementKeepsEveryoneReachable(grid, buildings, c.x, c.y) {
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

// scaledDepositCells applies an ore percentage directly to the map area --
// see coalMinPercent's doc comment for why there's no separate scaling
// layer on top any more.
func scaledDepositCells(area, percent int) int {
	return area * percent / 100
}

// scaledStoneDepositCells applies a stone percentage directly to the map
// area -- see stoneMinPercent's doc comment for why there's no separate
// scaling layer on top any more. The value only participates in fresh
// generation (and old-save migration that had no seeded stone yet); it
// never removes deposits from an existing map.
func scaledStoneDepositCells(area, percent int) int {
	return area * percent / 100
}

// seedStoneDeposits splits stoneMinPercent-stoneMaxPercent of the map's
// area across 2-5 separate regions, rather than one single patch, so the
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
	span := stoneMaxPercent - stoneMinPercent + 1
	percent := stoneMinPercent
	if span > 0 {
		percent += int(seed % uint32(span))
	}
	total := scaledStoneDepositCells(area, percent)
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
	total := scaledDepositCells(area, percent)
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

// fishRegrowthRadius bounds how far (BFS hops over water, not
// straight-line) a fry can respawn from where the fish that made room for
// it was actually caught. Per the user's explicit request ("рыба...
// должна спавнится в тех областях где ее собрали, а то сейчас по краям
// все" -- and its follow-up clarifying the real mechanism: a fisherman
// only ever fishes near their hut, so with regrowth spread over the whole
// connected sea, water near the hut slowly empties out while fish pile up
// far away that nobody ever reaches), tickFishRegrowth uses
// waterSectionCells instead of the whole-body waterBodyCells: both the
// population cap (fishBodyLimit) and the candidate search are now scoped
// to this local section, so a heavily-fished area refills locally instead
// of competing with the entire sea's population for room. seedFish (the
// one-time initial population at world generation) is deliberately left
// alone -- populating the whole sea once at the start is correct, this is
// only about where a fish respawns after being caught. Halved along with
// mapWidth/mapHeight, see that constant's doc comment -- otherwise a
// "local" 20-tile radius would swallow most of the smaller sea and stop
// being local at all.
const fishRegrowthRadius = 10

// waterSectionCells is waterBodyCells bounded to a local section: the same
// flood-fill, but limited to cells within radius BFS-hops of start, not
// the whole connected water body. See fishRegrowthRadius's doc comment for
// why this exists as a separate function from waterBodyCells.
func waterSectionCells(grid *world.Grid, start gridPoint, radius int) []gridPoint {
	if grid == nil || !grid.InBounds(start.x, start.y) || grid.At(start.x, start.y).Terrain != world.Water {
		return nil
	}
	dist := map[gridPoint]int{start: 0}
	queue := []gridPoint{start}
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		if dist[p] >= radius {
			continue
		}
		for _, d := range [...]gridPoint{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := gridPoint{p.x + d.x, p.y + d.y}
			if _, seen := dist[n]; seen || !grid.InBounds(n.x, n.y) || grid.At(n.x, n.y).Terrain != world.Water {
				continue
			}
			dist[n] = dist[p] + 1
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
		cells := waterSectionCells(g.grid, gridPoint{regrowth.waterX, regrowth.waterY}, fishRegrowthRadius)
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
		g.scheduleTreeRegrowth(tree.X, tree.Y)
		return
	}
}

func (g *Game) scheduleTreeRegrowth(originX, originY int) {
	seed := g.nextTreeSeed()
	target := treeRegrowthMinTicks + int(seed%uint32(treeRegrowthVariationTicks))
	g.treeRegrowth = append(g.treeRegrowth, treeRegrowth{originX: originX, originY: originY, target: target, seed: seed})
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

// treeRegrowthRadius/treeRegrowthLocalCap implement per-section local
// regrowth, per the user's explicit request ("нужно карту разделить на
// секции... вырубаешь в одной части карты и не трогаешь в другой - рыба
// и дерево появляется там где их собирают"): a cut tree respawns near
// where it was cut, and how many trees can occupy that local area is
// capped independently of the rest of the map. This *replaces* the old
// map-wide 1% ceiling entirely (not layered on top of it): thickets
// (seedThickets) alone can already push the global tree count above that
// ceiling, which would otherwise permanently block ALL regrowth
// everywhere, no matter how empty a heavily-logged area near a hut had
// become -- exactly the "far trees pile up untouched, the hut's own
// backyard stays bare forever" symptom the user reported.
// Both halved along with mapWidth/mapHeight (see that constant's doc
// comment): halving just the radius and leaving the cap at 12 would pack
// the same tree count into a quarter of the area (radius scales the
// linear map dimension, but the local cap is a density over that radius's
// *area*), so the cap is quartered too -- 12 * (10/20)^2 = 3 -- to keep
// the same roughly-1% local density the original pairing was tuned for.
const (
	treeRegrowthRadius   = 10
	treeRegrowthLocalCap = 3
)

func (g *Game) tickTreeRegrowth() {
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
		origin := gridPoint{regrowth.originX, regrowth.originY}
		if countTreesNear(g.buildings, origin, treeRegrowthRadius) >= treeRegrowthLocalCap {
			// This local section is already at its own cap -- retry later
			// rather than discarding the delayed respawn or (the old bug)
			// spawning it somewhere else on the map entirely.
			regrowth.ticks = regrowth.target - treeRegrowthRetryTicks
			if regrowth.ticks < 0 {
				regrowth.ticks = 0
			}
			i++
			continue
		}

		x, y, ok := g.findTreeSpawnCell(regrowth.seed, origin, treeRegrowthRadius)
		if !ok {
			// No room within this local section right now. Retry later
			// without discarding the delayed respawn or widening the
			// search to some unrelated part of the map.
			regrowth.ticks = regrowth.target - treeRegrowthRetryTicks
			if regrowth.ticks < 0 {
				regrowth.ticks = 0
			}
			i++
			continue
		}

		g.buildings = append(g.buildings, building.NewTree(x, y))
		g.treeRegrowth = append(g.treeRegrowth[:i], g.treeRegrowth[i+1:]...)
	}
}

// countTreesNear counts live Tree buildings within radius (straight-line)
// of origin.
func countTreesNear(buildings []*building.Building, origin gridPoint, radius int) int {
	radiusSq := float64(radius * radius)
	count := 0
	for _, b := range buildings {
		if b == nil || b.Kind != building.Tree {
			continue
		}
		dx, dy := float64(b.X-origin.x), float64(b.Y-origin.y)
		if dx*dx+dy*dy <= radiusSq {
			count++
		}
	}
	return count
}

func (g *Game) findTreeSpawnCell(seed uint32, origin gridPoint, radius int) (int, int, bool) {
	bestScore := ^uint32(0)
	bestX, bestY := 0, 0
	found := false
	minX, maxX := origin.x-radius, origin.x+radius
	minY, maxY := origin.y-radius, origin.y+radius
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX >= g.grid.Width {
		maxX = g.grid.Width - 1
	}
	if maxY >= g.grid.Height {
		maxY = g.grid.Height - 1
	}
	radiusSq := float64(radius * radius)
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			dx, dy := float64(x-origin.x), float64(y-origin.y)
			if dx*dx+dy*dy > radiusSq {
				continue
			}
			if !g.grid.At(x, y).Buildable() || !building.CanPlace(g.grid, g.buildings, building.Tree, x, y) {
				continue
			}
			if !treePlacementKeepsEveryoneReachable(g.grid, g.buildings, x, y) {
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

// showsAccessMarker reports whether a building kind gets the per-frame road-
// connection dot in Draw (see ui.DrawAccessMarker). Road/Tree/Fish/Warehouse
// and every finite deposit kind are never connected to the road network, so
// drawing the marker for them would be meaningless -- and, more importantly,
// each call also runs a full pathfind BFS (g.buildingConnected), so skipping
// deposits here is a real performance fix, not just a visual one: when ore/
// coal shipped, only StoneDeposit was added to this exclusion, not the three
// new ore kinds. On the original small map that was just a little wasteful;
// on the bigger procedural map (hundreds of individual deposit Buildings,
// each its own footprint-1 object) it meant hundreds of full BFS calls every
// single frame -- 60 times a second, not once per simulation tick -- which
// is what actually caused the "high CPU load, game hangs" symptom.
func showsAccessMarker(kind building.Kind) bool {
	switch kind {
	case building.Road, building.StoneWall, building.Gate, building.Tree, building.Fish, building.Warehouse,
		building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
		return false
	default:
		return true
	}
}

func (g *Game) serializeTreeRegrowth() []save.TreeRegrowthState {
	regrowth := make([]save.TreeRegrowthState, 0, len(g.treeRegrowth))
	for _, r := range g.treeRegrowth {
		regrowth = append(regrowth, save.TreeRegrowthState{OriginX: r.originX, OriginY: r.originY, Ticks: r.ticks, TargetTicks: r.target, Seed: r.seed})
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
		// OriginX/OriginY default to 0,0 in a save from before this field
		// existed -- tickTreeRegrowth just treats that as any other origin
		// point (retries harmlessly via the usual mechanism if nothing
		// qualifies nearby), so no separate migration path is needed.
		regrowth = append(regrowth, treeRegrowth{originX: state.OriginX, originY: state.OriginY, ticks: ticks, target: target, seed: state.Seed})
	}
	return regrowth
}

func (g *Game) Draw(screen *ebiten.Image) {
	if g.screen != screenPlay {
		g.drawFrontScreen(screen)
		return
	}
	// Same fix as drawFrontScreen's own screen.Bounds() read (see its
	// comment): in fullscreen, Update's ebiten.WindowSize() call can be
	// stale, and Layout's outsideWidth/outsideHeight isn't guaranteed to
	// have caught up either, but screen.Bounds() here is always the
	// actual draw-buffer size Draw is about to paint into -- this is what
	// the user hit directly ("карта смещена в верхний левый угол, справа
	// и внизу - чернота"): the front screen already self-corrected this
	// way every frame, gameplay never did, so a stale small viewport
	// from before switching to fullscreen (or before the real size was
	// first reported) could persist indefinitely once screenPlay started.
	if bounds := screen.Bounds(); bounds.Dx() > 0 && bounds.Dy() > 0 {
		g.resizeLayout(bounds.Dx(), bounds.Dy())
	}
	if !g.paused {
		render.Tick()
	}
	render.SetWorldTicks(g.worldTicks)
	render.DrawGrid(screen, g.grid, g.camera)
	// visibleConnectivity feeds both the building tint below and the
	// access-point dot further down -- each buildingConnected call is a
	// full pathfind BFS, so computing it once per visible building here
	// and reusing the result is what keeps this at the same one-BFS-per-
	// visible-building cost the access marker alone used to pay, instead
	// of quietly doubling it (see showsAccessMarker's doc comment for the
	// exact "hundreds of full BFS calls every frame" bug this pattern
	// exists to avoid repeating).
	visible := g.camera.VisibleTileBounds(2)
	visibleConnectivity := make(map[*building.Building]bool, len(g.buildings))
	disconnected := make(map[*building.Building]bool, len(g.buildings))
	for _, b := range g.buildings {
		if !showsAccessMarker(b.Kind) || !visible.Intersects(b.X, b.Y, building.Types[b.Kind].Footprint) {
			continue
		}
		connected := g.buildingConnected(b)
		visibleConnectivity[b] = connected
		if building.Types[b.Kind].Recipe.TicksToProduce > 0 && !connected {
			disconnected[b] = true
		}
	}
	render.DrawBuildings(screen, g.grid, g.buildings, g.camera, g.unstaffedWorkerBuildings(), disconnected)
	render.DrawSerfs(screen, g.logi.Serfs, g.camera)
	render.DrawVillagers(screen, g.vills.Villagers, g.camera)
	render.DrawLumberjacks(screen, g.jacks.Lumberjacks, g.camera)
	render.DrawFishermen(screen, g.fishers.Fishermen, g.camera)
	render.DrawQuarrymen(screen, g.quarry.Quarrymen, g.camera)
	render.DrawBuilders(screen, g.builders.Builders, g.camera)
	render.DrawMiners(screen, g.miners.Miners, g.camera)
	// Foreground layers (porches/fences/eaves) intentionally come after units;
	// current sprites have none, but the per-building art manifest can add them
	// without another change to the world render order.
	render.DrawBuildingForegrounds(screen, g.buildings, g.camera)
	render.DrawAmbientSkyLife(screen, g.grid, g.camera)
	render.DrawAtmosphericOverlay(screen, g.grid, g.camera)

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
	// connected, not a fresh g.buildingConnected(b) call, reuses the same
	// BFS result visibleConnectivity already paid for above.
	for b, connected := range visibleConnectivity {
		ui.DrawAccessMarker(screen, g.camera, b, connected)
	}
	ui.DrawSelectedRoute(screen, g.camera, g.selection)
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
	ui.BeginResourceTooltips()
	ui.DrawBuildPanel(screen, g.layout, g.palette, g.leftTab, g.demolitionMode, g.hireOptions(), g.finishedBuildingCounts())
	trimServesPrompt := ""
	if g.dialog == ui.DialogConfirmTrimServes {
		trimServesPrompt = fmt.Sprintf(i18n.T().TrimServesConfirmPrompt, len(g.logi.Serfs), g.recommendedServeCount())
	}
	ui.DrawInspectorPanel(screen, g.layout, g.selection, connected, g.stock, g.pop, g.completedTownBuildingCount(), g.playedFrames, occupants, showPriority, priorityLevel, g.dialog, trimServesPrompt)
	ui.DrawMinimapPanel(screen, g.layout, g.grid, g.buildings, g.camera)
	ui.DrawResourceTooltip(screen, g.layout)

	if g.statusMsg != "" {
		ui.DrawText(screen, g.statusMsg, float64(g.layout.LeftWidth+12), 10)
	}
	if g.advisorVisible != nil {
		ui.DrawAdvisorToast(screen, g.layout, advisorTipText(*g.advisorVisible), g.advisorVisible.Building != nil)
	}
	if g.paused {
		g.drawPauseMenu(screen)
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
	ebiten.SetFullscreen(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle(i18n.T().WindowTitle)
	if err := ebiten.RunGame(NewApplication()); err != nil {
		log.Fatal(err)
	}
}
