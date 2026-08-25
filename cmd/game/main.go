// Command game is the entry point for the strategy game.
package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/i18n"
	"strategy_game/internal/logistics"
	"strategy_game/internal/render"
	"strategy_game/internal/resource"
	"strategy_game/internal/save"
	"strategy_game/internal/ui"
	"strategy_game/internal/world"
)

const (
	screenWidth  = 1024
	screenHeight = 768
	panSpeed     = 8 // pixels per frame while a pan key is held

	framesPerSimTick = 30 // simulation ticks run at 2/sec on a 60fps display

	stockpileCapacity = 200
	startPopulation   = 5
	ticksPerMeal      = 6
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

	camera  *render.Camera
	palette *ui.Palette

	statusMsg string
}

func NewGame() *Game {
	warehouse := &building.Building{Kind: building.Warehouse, X: warehouseX, Y: warehouseY}
	initialRoad := &building.Building{Kind: building.Road, X: warehouseX, Y: warehouseY + 2}

	buildings := []*building.Building{warehouse, initialRoad}

	return &Game{
		grid:      world.NewTestGrid(),
		buildings: buildings,
		stock:     resource.NewStockpile(stockpileCapacity),
		pop:       economy.NewPopulation(startPopulation, ticksPerMeal),
		sim:       economy.NewSimulator(framesPerSimTick),
		logi:      logistics.NewController(warehouse, startingSerfs),
		camera:    render.NewCamera(),
		palette:   ui.NewPalette(),
	}
}

func (g *Game) Update() error {
	g.handleCameraPan()
	g.handlePaletteSelect()
	g.handlePlacement()
	g.handleSaveLoad()

	if g.sim.ShouldTick() {
		economy.Tick(g.buildings, g.pop, g.stock)
		g.logi.Tick(g.buildings, g.stock)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	return nil
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
		g.camera.Pan(dx, dy, g.grid.Width, g.grid.Height, screenWidth, screenHeight)
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
		}
	}
}

func (g *Game) handlePlacement() {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	mx, my := ebiten.CursorPosition()
	tx, ty := g.camera.ScreenToTile(mx, my)

	kind := g.palette.SelectedKind()
	if !building.CanPlace(g.grid, g.buildings, kind, tx, ty) {
		g.statusMsg = i18n.T().CantBuildHere
		return
	}
	g.buildings = append(g.buildings, &building.Building{Kind: kind, X: tx, Y: ty})
	g.statusMsg = ""
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

		// Serf positions/jobs aren't persisted (see save.GameState docs)
		// -- respawn a fresh crew at the warehouse instead.
		g.logi = logistics.NewController(warehouse, startingSerfs)

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
	render.DrawGrid(screen, g.grid, g.camera)
	render.DrawBuildings(screen, g.buildings, g.camera)
	render.DrawSerfs(screen, g.logi.Serfs, g.camera)

	mx, my := ebiten.CursorPosition()
	tx, ty := g.camera.ScreenToTile(mx, my)
	kind := g.palette.SelectedKind()
	valid := building.CanPlace(g.grid, g.buildings, kind, tx, ty)
	ui.DrawPlacementPreview(screen, g.camera, kind, tx, ty, valid)

	ui.DrawResourceBar(screen, g.stock, g.pop)
	ui.DrawPalette(screen, g.palette)

	ebitenutil.DebugPrintAt(screen, i18n.T().Help, 8, screenHeight-20)
	if g.statusMsg != "" {
		ebitenutil.DebugPrintAt(screen, g.statusMsg, 8, screenHeight-36)
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
