// Command game is the entry point for the strategy game.
package main

import (
	"image"
	"log"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/i18n"
	"strategy_game/internal/logistics"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/render"
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
)

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

	camera        *render.Camera
	palette       *ui.Palette
	layout        ui.Layout
	buildMode     bool
	selection     ui.Selection
	middlePanning bool
	lastMouseX    int
	lastMouseY    int

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

	layout := ui.NewLayout(screenWidth, screenHeight)
	camera := render.NewCamera()
	mapRect := layout.MapRect()
	camera.SetViewport(mapRect.Min.X, mapRect.Min.Y, mapRect.Dx(), mapRect.Dy())
	// The grove starts just beyond the warehouse area at x=25. Shift the
	// initial view four tiles right so at least its near edge is visible without
	// requiring the player to discover camera panning first.
	camera.Pan(float64(4*render.TileSize), 0, grid.Width, grid.Height, mapRect.Dx(), mapRect.Dy())

	return &Game{
		grid:      grid,
		buildings: buildings,
		stock:     resource.NewStockpile(stockpileCapacity),
		pop:       &economy.Population{},
		sim:       economy.NewSimulator(framesPerSimTick),
		logi:      logistics.NewController(warehouse, startingSerfs),
		vills:     villagers.NewController(),
		camera:    camera,
		palette:   ui.NewPalette(),
		layout:    layout,
	}
}

func (g *Game) Update() error {
	if width, height := ebiten.WindowSize(); width > 0 && height > 0 {
		g.resizeLayout(width, height)
	}
	g.handleCameraPan()
	g.handleCameraZoom()
	g.handlePaletteSelect()
	g.handleMouse()
	g.handleUnitActions()
	g.handleSaveLoad()

	for range g.sim.Advance() {
		for _, b := range g.buildings {
			b.TickGrowth()
		}
		economy.TickWithConnectivity(g.buildings, g.starvingBuildings(), g.disconnectedBuildings())
		g.logi.Tick(g.buildings, g.stock)
		g.vills.Tick(g.buildings)
		g.pop.Count = len(g.logi.Serfs) + len(g.vills.Villagers)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
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

// starvingBuildings reports which buildings currently have no worker
// physically at their post -- so economy.Tick can pause their
// production instead of quietly progressing an empty building. This is
// deliberately NOT the same as Villager.Starving: a worker who's merely
// hungry but still standing at home (e.g. because the Tavern has no
// Bread yet) keeps working. Gating production on Starving too would
// deadlock a fresh town's very first production cycle -- the Tavern
// can't get Bread until the Bakery makes some, and the Bakery can't
// work while "starving".
func (g *Game) starvingBuildings() map[*building.Building]bool {
	m := make(map[*building.Building]bool, len(g.vills.Villagers))
	for _, v := range g.vills.Villagers {
		if !v.Working() {
			m[v.Home] = true
		}
	}
	return m
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
	if index, ok := g.layout.BuildIndexAt(mx, my, len(g.palette.Kinds)); ok {
		g.palette.Select(index)
		g.buildMode = true
		g.statusMsg = ""
		return
	}
	if speed, ok := g.layout.SpeedAt(mx, my); ok {
		g.sim.SetSpeed(speed)
		return
	}
	if g.layout.HireAt(mx, my) {
		g.hireSerf()
		return
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
	placed := &building.Building{Kind: kind, X: tx, Y: ty}
	g.buildings = append(g.buildings, placed)
	if kind == building.Warehouse {
		g.logi.AddWarehouse(placed)
	}
	g.spawnVillagerFor(placed)
	g.statusMsg = ""
}

func (g *Game) handleUnitActions() {
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		g.hireSerf()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) {
		g.deleteSelectedBuilding()
	}
}

func (g *Game) hireSerf() {
	g.logi.Hire()
	g.pop.Count = len(g.logi.Serfs) + len(g.vills.Villagers)
	g.statusMsg = ""
}

// deleteSelectedBuilding removes the selected building and invalidates all
// active routes before the slice is changed. The Warehouse is the town's
// mandatory logistics root and cannot be deleted.
func (g *Game) deleteSelectedBuilding() {
	if g.selection.Kind != ui.SelectionBuilding || g.selection.Building == nil {
		return
	}
	b := g.selection.Building
	if b.Kind == building.Warehouse {
		g.statusMsg = i18n.T().CannotDeleteWarehouse
		return
	}
	if b.Kind == building.Tree {
		g.statusMsg = i18n.T().CannotDeleteTree
		return
	}

	g.logi.CancelAllJobs(g.stock)
	g.vills.RemoveHome(b)
	for i, candidate := range g.buildings {
		if candidate != b {
			continue
		}
		g.buildings = append(g.buildings[:i], g.buildings[i+1:]...)
		g.selection.Clear()
		g.statusMsg = i18n.T().Deleted
		return
	}
}

// selectionAt resolves map coordinates to a live game object. Units have
// priority over buildings because a worker standing beside a building is the
// more useful thing to inspect on a click.
func (g *Game) selectionAt(mx, my int) ui.Selection {
	tx, ty := g.camera.ScreenToTile(mx, my)
	for i := len(g.vills.Villagers) - 1; i >= 0; i-- {
		v := g.vills.Villagers[i]
		if v.X == tx && v.Y == ty {
			return ui.Selection{Kind: ui.SelectionVillager, Villager: v}
		}
	}
	for i := len(g.logi.Serfs) - 1; i >= 0; i-- {
		s := g.logi.Serfs[i]
		if s.X == tx && s.Y == ty {
			return ui.Selection{Kind: ui.SelectionSerf, Serf: s}
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

// spawnVillagerFor gives a newly placed Farm or Bakery its worker. Other
// building kinds don't get a villagers.Villager -- Warehouse/Road/Tavern
// have no production to tend, and serfs (package logistics) are a
// separate, town-wide pool rather than tied to one building.
func (g *Game) spawnVillagerFor(b *building.Building) {
	switch b.Kind {
	case building.Farm:
		g.vills.Spawn(villagers.Farmer, b)
	case building.Bakery:
		g.vills.Spawn(villagers.Baker, b)
	}
}

func (g *Game) handleSaveLoad() {
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		state := save.GameState{
			GridWidth:  g.grid.Width,
			GridHeight: g.grid.Height,
			Tiles:      g.grid.Tiles(),
			Buildings:  dereferenceBuildings(g.buildings),
			Stockpile:  *g.stock,
			Population: *g.pop,
			Units:      g.serializeUnits(),
			CameraX:    g.camera.X,
			CameraY:    g.camera.Y,
			CameraZoom: g.camera.Scale,
		}
		if err := save.Save(savePath, state); err != nil {
			g.statusMsg = i18n.T().SaveFailedPrefix + err.Error()
			return
		}
		g.statusMsg = i18n.T().Saved
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		state, err := save.Load(savePath)
		if err != nil {
			g.statusMsg = i18n.T().LoadFailedPrefix + err.Error()
			return
		}
		grid, err := world.NewGridFromTiles(state.GridWidth, state.GridHeight, state.Tiles)
		if err != nil {
			g.statusMsg = i18n.T().LoadFailedPrefix + err.Error()
			return
		}
		buildings := referenceBuildings(state.Buildings)
		buildings, hadTrees := ensureTrees(grid, buildings)
		warehouse := findWarehouse(buildings)
		if warehouse == nil {
			g.statusMsg = i18n.T().LoadFailedNoWarehouse
			return
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
				g.spawnVillagerFor(b)
			}
		} else {
			g.restoreUnits(state.Units, buildings)
		}
		g.pop.Count = len(g.logi.Serfs) + len(g.vills.Villagers)

		g.statusMsg = i18n.T().Loaded
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

func (g *Game) serializeUnits() []save.UnitState {
	units := make([]save.UnitState, 0, len(g.logi.Serfs)+len(g.vills.Villagers))
	for _, s := range g.logi.Serfs {
		units = append(units, save.UnitState{
			Kind:        save.UnitSerf,
			X:           s.X,
			Y:           s.Y,
			HomeIndex:   -1,
			HungerTicks: s.HungerTicks(),
			Starving:    s.Starving,
		})
	}
	for _, v := range g.vills.Villagers {
		kind := save.UnitFarmer
		if v.Profession == villagers.Baker {
			kind = save.UnitBaker
		}
		units = append(units, save.UnitState{
			Kind:        kind,
			X:           v.X,
			Y:           v.Y,
			HomeIndex:   indexOfBuilding(g.buildings, v.HomeBuilding()),
			HungerTicks: v.HungerTicks(),
			Starving:    v.Starving,
			State:       int(v.State()),
		})
	}
	return units
}

func (g *Game) restoreUnits(states []save.UnitState, buildings []*building.Building) {
	for _, state := range states {
		switch state.Kind {
		case save.UnitSerf:
			g.logi.RestoreSerf(state.X, state.Y, state.HungerTicks, state.Starving)
		case save.UnitFarmer, save.UnitBaker:
			if state.HomeIndex < 0 || state.HomeIndex >= len(buildings) {
				continue
			}
			home := buildings[state.HomeIndex]
			profession := villagers.Farmer
			if state.Kind == save.UnitBaker {
				profession = villagers.Baker
			}
			if (profession == villagers.Farmer && home.Kind != building.Farm) ||
				(profession == villagers.Baker && home.Kind != building.Bakery) {
				continue
			}
			g.vills.RestoreVillager(profession, home, state.X, state.Y, state.HungerTicks, state.Starving, villagers.State(state.State), buildings)
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

// ensureTrees migrates saves created before persistent tree objects existed.
// Current saves already contain at least one Tree and are left untouched so
// each tree's individual growth timer remains authoritative.
func ensureTrees(grid *world.Grid, buildings []*building.Building) ([]*building.Building, bool) {
	for _, b := range buildings {
		if b.Kind == building.Tree {
			return buildings, true
		}
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

func (g *Game) Draw(screen *ebiten.Image) {
	render.Tick()
	render.DrawGrid(screen, g.grid, g.camera)
	render.DrawBuildings(screen, g.buildings, g.camera)
	render.DrawSerfs(screen, g.logi.Serfs, g.camera)
	render.DrawVillagers(screen, g.vills.Villagers, g.camera)

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
		if b.Kind != building.Road && b.Kind != building.Tree && b.Kind != building.Warehouse {
			ui.DrawAccessMarker(screen, g.camera, b, g.buildingConnected(b))
		}
	}
	render.DrawWorkerMarkers(screen, g.buildings, g.vills.Villagers, g.camera)
	ui.DrawSelectionMarker(screen, g.camera, g.selection)
	connected := false
	if g.selection.Kind == ui.SelectionBuilding && g.selection.Building != nil {
		connected = g.buildingConnected(g.selection.Building)
	}
	ui.DrawResourceBarAt(screen, g.stock, g.pop, float64(g.layout.LeftWidth+16), 10)
	ui.DrawBuildPanel(screen, g.layout, g.palette)
	ui.DrawInspectorPanel(screen, g.layout, g.selection, connected, g.stock)
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
