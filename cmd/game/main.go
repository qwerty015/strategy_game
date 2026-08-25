// Command game is the entry point for the strategy game.
package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
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

	savePath = "saves/slot1.json"
)

// Game implements ebiten.Game and wires the logic packages (world,
// building, resource, economy) to rendering and input.
type Game struct {
	grid      *world.Grid
	buildings []*building.Building
	stock     *resource.Stockpile
	pop       *economy.Population
	sim       *economy.Simulator

	camera  *render.Camera
	palette *ui.Palette

	statusMsg string
}

func NewGame() *Game {
	return &Game{
		grid:      world.NewTestGrid(),
		buildings: nil,
		stock:     resource.NewStockpile(stockpileCapacity),
		pop:       economy.NewPopulation(startPopulation, ticksPerMeal),
		sim:       economy.NewSimulator(framesPerSimTick),
		camera:    render.NewCamera(),
		palette:   ui.NewPalette(),
	}
}

func (g *Game) Update() error {
	g.handleCameraPan()
	g.handlePaletteSelect()
	g.handlePlacement()
	g.handleSaveLoad()

	g.sim.Update(g.buildings, g.stock, g.pop)

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

func (g *Game) handlePaletteSelect() {
	for i, key := range []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3} {
		if inpututil.IsKeyJustPressed(key) {
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
		g.statusMsg = "Can't build there"
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
			g.statusMsg = "Save failed: " + err.Error()
			return
		}
		g.statusMsg = "Saved."
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		state, err := save.Load(savePath)
		if err != nil {
			g.statusMsg = "Load failed: " + err.Error()
			return
		}
		grid, err := world.NewGridFromTiles(state.GridWidth, state.GridHeight, state.Tiles)
		if err != nil {
			g.statusMsg = "Load failed: " + err.Error()
			return
		}
		g.grid = grid
		g.buildings = referenceBuildings(state.Buildings)
		stock := state.Stockpile
		g.stock = &stock
		pop := state.Population
		g.pop = &pop
		g.camera.X, g.camera.Y = state.CameraX, state.CameraY
		g.statusMsg = "Loaded."
	}
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

	mx, my := ebiten.CursorPosition()
	tx, ty := g.camera.ScreenToTile(mx, my)
	kind := g.palette.SelectedKind()
	valid := building.CanPlace(g.grid, g.buildings, kind, tx, ty)
	ui.DrawPlacementPreview(screen, g.camera, kind, tx, ty, valid)

	ui.DrawResourceBar(screen, g.stock, g.pop)
	ui.DrawPalette(screen, g.palette)

	help := "Arrows: pan | 1/2/3: select building | Click: place | S: save | L: load | Esc: quit"
	ebitenutil.DebugPrintAt(screen, help, 8, screenHeight-20)
	if g.statusMsg != "" {
		ebitenutil.DebugPrintAt(screen, g.statusMsg, 8, screenHeight-36)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("Strategy Game")
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
