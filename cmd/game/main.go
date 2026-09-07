// Command game is the entry point for the strategy game.
package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/advisor"
	"strategy_game/internal/audio"
	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/combat"
	"strategy_game/internal/economy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/hunger"
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
	"strategy_game/internal/sentry"
	"strategy_game/internal/soldier"
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
	sentries  *sentry.Controller
	soldiers  *soldier.Controller

	// ais holds every AI opponent faction in a duel match ("1×N против
	// ИИ") -- empty in ordinary single-player "free map" games, where
	// every existing g.xxx field above is the only town that exists. A
	// slice, not a map: iteration order must be deterministic (matters
	// for a future networked match too, not just this session), and
	// there are only ever 1-3 of these, so a linear scan by owner is
	// never a real cost. See cmd/game/ai.go's faction type and
	// cmd/game/ai_brain.go's aiBrain for the rest of this mode.
	ais []*faction

	// duelIsthmuses is growQuadrantWaterCross's own returned geometry,
	// captured once at map generation instead of recomputed later --
	// nil outside "N против ИИ". aiBuildDefenses (cmd/game/ai_brain.go)
	// uses this to fortify each AI faction's own two bordering
	// crossings (see quadrantAssignmentOrder for which two rectangles
	// belong to which quadrant/owner) without re-deriving the map's own
	// water-cross math a second time.
	duelIsthmuses []image.Rectangle

	// leftScrollBuild/leftScrollHire are the first-visible-card index for
	// the Build/Hire tab lists, per the user's explicit request to make
	// the left panel scrollable rather than keep shrinking cards forever
	// as more building/unit kinds get added -- see
	// ui.Layout.leftListWindow. Adjusted by the mouse wheel over the left
	// panel (handleLeftPanelScroll); clamped on read, not on write, so
	// this never needs to know the current list length in advance.
	leftScrollBuild int
	leftScrollHire  int

	// formationLines is how many ranks a right-click move order arranges a
	// selected soldier group into (see commandSoldierGroupTo) -- 1/2/3,
	// toggled with the number keys while a SelectionSoldierGroup is active.
	// Not persisted: it's an input-mode preference, the same "not real
	// world state" reasoning as the camera's zoom-drag state.
	formationLines int

	// deathEffects is presentation-only: it records a final position after a
	// unit has died, then the renderer plays the shared soul/skeleton loop.
	// It is deliberately neither simulation state nor save-game data.
	deathEffects []render.DeathEffect

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
	// aiDefeatedAnnounced marks which AI factions (by Owner) have already
	// had their elimination reported via KindFactionDefeated -- see
	// checkAIFactionDefeats. Permanent once set: a defeated faction never
	// un-defeats, so unlike advisorCooldowns this never expires.
	aiDefeatedAnnounced map[int]bool

	// lastAttackerOwner maps a defending faction's Owner to whoever most
	// recently landed a real combat kill against it (a building, soldier
	// or any other unit) -- recordLastAttacker updates this every tick
	// from soldier.TickResult/sentry.TickResult's own KillOwners. Used by
	// checkAIFactionDefeats/nearestSurvivingFactionTo to credit a
	// faction's eventual defeat to whoever actually did it, per the
	// user's own explicit request ("измени механику кто кого разгромил:
	// разгромил не тот кто ближе, а тот, кто нанес последний урон после
	// которого противника не стало") -- confirmed wrong twice in one
	// session by the old geography-only heuristic ("Красные разгромили
	// синих", "Зелёные разгромили красных", neither matching who the
	// player had actually just killed with their own hands). Overwritten
	// on every hit, not just a killing blow -- whatever value is present
	// at the moment a faction is noticed as newly defeated is, by
	// construction, whoever hit it last.
	lastAttackerOwner map[int]int

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

	// preModeSelectScreen/preModeSelectPaused capture where "Назад" on
	// the mode/difficulty-select screens should return to -- screenTitle
	// when reached from the title screen's own "Новая игра", or the
	// running paused game (screenPlay + paused=true restored) when
	// reached from the Esc pause menu's "Новая игра". See
	// enterModeSelect.
	preModeSelectScreen appScreen
	preModeSelectPaused bool

	// duelOpponentCount/duelDifficulties are the "4х4" duel setup flow's
	// own transient state -- duelOpponentCount is always maxDuelOpponents
	// now (no more player-chosen opponent count), but per the user's
	// explicit "подумай над выбором уровня сложности для каждого
	// противника", each of those opponents still gets its own difficulty
	// pick, one screenDifficultySelect visit at a time (see
	// startDuelGame). duelDifficulties accumulates one entry per bot
	// already picked; its length is also "which opponent's difficulty
	// screen is this" (0-indexed).
	duelOpponentCount int
	duelDifficulties  []aiDifficulty

	// duelResult is the "N против ИИ" win/loss outcome, checked once per
	// tick (see tickOnce) whenever g.ais is non-empty. A real gap found from
	// the user asking "что значит победа, как будет выглядеть" --
	// factionDefeated already existed (tested in isolation) but nothing
	// in the actual running game ever called it: there was no way at all
	// for a match to end, even after one side's every building and unit
	// was gone. Non-zero freezes the simulation (Update's early return,
	// same as g.paused) and shows drawDuelResult's overlay instead.
	duelResult duelResult

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
	// HP is set explicitly (building.MaxHP) -- a real bug found by
	// simulation while building the "1×1 против ИИ" duel mode: without
	// it, the single-player starting Warehouse's zero-value HP made
	// pruneDestroyedBuildings (added for the duel win condition, but it
	// runs in every mode) delete it on the very first simulation tick of
	// every new single-player game. Road is exempted from that check by
	// Kind regardless of HP, but Warehouse is not.
	warehouse := &building.Building{Kind: building.Warehouse, X: warehousePoint.x, Y: warehousePoint.y, HP: building.MaxHP}
	initialRoad := &building.Building{Kind: building.Road, X: warehousePoint.x, Y: warehousePoint.y + 1, HP: building.MaxHP}

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
		grid:           grid,
		buildings:      buildings,
		stock:          stock,
		pop:            &economy.Population{},
		sim:            economy.NewSimulator(framesPerSimTick),
		logi:           logistics.NewController(warehouse, startingSerfs),
		vills:          villagers.NewController(),
		jacks:          lumberjack.NewController(),
		fishers:        fishing.NewController(),
		quarry:         quarry.NewController(),
		builders:       builder.NewController(),
		miners:         miner.NewController(),
		sentries:       sentry.NewController(),
		soldiers:       soldier.NewController(),
		formationLines: 2,
		treeSeed:       defaultTreeSeed,
		fishSeed:       defaultFishSeed,
		stoneSeeded:    true,
		oreSeeded:      true,
		camera:         camera,
		palette:        ui.NewPalette(),
		layout:         layout,
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
	if g.duelResult != duelResultNone {
		return g.updateDuelResult()
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
	g.deathEffects = render.AdvanceDeathEffects(g.deathEffects)
	g.handleCameraPan()
	g.handleCameraZoom()
	g.handleLeftPanelScroll()

	// A modal owns every key and click, so its destructive confirmation or
	// save-name input cannot accidentally also change the map behind it.
	if g.dialog != ui.DialogNone {
		g.handleDialogInput()
	} else {
		g.handleMouse()
	}

	for range g.sim.Advance() {
		g.tickOnce()
	}

	return nil
}

// tickOnce advances the whole simulation -- both factions' economies,
// combat, events, autosave/advisor/audio bookkeeping -- by exactly one
// simulation tick. Factored out of Update() so tests (and any other
// non-interactive driver) can run the simulation forward directly,
// without touching ebiten input/pause/dialog handling at all -- see e.g.
// TestDuelSimulation_AIBuildsAndFactionsFight.
func (g *Game) tickOnce() {
	{
		g.worldTicks++
		for _, b := range g.buildings {
			b.TickGrowth()
		}
		g.tickTreeRegrowth()
		g.tickFishRegrowth()
		g.updateAutomaticGates()
		inactiveWorkers := g.inactiveWorkerBuildings()
		for _, f := range g.ais {
			// Each AI faction's own RequiresWorker buildings must be
			// judged by ITS OWN rosters, not the player's (or another
			// bot's) -- see inactiveWorkerBuildingsFor's doc comment.
			for b, v := range inactiveWorkerBuildingsFor(g.ownedBuildings(f.owner), f.vills, f.jacks, f.fishers, f.quarry, f.miners, f.sentries) {
				inactiveWorkers[b] = v
			}
		}
		economy.TickWithConnectivity(g.buildings, inactiveWorkers, g.disconnectedBuildings())
		tickArmories(g.buildings, inactiveWorkers)
		// Controllers remove hunger deaths from their own rosters during Tick.
		// Capture the final position one tick beforehand so every profession
		// can use the same neutral death animation without changing its API.
		g.queueStarvationDeathEffects()

		// playerBuildings scopes every player controller's Tick call to
		// the player's OWN buildings (plus natural resource nodes) --
		// see ownedBuildingsWithRoads' doc comment. A real bug found
		// from an actual duel-mode playtest report: tickAIFaction always
		// scoped the AI's own controllers this way, but the PLAYER's own
		// controllers here were never given the same treatment -- every
		// one of them (Tick calls below) was still passed the raw,
		// unfiltered g.buildings, which in a "1×1 против ИИ" game
		// contains BOTH factions' buildings in one shared slice. A
		// player's serf could path to, haul from, or even deliver
		// resources into the AI's own buildings, since nothing filtered
		// them out -- reported as "the opponent's servant delivers to
		// (or walks into) my warehouse". In an ordinary single-player
		// game (g.ai == nil, every building's Owner is its zero value 0
		// regardless) this returns exactly g.buildings, so it changes
		// nothing there.
		playerBuildings := g.ownedBuildingsWithRoads(0)

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
		g.sentries.Reserve(ledger)

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
		var sentryResult sentry.TickResult
		type unitStep struct {
			hunger int
			run    func()
		}
		steps := []unitStep{
			{g.logi.MaxWaitingHunger(), func() {
				serfResult = g.logi.Tick(g.grid, playerBuildings, g.buildings, g.stock, ledger, g.soldiers.Soldiers)
			}},
			{g.vills.MaxWaitingHunger(), func() { villagerDeaths = g.vills.Tick(playerBuildings, ledger) }},
			{g.jacks.MaxWaitingHunger(), func() { jackEvents = g.jacks.Tick(g.grid, playerBuildings, g.buildings, ledger) }},
			{g.fishers.MaxWaitingHunger(), func() { fishEvents = g.fishers.Tick(g.grid, playerBuildings, ledger) }},
			{g.quarry.MaxWaitingHunger(), func() { quarryEvents = g.quarry.Tick(g.grid, playerBuildings, g.buildings, ledger) }},
			{g.builders.MaxWaitingHunger(), func() { builderEvents = g.builders.Tick(g.grid, playerBuildings, g.buildings, g.stock, ledger) }},
			{g.miners.MaxWaitingHunger(), func() { minerEvents = g.miners.Tick(g.grid, playerBuildings, g.buildings, ledger) }},
			{g.sentries.MaxWaitingHunger(), func() {
				sentryResult = g.sentries.Tick(playerBuildings, g.opposingIntruderTargetsFor(g.sentries), ledger)
			}},
		}
		sort.SliceStable(steps, func(i, j int) bool { return steps[i].hunger > steps[j].hunger })
		for _, step := range steps {
			step.run()
		}
		// Soldiers don't compete for the shared ledger (they never fetch
		// their own food -- see package soldier's doc comment), so they sit
		// outside the fairness-ordered steps above; their combat damage
		// still needs to land before the prune below, same as a Sentry's.
		// g.buildings (the WHOLE map), not playerBuildings, for the
		// obstacle-avoidance param specifically -- a real bug found from
		// an actual playtest report ("юниты противника спокойно проходят
		// через мои ворота"): scoping a soldier's movement obstacles to
		// its own faction (like every other controller correctly does)
		// meant an opposing faction's walls/gates were never even in the
		// list to be blocked by, letting soldiers walk straight through
		// them. See soldier.Controller.Tick's doc comment and
		// pathfind.FindLandPathForFaction. opposingBuildingsFor still
		// supplies the enemy's buildings separately for targeting.
		soldierResult := g.soldiers.Tick(g.grid, g.buildings, g.opposingBuildingsFor(g.soldiers), g.opposingSoldiersFor(g.soldiers), g.opposingIntruderTargetsForSoldiers(g.soldiers))
		g.recordLastAttacker(0, soldierResult.KillOwners)
		g.recordLastAttacker(0, sentryResult.KillOwners)
		g.pop.Deaths += serfResult.Deaths + villagerDeaths + sentryResult.Deaths + soldierResult.Deaths
		g.pop.UnitsDismissed += serfResult.Dismissed
		// Real cross-faction kills/building destructions -- see
		// soldier.Controller.TickResult and sentry.Controller.TickResult's
		// own doc comments for the playtest report this fixes ("счетчик
		// убито врагов не считает юнитов").
		g.pop.Kills += soldierResult.Kills + sentryResult.Kills
		g.pop.EnemyBuildingsDestroyed += soldierResult.BuildingsDestroyed
		// The map's one shared, profession-free death animation (see
		// internal/render's DeathEffect) -- per the user's own report ("у
		// меня есть спрайт 3х кадровый смерти любого юнита... не вижу его
		// в последних коммитах"): real combat deaths/kills never
		// triggered it before, only the (now removed) sandbox debug enemy
		// did.
		for _, p := range soldierResult.DeathPositions {
			g.addDeathEffect(p.X, p.Y)
		}
		for _, p := range soldierResult.KillPositions {
			g.addDeathEffect(p.X, p.Y)
		}
		for _, p := range sentryResult.KillPositions {
			g.addDeathEffect(p.X, p.Y)
		}
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
		for _, f := range g.ais {
			g.tickAIFaction(f, g.grid)
		}
		g.decayDamagedBuildings()
		g.razeHopelessFactions()
		g.pruneDestroyedBuildings()
		g.checkDuelResult()

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
	return inactiveWorkerBuildingsFor(g.ownedBuildings(0), g.vills, g.jacks, g.fishers, g.quarry, g.miners, g.sentries)
}

// inactiveWorkerBuildingsFor is inactiveWorkerBuildings' logic, factored
// out so a second faction's economy.TickWithConnectivity call (see
// tickFaction) can use its own controllers/buildings instead of the
// player's -- an AI-owned RequiresWorker building must never be judged
// idle by the player's villager/lumberjack/... rosters, which don't
// contain a single AI unit.
func inactiveWorkerBuildingsFor(buildings []*building.Building, vills *villagers.Controller, jacks *lumberjack.Controller, fishers *fishing.Controller, quarry *quarry.Controller, miners *miner.Controller, sentries *sentry.Controller) map[*building.Building]bool {
	m := make(map[*building.Building]bool)
	for _, b := range buildings {
		if building.Types[b.Kind].RequiresWorker {
			m[b] = true
		}
	}
	for _, v := range vills.Villagers {
		if v.Home != nil {
			m[v.Home] = !v.Working()
		}
	}
	for _, j := range jacks.Lumberjacks {
		if j.HomeBuilding() != nil {
			m[j.HomeBuilding()] = !j.AtPost()
		}
	}
	for _, f := range fishers.Fishermen {
		if f.HomeBuilding() != nil {
			m[f.HomeBuilding()] = !f.AtPost()
		}
	}
	for _, q := range quarry.Quarrymen {
		if q.HomeBuilding() != nil {
			m[q.HomeBuilding()] = !q.AtPost()
		}
	}
	for _, mn := range miners.Miners {
		if mn.HomeBuilding() != nil {
			m[mn.HomeBuilding()] = !mn.AtPost()
		}
	}
	for _, s := range sentries.Sentries {
		if s.HomeBuilding() != nil {
			m[s.HomeBuilding()] = !s.Working()
		}
	}
	return m
}

// ownedBuildings returns every building belonging to owner, excluding
// Road -- for combat targeting and worker-presence bookkeeping, where a
// Road is never a meaningful entry. NOT what a faction's own controller
// Tick calls want as their buildings parameter -- those need Road tiles
// present to route serfs/soldiers over the road network at all; see
// ownedBuildingsWithRoads for that case. (A real bug found by actually
// simulating the AI faction: every one of its serfs starved because
// findTavernSupplyJob's road-only pathfind had no Road tiles to route
// over, having been handed this Road-stripped list instead.)
func (g *Game) ownedBuildings(owner int) []*building.Building {
	var out []*building.Building
	for _, b := range g.buildings {
		if (b.Owner == owner || isNaturalResourceKind(b.Kind)) && b.Kind != building.Road {
			out = append(out, b)
		}
	}
	return out
}

// ownedBuildingsWithRoads is ownedBuildings, but keeps Road tiles -- the
// building list a faction's own controllers (logistics, villagers,
// lumberjack, ...) actually need for their Tick calls to route over that
// faction's own road network. See ownedBuildings' doc comment for the
// bug this split fixes.
// A FINISHED Road tile is included regardless of Owner, same as
// isNaturalResourceKind's exemption -- a real bug found from an actual
// playtest report: an AI faction that's fully defeated leaves its own
// Road tiles behind (pruneDestroyedBuildings never removes Road, by
// design), still tagged with the AI's old Owner forever -- nothing ever
// reassigns it. Once the player takes over that territory (builds their
// own Tavern/workshops there), road-only pathfinding (Tavern hauling, a
// worker's own trip to eat) filtered those leftover road tiles out as
// "not mine", breaking delivery on the conquered island while off-road
// pathing (soldier feeding, most worker job-pathing, which never filters
// by Owner at all) kept working fine -- reported as "боевым юнитам
// доставляется еда слугами" [feeding works] "а рыболов не может пойти
// поесть" [tavern trips don't], "мистика!". A finished road is shared
// infrastructure, not faction property, the same way a tree or ore
// deposit already is -- per the user's own explicit suggestion ("дорога
// — нейтральна и не принадлежит ни одной из сторон").
//
// An UNFINISHED road, though, only counts when it's actually this
// faction's own -- a real bug found from an actual playtest report ("мои
// слуги помогают противнику, его слуги - мне"): a blanket Road exemption
// regardless of ConstructionStage let every faction's builder.Controller
// see every OTHER faction's still-under-construction road tiles too
// (builder.startSiteJob picks any candidate with ConstructionStage !=
// ConstructionNone from this exact list, with no owner check of its own
// -- every other building kind is already excluded from a foreign
// faction's list by the b.Owner == owner check above, so Road was the
// only leak). Confirmed live on an actual save: three different AI
// factions' builders each finishing the PLAYER's own unfinished road
// tiles. A finished road needs no owner to already be walkable
// (pathfind's roadSet/buildingOccupancy check Kind, not who built it) --
// only an unfinished one is actually "somebody's work in progress", and
// that somebody must be this faction, not a rival's.
func (g *Game) ownedBuildingsWithRoads(owner int) []*building.Building {
	var out []*building.Building
	for _, b := range g.buildings {
		ownRoad := b.Kind == building.Road && (b.Owner == owner || b.ConstructionStage == building.ConstructionNone)
		if b.Owner == owner || isNaturalResourceKind(b.Kind) || ownRoad {
			out = append(out, b)
		}
	}
	return out
}

// isNaturalResourceKind reports whether kind is a map-generated resource
// node (Tree/Fish/a mineral deposit) rather than a player/AI-constructed
// building. These are never assigned a faction Owner (it stays the zero
// value), so a strict Owner==owner filter would only ever match faction
// 0 by coincidence -- a real bug found by simulation: the AI faction's
// lumberjacks/fishermen/quarrymen/miners could never see a single tree,
// fish, or deposit (ownedBuildings(1)/ownedBuildingsWithRoads(1) filtered
// every one of them out), so the AI's whole raw-material economy (Log,
// StoneBlock, Coal, GoldOre, IronOre, Fish) never produced anything at
// all. Resource nodes are neutral map terrain, not faction property --
// both factions' controllers need to see all of them.
func isNaturalResourceKind(kind building.Kind) bool {
	switch kind {
	case building.Tree, building.Fish, building.StoneDeposit,
		building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
		return true
	default:
		return false
	}
}

// unstaffedWorkerBuildings distinguishes a permanently empty workplace from
// a resident who only stepped out to eat. The renderer uses it for the red
// building tint; gameplay pausing still uses inactiveWorkerBuildings above.
//
// Scoped to g.ownedBuildings(0) (the player's own buildings), not
// g.buildings -- a real bug found from an actual duel-mode playtest report:
// every AI-owned RequiresWorker building was being seeded here too (since
// this used to range over the whole shared g.buildings slice), then never
// cleared, because only the PLAYER's own controllers (g.vills/g.jacks/...)
// are consulted below to clear the flag -- the AI's own separate
// controllers (g.ai.vills/g.ai.jacks/...) never are. Every single AI
// building looked permanently "unstaffed" to the player: red-tinted on
// screen, and surfaced as idle-building advisor tips for buildings that
// were never the player's to manage in the first place.
func (g *Game) unstaffedWorkerBuildings() map[*building.Building]bool {
	m := make(map[*building.Building]bool)
	for _, b := range g.ownedBuildings(0) {
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
	for _, s := range g.sentries.Sentries {
		if s.HomeBuilding() != nil {
			m[s.HomeBuilding()] = false
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
// g.ownedBuildings(0), not g.buildings -- a real bug found from an actual
// duel-mode playtest report, same class as hireOptions'/hireFromTab's own
// fix: without this, the "(N)" next to a Build-tab card counted the AI's
// own finished buildings as if they were the player's.
func (g *Game) finishedBuildingCounts() map[building.Kind]int {
	counts := make(map[building.Kind]int)
	for _, b := range g.ownedBuildings(0) {
		if b.ConstructionStage == building.ConstructionNone {
			counts[b.Kind]++
		}
	}
	return counts
}

// hoveredExistingBuildingKind reports the building kind whose card is
// currently under the cursor in the Build tab (Layout.BuildIndexAt, the
// same hit-test the click handler already uses), but only when the
// player actually has at least one finished building of that kind --
// per the user's explicit request ("во вкладке 'Стройка', при наведении
// на постройку которая есть, подсвечивай на карте постройки"). Used by
// Draw to highlight every matching building on the map; false for every
// other tab/state (paused, a different tab open, hovering an empty
// palette slot) so nothing lights up with nothing to show for it.
func (g *Game) hoveredExistingBuildingKind() (building.Kind, bool) {
	if g.leftTab != ui.BuildTab {
		return 0, false
	}
	mx, my := ebiten.CursorPosition()
	index, ok := g.layout.BuildIndexAt(mx, my, len(g.palette.Kinds), g.leftScrollBuild)
	if !ok || index >= len(g.palette.Kinds) {
		return 0, false
	}
	kind := g.palette.Kinds[index]
	if g.finishedBuildingCounts()[kind] == 0 {
		return 0, false
	}
	return kind, true
}

// producedResourceTypes scans the player's own completed buildings once
// and reports every resource type at least one of them can currently
// output (primary or secondary, primary or alt recipe) -- the "a producer
// already exists" half of building.Unlocked's two-way check, computed
// once per caller rather than re-scanning every building per palette
// entry. g.ownedBuildings(0), same real-bug class as finishedBuildingCounts
// above: an AI-owned building in a duel game must never unlock the
// player's own palette.
func (g *Game) producedResourceTypes() map[resource.Type]bool {
	produced := map[resource.Type]bool{}
	for _, b := range g.ownedBuildings(0) {
		if b.ConstructionStage != building.ConstructionNone {
			continue
		}
		for _, r := range building.Types[b.Kind].AllRecipes() {
			if r.OutputAmount > 0 {
				produced[r.Output] = true
			}
			if r.SecondaryOutputAmount > 0 {
				produced[r.SecondaryOutput] = true
			}
		}
	}
	return produced
}

// buildingUnlocked reports whether kind is currently buildable -- see
// building.Unlocked's doc comment for the "producer built OR already on
// the stockpile" rule applied per input resource. produced is a
// caller-supplied producedResourceTypes() result: pass the same one
// across every kind checked in a single Draw/click rather than
// recomputing it per card.
func (g *Game) buildingUnlocked(kind building.Kind, produced map[resource.Type]bool) bool {
	return building.Unlocked(kind,
		func(t resource.Type) bool { return produced[t] },
		func(t resource.Type) bool { return g.stock.Amount(t) > 0 },
	)
}

// buildingLockReason renders why kind is locked (building.MissingInputs,
// turned into localized resource names) -- a direct follow-up to a
// playtest report that a merely greyed-out card wasn't visibly different
// enough on its own ("визуально нельзя отличить то можно построить сейчас
// от того что нельзя"): the card's hover tooltip now also names what's
// actually missing, the user's own example being "отсутствует построенная
// ферма или пшеница на складе" for the Pig Farm. Empty once kind is
// unlocked.
func (g *Game) buildingLockReason(kind building.Kind, produced map[resource.Type]bool) string {
	missing := building.MissingInputs(kind,
		func(t resource.Type) bool { return produced[t] },
		func(t resource.Type) bool { return g.stock.Amount(t) > 0 },
	)
	if len(missing) == 0 {
		return ""
	}
	names := make([]string, len(missing))
	for i, t := range missing {
		names[i] = i18n.T().ResourceName[t]
	}
	return fmt.Sprintf(i18n.T().BuildLockedReason, strings.Join(names, ", "))
}

// paletteUnlocked computes buildingUnlocked for every palette entry at
// once, sharing one producedResourceTypes() scan across all of them --
// used both by the Build tab's Draw call and by its click handler (see
// BuildIndexAt's call site), so a locked card can neither be drawn as
// available nor actually selected for placement.
func (g *Game) paletteUnlocked() map[building.Kind]bool {
	produced := g.producedResourceTypes()
	unlocked := make(map[building.Kind]bool, len(g.palette.Kinds))
	for _, kind := range g.palette.Kinds {
		unlocked[kind] = g.buildingUnlocked(kind, produced)
	}
	return unlocked
}

// paletteLockReasons is paletteUnlocked's sibling for the tooltip text --
// one buildingLockReason() per currently-locked palette entry, sharing the
// same producedResourceTypes() scan.
func (g *Game) paletteLockReasons() map[building.Kind]string {
	produced := g.producedResourceTypes()
	reasons := make(map[building.Kind]string, len(g.palette.Kinds))
	for _, kind := range g.palette.Kinds {
		if reason := g.buildingLockReason(kind, produced); reason != "" {
			reasons[kind] = reason
		}
	}
	return reasons
}

// completedTownBuildingCount reports finished player structures for the town
// summary. Roads and naturally generated objects are deliberately excluded:
// the number answers "how many buildings does my town have?", not "how many
// occupied map cells exist?". g.ownedBuildings(0), not g.buildings -- same
// real bug class as finishedBuildingCounts above: this feeds both the town
// summary and developmentScore, so an AI building used to inflate the
// player's own building count and score.
func (g *Game) completedTownBuildingCount() int {
	return g.completedBuildingCountFor(0)
}

// completedBuildingCountFor is completedTownBuildingCount generalized to
// any faction -- see developmentLeaderboard, the one other caller (an AI
// faction's own building count for its development score).
func (g *Game) completedBuildingCountFor(owner int) int {
	count := 0
	for _, b := range g.ownedBuildings(owner) {
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
	// b.Owner == 0, not every building on the map -- a real bug found
	// from an actual duel-mode playtest report: "у меня отображается 1
	// доступный рыбак хотя хижину я еще не построил". Before this, the
	// AI's own finished LumberjackHut/FisherHut/QuarryHut/... counted
	// toward the PLAYER's hire-tab "Limit" the instant the AI built one,
	// showing a vacancy that didn't exist on the player's own side at
	// all. See hireFromTab's identical fix -- clicking that phantom card
	// would have actually spawned a player worker into the AI's
	// building, not merely miscounted.
	countBuildings := func(kind building.Kind) int {
		n := 0
		for _, b := range g.buildings {
			if b.Kind == kind && b.Owner == 0 && b.ConstructionStage == building.ConstructionNone {
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
		limited(ui.HireWeaponsmith, building.Armory, countProfession(villagers.Weaponsmith)),
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
	// findWarehouseOwnedBy(..., 0) and g.ownedBuildingsWithRoads(0), not
	// findWarehouse(g.buildings) and g.buildings -- a real bug found
	// from an actual duel-mode playtest report, same class as every
	// other fix in this area: findWarehouse just grabs the first
	// Warehouse found, and without the ownership filter the AI's own
	// buildings/hauling distances would inflate the PLAYER's serf
	// recommendation.
	playerBuildings := g.ownedBuildingsWithRoads(0)
	warehouse := findWarehouseOwnedBy(playerBuildings, 0)
	total := 0.0
	if warehouse != nil {
		for _, b := range playerBuildings {
			if b == nil || b == warehouse || b.ConstructionStage != building.ConstructionNone {
				continue
			}
			amount, ticks := serfHaulOutputRate(b.Kind)
			if ticks <= 0 || amount <= 0 {
				continue
			}
			path, ok := pathfind.FindPath(playerBuildings, warehouse, b)
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
	case ui.HireWeaponsmith:
		g.hireVillagerInto(villagers.Weaponsmith, building.Armory)
	case ui.HireLumberjack:
		// g.ownedBuildings(0), not g.buildings -- a real bug found from
		// an actual duel-mode playtest report ("у меня отображается 1
		// доступный рыбак хотя хижину я еще не построил"): this used to
		// search the whole map's buildings, so an AI-owned finished hut
		// with no PLAYER controller registered as its resident (which
		// is every AI hut, always -- it's staffed by g.ai's own,
		// separate controllers) looked exactly like an empty vacancy
		// and would have actually spawned a player worker into it. See
		// hireOptions' identical fix for the matching Hire-tab miscount.
		for _, b := range g.ownedBuildings(0) {
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
		for _, b := range g.ownedBuildings(0) {
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
		for _, b := range g.ownedBuildings(0) {
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
		for _, b := range g.ownedBuildings(0) {
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
	// g.ownedBuildings(0), not g.buildings -- see hireFromTab's
	// HireLumberjack case for the identical, real bug this guards
	// against (a Farmer/Baker/Winemaker/... hire would otherwise have
	// been able to land in the AI's own building).
	for _, b := range g.ownedBuildings(0) {
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

// handleLeftPanelScroll lets the mouse wheel scroll the Build/Hire card
// list when a list has grown too long to keep shrinking cards to fit --
// per the user's explicit request ("подумай над прокруткой, чтобы можно
// было прокручивать список вверх и вниз"). Scrolls whichever tab is
// currently active; Layout.leftListWindow clamps the result on every read,
// so the raw accumulator only needs a floor at zero here.
func (g *Game) handleLeftPanelScroll() {
	mx, my := ebiten.CursorPosition()
	if !image.Pt(mx, my).In(g.layout.LeftPanel()) {
		return
	}
	_, wheelY := ebiten.Wheel()
	if wheelY == 0 {
		return
	}
	delta := -int(wheelY)
	if delta == 0 {
		if wheelY > 0 {
			delta = -1
		} else {
			delta = 1
		}
	}
	if g.leftTab == ui.HireTab {
		g.leftScrollHire += delta
		if g.leftScrollHire < 0 {
			g.leftScrollHire = 0
		}
		return
	}
	g.leftScrollBuild += delta
	if g.leftScrollBuild < 0 {
		g.leftScrollBuild = 0
	}
}
func (g *Game) handleMouse() {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		mx, my := ebiten.CursorPosition()
		if g.handleHireCardDismissRightClick(mx, my) {
			return
		}
		// Per the user's explicit "клик ПКМ +/- 10 штук в очередь": a
		// right-click on an Armory queue button moves ten instead of one.
		if g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil &&
			g.selection.Building.Kind == building.Armory && g.selection.Building.ConstructionStage == building.ConstructionNone {
			if row, delta, ok := g.layout.ArmoryQueueButtonAt(mx, my); ok {
				g.adjustArmoryQueue(g.selection.Building, row, delta*10)
				return
			}
		}
		// Per the user's explicit request: a selected soldier group's
		// right-click either orders an attack (cursor landed on an
		// opposing target) or a formation move (empty tile) -- see
		// commandSoldierGroupAttackFaction/commandSoldierGroupTo.
		//
		// opposingBuildingAt/opposingSoldierAt/opposingIntruderAt are
		// checked too -- a real bug found from an actual playtest report
		// ("клик боевым юнитом на постройку противника - перемещает
		// юнитов, но не уничтожает постройку врага", later widened to
		// "клик боевым юнитом на любого юнита/постройку противника,
		// должен переходить в режим атаки"): clicking ANY opposing
		// faction target in "N против ИИ" (building, rival soldier, or
		// any other unit) fell straight through to a plain move order,
		// which never actually set anything to fight. See
		// commandSoldierGroupAttackFaction/AttackFactionOrder and its two
		// siblings below.
		if g.selection.Kind == ui.SelectionSoldierGroup && len(g.selection.SoldierGroup) > 0 && image.Pt(mx, my).In(g.layout.MapRect()) {
			tx, ty := g.camera.ScreenToTile(mx, my)
			buildingTarget := g.opposingBuildingAt(tx, ty)
			soldierTarget := g.opposingSoldierAt(tx, ty)
			intruderTarget, hasIntruderTarget := g.opposingIntruderAt(tx, ty)
			switch {
			case buildingTarget != nil:
				g.commandSoldierGroupAttackFaction(buildingTarget)
			case soldierTarget != nil:
				g.commandSoldierGroupAttackSoldier(soldierTarget)
			case hasIntruderTarget:
				g.commandSoldierGroupAttackIntruder(intruderTarget)
			default:
				g.commandSoldierGroupTo(mx, my)
			}
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
	index, ok := g.layout.HireIndexAt(mx, my, len(options), g.leftScrollHire)
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
		if index, ok := g.layout.HireIndexAt(mx, my, len(options), g.leftScrollHire); ok {
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
		if index, ok := g.layout.BuildIndexAt(mx, my, len(g.palette.Kinds), g.leftScrollBuild); ok {
			if index < len(g.palette.Kinds) && g.buildingUnlocked(g.palette.Kinds[index], g.producedResourceTypes()) {
				g.palette.Select(index)
				g.buildMode = true
				g.clearWallAnchor()
				g.statusMsg = ""
			}
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
	if g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil &&
		g.selection.Building.Kind == building.Barracks && g.selection.Building.ConstructionStage == building.ConstructionNone {
		if index, ok := g.layout.BarracksHireIndexAt(mx, my); ok {
			switch index {
			case 0:
				g.hireSentry(g.selection.Building)
			case 1:
				g.hireArcher(g.selection.Building)
			case 2:
				g.hireSwordsman(g.selection.Building)
			}
			return
		}
	}
	if g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil &&
		g.selection.Building.Kind == building.Armory && g.selection.Building.ConstructionStage == building.ConstructionNone {
		if row, delta, ok := g.layout.ArmoryQueueButtonAt(mx, my); ok {
			g.adjustArmoryQueue(g.selection.Building, row, delta)
			return
		}
	}
	if g.selection.Kind == ui.SelectionSoldierGroup && len(g.selection.SoldierGroup) > 0 {
		if n, ok := g.layout.FormationLinesAt(mx, my); ok {
			g.formationLines = n
			return
		}
		if g.layout.SoldierGroupSplitAt(mx, my) {
			if g.selection.SoldierGroupAnchor != nil {
				g.selection.SoldierGroup = []*soldier.Soldier{g.selection.SoldierGroupAnchor}
			}
			return
		}
		if g.layout.SoldierGroupByProfessionAt(mx, my) {
			g.selection.SoldierGroup = g.soldierGroupOfProfession(g.selection.SoldierGroup[0].Profession)
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
		// Shift+click on another soldier merges its own proximity group
		// into the currently selected one instead of replacing it -- per
		// the user's explicit request for a way to combine e.g. an Archer
		// group and a Swordsman group into one squad.
		if selected.Kind == ui.SelectionSoldierGroup && g.selection.Kind == ui.SelectionSoldierGroup &&
			ebiten.IsKeyPressed(ebiten.KeyShift) {
			g.selection.SoldierGroup = mergeSoldierGroups(g.selection.SoldierGroup, selected.SoldierGroup)
			g.selection.SoldierGroupAnchor = selected.SoldierGroupAnchor
			return
		}
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
// pieces can close a loop or extend a run, but a join may never create a
// T/cross-shaped section: the settlement supports only straight walls and
// 90-degree turns.
func (g *Game) commitWallPath(path []building.Point) bool {
	if !building.CanCreateWallTopology(g.buildings, path) {
		return false
	}
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
		g.removeRoadUnderneath(point.X, point.Y)
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

// removeRoadUnderneath removes a finished Road building sitting at
// exactly (x, y), if any -- building.CanPlace now deliberately allows a
// fresh StoneWall to be placed on top of a finished Road tile instead of
// rejecting the whole placement outright (per the user's own explicit
// request, "разреши строительство стены поверх участка дороги... дорога
// не является чем-то запрещенным"), but CanPlace itself never mutates
// anything -- every wall-committing call site (commitWallPath below,
// aiFortifyIsthmus in ai_brain.go) must call this right after actually
// placing the wall there, or the same tile would end up holding two
// buildings at once.
func (g *Game) removeRoadUnderneath(x, y int) {
	for i, b := range g.buildings {
		if b != nil && b.Kind == building.Road && b.ConstructionStage == building.ConstructionNone && b.X == x && b.Y == y {
			g.buildings = append(g.buildings[:i], g.buildings[i+1:]...)
			return
		}
	}
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

// firstFreeWatchTower returns the first finished WatchTower without a
// resident Sentry, or nil if none exists -- the same "one worker per
// matching finished building" rule every other profession already
// follows (see hireOptions' doc comment), just checked directly instead
// of surfaced as a left-panel card, since a Sentry is hired from the
// Barracks' own inspector instead (see the user's explicit request).
// g.ownedBuildings(0), not g.buildings -- same real bug class as
// hireFromTab's HireLumberjack case: without this, hiring a Sentry from
// the Barracks could have placed a player unit into the AI's own
// WatchTower (unstaffed from the player's g.sentries controller's point
// of view, since the AI's is staffed by its own, separate one).
func (g *Game) firstFreeWatchTower() *building.Building {
	for _, b := range g.ownedBuildings(0) {
		if b.Kind == building.WatchTower && b.ConstructionStage == building.ConstructionNone && !g.sentries.HasHome(b) {
			return b
		}
	}
	return nil
}

// canHireSentry reports whether clicking the given Barracks' hire button
// would actually succeed right now: it has its own gold delivered (not
// the shared stockpile -- see the Barracks Type's PassiveInputs doc
// comment) and a free WatchTower exists for the new Sentry to occupy.
func (g *Game) canHireSentry(barracks *building.Building) bool {
	if barracks == nil || barracks.Kind != building.Barracks || barracks.ConstructionStage != building.ConstructionNone {
		return false
	}
	return barracks.InputBuffer[resource.Gold] >= unitHireCost && g.firstFreeWatchTower() != nil
}

// hireSentry spends 1 gold from the Barracks' own InputBuffer (not the
// shared stockpile, unlike every other unit hire -- see trySpendGold) and
// spawns a Sentry into the first free WatchTower. A no-op if
// canHireSentry would report false, so a click site can call this
// unconditionally after the button is drawn only when hireable.
func (g *Game) hireSentry(barracks *building.Building) {
	if !g.canHireSentry(barracks) {
		return
	}
	tower := g.firstFreeWatchTower()
	if tower == nil || !barracks.TakeInput(resource.Gold, unitHireCost) {
		return
	}
	g.sentries.Spawn(tower)
}

// canHireArcher/canHireSwordsman report whether the given Barracks holds
// enough of its own delivered equipment (see Types[Barracks].PassiveInputs)
// to hire one -- gold plus a Bow/Sword and a LeatherArmor, per the user's
// explicit "Лучник: лук деревянный, кожаный доспех + 1 ед золота. Мечник:
// меч железный, кожаный доспех + 1 ед золота." Unlike a Sentry, neither
// needs a free WatchTower: they're mobile, no "home" building at all.
func (g *Game) canHireArcher(barracks *building.Building) bool {
	return canHireEquippedSoldier(barracks, resource.Bow)
}

func (g *Game) canHireSwordsman(barracks *building.Building) bool {
	return canHireEquippedSoldier(barracks, resource.Sword)
}

func canHireEquippedSoldier(barracks *building.Building, weapon resource.Type) bool {
	if barracks == nil || barracks.Kind != building.Barracks || barracks.ConstructionStage != building.ConstructionNone {
		return false
	}
	return barracks.InputBuffer[resource.Gold] >= unitHireCost &&
		barracks.InputBuffer[weapon] >= 1 &&
		barracks.InputBuffer[resource.LeatherArmor] >= 1
}

// hireArcher/hireSwordsman spend the Barracks' own gold + weapon + armor
// (not the shared stockpile, same convention as hireSentry) and spawn the
// new soldier onto a free ground tile near the Barracks -- see
// freeGroundTileNear. A no-op if the matching canHire* would report false.
func (g *Game) hireArcher(barracks *building.Building) {
	g.hireEquippedSoldier(barracks, soldier.Archer, resource.Bow, g.canHireArcher)
}

func (g *Game) hireSwordsman(barracks *building.Building) {
	g.hireEquippedSoldier(barracks, soldier.Swordsman, resource.Sword, g.canHireSwordsman)
}

func (g *Game) hireEquippedSoldier(barracks *building.Building, profession soldier.Profession, weapon resource.Type, canHire func(*building.Building) bool) {
	if !canHire(barracks) {
		return
	}
	access := barracks.AccessPoint()
	x, y, ok := g.freeGroundTileNear(access.X, access.Y)
	if !ok {
		return
	}
	if !barracks.TakeInput(resource.Gold, unitHireCost) || !barracks.TakeInput(weapon, 1) || !barracks.TakeInput(resource.LeatherArmor, 1) {
		return
	}
	// .Owner = 0 is already Go's zero value here, so this is a no-op for
	// the player -- kept explicit anyway so nobody has to rediscover
	// aiHireSoldiers' Owner bug fix (cmd/game/ai_brain.go) by staring at
	// an inconsistency between this call site and that one.
	g.soldiers.Spawn(profession, x, y).Owner = 0
}

// freeGroundTileNear searches outward in growing square rings from (cx, cy)
// for the nearest dry tile with no building and no other unit already
// standing on it -- used to place a freshly hired Archer/Swordsman, which
// unlike a Sentry has no "home" building of its own to appear in.
func (g *Game) freeGroundTileNear(cx, cy int) (int, int, bool) {
	if g.groundTileFree(cx, cy) {
		return cx, cy, true
	}
	for r := 1; r <= 8; r++ {
		for dx := -r; dx <= r; dx++ {
			for dy := -r; dy <= r; dy++ {
				if dx > -r && dx < r && dy > -r && dy < r {
					continue // interior already covered by a smaller ring
				}
				x, y := cx+dx, cy+dy
				if g.groundTileFree(x, y) {
					return x, y, true
				}
			}
		}
	}
	return 0, 0, false
}

func (g *Game) groundTileFree(x, y int) bool {
	if !g.grid.InBounds(x, y) || g.grid.At(x, y).Terrain == world.Water {
		return false
	}
	for _, b := range g.buildings {
		footprint := building.Types[b.Kind].Footprint
		if x >= b.X && x < b.X+footprint && y >= b.Y && y < b.Y+footprint {
			return false
		}
	}
	for _, s := range g.soldiers.Soldiers {
		if s.X == x && s.Y == y {
			return false
		}
	}
	return true
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
	g.sentries.RemoveHome(b)
	g.sentries.CancelRouteTo(b) // same, for sentries
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
	case ui.SelectionSoldierGroup:
		// A starved soldier is dropped from g.soldiers.Soldiers without its
		// HP being touched (see soldier.Controller.Tick), so Alive() alone
		// can't tell a departed group member from a living one -- an actual
		// roster membership check is needed here, unlike every case above.
		var alive []*soldier.Soldier
		for _, sd := range g.selection.SoldierGroup {
			for _, live := range g.soldiers.Soldiers {
				if live == sd {
					alive = append(alive, sd)
					break
				}
			}
		}
		if len(alive) > 0 {
			g.selection.SoldierGroup = alive
			anchorAlive := false
			for _, sd := range alive {
				if sd == g.selection.SoldierGroupAnchor {
					anchorAlive = true
					break
				}
			}
			if !anchorAlive {
				g.selection.SoldierGroupAnchor = alive[0]
			}
			return
		}
	default:
		return
	}
	g.selection.Clear()
}

// refreshPopulation rebuilds the live headcount while retaining the
// persistent death/removal history shown in the empty inspector panel.
func (g *Game) refreshPopulation() {
	g.pop.Count = len(g.logi.Serfs) + len(g.vills.Villagers) + len(g.jacks.Lumberjacks) + len(g.fishers.Fishermen) + len(g.quarry.Quarrymen) + len(g.builders.Builders) + len(g.miners.Miners) + len(g.sentries.Sentries) + len(g.soldiers.Soldiers)
}

// developmentScore is a display-only "how far has this town come" number
// for the empty-selection town summary panel, per the user's explicit
// request for a kill counter plus "some kind of development score". A
// simple weighted sum of counters the summary already tracks (plus the
// warehouse's gold) -- not itself simulation state, recomputed fresh every
// draw, never saved. Weights are a first pass, easy to retune once this
// is actually played with: a finished building or a kill is worth more
// than one more citizen or one more gold, since either is a bigger
// investment of the player's time.
func (g *Game) developmentScore() int {
	return developmentScoreFrom(g.completedTownBuildingCount(), g.pop, g.stock)
}

// developmentScoreFrom is developmentScore's formula, generalized to any
// faction's own building count/population/stockpile -- see
// developmentLeaderboard, which uses it for every AI faction the same
// way developmentScore uses it for the player.
func developmentScoreFrom(completedBuildings int, pop *economy.Population, stock *resource.Stockpile) int {
	if pop == nil {
		return 0
	}
	gold := 0
	if stock != nil {
		gold = stock.Amount(resource.Gold)
	}
	return completedBuildings*10 + pop.Count*5 + pop.Kills*20 + gold
}

// developmentLeaderboard ranks every faction (the player, Owner 0,
// included) by developmentScoreFrom, descending -- per the user's
// explicit request ("добавь в правое меню в режиме игры с противником
// топ игроков по уровню развития"). A single-entry slice (just the
// player) outside "N против ИИ" -- see drawTownSummary's own doc comment
// on why that means the section doesn't render at all: nothing to rank
// against.
//
// A defeated faction (factionDefeated) always scores 0, regardless of
// developmentScoreFrom's own formula -- a real playtest report ("2
// противника уничтожены, но у них высокие очки, а тот кто их уничтожил -
// имеет самые низкие очки"): g.ais never removes an eliminated faction
// (see tickAIFaction's own doc comment on why it keeps ticking one
// forever), and its stock/Kills don't reset on defeat either -- a rich,
// battle-hardened faction wiped out at its economic peak kept outranking
// its own killer, who had just spent gold on soldiers/an Armory to
// actually win the fight. Once every real building and unit is gone,
// there is nothing left to rank as "developing".
func (g *Game) developmentLeaderboard() []ui.LeaderboardEntry {
	scoreFor := func(owner, completedBuildings int, pop *economy.Population, stock *resource.Stockpile) int {
		if g.factionDefeated(owner) {
			return 0
		}
		return developmentScoreFrom(completedBuildings, pop, stock)
	}
	entries := make([]ui.LeaderboardEntry, 0, 1+len(g.ais))
	entries = append(entries, ui.LeaderboardEntry{Owner: 0, Score: scoreFor(0, g.completedTownBuildingCount(), g.pop, g.stock)})
	for _, f := range g.ais {
		score := scoreFor(f.owner, g.completedBuildingCountFor(f.owner), f.pop, f.stock)
		entries = append(entries, ui.LeaderboardEntry{Owner: f.owner, Score: score})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Score > entries[j].Score })
	return entries
}

// buildingSelectionAt resolves only a building, including a Road. It is used
// by continuous demolition so a passer-by never intercepts a road click.
//
// Owner != 0 buildings are skipped entirely -- a real bug found from an
// actual duel-mode playtest report ("я почему-то могу выбирать постройки
// и юнитов противника... а также могу удалить его постройки"): this
// function is what continuous demolition itself resolves a click through,
// so without this check a player could select AND DELETE the AI's own
// buildings directly. In an ordinary single-player game every building's
// Owner is its zero value 0 regardless, so this changes nothing there.
func (g *Game) buildingSelectionAt(mx, my int) ui.Selection {
	tx, ty := g.camera.ScreenToTile(mx, my)
	for i := len(g.buildings) - 1; i >= 0; i-- {
		b := g.buildings[i]
		if b == nil || b.Owner != 0 {
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
	// Owner != 0 buildings are skipped -- see buildingSelectionAt's
	// identical, real bug fix: without this, clicking any AI-owned
	// building selected and inspected it exactly like the player's own
	// (its stockpile, its contents), which is where the "могу
	// выбирать... смотреть что у него на складе" report came from.
	var roadHit *building.Building
	for i := len(g.buildings) - 1; i >= 0; i-- {
		b := g.buildings[i]
		if b.Owner != 0 {
			continue
		}
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
	for i := len(g.soldiers.Soldiers) - 1; i >= 0; i-- {
		sd := g.soldiers.Soldiers[i]
		if sd.X == tx && sd.Y == ty && sd.Alive() {
			return ui.Selection{Kind: ui.SelectionSoldierGroup, SoldierGroup: g.soldierGroupNear(sd), SoldierGroupAnchor: sd}
		}
	}
	// Opposing (any Owner != 0) buildings and units, read-only -- per the
	// user's explicit request ("разреши клик на юнитов противника, и
	// отображай в правом окне информацию о нем, но без управления им").
	// Tried only after every player-owned check above has already missed,
	// so a click never prefers an opponent's object over the player's own
	// when both happen to sit on the same tile (buildings can't overlap,
	// but a unit could in principle share a tile with an opposing one).
	if sel, ok := g.opposingSelectionAt(tx, ty); ok {
		return sel
	}
	if roadHit != nil {
		return ui.Selection{Kind: ui.SelectionBuilding, Building: roadHit}
	}
	return ui.Selection{}
}

// opposingSelectionAt looks for any AI faction's building or unit
// standing at (tx, ty) -- see selectionAt's own doc comment on why this
// is read-only (SelectionOpposingBuilding/SelectionOpposingUnit never
// match any of this package's action-gating checks, so nothing here can
// accidentally let the player command an opponent's object).
func (g *Game) opposingSelectionAt(tx, ty int) (ui.Selection, bool) {
	for i := len(g.buildings) - 1; i >= 0; i-- {
		b := g.buildings[i]
		if b.Owner == 0 || b.HP <= 0 {
			continue
		}
		footprint := building.Types[b.Kind].Footprint
		if tx >= b.X && tx < b.X+footprint && ty >= b.Y && ty < b.Y+footprint {
			return ui.Selection{Kind: ui.SelectionOpposingBuilding, Building: b}, true
		}
	}
	for _, f := range g.ais {
		for i := len(f.soldiers.Soldiers) - 1; i >= 0; i-- {
			sd := f.soldiers.Soldiers[i]
			if sd.X == tx && sd.Y == ty && sd.Alive() {
				name := i18n.T().UnitArcher
				if sd.Profession == soldier.Swordsman {
					name = i18n.T().UnitSwordsman
				}
				return ui.Selection{Kind: ui.SelectionOpposingUnit, OpposingUnitOwner: f.owner, OpposingUnitKind: name, OpposingUnitHP: sd.HP, OpposingUnitMaxHP: combat.MaxHP}, true
			}
		}
		for i := len(f.sentries.Sentries) - 1; i >= 0; i-- {
			s := f.sentries.Sentries[i]
			if s.X == tx && s.Y == ty {
				return ui.Selection{Kind: ui.SelectionOpposingUnit, OpposingUnitOwner: f.owner, OpposingUnitKind: i18n.T().UnitSentry}, true
			}
		}
		for i := len(f.logi.Serfs) - 1; i >= 0; i-- {
			s := f.logi.Serfs[i]
			if s.X == tx && s.Y == ty {
				return ui.Selection{Kind: ui.SelectionOpposingUnit, OpposingUnitOwner: f.owner, OpposingUnitKind: i18n.T().UnitSerf}, true
			}
		}
		for i := len(f.vills.Villagers) - 1; i >= 0; i-- {
			v := f.vills.Villagers[i]
			if v.X == tx && v.Y == ty && v.VisibleOnMap() {
				return ui.Selection{Kind: ui.SelectionOpposingUnit, OpposingUnitOwner: f.owner, OpposingUnitKind: villagerProfessionName(v.Profession)}, true
			}
		}
		for i := len(f.jacks.Lumberjacks) - 1; i >= 0; i-- {
			j := f.jacks.Lumberjacks[i]
			if j.X == tx && j.Y == ty && j.VisibleOnMap() {
				return ui.Selection{Kind: ui.SelectionOpposingUnit, OpposingUnitOwner: f.owner, OpposingUnitKind: i18n.T().UnitLumberjack}, true
			}
		}
		for i := len(f.fishers.Fishermen) - 1; i >= 0; i-- {
			fh := f.fishers.Fishermen[i]
			if fh.X == tx && fh.Y == ty && fh.VisibleOnMap() {
				return ui.Selection{Kind: ui.SelectionOpposingUnit, OpposingUnitOwner: f.owner, OpposingUnitKind: i18n.T().UnitFisherman}, true
			}
		}
		for i := len(f.quarry.Quarrymen) - 1; i >= 0; i-- {
			q := f.quarry.Quarrymen[i]
			if q.X == tx && q.Y == ty && q.VisibleOnMap() {
				return ui.Selection{Kind: ui.SelectionOpposingUnit, OpposingUnitOwner: f.owner, OpposingUnitKind: i18n.T().UnitQuarryman}, true
			}
		}
		for i := len(f.builders.Builders) - 1; i >= 0; i-- {
			bl := f.builders.Builders[i]
			if bl.X == tx && bl.Y == ty {
				return ui.Selection{Kind: ui.SelectionOpposingUnit, OpposingUnitOwner: f.owner, OpposingUnitKind: i18n.T().UnitBuilder}, true
			}
		}
		for i := len(f.miners.Miners) - 1; i >= 0; i-- {
			m := f.miners.Miners[i]
			if m.X == tx && m.Y == ty && m.VisibleOnMap() {
				return ui.Selection{Kind: ui.SelectionOpposingUnit, OpposingUnitOwner: f.owner, OpposingUnitKind: i18n.T().UnitMiner}, true
			}
		}
	}
	return ui.Selection{}, false
}

// villagerProfessionName maps a villagers.Profession to its localized
// display name -- cmd/game's own small mirror of internal/ui's
// unexported hireKindForProfession/hireName pair, needed here because
// opposingSelectionAt has no HireKind of its own to go through.
func villagerProfessionName(p villagers.Profession) string {
	t := i18n.T()
	switch p {
	case villagers.Farmer:
		return t.UnitFarmer
	case villagers.Baker:
		return t.UnitBaker
	case villagers.Winemaker:
		return t.UnitWinemaker
	case villagers.Swineherd:
		return t.UnitSwineherd
	case villagers.Butcher:
		return t.UnitButcher
	case villagers.Carpenter:
		return t.UnitCarpenter
	case villagers.Smelter:
		return t.UnitSmelter
	case villagers.Weaponsmith:
		return t.UnitWeaponsmith
	default:
		return t.UnitFarmer
	}
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
	for _, s := range g.sentries.Sentries {
		if within(s.X, s.Y) {
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
	// findWarehouseOwnedBy, not findWarehouse: in "1×1 против ИИ" mode
	// g.buildings holds both factions' buildings, geographically split by
	// water with no road between them (see the duel map generator). A
	// building must connect to a warehouse of its OWN Owner, never
	// whichever warehouse simply comes first in the slice -- otherwise
	// every AI building would test permanently disconnected against the
	// player's unreachable warehouse. In ordinary single-player play every
	// building (including the one warehouse) is Owner 0 by construction,
	// so this behaves identically to the old findWarehouse(g.buildings).
	warehouse := findWarehouseOwnedBy(g.buildings, b.Owner)
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

	// g.ownedBuildings(0) and a player-only disconnected map, not
	// g.buildings/g.disconnectedBuildings() unfiltered -- the advisor is
	// entirely player-facing (g.stock/g.pop passed alongside it already
	// are the player's own), so every input into it needs the same
	// scoping. A real bug found from an actual duel-mode playtest
	// report, same class as unstaffedWorkerBuildings' own fix: without
	// this, the AI's own disconnected/shortage-prone buildings could
	// surface an advisor tip about a building that was never the
	// player's to manage.
	playerDisconnected := make(map[*building.Building]bool)
	for b, v := range g.disconnectedBuildings() {
		if b != nil && b.Owner == 0 {
			playerDisconnected[b] = v
		}
	}
	tips := advisor.Evaluate(g.ownedBuildings(0), g.stock, g.pop, playerDisconnected, g.advisorIdleSince, g.advisorGatherStuckSince, g.worldTicks, g.recommendedServeCount(), len(g.logi.Serfs))
	if tip, blocked := g.enclosedGatherWorkerTip(); blocked {
		tips = append(tips, tip)
	}
	for _, tip := range tips {
		g.queueAdvisorTip(tip)
	}
	g.checkAIFactionDefeats()
}

// checkAIFactionDefeats reports each AI faction's elimination, once, the
// first time this notices it -- per the user's explicit request ("добавь
// в игровые уведомления (советник), когда синий противник побеждает
// зеленого например, или другие цвета"). A no-op outside "N против ИИ"
// (g.ais empty). checkDuelResult already knows how to tell "defeated"
// (factionDefeated) and when the WHOLE match ends; this is the one-off,
// per-faction announcement layered on top of it -- every bot faction
// still gets checked even after the match is decided (so the last one or
// two eliminations before an eventual player victory still get their own
// toast), since Update's own early return on g.duelResult freezes the
// simulation (and so this call) at that point anyway.
func (g *Game) checkAIFactionDefeats() {
	if len(g.ais) == 0 {
		return
	}
	if g.aiDefeatedAnnounced == nil {
		g.aiDefeatedAnnounced = make(map[int]bool)
	}
	for _, f := range g.ais {
		if g.aiDefeatedAnnounced[f.owner] || !g.factionDefeated(f.owner) {
			continue
		}
		g.aiDefeatedAnnounced[f.owner] = true
		victor := g.creditFactionDefeat(f.owner)
		g.queueAdvisorTip(advisor.Tip{Kind: advisor.KindFactionDefeated, DefeatedOwner: f.owner, VictorOwner: victor})
	}
}

// recordLastAttacker updates lastAttackerOwner for every entry in
// defenderOwners (one per real kill this tick -- see soldier.TickResult/
// sentry.TickResult's own KillOwners) to credit attackerOwner. Called
// once per faction's own combat tick (the player's in Update, each AI's
// in tickAIFaction), for both its soldiers' and its sentries' results.
func (g *Game) recordLastAttacker(attackerOwner int, defenderOwners []int) {
	if len(defenderOwners) == 0 {
		return
	}
	if g.lastAttackerOwner == nil {
		g.lastAttackerOwner = make(map[int]int)
	}
	for _, defender := range defenderOwners {
		g.lastAttackerOwner[defender] = attackerOwner
	}
}

// creditFactionDefeat decides who gets credit for defeating owner --
// whoever recordLastAttacker most recently logged a real kill against
// them for, if any, per the user's own explicit request ("измени
// механику кто кого разгромил: разгромил не тот кто ближе, а тот, кто
// нанес последний урон после которого противника не стало"). Falls back
// to nearestSurvivingFactionTo's geography guess only when nothing was
// ever recorded at all -- e.g. razeHopelessFactions auto-defeating a
// faction that simply starved out economically, with no specific final
// blow from anyone to credit.
func (g *Game) creditFactionDefeat(owner int) int {
	if g.lastAttackerOwner != nil {
		if attacker, ok := g.lastAttackerOwner[owner]; ok {
			return attacker
		}
	}
	// f.logi.Warehouse itself is never nil (every faction starts with
	// one) and, though pruneDestroyedBuildings has by now removed it
	// from g.buildings, the struct itself is untouched -- its X/Y still
	// marks where this faction's territory was, the reference point
	// nearestSurvivingFactionTo needs.
	f := g.factionByOwner(owner)
	if f == nil {
		return -1
	}
	return g.nearestSurvivingFactionTo(f.logi.Warehouse.X, f.logi.Warehouse.Y, owner)
}

// nearestSurvivingFactionTo heuristically credits a faction's elimination
// to whichever other still-living faction (the player, Owner 0, included)
// has the nearest warehouse to (x, y) -- typically the just-defeated
// faction's own last warehouse position. Only ever used by
// creditFactionDefeat as a last resort, when no real kill was ever
// recorded against this faction at all (see recordLastAttacker) --
// otherwise a real kill-attribution always wins. Returns -1 if no other
// faction is currently alive (should be unreachable in practice: the
// match isn't over, or checkAIFactionDefeats wouldn't still be running --
// see Update's early return on g.duelResult -- so some other faction must
// still stand).
func (g *Game) nearestSurvivingFactionTo(x, y, excludeOwner int) int {
	best := -1
	bestDistSq := -1
	consider := func(owner, wx, wy int) {
		dx, dy := wx-x, wy-y
		distSq := dx*dx + dy*dy
		if best == -1 || distSq < bestDistSq {
			best, bestDistSq = owner, distSq
		}
	}
	if excludeOwner != 0 && !g.factionDefeated(0) {
		consider(0, g.logi.Warehouse.X, g.logi.Warehouse.Y)
	}
	for _, f := range g.ais {
		if f.owner == excludeOwner || g.factionDefeated(f.owner) {
			continue
		}
		consider(f.owner, f.logi.Warehouse.X, f.logi.Warehouse.Y)
	}
	return best
}

// enclosedGatherWorkerTip reports the actual consequence of a sealed wall,
// rather than merely detecting that the player drew a loop. A lumberjack,
// quarryman or miner is actionable when either the worker or their home is in
// a closed land region: without an open/automatic gate they cannot reliably
// leave to collect resources or return with cargo. Future land gatherers can
// join this list without changing the generic wall-region algorithm.
func (g *Game) enclosedGatherWorkerTip() (advisor.Tip, bool) {
	closed := pathfind.ClosedWallAreas(g.grid, g.buildings)
	if len(closed) == 0 {
		return advisor.Tip{}, false
	}
	count := 0
	var example *building.Building
	inside := func(home *building.Building, x, y int) bool {
		if home == nil {
			return false
		}
		if closed[pathfind.Point{X: x, Y: y}] {
			return true
		}
		access := home.AccessPoint()
		return closed[pathfind.Point{X: access.X, Y: access.Y}]
	}
	record := func(home *building.Building, x, y int) {
		if !inside(home, x, y) {
			return
		}
		count++
		if example == nil {
			example = home
		}
	}
	for _, j := range g.jacks.Lumberjacks {
		record(j.HomeBuilding(), j.X, j.Y)
	}
	for _, q := range g.quarry.Quarrymen {
		record(q.HomeBuilding(), q.X, q.Y)
	}
	for _, m := range g.miners.Miners {
		record(m.HomeBuilding(), m.X, m.Y)
	}
	if count == 0 {
		return advisor.Tip{}, false
	}
	return advisor.Tip{Kind: advisor.KindGatherWorkerEnclosed, Building: example, Count: count}, true
}

// queueAdvisorTip keeps one visible/queued tip of each kind and respects the
// acknowledgement cooldown. Placement calls it immediately for a newly
// unaffordable construction site; periodic evaluation uses the same path.
//
// KindFactionDefeated skips all of that: it's a discrete, one-time event
// (see its own doc comment), not an ongoing situation, so the usual
// per-Kind dedup/cooldown -- built for "only one instance of this
// situation ever matters at a time" -- would wrongly swallow every
// faction's defeat notification after the first one shown/queued/on
// cooldown. checkAIFactionDefeats already guarantees each owner is only
// ever queued once (aiDefeatedAnnounced), so there's nothing left for
// this function to protect against for that Kind.
func (g *Game) queueAdvisorTip(tip advisor.Tip) {
	if tip.Kind != advisor.KindFactionDefeated {
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
	exampleName := ""
	if tip.Building != nil {
		// A real gap found from an actual playtest report ("почему нет
		// названия здания, что за например" / "а что за цифры? есть же
		// кнопка перехода к зданию, у здания есть название, выводи
		// название и все!"): every one of these messages used to show
		// bare coordinates instead of the building's name, and the
		// Advisor panel already has a "go to building" button
		// (focusAdvisorBuilding) that jumps the camera straight to
		// tip.Building itself -- coordinates in the text were pure
		// clutter on top of a real navigation feature. t.BuildingName is
		// the same localized lookup every other panel in the game
		// already uses (inspector titles, route labels, ...).
		exampleName = t.BuildingName[tip.Building.Kind]
	}
	switch tip.Kind {
	case advisor.KindFoodRunningOut:
		return fmt.Sprintf(t.AdvisorTipFoodRunningOut, tip.TicksLeft)
	case advisor.KindIdleBuilding:
		return fmt.Sprintf(t.AdvisorTipIdleBuilding, tip.Count, exampleName)
	case advisor.KindDisconnectedBuilding:
		return fmt.Sprintf(t.AdvisorTipDisconnectedBuilding, tip.Count, exampleName)
	case advisor.KindServeCountLow:
		return fmt.Sprintf(t.AdvisorTipServeCountLow, tip.Current, tip.Recommended)
	case advisor.KindServeCountHigh:
		return fmt.Sprintf(t.AdvisorTipServeCountHigh, tip.Current, tip.Recommended)
	case advisor.KindGatherWorkerStuck:
		return fmt.Sprintf(t.AdvisorTipGatherWorkerStuck, tip.Count, exampleName)
	case advisor.KindGatherWorkerEnclosed:
		return fmt.Sprintf(t.AdvisorTipGatherWorkerEnclosed, tip.Count, exampleName)
	case advisor.KindConstructionMaterialsMissing:
		return fmt.Sprintf(t.AdvisorTipConstructionMaterialsMissing, t.ResourceName[tip.Resource], tip.Missing, exampleName)
	case advisor.KindFactionDefeated:
		defeated := t.FactionColorAccusative[tip.DefeatedOwner]
		if victor, ok := t.FactionColorNominative[tip.VictorOwner]; ok {
			return fmt.Sprintf(t.FactionDefeatedByFmt, victor, defeated)
		}
		// No attribution (nearestSurvivingFactionTo found nobody, should
		// be unreachable in practice) -- still worth telling the player
		// someone's gone, just without crediting anyone specific.
		return fmt.Sprintf(t.FactionDefeatedFmt, t.FactionColorNominative[tip.DefeatedOwner])
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

// resetToNewGame discards the running town and replaces it with a fresh
// free-map one, skipping the mode-select screen entirely -- a thin
// wrapper around startFreeMapGame (title.go) kept only because tests
// reach for it directly as "just start over" without simulating menu
// clicks. The Esc pause menu's own "New game" button no longer calls
// this: it goes through enterModeSelect (title.go) so the player can
// also choose "1×1 против ИИ" instead of always resetting to a free map.
func (g *Game) resetToNewGame() {
	g.startFreeMapGame()
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
	state := save.GameState{
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
		SentryMealSeed:     g.sentries.MealSeed(),
		CameraX:            g.camera.X,
		CameraY:            g.camera.Y,
		CameraZoom:         g.camera.Scale,
	}
	if len(g.ais) > 0 {
		state.IsDuelGame = true
		for _, f := range g.ais {
			state.AIFactions = append(state.AIFactions, save.AIFactionSave{
				Owner:              f.owner,
				Difficulty:         int(f.brain.difficulty),
				Stockpile:          *f.stock,
				Population:         *f.pop,
				BrainCooldown:      f.brain.cooldown,
				BrainBuildIndex:    f.brain.buildIndex,
				BrainBuildAttempts: f.brain.buildAttempts,
			})
		}
	}
	return state
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
	// findWarehouseOwnedBy, not findWarehouse: a real bug found from an
	// actual playtest report ("создай слуг от юзера, они начинают ходить
	// по кругу карты без перерыва") -- in an ordinary single-player save
	// every building (including the one warehouse) is Owner 0 by
	// construction, so this made no difference there, but in a duel save
	// where the player's OWN Warehouse had been destroyed before saving,
	// the owner-blind findWarehouse happily returned an AI faction's
	// Warehouse instead (whichever came first in the array) -- silently
	// anchoring the player's own logistics controller to an enemy
	// building on the far side of the map. A newly hired serf spawned
	// there, found no reachable player-owned job anywhere near it, and
	// wandered indefinitely. A player with no Warehouse of their own left
	// has no economy to load into regardless of what other buildings
	// survive -- errNoWarehouseInSave below is exactly the right outcome
	// for that, not a silent substitution.
	warehouse := findWarehouseOwnedBy(buildings, 0)
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
	// A loaded game is never mid-result -- see duelResult's own doc
	// comment on why a save can in practice never actually happen while
	// it's set (Update's early return freezes the simulation loop
	// entirely once a match is decided), but resetting it explicitly
	// here costs nothing and documents the invariant.
	g.duelResult = duelResultNone

	// g.ais is reconstructed before any unit restoration below --
	// restoreUnits dispatches every Owner>0 UnitState into its matching
	// faction's own controllers, which must already exist for that to do
	// anything. See GameState.IsDuelGame's doc comment: false means this
	// isn't a duel save at all, the only thing every save from before
	// duel-mode saving existed can mean.
	g.ais = nil
	if state.IsDuelGame {
		factions := state.AIFactions
		if len(factions) == 0 {
			// A save written before AIFactions existed (at most one
			// opponent, Owner 1, in the legacy flat fields) -- see
			// save.GameState's own doc comment on this fallback.
			factions = []save.AIFactionSave{{
				Owner:              1,
				Difficulty:         state.AIDifficulty,
				Stockpile:          state.AIStockpile,
				Population:         state.AIPopulation,
				BrainCooldown:      state.AIBrainCooldown,
				BrainBuildIndex:    state.AIBrainBuildIndex,
				BrainBuildAttempts: state.AIBrainBuildAttempts,
			}}
		}
		for _, fs := range factions {
			aiWarehouse := findWarehouseOwnedBy(buildings, fs.Owner)
			if aiWarehouse == nil {
				// A real playtest bug found from actual saves ("слот 4:
				// мои 6 лучников на базе красного... он не защищается",
				// "слот 5: у противника остановилось развитие, новые
				// юниты не создаются"): a faction whose only Warehouse
				// was already destroyed used to be dropped entirely on
				// reload -- no brain, no soldiers, no sentries -- even
				// when other real buildings of theirs (see
				// nearestRealBuildingOwnedBy, the same "straggler" case
				// aiConsiderAttack already knows to keep hunting) were
				// still standing. Worse, every one of its saved soldier/
				// sentry units was silently discarded too (see
				// restoreUnits' owner->faction lookup, which requires a
				// live faction to dispatch into) -- so a straggler that
				// still had real defenders when the save happened came
				// back from a reload with none. If truly nothing real is
				// left, factionDefeated already agrees this faction is
				// done, and it's still correctly skipped below exactly
				// as before.
				anchor := g.nearestRealBuildingOwnedBy(fs.Owner, 0, 0)
				if anchor == nil {
					// Nothing "real" left (Warehouse, Barracks, ...), but a
					// faction can still have its own defensive perimeter
					// standing (aiBuildDefenses' walls/gates) with nothing
					// else -- see nearestAnyBuildingOwnedBy's own doc
					// comment for the real bug this recovers from. Only
					// truly nothing at all (not even a wall) still means
					// "skip", same as before.
					anchor = g.nearestAnyBuildingOwnedBy(fs.Owner, 0, 0)
				}
				if anchor == nil {
					continue
				}
				// A dummy Warehouse-shaped anchor, deliberately never
				// added to g.buildings -- invisible/unselectable, and
				// doesn't count toward factionDefeated -- just enough to
				// satisfy logistics.Controller's hard non-nil Warehouse
				// requirement safely. ForceRemoveWarehouse right after
				// construction keeps it from ever being offered as an
				// actual delivery destination (see that method's own doc
				// comment): this faction has no real economy left, only
				// its restored soldiers/sentries can still function.
				stub := &building.Building{Kind: building.Warehouse, Owner: fs.Owner, X: anchor.X, Y: anchor.Y, HP: building.MaxHP, ConstructionStage: building.ConstructionNone}
				f := restoreFaction(fs.Owner, stub, aiDifficulty(fs.Difficulty), fs.Stockpile, fs.Population, fs.BrainCooldown, fs.BrainBuildIndex, fs.BrainBuildAttempts)
				f.logi.ForceRemoveWarehouse(stub)
				g.ais = append(g.ais, f)
				continue
			}
			g.ais = append(g.ais, restoreFaction(fs.Owner, aiWarehouse, aiDifficulty(fs.Difficulty), fs.Stockpile, fs.Population, fs.BrainCooldown, fs.BrainBuildIndex, fs.BrainBuildAttempts))
		}
	}

	// Jobs are rebuilt from the saved positions. The roster itself is
	// restored, so hiring extra serfs or saving a worker halfway to the
	// Tavern no longer silently resets the town. b.Owner == 0 below --
	// a real bug found while adding duel-mode saving: this used to add
	// ANY other operational warehouse regardless of owner, which would
	// have handed the player's own logistics controller a route into
	// the AI's warehouse too.
	g.logi = logistics.NewController(warehouse, 0)
	for _, b := range buildings {
		if b.Owner == 0 && b.IsOperationalWarehouse() && b != warehouse {
			g.logi.AddWarehouse(b)
		}
	}
	g.vills = villagers.NewController()
	g.jacks = lumberjack.NewController()
	g.fishers = fishing.NewController()
	g.quarry = quarry.NewController()
	g.builders = builder.NewController()
	g.miners = miner.NewController()
	g.sentries = sentry.NewController()
	g.soldiers = soldier.NewController()
	g.deathEffects = nil
	if len(state.Units) == 0 {
		// Saves from before unit persistence did not contain a roster.
		// Keep those saves playable with the old sensible defaults.
		g.logi = logistics.NewController(warehouse, startingSerfs)
		for _, b := range buildings {
			if b.Owner == 0 && b.IsOperationalWarehouse() && b != warehouse {
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
	if state.SentryMealSeed != 0 {
		g.sentries.SetMealSeed(state.SentryMealSeed)
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

// findWarehouseOwnedBy is findWarehouse restricted to one faction's
// warehouse -- see buildingConnected's doc comment for why this matters
// once g.buildings can hold more than one faction's buildings.
func findWarehouseOwnedBy(buildings []*building.Building, owner int) *building.Building {
	for _, b := range buildings {
		if b.IsOperationalWarehouse() && b.Owner == owner {
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

// serializeUnits returns the player's own units, plus every AI faction's
// (see UnitState.Owner) in g.ais -- see buildSaveState's doc comment on
// the real gap this closes: a duel save used to be refused entirely
// rather than lose the AI's roster silently.
func (g *Game) serializeUnits() []save.UnitState {
	units := serializeUnitsFor(0, g.buildings, g.logi, g.vills, g.jacks, g.fishers, g.quarry, g.builders, g.miners, g.sentries, g.soldiers)
	for _, f := range g.ais {
		units = append(units, serializeUnitsFor(f.owner, g.buildings, f.logi, f.vills, f.jacks, f.fishers, f.quarry, f.builders, f.miners, f.sentries, f.soldiers)...)
	}
	return units
}

// serializeUnitsFor is serializeUnits' body, parameterized over which
// faction's controllers to read from -- owner is stamped onto every
// entry (see UnitState.Owner) so restoreUnits knows which faction's
// controllers to restore each one into. buildings is always the full,
// shared g.buildings (both factions' buildings share one list, same as
// everywhere else in this codebase), since HomeIndex/TargetIndex point
// into it regardless of which faction the unit itself belongs to.
func serializeUnitsFor(
	owner int,
	buildings []*building.Building,
	logi *logistics.Controller,
	vills *villagers.Controller,
	jacks *lumberjack.Controller,
	fishers *fishing.Controller,
	quarryC *quarry.Controller,
	builders *builder.Controller,
	miners *miner.Controller,
	sentries *sentry.Controller,
	soldiers *soldier.Controller,
) []save.UnitState {
	units := make([]save.UnitState, 0, len(logi.Serfs)+len(vills.Villagers)+len(jacks.Lumberjacks)+len(fishers.Fishermen)+len(quarryC.Quarrymen)+len(builders.Builders)+len(miners.Miners)+len(sentries.Sentries))
	for _, s := range logi.Serfs {
		units = append(units, save.UnitState{
			Kind:        save.UnitSerf,
			X:           s.X,
			Y:           s.Y,
			HomeIndex:   -1,
			HungerTicks: s.HungerTicks(),
			Starving:    s.Starving,
			Dismissing:  s.Dismissing(),
			Owner:       owner,
		})
	}
	for _, v := range vills.Villagers {
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
		case villagers.Weaponsmith:
			kind = save.UnitWeaponsmith
		default:
			kind = save.UnitFarmer
		}
		units = append(units, save.UnitState{
			Kind:        kind,
			X:           v.X,
			Y:           v.Y,
			HomeIndex:   indexOfBuilding(buildings, v.HomeBuilding()),
			HungerTicks: v.HungerTicks(),
			Starving:    v.Starving,
			State:       int(v.State()),
			Meal:        v.Meal(),
			Owner:       owner,
		})
	}
	for _, j := range jacks.Lumberjacks {
		_, cargoAmount := j.Cargo()
		targetIndex := indexOfBuilding(buildings, j.TargetTree())
		units = append(units, save.UnitState{
			Kind:        save.UnitLumberjack,
			X:           j.X,
			Y:           j.Y,
			HomeIndex:   indexOfBuilding(buildings, j.HomeBuilding()),
			HungerTicks: j.HungerTicks(),
			Starving:    j.Starving,
			State:       int(j.State()),
			TargetIndex: targetIndex,
			WorkTicks:   j.WorkTicks(),
			Cargo:       resource.Log,
			CargoAmount: cargoAmount,
			Meal:        j.Meal(),
			Owner:       owner,
		})
	}
	for _, f := range fishers.Fishermen {
		_, cargoAmount := f.Cargo()
		targetIndex := indexOfBuilding(buildings, f.TargetFish())
		units = append(units, save.UnitState{
			Kind:        save.UnitFisherman,
			X:           f.X,
			Y:           f.Y,
			HomeIndex:   indexOfBuilding(buildings, f.HomeBuilding()),
			HungerTicks: f.HungerTicks(),
			Starving:    f.Starving,
			State:       int(f.State()),
			TargetIndex: targetIndex,
			WorkTicks:   f.WorkTicks(),
			Cargo:       resource.Fish,
			CargoAmount: cargoAmount,
			Meal:        f.Meal(),
			Owner:       owner,
		})
	}
	for _, q := range quarryC.Quarrymen {
		targetIndex := indexOfBuilding(buildings, q.TargetDeposit())
		units = append(units, save.UnitState{
			Kind:        save.UnitQuarryman,
			X:           q.X,
			Y:           q.Y,
			HomeIndex:   indexOfBuilding(buildings, q.HomeBuilding()),
			HungerTicks: q.HungerTicks(),
			Starving:    q.Starving,
			State:       int(q.State()),
			TargetIndex: targetIndex,
			WorkTicks:   q.WorkTicks(),
			Cargo:       resource.StoneBlock,
			CargoAmount: q.RawCargo(),
			Meal:        q.Meal(),
			Owner:       owner,
		})
	}
	for _, bl := range builders.Builders {
		targetIndex := indexOfBuilding(buildings, bl.TargetSite())
		units = append(units, save.UnitState{
			Kind:        save.UnitBuilder,
			X:           bl.X,
			Y:           bl.Y,
			HomeIndex:   indexOfBuilding(buildings, bl.Warehouse),
			HungerTicks: bl.HungerTicks(),
			Starving:    bl.Starving,
			State:       int(bl.State()),
			TargetIndex: targetIndex,
			WorkTicks:   bl.WorkTicks(),
			Meal:        bl.Meal(),
			Owner:       owner,
		})
	}
	for _, mn := range miners.Miners {
		cargoResource, cargoAmount := mn.Cargo()
		targetIndex := indexOfBuilding(buildings, mn.TargetDeposit())
		quotaIndex, quotaProgress := mn.QuotaProgress()
		units = append(units, save.UnitState{
			Kind:          save.UnitMiner,
			X:             mn.X,
			Y:             mn.Y,
			HomeIndex:     indexOfBuilding(buildings, mn.HomeBuilding()),
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
			Owner:         owner,
		})
	}
	for _, s := range sentries.Sentries {
		units = append(units, save.UnitState{
			Kind:        save.UnitSentry,
			X:           s.X,
			Y:           s.Y,
			HomeIndex:   indexOfBuilding(buildings, s.HomeBuilding()),
			HungerTicks: s.HungerTicks(),
			Starving:    s.Starving,
			State:       int(s.State()),
			Meal:        s.Meal(),
			Owner:       owner,
		})
	}
	for _, sd := range soldiers.Soldiers {
		kind := save.UnitArcher
		if sd.Profession == soldier.Swordsman {
			kind = save.UnitSwordsman
		}
		units = append(units, save.UnitState{
			Kind:        kind,
			X:           sd.X,
			Y:           sd.Y,
			HomeIndex:   -1,
			HungerTicks: sd.HungerTicks(),
			HP:          sd.HP,
			Owner:       owner,
		})
	}
	return units
}

// restoreUnits dispatches each state to the right FACTION's controllers
// by state.Owner -- Owner: 0 always means the player's own g.logi/
// g.vills/... (unchanged from before duel saves existed); any other
// Owner means the matching faction in g.ais, which must already exist
// (see the AI-faction reconstruction loop, run before this from
// loadGame) for that state to do anything.
func (g *Game) restoreUnits(states []save.UnitState, buildings []*building.Building) {
	for _, state := range states {
		if state.Owner != 0 {
			f := g.factionByOwner(state.Owner)
			if f == nil {
				continue
			}
			restoreUnitStateInto(state, g.grid, buildings, f.logi, f.vills, f.jacks, f.fishers, f.quarry, f.builders, f.miners, f.sentries, f.soldiers)
			continue
		}
		restoreUnitStateInto(state, g.grid, buildings, g.logi, g.vills, g.jacks, g.fishers, g.quarry, g.builders, g.miners, g.sentries, g.soldiers)
	}
}

// restoreUnitStateInto is restoreUnits' original per-unit body,
// parameterized over which faction's controllers to restore into --
// mirrors serializeUnitsFor's identical split.
func restoreUnitStateInto(
	state save.UnitState,
	grid *world.Grid,
	buildings []*building.Building,
	logi *logistics.Controller,
	vills *villagers.Controller,
	jacks *lumberjack.Controller,
	fishers *fishing.Controller,
	quarryC *quarry.Controller,
	builders *builder.Controller,
	miners *miner.Controller,
	sentries *sentry.Controller,
	soldiers *soldier.Controller,
) {
	switch state.Kind {
	case save.UnitSerf:
		logi.RestoreSerf(state.X, state.Y, state.HungerTicks, state.Starving, state.Dismissing)
	case save.UnitFarmer, save.UnitBaker, save.UnitWinemaker, save.UnitSwineherd, save.UnitButcher, save.UnitCarpenter, save.UnitSmelter, save.UnitWeaponsmith:
		if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) {
			return
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
		case save.UnitWeaponsmith:
			profession = villagers.Weaponsmith
		default:
			profession = villagers.Farmer
		}
		if (profession == villagers.Farmer && home.Kind != building.Farm) ||
			(profession == villagers.Baker && home.Kind != building.Bakery) ||
			(profession == villagers.Winemaker && home.Kind != building.Winery) ||
			(profession == villagers.Swineherd && home.Kind != building.PigFarm) ||
			(profession == villagers.Butcher && home.Kind != building.MeatWorkshop) ||
			(profession == villagers.Carpenter && home.Kind != building.CarpentryWorkshop) ||
			(profession == villagers.Smelter && home.Kind != building.Smeltery) ||
			(profession == villagers.Weaponsmith && home.Kind != building.Armory) {
			return
		}
		vills.RestoreVillager(profession, home, state.X, state.Y, state.HungerTicks, state.Starving, villagers.State(state.State), buildings, state.Meal)
	case save.UnitLumberjack:
		if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.LumberjackHut {
			return
		}
		var target *building.Building
		if state.TargetIndex >= 0 && state.TargetIndex < len(buildings) && buildings[state.TargetIndex].Kind == building.Tree {
			target = buildings[state.TargetIndex]
		}
		jacks.Restore(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, lumberjack.State(state.State), target, state.WorkTicks, state.CargoAmount, grid, buildings, buildings, state.Meal)
	case save.UnitFisherman:
		if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.FisherHut {
			return
		}
		var target *building.Building
		if state.TargetIndex >= 0 && state.TargetIndex < len(buildings) && buildings[state.TargetIndex].Kind == building.Fish {
			target = buildings[state.TargetIndex]
		}
		fishers.Restore(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, fishing.State(state.State), target, state.WorkTicks, state.CargoAmount, grid, buildings, state.Meal)
	case save.UnitQuarryman:
		if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.QuarryHut {
			return
		}
		var target *building.Building
		if state.TargetIndex >= 0 && state.TargetIndex < len(buildings) && buildings[state.TargetIndex].Kind == building.StoneDeposit {
			target = buildings[state.TargetIndex]
		}
		quarryC.Restore(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, quarry.State(state.State), target, state.WorkTicks, state.CargoAmount, grid, buildings, buildings, state.Meal)
	case save.UnitBuilder:
		if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.Warehouse {
			return
		}
		var target *building.Building
		if state.TargetIndex >= 0 && state.TargetIndex < len(buildings) {
			candidate := buildings[state.TargetIndex]
			// Either a fresh construction site, or a damaged, already
			// finished building the builder was walking to or working
			// on repairing -- see builder.StateRepairing.
			if candidate.ConstructionStage != building.ConstructionNone ||
				(candidate.HP > 0 && candidate.HP < building.MaxHP) {
				target = candidate
			}
		}
		builders.Restore(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, builder.State(state.State), target, state.WorkTicks, grid, buildings, buildings, state.Meal)
	case save.UnitSentry:
		if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.WatchTower {
			return
		}
		sentries.RestoreSentry(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, sentry.State(state.State), buildings, state.Meal)
	case save.UnitMiner:
		if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) || buildings[state.HomeIndex].Kind != building.MinerHut {
			return
		}
		var target *building.Building
		if state.TargetIndex >= 0 && state.TargetIndex < len(buildings) {
			switch buildings[state.TargetIndex].Kind {
			case building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
				target = buildings[state.TargetIndex]
			}
		}
		miners.Restore(buildings[state.HomeIndex], state.X, state.Y, state.HungerTicks, state.Starving, miner.State(state.State), target, state.WorkTicks, state.CargoAmount, state.Cargo, state.QuotaIndex, state.QuotaProgress, grid, buildings, buildings, state.Meal)
	case save.UnitArcher, save.UnitSwordsman:
		profession := soldier.Archer
		if state.Kind == save.UnitSwordsman {
			profession = soldier.Swordsman
		}
		// .Owner = state.Owner: see aiHireSoldiers' doc comment (cmd/game/
		// ai_brain.go) for the real playtest bug this closes -- Restore
		// never set it, leaving every restored soldier's Owner at 0
		// regardless of which faction it actually belongs to.
		soldiers.Restore(profession, state.X, state.Y, state.HungerTicks, state.HP).Owner = state.Owner
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
	return seedTreesWithCount(grid, buildings, grid.Width*grid.Height/100)
}

// seedTreesWithCount is seedTrees with an explicit tile budget instead of
// the single-player one-percent-of-this-grid's-own-area rule -- see
// newDuelGame's use of it: the duel map's own grid is the WHOLE
// 4-quadrant map, much bigger than a single-player map, so reusing
// seedTrees' own area-based formula there scattered several times as many
// trees as intended (a real playtest report, "деревьев очень много
// спавнится на карте... что-то явно сломалось в генерации" -- confirmed:
// seedTrees alone already scales with the whole map, and seedThickets
// piled dense clusters on top of that, scaling the same way). The caller
// works out the right budget for its own map (see duelTreeTilesPerQuadrant).
func seedTreesWithCount(grid *world.Grid, buildings []*building.Building, maxTrees int) []*building.Building {
	if maxTrees <= 0 {
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

// hoveredOrSelectedWatchTower returns the finished WatchTower currently
// under the cursor (tx, ty) or currently selected, for the range-circle
// overlay -- see its call site in Draw. nil covers "neither", which is
// the common case and must not draw anything.
func (g *Game) hoveredOrSelectedWatchTower(tx, ty int) *building.Building {
	if g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil &&
		g.selection.Building.Kind == building.WatchTower && g.selection.Building.ConstructionStage == building.ConstructionNone {
		return g.selection.Building
	}
	// g.ownedBuildings(0): a hover shouldn't preview the AI's own
	// WatchTower range circle either, same faction-separation principle
	// as buildingSelectionAt/selectionAt's own real bug fix.
	for _, b := range g.ownedBuildings(0) {
		if b.Kind == building.WatchTower && b.ConstructionStage == building.ConstructionNone && b.X == tx && b.Y == ty {
			return b
		}
	}
	return nil
}

// pruneDestroyedBuildings removes any building whose HP has been brought
// to 0 through combat. Applies in every mode, not just duels: previously
// nothing in this game ever actually removed a building at 0 HP (only
// the repairable-damage path existed), which is otherwise harmless in
// the ordinary single-player game but would have made the "все здания
// уничтожены" win condition unsatisfiable -- a wrecked building would
// just sit there forever, still "existing". Every player/AI-constructed
// building sets HP explicitly to building.MaxHP (construction sites via
// NewConstructionSite, the duel map's starting buildings, roads -- see
// duel.go's comment on that), so this can never misfire on one of those.
//
// Road alone is exempted regardless of HP -- it is never a combat
// target at all (opposingBuildingsFor/ownedBuildings always exclude
// Kind==Road), so its HP never legitimately reaches 0 through play; the
// exemption is only a defensive guard against a save/legacy tile with an
// unset HP field. StoneWall and Gate are NOT exempted, despite
// factionDefeated/the win condition also excluding them from counting
// as "real" buildings -- that is a separate concern (a pile of wall
// rubble shouldn't keep a faction "alive"), not a reason to leave an
// actually-destroyed wall segment sitting on the map forever. A real
// bug found from an actual playtest report ("это чужая стена с чужими
// воротами, я должен иметь возможность её уничтожить"): before this,
// EVERY StoneWall/Gate was covered by the same blanket exemption as
// Road, so a wall combat had already reduced to 0 HP simply never
// disappeared -- permanently blocking movement through that tile (see
// pathfind's occupancy checks, which never look at HP either) while
// also becoming permanently unattackable (opposingBuildingAt skips any
// HP<=0 candidate) -- an unbreachable, unremovable ghost exactly where
// the player had just finished breaking through.
//
// Natural resource nodes (Tree/Fish/StoneDeposit/CoalDeposit/
// GoldOreDeposit/IronOreDeposit) are also represented as *building.
// Building for convenience, but were NEVER given a real HP value --
// they're removed by their own dedicated mechanisms (cutTree/catchFish/
// removeDeposit), not combat. A real bug found by simulating the duel
// map: without this exemption, every tree/fish/deposit on the whole map
// (615 of 619 starting buildings) had HP==0 and was wiped out on the
// very first tick this function ever ran.

// decayDamagedBuildings applies the user's own explicit passive-damage
// rule to every finished building sitting below full health with
// nobody actively repairing it: "если здание не восстанавливать, ХП
// уменьшается по 1% за 10 тиков" (combat.DecayIntervalTicks/
// DecayAmount). Never brings a building below combat.DecayFloor --
// confirmed with the user directly ("может ли здание само развалиться
// до 0 без боя" -> нет, есть пол): only real combat damage can finish
// the job. DecayTicks resets to 0 whenever a building isn't currently
// eligible (full health, destroyed, still under construction, or a
// builder is actively StateRepairing it right now -- see
// buildingsUnderActiveRepair), so decay never "banks" progress across a
// pause and always restarts cleanly once it resumes.
func (g *Game) decayDamagedBuildings() {
	repairing := g.buildingsUnderActiveRepair()
	for _, b := range g.buildings {
		if b == nil || b.ConstructionStage != building.ConstructionNone {
			continue
		}
		if b.HP <= 0 || b.HP >= building.MaxHP || repairing[b] {
			b.DecayTicks = 0
			continue
		}
		if b.HP <= combat.DecayFloor {
			continue
		}
		b.DecayTicks++
		if b.DecayTicks < combat.DecayIntervalTicks {
			continue
		}
		b.DecayTicks = 0
		b.HP -= combat.DecayAmount
		if b.HP < combat.DecayFloor {
			b.HP = combat.DecayFloor
		}
	}
}

// buildingsUnderActiveRepair collects every building any faction's
// builder is currently, actively repairing (builder.StateRepairing) --
// see decayDamagedBuildings, which pauses passive decay for exactly
// these. Deliberately narrower than "a builder is walking toward it"
// (StateToSite): nothing has actually started yet at that point, so
// decay keeps running until real work does.
func (g *Game) buildingsUnderActiveRepair() map[*building.Building]bool {
	active := make(map[*building.Building]bool)
	mark := func(bc *builder.Controller) {
		for _, b := range bc.Builders {
			if b.State() == builder.StateRepairing {
				if t := b.TargetSite(); t != nil {
					active[t] = true
				}
			}
		}
	}
	mark(g.builders)
	for _, f := range g.ais {
		mark(f.builders)
	}
	return active
}

// razeHopelessFactions auto-defeats an AI faction that has fallen into a
// state it can structurally never recover from on its own: zero living
// units of any profession AND less gold banked than a single hire costs.
// Every hire (aiHireServes/aiHireBuilder/aiStaffBuildings) costs exactly
// unitHireCost gold, and with zero population nothing this faction owns
// can ever earn another unit of ANY resource -- every gathering/
// production building sits idle without an assigned worker, and there is
// nobody left to assign one. Once both conditions hold at once, they
// hold forever; nothing in a later tick can change either number back.
//
// Per a real playtest report (a duel-mode save where two crippled AI
// factions sat for the rest of the match with a handful of harmless
// leftover buildings nobody had bothered finishing off, silently
// outscoring the actual leader on lifetime Kills alone): "если уже без
// шансов - все здания автоматически уничтожаются и он объявляется
// побежденным". Setting HP to 0 here (matching combat-destroyed
// buildings) and letting pruneDestroyedBuildings' own removal run right
// after reuses its existing warehouse-re-anchor/logistics-teardown code
// path verbatim, instead of a second, parallel removal path -- and
// leaves checkAIFactionDefeats to notice and announce the elimination
// exactly as it already does for a combat kill.
//
// StoneWall/Gate are razed too, not just "real" buildings -- a
// follow-up to the same report, once the player found exactly this
// leftover scenario in play ("у стен должен быть хозяин... после того
// как противник уничтожен - его стены также уничтожаются автоматически
// если у него нет возможности восстановиться"): a hopeless faction's
// own fortifications are exactly as abandoned as everything else it
// owned, and leaving them standing was what originally let a wall end
// up belonging to a faction with no g.ais entry at all (see
// nearestAnyBuildingOwnedBy's own doc comment on the reload-side half of
// that same bug). Road is still exempt -- it stays shared, neutral
// infrastructure regardless of whose territory it was originally built
// in.
func (g *Game) razeHopelessFactions() {
	for _, f := range g.ais {
		if g.factionDefeated(f.owner) {
			continue // already counted defeated, nothing left to raze
		}
		if g.factionUnitCount(f.owner) != 0 || f.stock.Amount(resource.Gold) >= unitHireCost {
			continue
		}
		for _, b := range g.buildings {
			if b.Owner != f.owner || isNaturalResourceKind(b.Kind) || b.Kind == building.Road {
				continue
			}
			b.HP = 0
		}
	}
}

func (g *Game) pruneDestroyedBuildings() {
	alive := g.buildings[:0]
	changed := false
	var lostLastWarehouseOwners []int
	for _, b := range g.buildings {
		if b.Kind == building.Road {
			alive = append(alive, b)
			continue
		}
		if isNaturalResourceKind(b.Kind) {
			alive = append(alive, b)
			continue
		}
		if b.HP <= 0 {
			changed = true
			if b.Kind == building.Warehouse {
				// A real playtest bug ("слуги продолжают носить рыбу
				// куда-то" after "красный вроде не осталось склада"):
				// removing a destroyed Warehouse from g.buildings here
				// used to be the only cleanup -- the owning faction's
				// logistics.Controller kept a raw pointer to the same
				// (now off-map) building and happily kept sending serfs
				// to "deliver" to it forever. See
				// logistics.Controller.ForceRemoveWarehouse's own doc
				// comment for the full explanation.
				if logi := g.logiFor(b.Owner); logi != nil {
					logi.ForceRemoveWarehouse(b)
					if builders := g.buildersFor(b.Owner); builders != nil {
						builders.RemoveWarehouse(b, logi.Warehouse)
					}
					// CancelAllJobs re-anchors every serf already mid-haul
					// (crediting any cargo already in hand to stock,
					// exactly as the manual player-delete path already
					// does) instead of leaving them walking toward a
					// building that no longer exists on the map.
					if stock := g.stockFor(b.Owner); stock != nil {
						logi.CancelAllJobs(stock)
					}
					if len(logi.Warehouses) == 0 {
						lostLastWarehouseOwners = append(lostLastWarehouseOwners, b.Owner)
					}
				}
			}
			continue
		}
		alive = append(alive, b)
	}
	g.buildings = alive
	// Re-anchor a faction that just lost its LAST warehouse onto one of
	// its own surviving real buildings, if any -- a real playtest bug
	// found from an actual save ("создаю слугу и он сразу умирает", a
	// whole squad having just wiped out the warehouse and everyone
	// standing at it): Hire() always spawns a new unit at
	// c.Warehouse.X/Y, and CancelAllJobs above just re-anchored every
	// mid-haul serf there too -- left pointing at the destroyed
	// warehouse's own tile, every new hire walked straight back into
	// whatever had just destroyed that warehouse, dying almost
	// instantly. Done only now that g.buildings is fully updated, so the
	// anchor search (nearestRealBuildingOwnedBy) never picks a building
	// that dies later in this very same pass.
	for _, owner := range lostLastWarehouseOwners {
		logi := g.logiFor(owner)
		if logi == nil || logi.Warehouse == nil {
			continue
		}
		if anchor := g.nearestRealBuildingOwnedBy(owner, logi.Warehouse.X, logi.Warehouse.Y); anchor != nil {
			logi.Warehouse = anchor
		}
	}
	if changed {
		g.invalidateConnectionCache()
		g.clearMissingUnitSelection()
	}
}

// queueStarvationDeathEffects records every unit that its own controller will
// remove on this exact simulation tick. All current civilian controllers
// increment hunger first and call hunger.Dead immediately afterwards, so this
// reads the last real position without altering their lifecycle or events.
func (g *Game) queueStarvationDeathEffects() {
	diesThisTick := hunger.MaxTicks - 1
	for _, unit := range g.logi.Serfs {
		if unit.HungerTicks() >= diesThisTick {
			g.addDeathEffect(unit.X, unit.Y)
		}
	}
	for _, unit := range g.vills.Villagers {
		if unit.HungerTicks() >= diesThisTick {
			g.addDeathEffect(unit.X, unit.Y)
		}
	}
	for _, unit := range g.jacks.Lumberjacks {
		if unit.HungerTicks() >= diesThisTick {
			g.addDeathEffect(unit.X, unit.Y)
		}
	}
	for _, unit := range g.fishers.Fishermen {
		if unit.HungerTicks() >= diesThisTick {
			g.addDeathEffect(unit.X, unit.Y)
		}
	}
	for _, unit := range g.quarry.Quarrymen {
		if unit.HungerTicks() >= diesThisTick {
			g.addDeathEffect(unit.X, unit.Y)
		}
	}
	for _, unit := range g.builders.Builders {
		if unit.HungerTicks() >= diesThisTick {
			g.addDeathEffect(unit.X, unit.Y)
		}
	}
	for _, unit := range g.miners.Miners {
		if unit.HungerTicks() >= diesThisTick {
			g.addDeathEffect(unit.X, unit.Y)
		}
	}
	for _, unit := range g.sentries.Sentries {
		if unit.HungerTicks() >= diesThisTick {
			g.addDeathEffect(unit.X, unit.Y)
		}
	}
	for _, unit := range g.soldiers.Soldiers {
		if unit.HungerTicks() >= diesThisTick {
			g.addDeathEffect(unit.X, unit.Y)
		}
	}
}

// addDeathEffect has no game-rule side effects. It exists so starvation,
// enemy kills and any future combat source use one universal visual record.
func (g *Game) addDeathEffect(x, y int) {
	g.deathEffects = append(g.deathEffects, render.NewDeathEffect(x, y))
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

// drawSoldierStackCounts overlays a small count badge over any tile where
// more than one living soldier of this group is standing, per the user's
// explicit request following a real siege ("если несколько боевых
// юнитов находятся в одной клетке, отображай над ними счетчик... чтобы
// визуально было понятно что мне противостоит"): several soldiers
// sharing one tile render exactly on top of each other via
// render.DrawSoldiers, with nothing on screen to tell there's more than
// one -- a whole enemy squad stacked on a single tile looked like a
// single soldier. Dead soldiers (HP <= 0) never count -- see
// soldier.Controller.Tick's own fix for the real bug that let a soldier
// already at 0 HP keep marching as a "zombie" in the first place.
func (g *Game) drawSoldierStackCounts(screen *ebiten.Image, soldiers []*soldier.Soldier) {
	type tile struct{ x, y int }
	counts := make(map[tile]int)
	for _, s := range soldiers {
		if s == nil || !s.Alive() {
			continue
		}
		counts[tile{s.X, s.Y}]++
	}
	tilePixels := g.camera.TilePixels()
	for pos, n := range counts {
		if n <= 1 {
			continue
		}
		sx, sy := g.camera.TileToScreen(pos.x, pos.y)
		label := fmt.Sprintf("%d", n)
		badgeW, badgeH := tilePixels*0.36, tilePixels*0.28
		bx, by := sx+tilePixels-badgeW*0.7, sy-badgeH*0.3
		vector.FillRect(screen, float32(bx), float32(by), float32(badgeW), float32(badgeH), color.RGBA{R: 30, G: 20, B: 20, A: 220}, false)
		ui.DrawText(screen, label, bx+badgeW*0.22, by+badgeH*0.12)
	}
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
	var highlighted map[*building.Building]bool
	if kind, ok := g.hoveredExistingBuildingKind(); ok {
		highlighted = make(map[*building.Building]bool)
		for _, b := range g.ownedBuildings(0) {
			if b.Kind == kind && b.ConstructionStage == building.ConstructionNone {
				highlighted[b] = true
			}
		}
	}
	render.DrawBuildings(screen, g.grid, g.buildings, g.camera, g.unstaffedWorkerBuildings(), disconnected, highlighted)
	render.DrawSerfs(screen, g.logi.Serfs, g.camera, 0)
	render.DrawVillagers(screen, g.vills.Villagers, g.camera, 0)
	render.DrawLumberjacks(screen, g.jacks.Lumberjacks, g.camera, 0)
	render.DrawFishermen(screen, g.fishers.Fishermen, g.camera, 0)
	render.DrawQuarrymen(screen, g.quarry.Quarrymen, g.camera, 0)
	render.DrawBuilders(screen, g.builders.Builders, g.camera, 0)
	render.DrawMiners(screen, g.miners.Miners, g.camera, 0)
	render.DrawSentries(screen, g.sentries.Sentries, g.camera, 0)
	render.DrawSoldiers(screen, g.soldiers.Soldiers, g.camera, 0)
	// A real gap found from an actual duel-mode playtest report: an AI
	// opponent's own entire population was never drawn at all, only its
	// buildings (g.buildings is one shared slice, so those already
	// rendered) -- every serf/villager/soldier of theirs was completely
	// invisible on screen. Passing f.owner marks each with
	// render.DrawOpponentUnitMarker, in that bot's own color (see
	// render.colorForOwner), so the player can tell every faction's
	// people apart -- their own, and each bot's -- once they're actually
	// visible.
	for _, f := range g.ais {
		render.DrawSerfs(screen, f.logi.Serfs, g.camera, f.owner)
		render.DrawVillagers(screen, f.vills.Villagers, g.camera, f.owner)
		render.DrawLumberjacks(screen, f.jacks.Lumberjacks, g.camera, f.owner)
		render.DrawFishermen(screen, f.fishers.Fishermen, g.camera, f.owner)
		render.DrawQuarrymen(screen, f.quarry.Quarrymen, g.camera, f.owner)
		render.DrawBuilders(screen, f.builders.Builders, g.camera, f.owner)
		render.DrawMiners(screen, f.miners.Miners, g.camera, f.owner)
		render.DrawSentries(screen, f.sentries.Sentries, g.camera, f.owner)
		render.DrawSoldiers(screen, f.soldiers.Soldiers, g.camera, f.owner)
	}
	g.drawSoldierStackCounts(screen, g.soldiers.Soldiers)
	for _, f := range g.ais {
		g.drawSoldierStackCounts(screen, f.soldiers.Soldiers)
	}
	render.DrawSentryProjectiles(screen, g.sentries.Sentries, g.camera)
	for _, f := range g.ais {
		render.DrawSentryProjectiles(screen, f.sentries.Sentries, g.camera)
	}
	render.DrawDeathEffects(screen, g.deathEffects, g.camera)
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
		if kind == building.WatchTower {
			render.DrawTowerRange(screen, g.camera, tx, ty)
		}
	} else if tower := g.hoveredOrSelectedWatchTower(tx, ty); tower != nil {
		// Per the user's explicit request, the ring shows only on hover or
		// selection -- never unconditionally for every finished tower at
		// once (that would clutter the map once several exist).
		render.DrawTowerRange(screen, g.camera, tower.X, tower.Y)
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
	ui.DrawBuildPanel(screen, g.layout, g.palette, g.leftTab, g.demolitionMode, g.hireOptions(), g.finishedBuildingCounts(), g.leftScrollBuild, g.leftScrollHire, g.paletteUnlocked(), g.paletteLockReasons())
	trimServesPrompt := ""
	if g.dialog == ui.DialogConfirmTrimServes {
		trimServesPrompt = fmt.Sprintf(i18n.T().TrimServesConfirmPrompt, len(g.logi.Serfs), g.recommendedServeCount())
	}
	var canHire ui.BarracksHireAvailability
	if g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil && g.selection.Building.Kind == building.Barracks {
		canHire = ui.BarracksHireAvailability{
			Sentry:    g.canHireSentry(g.selection.Building),
			Archer:    g.canHireArcher(g.selection.Building),
			Swordsman: g.canHireSwordsman(g.selection.Building),
		}
	}
	var armoryState ui.ArmoryProductionState
	if g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil && g.selection.Building.Kind == building.Armory {
		armory := g.selection.Building
		if item, queued := activeArmoryItem(armory); queued {
			armoryState = ui.ArmoryProductionState{
				Item:     item,
				Queued:   true,
				Progress: armory.ProgressTicks,
				Total:    armoryTicksToProduce,
			}
			switch {
			case g.inactiveWorkerBuildings()[armory]:
				armoryState.WaitingForWorker = true
			case !armoryHasMaterial(armory, item):
				armoryState.WaitingForMaterials = true
			case armory.OutputBuffer[item] >= armory.OutputLimit():
				armoryState.WaitingForOutput = true
			default:
				armoryState.Producing = true
			}
		}
	}
	ui.DrawInspectorPanel(screen, g.layout, g.selection, connected, g.stock, g.pop, g.completedTownBuildingCount(), g.playedFrames, occupants, showPriority, priorityLevel, g.dialog, trimServesPrompt, canHire, armoryState, g.formationLines, g.developmentScore(), g.developmentLeaderboard())
	ui.DrawMinimapPanel(screen, g.layout, g.grid, g.buildings, g.camera)
	ui.DrawResourceTooltip(screen, g.layout)
	ui.DrawBuildLockTooltip(screen, g.layout)

	if g.statusMsg != "" {
		ui.DrawText(screen, g.statusMsg, float64(g.layout.LeftWidth+12), 10)
	}
	if g.advisorVisible != nil {
		ui.DrawAdvisorToast(screen, g.layout, advisorTipText(*g.advisorVisible), g.advisorVisible.Building != nil)
	}
	if g.paused {
		g.drawPauseMenu(screen)
	}
	if g.duelResult != duelResultNone {
		g.drawDuelResult(screen)
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
