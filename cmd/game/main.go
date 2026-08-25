// Command game is the entry point for the strategy game.
package main

import (
	"image"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/i18n"
	"strategy_game/internal/logistics"
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

	stockpileCapacity = 200
	startingSerfs     = 3

	// Fixed spot for the town's one Warehouse, chosen to sit on plain
	// grass in world.NewTestGrid (away from the fertile/forest/water/
	// stone patches). A single Road tile just south of it gives the
	// player something to extend from immediately.
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

	camera    *render.Camera
	palette   *ui.Palette
	layout    ui.Layout
	buildMode bool
	selection ui.Selection

	statusMsg string
}

func NewGame() *Game {
	warehouse := &building.Building{Kind: building.Warehouse, X: warehouseX, Y: warehouseY}
	initialRoad := &building.Building{Kind: building.Road, X: warehouseX, Y: warehouseY + 1}

	buildings := []*building.Building{warehouse, initialRoad}

	layout := ui.NewLayout(screenWidth, screenHeight)
	camera := render.NewCamera()
	mapRect := layout.MapRect()
	camera.SetViewport(mapRect.Min.X, mapRect.Min.Y, mapRect.Dx(), mapRect.Dy())

	return &Game{
		grid:      world.NewTestGrid(),
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
	g.handleCameraPan()
	g.handlePaletteSelect()
	g.handleMouse()
	g.handleSaveLoad()

	for range g.sim.Advance() {
		economy.Tick(g.buildings, g.starvingBuildings())
		g.logi.Tick(g.buildings, g.stock)
		g.vills.Tick(g.buildings)
		g.pop.Count = len(g.logi.Serfs) + len(g.vills.Villagers)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	return nil
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
	g.spawnVillagerFor(placed)
	g.statusMsg = ""
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
			CameraX:    g.camera.X,
			CameraY:    g.camera.Y,
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
		warehouse := findWarehouse(buildings)
		if warehouse == nil {
			g.statusMsg = i18n.T().LoadFailedNoWarehouse
			return
		}

		g.grid = grid
		g.buildings = buildings
		stock := state.Stockpile
		g.stock = &stock
		pop := state.Population
		g.pop = &pop
		g.camera.X, g.camera.Y = state.CameraX, state.CameraY
		g.selection.Clear()

		// Serf/villager positions and jobs aren't persisted (see
		// save.GameState docs) -- respawn a fresh crew instead, one
		// villager per Farm/Bakery that was actually saved.
		g.logi = logistics.NewController(warehouse, startingSerfs)
		g.vills = villagers.NewController()
		for _, b := range buildings {
			g.spawnVillagerFor(b)
		}

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

func referenceBuildings(in []building.Building) []*building.Building {
	out := make([]*building.Building, len(in))
	for i := range in {
		out[i] = &in[i]
	}
	return out
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
	ui.DrawSelectionMarker(screen, g.camera, g.selection)
	ui.DrawResourceBarAt(screen, g.stock, g.pop, float64(g.layout.LeftWidth+16), 10)
	ui.DrawBuildPanel(screen, g.layout, g.palette)
	ui.DrawInspectorPanel(screen, g.layout, g.selection)
	ui.DrawSpeedPanel(screen, g.layout, g.sim.Speed())

	ui.DrawText(screen, i18n.T().Help, float64(g.layout.LeftWidth+16), float64(screenHeight-20))
	if g.statusMsg != "" {
		ui.DrawText(screen, g.statusMsg, 8, float64(screenHeight-36))
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
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
