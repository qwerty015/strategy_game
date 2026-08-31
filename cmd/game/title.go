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
	"strategy_game/internal/villagers"
	"strategy_game/internal/world"
	"strategy_game/internal/worldclock"
)

const (
	// The title map is intentionally much wider than a typical window at 200%.
	// The camera sweeps across it and reverses before either map edge is shown.
	titleMapWidth          = 168
	titleMapHeight         = 74
	titleMapZoom           = 2.00
	titleCameraPeriod      = 5400 // 90 seconds at 60 fps, left → right → left
	titleCameraMarginTiles = 5
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
	titleActionExit
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
			title: i18n.T().WindowTitle, subtitle: "Build a living settlement",
			newGame: "New game", load: "Load game", help: "Help",
			back: "Back", previous: "Previous", next: "Next",
			loadTitle: "Load a save", noSaves: "No saved games yet",
		}
	}
	return titleCopy{
		title: i18n.T().WindowTitle, subtitle: "Постройте живое поселение",
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

// NewApplication creates a non-simulating, self-contained presentation map.
// It never reads, writes or reserves a player save slot: the title town is a
// hand-authored showcase inspired by the first demo settlement's food,
// workshop and mining districts. NewGame remains the normal gameplay factory.
func NewApplication() *Game {
	game := newGameWithSize(titleMapWidth, titleMapHeight)
	game.screen = screenTitle
	game.helpPages = parseHelpMarkdown(gamehelp.HelpMarkdown)
	game.populateTitleTown()
	game.camera.Scale = titleMapZoom
	game.camera.SetViewport(0, 0, screenWidth, screenHeight)
	game.positionTitleCamera()
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
	frontWidth, frontHeight := g.frontScreenDimensions()
	g.camera.SetViewport(0, 0, frontWidth, frontHeight)
	g.advanceTitleCamera()

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		// The title screen must never close the application accidentally.
		// Esc only returns from its secondary screens; explicit Exit is a button.
		if g.screen != screenTitle {
			g.screen = screenTitle
		}
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
		switch action, ok := titleActionAt(mx, my, frontWidth, frontHeight); {
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
		case action == titleActionExit:
			return ebiten.Termination
		}
	case screenLoad:
		if image.Pt(mx, my).In(titleBackRect(frontWidth, frontHeight)) {
			g.screen = screenTitle
			return nil
		}
		if slot, ok := titleLoadSlotAt(mx, my, frontWidth, frontHeight, len(g.slotCache)); ok && g.slotCache[slot-1].Occupied {
			if err := g.loadGame(slotPath(slot)); err == nil {
				g.screen = screenPlay
				g.statusMsg = i18n.T().Loaded
			} else {
				g.statusMsg = i18n.T().LoadFailedPrefix + err.Error()
			}
		}
	case screenHelp:
		back, previous, next := helpNavRects(frontWidth, frontHeight)
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

// frontScreenDimensions returns the actual dimensions last used to draw the
// title UI. Ebiten can report a stale logical WindowSize in fullscreen, while
// input coordinates are in the draw-buffer coordinate system.
func (g *Game) frontScreenDimensions() (int, int) {
	if g.frontWidth > 0 && g.frontHeight > 0 {
		return g.frontWidth, g.frontHeight
	}
	return g.layout.Width, g.layout.Height
}
func (g *Game) advanceTitleCamera() {
	if g.grid == nil || g.camera.Scale <= 0 {
		return
	}
	g.positionTitleCamera()
}

// positionTitleCamera moves through the wide showcase in a smooth ping-pong
// cycle. It is intentionally not connected to map input or save-state camera
// data, so the presentation cannot expose an edge or alter a player's save.
func (g *Game) positionTitleCamera() {
	if g.grid == nil || g.camera.Scale <= 0 {
		return
	}
	viewWidth := float64(g.layout.Width) / g.camera.Scale
	viewHeight := float64(g.layout.Height) / g.camera.Scale
	worldWidth := float64(g.grid.Width * render.TileSize)
	worldHeight := float64(g.grid.Height * render.TileSize)
	g.camera.X = titleCameraAxis(g.titleFrame, worldWidth, viewWidth, 0)
	// Keep the showcase on the settlement band. The former independent vertical
	// sweep often travelled over a road-only row while the actual town was just
	// above or below it, which made the title screen look empty.
	showcaseY := float64(20 * render.TileSize)
	maxY := worldHeight - viewHeight
	if maxY < 0 {
		g.camera.Y = maxY / 2
	} else if showcaseY > maxY {
		g.camera.Y = maxY
	} else {
		g.camera.Y = showcaseY
	}
}

// titleCameraAxis returns an in-bounds coordinate that travels to the far
// side of an axis and then back. The cosine easing removes a mechanical hard
// turn at either end while retaining the requested endless left/right feel.
func titleCameraAxis(frame int, worldPixels, viewPixels float64, phase int) float64 {
	margin := float64(titleCameraMarginTiles * render.TileSize)
	minimum := margin
	maximum := worldPixels - viewPixels - margin
	if maximum <= minimum {
		return (worldPixels - viewPixels) / 2
	}
	cycle := float64((frame+phase)%titleCameraPeriod) / titleCameraPeriod
	progress := (1 - math.Cos(cycle*2*math.Pi)) / 2
	return minimum + (maximum-minimum)*progress
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

// populateTitleTown builds a deliberately authored demonstration settlement.
// It is independent from saves: clearing slot 1, moving the saves directory or
// starting on a new computer cannot change this map. Its layout echoes the
// player's demo: several farms and vineyards, a busy central store, then wood,
// stone and ore work further along the road, with fishing by the shoreline.
func (g *Game) populateTitleTown() {
	g.grid = world.NewGrid(titleMapWidth, titleMapHeight)
	g.buildings = nil

	// A calm irregular coast frames the settlement without cutting through it.
	for x := 0; x < g.grid.Width; x++ {
		shore := 61 + (x*7+x/9)%3
		for y := shore; y < g.grid.Height; y++ {
			g.grid.Set(x, y, world.Tile{Terrain: world.Water})
		}
	}

	roads := make(map[titlePoint]struct{}, 256)
	road := func(x, y int) {
		point := titlePoint{x: x, y: y}
		if !g.grid.InBounds(x, y) || g.grid.At(x, y).Terrain == world.Water {
			return
		}
		if _, exists := roads[point]; exists {
			return
		}
		roads[point] = struct{}{}
		g.buildings = append(g.buildings, &building.Building{Kind: building.Road, X: x, Y: y})
	}
	hRoad := func(y, from, to int) {
		for x := from; x <= to; x++ {
			road(x, y)
		}
	}
	vRoad := func(x, from, to int) {
		for y := from; y <= to; y++ {
			road(x, y)
		}
	}

	// The long central spine lets the camera discover a new district as it
	// travels, rather than showing one noisy random pile of buildings.
	hRoad(35, 4, 163)
	hRoad(57, 4, 163)
	for _, x := range []int{10, 18, 29, 39, 51, 70, 86, 103, 118, 132, 146, 158} {
		vRoad(x, 8, 57)
	}
	hRoad(9, 10, 70)
	hRoad(20, 10, 70)
	hRoad(47, 4, 158)

	add := func(kind building.Kind, x, y int) *building.Building {
		b := &building.Building{Kind: kind, X: x, Y: y}
		if recipe := building.Types[kind].Recipe; recipe.TicksToProduce > 0 {
			// Mature fields and active furnaces make the title town look lived-in
			// even though its economy is intentionally frozen.
			b.ProgressTicks = recipe.TicksToProduce * 4 / 5
		}
		g.buildings = append(g.buildings, b)
		return b
	}
	addTree := func(x, y int) {
		tree := building.NewTree(x, y)
		tree.GrowthTicks = tree.GrowthTargetTicks
		g.buildings = append(g.buildings, tree)
	}
	addOre := func(kind building.Kind, x, y int) {
		g.buildings = append(g.buildings, building.NewOreDeposit(kind, x, y))
	}

	// Food district: wheat, wine, milling, baking and a pig farm as in the
	// reference settlement, laid out as readable blocks instead of a copy of
	// the save itself.
	farmHomes := make([]*building.Building, 0, 3)
	for _, point := range []titlePoint{{12, 23}, {21, 23}, {12, 27}} {
		farmHomes = append(farmHomes, add(building.Farm, point.x, point.y))
	}
	wineryHomes := make([]*building.Building, 0, 3)
	for _, point := range []titlePoint{{42, 23}, {53, 23}, {42, 27}} {
		wineryHomes = append(wineryHomes, add(building.Winery, point.x, point.y))
	}
	// Short spurs from each door to the nearest spine/cross-street tile --
	// without these none of the food-district buildings actually touch the
	// road network (every AccessX/AccessY defaults to 0,0, i.e. the door is
	// the building's own top-left tile), which is what made the showcase
	// read as a disconnected, "dead" town rather than a working one. Each
	// spur stops one tile short of the building's own footprint -- a road
	// tile must sit next to the door, not on top of it.
	hRoad(23, 10, 11)
	hRoad(23, 18, 20)
	hRoad(27, 10, 11)
	vRoad(42, 20, 22)
	hRoad(23, 51, 52)
	hRoad(27, 39, 41)

	add(building.Mill, 64, 31)
	add(building.Bakery, 68, 31)
	add(building.PigFarm, 74, 31)
	add(building.MeatWorkshop, 79, 31)

	// Central exchange: warehouse/tavern pair, plus a second processing row.
	add(building.Warehouse, 76, 31)
	add(building.Tavern, 82, 31)
	add(building.Mill, 88, 31)
	add(building.Bakery, 92, 31)
	add(building.PigFarm, 97, 31)
	add(building.MeatWorkshop, 101, 31)

	// Same reasoning as the food district: this whole row sits between the
	// y=20 and y=35 spines, so each door gets a stub either down to y=35 or
	// sideways to the nearest north-south column, again stopping one tile
	// short of the building itself.
	vRoad(64, 32, 35)
	hRoad(31, 69, 70)
	hRoad(31, 70, 73)
	vRoad(79, 32, 35)
	vRoad(76, 32, 35)
	hRoad(31, 83, 86)
	hRoad(31, 86, 87)
	vRoad(92, 32, 35)
	vRoad(97, 32, 35)
	hRoad(31, 102, 103)

	// Workshops and raw materials form the final third of the tour.
	add(building.LumberjackHut, 108, 31)
	add(building.CarpentryWorkshop, 113, 31)
	add(building.QuarryHut, 124, 31)
	add(building.MinerHut, 138, 31)
	add(building.Smeltery, 143, 31)
	vRoad(108, 32, 35)
	vRoad(113, 32, 35)
	vRoad(124, 32, 35)
	vRoad(138, 32, 35)
	hRoad(31, 144, 146)
	for _, point := range []titlePoint{{105, 16}, {110, 18}, {114, 14}, {118, 20}, {121, 16}, {116, 25}, {111, 28}} {
		addTree(point.x, point.y)
	}
	for _, point := range []titlePoint{{126, 18}, {129, 19}, {132, 17}, {127, 22}, {131, 23}} {
		g.buildings = append(g.buildings, building.NewStoneDeposit(point.x, point.y))
	}
	for _, point := range []titlePoint{{145, 18}, {148, 18}, {150, 21}, {154, 20}} {
		addOre(building.CoalDeposit, point.x, point.y)
	}
	for _, point := range []titlePoint{{151, 25}, {155, 26}} {
		addOre(building.IronOreDeposit, point.x, point.y)
	}
	addOre(building.GoldOreDeposit, 158, 22)

	// Fisher huts look out over the water while the road remains on land.
	// Each one gets a stub straight up to the y=57 spine, stopping one tile
	// above its own door, so it isn't just standing alone at the shoreline.
	for _, x := range []int{16, 45, 92, 119, 151} {
		shore := 61 + (x*7+x/9)%3
		add(building.FisherHut, x, shore-1)
		vRoad(x, 57, shore-2)
	}
	// Title workers are real renderable units, but the title never advances the
	// economy. Farmers and winemakers animate among the mature fields while
	// serfs stand along the road, making the showcase read as a living town.
	if g.vills != nil {
		for _, home := range farmHomes {
			g.vills.Spawn(villagers.Farmer, home)
		}
		for _, home := range wineryHomes {
			g.vills.Spawn(villagers.Winemaker, home)
		}
	}
	if g.logi != nil {
		serfPositions := []titlePoint{{30, 35}, {58, 35}, {72, 35}, {90, 35}, {116, 35}, {136, 35}, {153, 35}}
		for index, serf := range g.logi.Serfs {
			point := serfPositions[index%len(serfPositions)]
			serf.X, serf.Y = point.x, point.y
		}
	}
	for _, point := range []titlePoint{{24, 65}, {33, 67}, {54, 64}, {97, 66}, {123, 67}, {140, 65}} {
		fish := building.NewFish(point.x, point.y)
		fish.GrowthTicks = fish.GrowthTargetTicks * 2 / 3
		g.buildings = append(g.buildings, fish)
	}
}

type titlePoint struct{ x, y int }

func (g *Game) drawFrontScreen(screen *ebiten.Image) {
	// In fullscreen Ebiten's draw buffer can be wider than the last window-size
	// value seen by Update. The title world owns every pixel, unlike gameplay's
	// center map strip, so use the actual buffer dimensions here.
	bounds := screen.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	g.frontWidth, g.frontHeight = width, height
	g.camera.SetViewport(0, 0, width, height)
	g.positionTitleCamera()

	render.Tick()
	// The showcase town always reads as a bright, midday scene -- it never
	// ticks a real simulation, so there's no meaningful "current time" to
	// show, and picking noon keeps the demo from looking artificially
	// darkened or from ever hiding buildings behind fireflies/butterflies
	// the player hasn't triggered themselves.
	render.SetWorldTicks(worldclock.TicksPerDay / 2)
	render.DrawGrid(screen, g.grid, g.camera)
	render.DrawAmbientGroundLife(screen, g.grid, g.camera)
	render.DrawBuildings(screen, g.grid, g.buildings, g.camera, map[*building.Building]bool{}, map[*building.Building]bool{})
	render.DrawSerfs(screen, g.logi.Serfs, g.camera)
	render.DrawVillagers(screen, g.vills.Villagers, g.camera)
	render.DrawAmbientSkyLife(screen, g.grid, g.camera)
	render.DrawAtmosphericOverlay(screen, g.grid, g.camera)
	vector.FillRect(screen, 0, 0, float32(width), float32(height), color.RGBA{R: 19, G: 20, B: 23, A: 124}, false)

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
	bounds := screen.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	ui.DrawTitleText(screen, copy.title, 66, float64(height)*0.17, 4)
	ui.DrawTitleText(screen, copy.subtitle, 70, float64(height)*0.17+42, 1.45)
	for index, label := range []string{copy.newGame, copy.load, copy.help, i18n.T().ExitButton} {
		r := titleButtonRects(width, height)[index]
		drawTitleButton(screen, r, label, false)
	}
}

func (g *Game) drawLoadScreen(screen *ebiten.Image) {
	copy := activeTitleCopy()
	bounds := screen.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
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
	bounds := screen.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
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
	case "building_farm.png", "building_farm_v2.png":
		img = assets.FarmHouse
	case "building_mill.png":
		img = assets.MillFrames[0]
	case "building_bakery.png", "building_bakery_v2.png":
		img = assets.Bakery
	case "building_tavern.png", "building_tavern_v2.png":
		img = assets.Tavern
	case "building_warehouse_v2.png", "building_warehouse_v3.png":
		img = assets.Warehouse
	case "building_winery.png", "building_winery_v2.png":
		img = assets.Winery
	case "building_fisher_hut.png", "building_fisher_hut_v2.png":
		img = assets.FisherHutFrames[0]
	case "building_lumberjack_hut.png", "building_lumberjack_hut_v2.png":
		img = assets.LumberjackHut
	case "building_quarry_hut.png", "building_quarry_hut_v2.png":
		img = assets.QuarryHut
	case "building_miner_hut.png", "building_miner_hut_v2.png":
		img = assets.MinerHut
	case "building_carpentry_workshop.png", "building_carpentry_workshop_v2.png":
		img = assets.CarpentryWorkshop
	case "building_pig_farm.png", "building_pig_farm_v2.png":
		img = assets.PigFarm
	case "building_meat_workshop.png", "building_meat_workshop_v2.png":
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
		image.Rect(x, y+156, x+w, y+198),
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
