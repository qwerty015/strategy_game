// Command game is the entry point for the strategy game.
package main

import (
	"errors"
	"fmt"
	"image"
	"log"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/i18n"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
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

	// Fixed spot for the town's primary Warehouse, chosen to sit on plain
	// grass in world.NewTestGrid (away from the fertile/forest/water/
	// stone patches). A single Road tile just south of it gives the
	// player something to extend from immediately; more warehouses can be
	// placed later and share the same stockpile.
	warehouseX, warehouseY = 18, 10

	savePath = "saves/slot1.json"

	// Named save-panel slots (side panel, settings tab) live in their own
	// files, distinct from the S/L quicksave above, so neither mechanism
	// can collide with or silently overwrite the other.
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

	treeRegrowth []treeRegrowth
	treeSeed     uint32
	fishRegrowth []fishRegrowth
	fishSeed     uint32
	// stoneSeeded mirrors save.GameState.StoneSeeded: true once this world
	// has a stone-deposit region, so loading a save never regenerates one
	// over a legitimately fully-mined town. Always true after NewGame.
	stoneSeeded bool

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
	grid := world.NewTestGrid()
	warehouse := &building.Building{Kind: building.Warehouse, X: warehouseX, Y: warehouseY}
	initialRoad := &building.Building{Kind: building.Road, X: warehouseX, Y: warehouseY + 1}

	buildings := []*building.Building{warehouse, initialRoad}
	// Trees are sparse persistent world objects, scattered across all free
	// dry cells rather than confined to a special forest area.
	buildings = seedTrees(grid, buildings)
	buildings = seedFish(grid, buildings)
	buildings = seedStoneDeposits(grid, buildings, defaultStoneSeed)

	stock := resource.NewStockpile(stockpileCapacity)
	stock.Add(resource.Plank, startingPlanks)
	stock.Add(resource.StoneBlock, startingStone)
	stock.Add(resource.Bread, startingBread)
	stock.Add(resource.Fish, startingFish)
	stock.Add(resource.Sausage, startingSausage)
	stock.Add(resource.Wine, startingWine)

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
		treeSeed:    defaultTreeSeed,
		fishSeed:    defaultFishSeed,
		stoneSeeded: true,
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
		g.handlePaletteSelect()
		g.handleMouse()
		g.handleUnitActions()
		g.handleSaveLoad()
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
				g.removeStoneDeposit(event.Deposit)
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
	return m
}

// hireOptions reports the current headcount, building-based limit, and
// availability for every hireable unit kind, for the left panel's Hire tab.
// Serfs are the only unlimited option; every profession is capped at one
// worker per matching building (Spawn/HasHome already enforce this
// one-to-one rule -- this just surfaces it to the player before they click).
func (g *Game) hireOptions() []ui.HireOption {
	countBuildings := func(kind building.Kind) int {
		n := 0
		for _, b := range g.buildings {
			if b.Kind == kind {
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
	limited := func(kind ui.HireKind, bKind building.Kind, current int) ui.HireOption {
		limit := countBuildings(bKind)
		return ui.HireOption{Kind: kind, Current: current, Limit: limit, Available: current < limit}
	}
	return []ui.HireOption{
		{Kind: ui.HireSerf, Current: len(g.logi.Serfs), Limit: 0, Available: true},
		limited(ui.HireFarmer, building.Farm, countProfession(villagers.Farmer)),
		limited(ui.HireBaker, building.Bakery, countProfession(villagers.Baker)),
		limited(ui.HireWinemaker, building.Winery, countProfession(villagers.Winemaker)),
		limited(ui.HireLumberjack, building.LumberjackHut, len(g.jacks.Lumberjacks)),
		limited(ui.HireFisherman, building.FisherHut, len(g.fishers.Fishermen)),
		limited(ui.HireSwineherd, building.PigFarm, countProfession(villagers.Swineherd)),
		limited(ui.HireButcher, building.MeatWorkshop, countProfession(villagers.Butcher)),
		limited(ui.HireCarpenter, building.CarpentryWorkshop, countProfession(villagers.Carpenter)),
		limited(ui.HireQuarryman, building.QuarryHut, len(g.quarry.Quarrymen)),
		{Kind: ui.HireBuilder, Current: len(g.builders.Builders), Limit: maxBuilders, Available: len(g.builders.Builders) < maxBuilders},
	}
}

// hireFromTab executes a click on an available Hire-tab card. It finds the
// first matching building without a resident and assigns a fresh worker to
// it -- the same one-worker-per-building placement spawnWorkersFor uses when
// a building is first built, so a hired replacement behaves identically to
// an original resident.
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
	case ui.HireLumberjack:
		for _, b := range g.buildings {
			if b.Kind == building.LumberjackHut && !g.jacks.HasHome(b) {
				g.jacks.Spawn(b)
				g.refreshPopulation()
				g.statusMsg = ""
				return
			}
		}
	case ui.HireFisherman:
		for _, b := range g.buildings {
			if b.Kind == building.FisherHut && !g.fishers.HasHome(b) {
				g.fishers.Spawn(b)
				g.refreshPopulation()
				g.statusMsg = ""
				return
			}
		}
	case ui.HireQuarryman:
		for _, b := range g.buildings {
			if b.Kind == building.QuarryHut && !g.quarry.HasHome(b) {
				g.quarry.Spawn(b)
				g.refreshPopulation()
				g.statusMsg = ""
				return
			}
		}
	case ui.HireBuilder:
		if len(g.builders.Builders) < maxBuilders {
			g.builders.Hire(g.logi.Warehouse)
			g.refreshPopulation()
			g.statusMsg = ""
		}
	}
}

func (g *Game) hireVillagerInto(profession villagers.Profession, kind building.Kind) {
	for _, b := range g.buildings {
		if b.Kind == kind && !g.vills.HasHome(b) {
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

var paletteKeys = []ebiten.Key{
	ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4,
	ebiten.Key5, ebiten.Key6, ebiten.Key7, ebiten.Key8, ebiten.Key9,
	ebiten.Key0, ebiten.KeyQ,
}

func (g *Game) handlePaletteSelect() {
	for i := 0; i < len(g.palette.Kinds) && i < len(paletteKeys); i++ {
		if inpututil.IsKeyJustPressed(paletteKeys[i]) {
			g.palette.Select(i)
			g.buildMode = true
		}
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

func (g *Game) hireSerf() {
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
	if b.Kind == building.StoneDeposit {
		g.statusMsg = i18n.T().CannotDeleteStoneDeposit
		return
	}

	if b.Kind == building.Warehouse && !g.logi.RemoveWarehouse(b) {
		g.statusMsg = i18n.T().CannotDeleteWarehouse
		return
	}
	g.logi.CancelAllJobs(g.stock)
	g.vills.RemoveHome(b)
	g.vills.CancelRouteTo(b) // in case b is a Tavern someone is mid-trip to eat at
	g.jacks.RemoveHome(b, g.stock)
	g.jacks.CancelRouteTo(b) // same, for lumberjacks
	g.fishers.RemoveHome(b, g.stock)
	g.fishers.CancelRouteTo(b) // same, for fishermen
	g.quarry.RemoveHome(b, g.stock)
	g.quarry.CancelRouteTo(b) // same, for quarrymen
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
	default:
		return
	}
	g.selection.Clear()
}

// refreshPopulation rebuilds the live headcount while retaining the
// persistent death/removal history shown in the HUD.
func (g *Game) refreshPopulation() {
	g.pop.Count = len(g.logi.Serfs) + len(g.vills.Villagers) + len(g.jacks.Lumberjacks) + len(g.fishers.Fishermen) + len(g.quarry.Quarrymen) + len(g.builders.Builders)
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
// pool, while a lumberjack is tied to one Lumberjack Hut.
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
	}
}

// finishConstruction reacts to builder.ConstructionComplete: a freshly
// finished building gets exactly the same treatment a normally-placed one
// always has -- its resident worker, and, for a Warehouse specifically,
// registration as an additional logistics endpoint. Both were deliberately
// deferred from placement time to this point (see handleMouse) so an
// unfinished building never produces or acts as a warehouse before a
// Builder has actually finished it.
func (g *Game) finishConstruction(b *building.Building) {
	if b.Kind == building.Warehouse {
		g.logi.AddWarehouse(b)
	}
	g.spawnWorkersFor(b)
	g.statusMsg = ""
}

func (g *Game) handleSaveLoad() {
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		if err := g.saveGame(savePath, ""); err != nil {
			g.statusMsg = i18n.T().SaveFailedPrefix + err.Error()
		} else {
			g.statusMsg = i18n.T().Saved
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		g.loadAndReport(savePath)
	}
}

// slotPath returns the file path for save-panel slot n (1-slotCount). It is
// deliberately distinct from savePath, the S/L quicksave file, so the two
// save mechanisms never collide.
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
	}
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
		SerfMealSeed:       g.logi.MealSeed(),
		VillagerMealSeed:   g.vills.MealSeed(),
		LumberjackMealSeed: g.jacks.MealSeed(),
		FishermanMealSeed:  g.fishers.MealSeed(),
		QuarrymanMealSeed:  g.quarry.MealSeed(),
		BuilderMealSeed:    g.builders.MealSeed(),
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
	buildings = ensureStoneDeposits(grid, buildings, state.StoneSeeded, defaultStoneSeed)
	warehouse := findWarehouse(buildings)
	if warehouse == nil {
		return errNoWarehouseInSave
	}

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
	// ensureStoneDeposits above guarantees a region exists one way or
	// another (freshly generated, or already present in the save), so from
	// here on this world always counts as seeded.
	g.stoneSeeded = true
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
	units := make([]save.UnitState, 0, len(g.logi.Serfs)+len(g.vills.Villagers)+len(g.jacks.Lumberjacks)+len(g.fishers.Fishermen)+len(g.quarry.Quarrymen)+len(g.builders.Builders))
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
	return units
}

func (g *Game) restoreUnits(states []save.UnitState, buildings []*building.Building) {
	for _, state := range states {
		switch state.Kind {
		case save.UnitSerf:
			g.logi.RestoreSerf(state.X, state.Y, state.HungerTicks, state.Starving, state.Dismissing)
		case save.UnitFarmer, save.UnitBaker, save.UnitWinemaker, save.UnitSwineherd, save.UnitButcher, save.UnitCarpenter:
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
			default:
				profession = villagers.Farmer
			}
			if (profession == villagers.Farmer && home.Kind != building.Farm) ||
				(profession == villagers.Baker && home.Kind != building.Bakery) ||
				(profession == villagers.Winemaker && home.Kind != building.Winery) ||
				(profession == villagers.Swineherd && home.Kind != building.PigFarm) ||
				(profession == villagers.Butcher && home.Kind != building.MeatWorkshop) ||
				(profession == villagers.Carpenter && home.Kind != building.CarpentryWorkshop) {
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
// and ensureStoneDeposits.
func seedStoneDeposits(grid *world.Grid, buildings []*building.Building, seed uint32) []*building.Building {
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
		buildings = growStoneRegion(grid, buildings, target, regionSeed)
	}
	return buildings
}

// growStoneRegion places one contiguous blob of up to target stone-deposit
// cells, starting from a deterministically chosen free tile and expanding
// outward. Called once per region by seedStoneDeposits.
func growStoneRegion(grid *world.Grid, buildings []*building.Building, target int, seed uint32) []*building.Building {
	if target <= 0 {
		return buildings
	}
	start, ok := findStoneStart(grid, buildings, seed)
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

		if !building.CanPlace(grid, buildings, building.StoneDeposit, p.x, p.y) {
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

func findStoneStart(grid *world.Grid, buildings []*building.Building, seed uint32) (gridPoint, bool) {
	bestScore := ^uint32(0)
	var best gridPoint
	found := false
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			if !building.CanPlace(grid, buildings, building.StoneDeposit, x, y) {
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
// infer it from, unlike trees or fish.
func ensureStoneDeposits(grid *world.Grid, buildings []*building.Building, alreadySeeded bool, seed uint32) []*building.Building {
	if alreadySeeded {
		return buildings
	}
	return seedStoneDeposits(grid, buildings, seed)
}

// removeStoneDeposit deletes an exhausted deposit from the world once its
// Reserve reaches zero. The tile underneath needs no separate change: it was
// always ordinary buildable ground, the same way felling a tree reveals the
// grass it always stood on.
func (g *Game) removeStoneDeposit(deposit *building.Building) {
	if deposit == nil || deposit.Kind != building.StoneDeposit {
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
