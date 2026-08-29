package main

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	gamehelp "strategy_game"
	"strategy_game/internal/assets"
	"strategy_game/internal/building"
	"strategy_game/internal/i18n"
	"strategy_game/internal/render"
	"strategy_game/internal/ui"
)

const (
	titleMapWidth  = 128
	titleMapHeight = 96
	titleMapZoom   = 2.00

	gameTitleRU = "Земли ремесла"
	gameTitleEN = "Lands of Craft"
)

type appScreen uint8

const (
	screenPlay appScreen = iota
	screenTitle
	screenLoad
	screenHelp
)

type titleAction uint8

const (
	titleActionNewGame titleAction = iota
	titleActionLoad
	titleActionHelp
)

type titleCopy struct {
	title, subtitle      string
	newGame, load, help  string
	back, previous, next string
	loadTitle, noSaves   string
}

func activeTitleCopy() titleCopy {
	if i18n.Current() == i18n.EN {
		return titleCopy{
			title: gameTitleEN, subtitle: "Build a living settlement",
			newGame: "New game", load: "Load game", help: "Help",
			back: "Back", previous: "Previous", next: "Next",
			loadTitle: "Load a save", noSaves: "No saved games yet",
		}
	}
	return titleCopy{
		title: gameTitleRU, subtitle: "Постройте живое поселение",
		newGame: "Новая игра", load: "Загрузить игру", help: "Справка",
		back: "Назад", previous: "Назад", next: "Далее",
		loadTitle: "Загрузить сохранение", noSaves: "Сохранений пока нет",
	}
}

type helpLineKind uint8

const (
	helpLineText helpLineKind = iota
	helpLineHeading
	helpLineImage
)

type helpLine struct {
	kind  helpLineKind
	text  string
	image string
}

type helpPage struct {
	title string
	lines []helpLine
}

// NewApplication creates a non-simulating presentation map first. NewGame
// remains the gameplay factory used by tests and the in-game reset button.
func NewApplication() *Game {
	game := newGameWithSize(titleMapWidth, titleMapHeight)
	game.screen = screenTitle
	game.helpPages = parseHelpMarkdown(gamehelp.HelpMarkdown)
	game.populateTitleTown()
	game.camera.Scale = titleMapZoom
	game.camera.SetViewport(0, 0, screenWidth, screenHeight)
	game.camera.X = float64(8 * render.TileSize)
	game.camera.Y = float64(6 * render.TileSize)
	return game
}

func parseHelpMarkdown(markdown string) []helpPage {
	pages := make([]helpPage, 0, 8)
	current := helpPage{}
	flush := func() {
		if current.title != "" || len(current.lines) > 0 {
			pages = append(pages, current)
		}
	}
	for _, raw := range strings.Split(markdown, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "" || strings.HasPrefix(line, "# "):
			continue
		case strings.HasPrefix(line, "## "):
			flush()
			current = helpPage{title: strings.TrimSpace(strings.TrimPrefix(line, "## "))}
		case strings.HasPrefix(line, "### "):
			current.lines = append(current.lines, helpLine{kind: helpLineHeading, text: cleanMarkdown(strings.TrimSpace(strings.TrimPrefix(line, "### ")))})
		case strings.HasPrefix(line, "!["):
			if start := strings.Index(line, "]("); start >= 0 && strings.HasSuffix(line, ")") {
				current.lines = append(current.lines, helpLine{kind: helpLineImage, text: line[2:start], image: line[start+2 : len(line)-1]})
			}
		default:
			line = strings.TrimSpace(strings.TrimLeft(line, "- "))
			if line != "" {
				current.lines = append(current.lines, helpLine{kind: helpLineText, text: cleanMarkdown(line)})
			}
		}
	}
	flush()
	return pages
}

func cleanMarkdown(value string) string {
	value = strings.ReplaceAll(value, "**", "")
	value = strings.ReplaceAll(value, "`", "")
	return value
}

func (g *Game) updateFrontScreen() error {
	g.titleFrame++
	g.camera.SetViewport(0, 0, g.layout.Width, g.layout.Height)
	g.advanceTitleCamera()

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if g.screen == screenTitle {
			return ebiten.Termination
		}
		g.screen = screenTitle
		return nil
	}
	if g.screen == screenHelp {
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) && g.helpPage > 0 {
			g.helpPage--
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) && g.helpPage+1 < len(g.helpPages) {
			g.helpPage++
		}
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return nil
	}
	mx, my := ebiten.CursorPosition()
	switch g.screen {
	case screenTitle:
		switch action, ok := titleActionAt(mx, my, g.layout.Width, g.layout.Height); {
		case !ok:
			return nil
		case action == titleActionNewGame:
			g.startNewGameFromTitle()
		case action == titleActionLoad:
			g.refreshSlotCache()
			g.screen = screenLoad
		case action == titleActionHelp:
			g.helpPage = 0
			g.screen = screenHelp
		}
	case screenLoad:
		if image.Pt(mx, my).In(titleBackRect(g.layout.Width, g.layout.Height)) {
			g.screen = screenTitle
			return nil
		}
		if slot, ok := titleLoadSlotAt(mx, my, g.layout.Width, g.layout.Height, len(g.slotCache)); ok && g.slotCache[slot-1].Occupied {
			if err := g.loadGame(slotPath(slot)); err == nil {
				g.screen = screenPlay
				g.statusMsg = i18n.T().Loaded
			} else {
				g.statusMsg = i18n.T().LoadFailedPrefix + err.Error()
			}
		}
	case screenHelp:
		back, previous, next := helpNavRects(g.layout.Width, g.layout.Height)
		point := image.Pt(mx, my)
		switch {
		case point.In(back):
			g.screen = screenTitle
		case point.In(previous) && g.helpPage > 0:
			g.helpPage--
		case point.In(next) && g.helpPage+1 < len(g.helpPages):
			g.helpPage++
		}
	}
	return nil
}

func (g *Game) advanceTitleCamera() {
	if g.grid == nil || g.camera.Scale <= 0 {
		return
	}
	// The presentation map stays at 200%; its movement wraps through a much
	// larger demo world instead of reaching the edge and freezing.
	g.camera.X += 0.16
	g.camera.Y += math.Sin(float64(g.titleFrame)/240) * 0.012
	visibleWidth := float64(g.layout.Width) / g.camera.Scale
	visibleHeight := float64(g.layout.Height) / g.camera.Scale
	maxX := float64(g.grid.Width*render.TileSize) - visibleWidth
	maxY := float64(g.grid.Height*render.TileSize) - visibleHeight
	if maxX > 0 && g.camera.X > maxX {
		g.camera.X = 0
	}
	if maxY > 0 && g.camera.Y > maxY {
		g.camera.Y = 0
	}
}

func (g *Game) startNewGameFromTitle() {
	layout := g.layout
	fresh := NewGame()
	fresh.layout = layout
	mapRect := layout.MapRect()
	fresh.camera.SetViewport(mapRect.Min.X, mapRect.Min.Y, mapRect.Dx(), mapRect.Dy())
	fresh.statusMsg = i18n.T().NewGameStarted
	*g = *fresh
}

func (g *Game) populateTitleTown() {
	kinds := []building.Kind{
		building.Farm, building.Farm, building.Winery, building.Mill, building.Bakery,
		building.Tavern, building.Warehouse, building.LumberjackHut, building.FisherHut,
		building.PigFarm, building.MeatWorkshop, building.CarpentryWorkshop,
		building.QuarryHut, building.MinerHut, building.Smeltery,
	}
	seed := uint32(0x6d2b79f5)
	for _, kind := range kinds {
		for attempt := 0; attempt < 180; attempt++ {
			seed = seed*1664525 + 1013904223
			footprint := building.Types[kind].Footprint
			x := 4 + int(seed%uint32(g.grid.Width-footprint-8))
			seed = seed*1664525 + 1013904223
			y := 4 + int(seed%uint32(g.grid.Height-footprint-8))
			if !building.CanPlace(g.grid, g.buildings, kind, x, y) {
				continue
			}
			b := &building.Building{Kind: kind, X: x, Y: y}
			if recipe := building.Types[kind].Recipe; recipe.TicksToProduce > 0 {
				b.ProgressTicks = recipe.TicksToProduce / 2
			} else if len(building.Types[kind].AltRecipes) > 0 {
				b.ProgressTicks = 1
			}
			g.buildings = append(g.buildings, b)
			entry := b.AccessPoint()
			if g.grid.InBounds(entry.X, entry.Y) && building.CanPlace(g.grid, g.buildings, building.Road, entry.X, entry.Y) {
				g.buildings = append(g.buildings, &building.Building{Kind: building.Road, X: entry.X, Y: entry.Y})
			}
			break
		}
	}
}

func (g *Game) drawFrontScreen(screen *ebiten.Image) {
	render.Tick()
	render.DrawGrid(screen, g.grid, g.camera)
	render.DrawAmbientGroundLife(screen, g.grid, g.camera)
	render.DrawBuildings(screen, g.grid, g.buildings, g.camera, map[*building.Building]bool{}, map[*building.Building]bool{})
	render.DrawAmbientSkyLife(screen, g.grid, g.camera)
	render.DrawAtmosphericOverlay(screen, g.grid, g.camera)
	vector.FillRect(screen, 0, 0, float32(g.layout.Width), float32(g.layout.Height), color.RGBA{R: 19, G: 20, B: 23, A: 124}, false)

	switch g.screen {
	case screenLoad:
		g.drawLoadScreen(screen)
	case screenHelp:
		g.drawHelpScreen(screen)
	default:
		g.drawTitleScreen(screen)
	}
}

func (g *Game) drawTitleScreen(screen *ebiten.Image) {
	copy := activeTitleCopy()
	width, height := g.layout.Width, g.layout.Height
	ui.DrawTitleText(screen, copy.title, 66, float64(height)*0.17, 4)
	ui.DrawTitleText(screen, copy.subtitle, 70, float64(height)*0.17+42, 1.45)
	for index, label := range []string{copy.newGame, copy.load, copy.help} {
		r := titleButtonRects(width, height)[index]
		drawTitleButton(screen, r, label, false)
	}
}

func (g *Game) drawLoadScreen(screen *ebiten.Image) {
	copy := activeTitleCopy()
	width, height := g.layout.Width, g.layout.Height
	panel := image.Rect(width/2-230, height/2-205, width/2+230, height/2+205)
	drawTitlePanel(screen, panel)
	ui.DrawTitleText(screen, copy.loadTitle, float64(panel.Min.X+24), float64(panel.Min.Y+24), 2)
	hasSave := false
	for index, info := range g.slotCache {
		r := titleLoadSlotRect(width, height, index)
		label := info.Name
		if label == "" {
			label = copy.noSaves
		}
		drawTitleButton(screen, r, label, !info.Occupied)
		hasSave = hasSave || info.Occupied
	}
	if !hasSave {
		ui.DrawMenuText(screen, copy.noSaves, float64(panel.Min.X+24), float64(panel.Max.Y-78))
	}
	drawTitleButton(screen, titleBackRect(width, height), copy.back, false)
}

func (g *Game) drawHelpScreen(screen *ebiten.Image) {
	copy := activeTitleCopy()
	width, height := g.layout.Width, g.layout.Height
	panel := image.Rect(44, 34, width-44, height-34)
	drawTitlePanel(screen, panel)
	if len(g.helpPages) == 0 {
		ui.DrawTitleText(screen, copy.help, float64(panel.Min.X+24), float64(panel.Min.Y+24), 2)
		return
	}
	if g.helpPage < 0 || g.helpPage >= len(g.helpPages) {
		g.helpPage = 0
	}
	page := g.helpPages[g.helpPage]
	ui.DrawTitleText(screen, page.title, float64(panel.Min.X+24), float64(panel.Min.Y+22), 2)
	ui.DrawMenuText(screen, strings.TrimSpace(strings.Join([]string{itoa(g.helpPage + 1), "/", itoa(len(g.helpPages))}, "")), float64(panel.Max.X-62), float64(panel.Min.Y+28))

	y := panel.Min.Y + 68
	for _, line := range page.lines {
		if y >= panel.Max.Y-74 {
			break
		}
		switch line.kind {
		case helpLineHeading:
			ui.DrawTitleText(screen, line.text, float64(panel.Min.X+28), float64(y), 1.35)
			y += 24
		case helpLineImage:
			if drawHelpAsset(screen, line.image, panel.Min.X+28, y, 54) {
				ui.DrawMenuText(screen, line.text, float64(panel.Min.X+94), float64(y+17))
				y += 60
			}
		default:
			for _, row := range wrapHelpText(line.text, panel.Dx()-58) {
				ui.DrawMenuText(screen, row, float64(panel.Min.X+28), float64(y))
				y += 13
			}
			y += 5
		}
	}
	back, previous, next := helpNavRects(width, height)
	drawTitleButton(screen, back, copy.back, false)
	drawTitleButton(screen, previous, copy.previous, g.helpPage == 0)
	drawTitleButton(screen, next, copy.next, g.helpPage+1 >= len(g.helpPages))
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	out := ""
	for value > 0 {
		out = string(rune('0'+value%10)) + out
		value /= 10
	}
	return out
}

func wrapHelpText(value string, width int) []string {
	maxRunes := width / 8
	if maxRunes < 20 {
		maxRunes = 20
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		return nil
	}
	rows := make([]string, 0, 3)
	row := ""
	for _, word := range words {
		candidate := word
		if row != "" {
			candidate = row + " " + word
		}
		if len([]rune(candidate)) > maxRunes && row != "" {
			rows = append(rows, row)
			row = word
			continue
		}
		row = candidate
	}
	if row != "" {
		rows = append(rows, row)
	}
	return rows
}

func drawHelpAsset(screen *ebiten.Image, source string, x, y, size int) bool {
	name := source
	if index := strings.LastIndex(source, "/"); index >= 0 {
		name = source[index+1:]
	}
	var img *ebiten.Image
	switch name {
	case "building_farm.png":
		img = assets.FarmHouse
	case "building_mill.png":
		img = assets.MillFrames[0]
	case "building_bakery.png":
		img = assets.Bakery
	case "building_tavern.png":
		img = assets.Tavern
	case "building_warehouse_v2.png":
		img = assets.Warehouse
	case "building_winery.png":
		img = assets.Winery
	case "building_fisher_hut.png":
		img = assets.FisherHutFrames[0]
	case "building_lumberjack_hut.png":
		img = assets.LumberjackHut
	case "building_quarry_hut.png":
		img = assets.QuarryHut
	case "building_miner_hut.png":
		img = assets.MinerHut
	case "building_carpentry_workshop.png":
		img = assets.CarpentryWorkshop
	case "building_pig_farm.png":
		img = assets.PigFarm
	case "building_meat_workshop.png":
		img = assets.MeatWorkshop
	case "building_smeltery.png":
		img = assets.Smeltery
	case "construction_foundation.png":
		img = assets.ConstructionFoundation
	}
	if img == nil {
		return false
	}
	bounds := img.Bounds()
	scale := float64(size) / float64(bounds.Dy())
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	op.Blend = ebiten.BlendSourceOver
	screen.DrawImage(img, op)
	return true
}

func titleButtonRects(width, height int) []image.Rectangle {
	x := 70
	w := width / 4
	if w < 240 {
		w = 240
	}
	if w > 340 {
		w = 340
	}
	y := height/2 - 30
	return []image.Rectangle{
		image.Rect(x, y, x+w, y+42),
		image.Rect(x, y+52, x+w, y+94),
		image.Rect(x, y+104, x+w, y+146),
	}
}

func titleActionAt(x, y, width, height int) (titleAction, bool) {
	point := image.Pt(x, y)
	for index, rect := range titleButtonRects(width, height) {
		if point.In(rect) {
			return titleAction(index), true
		}
	}
	return titleActionNewGame, false
}

func titleBackRect(width, height int) image.Rectangle {
	return image.Rect(width/2-74, height-72, width/2+74, height-36)
}

func titleLoadSlotRect(width, height, index int) image.Rectangle {
	x := width/2 - 204
	y := height/2 - 148 + index*50
	return image.Rect(x, y, x+408, y+38)
}

func titleLoadSlotAt(x, y, width, height, count int) (int, bool) {
	point := image.Pt(x, y)
	for index := 0; index < count; index++ {
		if point.In(titleLoadSlotRect(width, height, index)) {
			return index + 1, true
		}
	}
	return 0, false
}

func helpNavRects(width, height int) (back, previous, next image.Rectangle) {
	back = image.Rect(64, height-76, 208, height-38)
	previous = image.Rect(width-324, height-76, width-184, height-38)
	next = image.Rect(width-168, height-76, width-44, height-38)
	return
}

func drawTitlePanel(screen *ebiten.Image, rect image.Rectangle) {
	vector.FillRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), color.RGBA{R: 30, G: 25, B: 24, A: 235}, false)
	vector.StrokeRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), 2, color.RGBA{R: 184, G: 132, B: 55, A: 230}, false)
}

func drawTitleButton(screen *ebiten.Image, rect image.Rectangle, label string, disabled bool) {
	fill := color.RGBA{R: 124, G: 82, B: 39, A: 238}
	border := color.RGBA{R: 224, G: 168, B: 68, A: 255}
	if disabled {
		fill = color.RGBA{R: 58, G: 49, B: 46, A: 215}
		border = color.RGBA{R: 97, G: 79, B: 63, A: 220}
	}
	vector.FillRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), fill, false)
	vector.StrokeRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), 2, border, false)
	ui.DrawTitleText(screen, label, float64(rect.Min.X+14), float64(rect.Min.Y+11), 1.35)
}
