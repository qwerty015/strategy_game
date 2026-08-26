package ui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/i18n"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/render"
	"strategy_game/internal/resource"
	"strategy_game/internal/villagers"
)

var (
	panelColor      = color.RGBA{R: 30, G: 25, B: 26, A: 238}
	panelInnerColor = color.RGBA{R: 54, G: 43, B: 38, A: 245}
	panelEdgeColor  = color.RGBA{R: 151, G: 111, B: 64, A: 255}
	selectedColor   = color.RGBA{R: 179, G: 126, B: 48, A: 255}
	mutedTextColor  = color.RGBA{R: 201, G: 190, B: 168, A: 255}
)

func drawPanel(screen *ebiten.Image, r imageRect, title string) {
	vector.FillRect(screen, float32(r.x), float32(r.y), float32(r.w), float32(r.h), panelColor, false)
	vector.FillRect(screen, float32(r.x+5), float32(r.y+5), float32(r.w-10), 34, panelInnerColor, false)
	DrawText(screen, title, float64(r.x+14), float64(r.y+13))
}

// imageRect is a tiny local rectangle type to keep panel drawing independent
// from image.Rectangle arithmetic and make the intended pixel layout clear.
type imageRect struct{ x, y, w, h int }

// DrawBuildPanel renders both construction and NPC hiring in the same left
// panel. A professional card is muted when every matching workplace already
// has a resident, making the one-worker-per-building limit visible.
func DrawBuildPanel(screen *ebiten.Image, layout Layout, p *Palette, tab LeftTab, options []HireOption) {
	r := layout.LeftPanel()
	drawPanel(screen, imageRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, i18n.T().BuildMenuTitle)

	drawMenuTabs(screen, layout, tab)
	if tab == HireTab {
		drawHireCards(screen, layout, options)
		return
	}
	for i, kind := range p.Kinds {
		x, y := 12, leftCardsStartY+i*leftCardStride
		w, h := layout.LeftWidth-24, leftCardHeight
		fill := panelInnerColor
		if i == p.Selected {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)
		vector.FillRect(screen, float32(x), float32(y+h-3), float32(w), 3, panelEdgeColor, false)

		drawBuildingIcon(screen, kind, x+8, y+6, 28)
		DrawText(screen, i18n.T().BuildingName[kind], float64(x+44), float64(y+5))
		shortcut := paletteShortcut(i)
		DrawText(screen, fmt.Sprintf("%d×%d%s", building.Types[kind].Footprint, building.Types[kind].Footprint, shortcut), float64(x+50), float64(y+27))
	}
}

func drawMenuTabs(screen *ebiten.Image, layout Layout, active LeftTab) {
	labels := []string{i18n.T().BuildTab, i18n.T().HireTab}
	width := (layout.LeftWidth - 30) / 2
	for i, label := range labels {
		x := 12 + i*(width+6)
		fill := panelInnerColor
		if LeftTab(i) == active {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(x), leftTabY, float32(width), leftTabHeight, fill, false)
		DrawText(screen, label, float64(x+8), float64(leftTabY+7))
	}
}

func drawHireCards(screen *ebiten.Image, layout Layout, options []HireOption) {
	for i, option := range options {
		x, y := 12, leftCardsStartY+i*leftCardStride
		w, h := layout.LeftWidth-24, leftCardHeight
		fill := panelInnerColor
		if !option.Available {
			fill = color.RGBA{R: 69, G: 50, B: 48, A: 245}
		}
		vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)
		vector.FillRect(screen, float32(x), float32(y+h-3), float32(w), 3, panelEdgeColor, false)

		drawHireIcon(screen, option.Kind, x+8, y+6, 28)
		DrawText(screen, hireName(option.Kind), float64(x+44), float64(y+5))
		count := fmt.Sprintf("%d", option.Current)
		if option.Limit > 0 {
			count = fmt.Sprintf("%d/%d", option.Current, option.Limit)
		}
		DrawText(screen, count, float64(x+44), float64(y+21))
	}
}

func hireName(kind HireKind) string {
	t := i18n.T()
	switch kind {
	case HireFarmer:
		return t.UnitFarmer
	case HireBaker:
		return t.UnitBaker
	case HireWinemaker:
		return t.UnitWinemaker
	case HireLumberjack:
		return t.UnitLumberjack
	case HireFisherman:
		return t.UnitFisherman
	case HireSwineherd:
		return t.UnitSwineherd
	case HireButcher:
		return t.UnitButcher
	default:
		return t.UnitSerf
	}
}

func drawHireIcon(screen *ebiten.Image, kind HireKind, x, y, size int) {
	var img *ebiten.Image
	switch kind {
	case HireFarmer:
		img = assets.Farmer[0]
	case HireBaker:
		img = assets.Baker[0]
	case HireWinemaker:
		img = assets.Winemaker[0]
	case HireLumberjack:
		img = assets.Lumberjack[0]
	case HireFisherman:
		img = assets.Fisherman[0]
	case HireSwineherd:
		img = assets.Swineherd[0]
	case HireButcher:
		img = assets.Butcher[0]
	default:
		img = assets.Serf[0]
	}
	b := img.Bounds()
	scale := float64(size) / float64(b.Dy())
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	op.Blend = ebiten.BlendSourceOver
	screen.DrawImage(img, op)
}

func paletteShortcut(index int) string {
	switch {
	case index < 9:
		return fmt.Sprintf("  [%d]", index+1)
	case index == 9:
		return "  [0]"
	case index == 10:
		return "  [Q]"
	default:
		return ""
	}
}

// DrawInspectorPanel renders the currently selected object. It reads only
// public accessors from the logic packages, keeping display formatting out of
// the simulation.
func DrawInspectorPanel(screen *ebiten.Image, layout Layout, selection Selection, connected bool, stock *resource.Stockpile, occupants int, showPriority bool, priorityLevel int) {
	r := layout.RightPanel()
	drawPanel(screen, imageRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, i18n.T().InspectorTitle)
	if selection.Kind == SelectionNone {
		DrawText(screen, i18n.T().InspectorHint, float64(r.Min.X+18), 62)
		return
	}

	switch selection.Kind {
	case SelectionBuilding:
		drawBuildingInspector(screen, r.Min.X+18, 62, selection.Building, connected, stock, occupants)
	case SelectionSerf:
		drawSerfInspector(screen, r.Min.X+18, 62, selection.Serf)
	case SelectionVillager:
		drawVillagerInspector(screen, r.Min.X+18, 62, selection.Villager)
	case SelectionLumberjack:
		drawLumberjackInspector(screen, r.Min.X+18, 62, selection.Lumberjack)
	case SelectionFisherman:
		drawFishermanInspector(screen, r.Min.X+18, 62, selection.Fisherman)
	}
	if showPriority {
		drawPriorityControl(screen, layout, priorityLevel)
	}
}

// drawPriorityControl draws the five-segment supply-priority slider docked
// at the bottom of the inspector panel, for a building kind that actually
// competes for a limited input (see cmd/game's eligibility check -- only
// building kinds with a non-empty Recipe.Inputs get this control). The
// selected segment is the currently set level (see
// logistics.Controller.SetPriority); Layout.PriorityLevelAt hit-tests the
// identical geometry, so the two can never drift apart.
func drawPriorityControl(screen *ebiten.Image, layout Layout, current int) {
	r := layout.RightPanel()
	rowY := r.Max.Y - priorityRowHeight - priorityBottomGap
	startX := r.Min.X + priorityMargin
	segW := (r.Dx() - 2*priorityMargin) / 5

	t := i18n.T()
	DrawText(screen, t.PriorityLabel, float64(r.Min.X+priorityMargin), float64(rowY-18))

	labels := [5]string{"--", "-", "•", "+", "++"}
	for i, label := range labels {
		level := i - 2
		x := startX + i*segW
		fill := panelInnerColor
		if level == current {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(x), float32(rowY), float32(segW-2), float32(priorityRowHeight), fill, false)
		DrawText(screen, label, float64(x+segW/2-6), float64(rowY+7))
	}
}

func drawBuildingInspector(screen *ebiten.Image, x, y int, b *building.Building, connected bool, stock *resource.Stockpile, occupants int) {
	t := i18n.T()
	bt := building.Types[b.Kind]
	DrawText(screen, t.BuildingName[b.Kind], float64(x), float64(y))
	y += 24
	if b.Kind == building.Tree {
		DrawText(screen, fmt.Sprintf("%s: %d%%", t.GrowthLabel, int(b.GrowthProgress()*100)), float64(x), float64(y))
		y += 20
		DrawText(screen, t.HarvestableLabel, float64(x), float64(y))
		return
	}
	if b.Kind == building.Fish {
		DrawText(screen, fmt.Sprintf("%s: %d%%", t.GrowthLabel, int(b.GrowthProgress()*100)), float64(x), float64(y))
		y += 20
		if b.GrowthStage() >= 2 {
			DrawText(screen, t.CatchableLabel, float64(x), float64(y))
		}
		return
	}
	// Every real building, from here on: how many units currently stand
	// on its footprint. Not just an assigned resident -- a serf mid-drop
	// off or a hungry unit eating at the Tavern counts too, so it works
	// the same way on buildings that never have a dedicated resident
	// (Tavern, Warehouse) as it does on a workplace (Farm, Bakery, ...).
	DrawText(screen, fmt.Sprintf("%s: %d", t.PeopleInsideLabel, occupants), float64(x), float64(y))
	y += 20
	if b.Kind == building.LumberjackHut {
		DrawText(screen, t.ContentsLabel, float64(x), float64(y))
		y += 20
		DrawText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.Log], b.OutputBuffer[resource.Log], building.BufferCapacity), float64(x), float64(y))
		y += 20
		routeState := t.Disconnected
		if connected {
			routeState = t.Connected
		}
		DrawText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, routeState), float64(x), float64(y))
		return
	}
	if b.Kind == building.FisherHut {
		DrawText(screen, t.ContentsLabel, float64(x), float64(y))
		y += 20
		DrawText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.Fish], b.OutputBuffer[resource.Fish], building.BufferCapacity), float64(x), float64(y))
		y += 20
		routeState := t.Disconnected
		if connected {
			routeState = t.Connected
		}
		DrawText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, routeState), float64(x), float64(y))
		return
	}
	if b.Kind == building.Warehouse && stock != nil {
		DrawText(screen, t.ContentsLabel, float64(x), float64(y))
		y += 20
		for _, rt := range resource.AllTypes() {
			capacity := fmt.Sprintf("%d", stock.Capacity)
			if stock.Capacity <= 0 {
				capacity = t.UnlimitedLabel
			}
			DrawText(screen, fmt.Sprintf("%s: %d/%s", t.ResourceName[rt], stock.Amount(rt), capacity), float64(x), float64(y))
			y += 18
		}
	}
	if bt.Recipe.TicksToProduce > 0 {
		DrawText(screen, fmt.Sprintf("%s: %d/%d", t.StateLabel, b.ProgressTicks, bt.Recipe.TicksToProduce), float64(x), float64(y))
		y += 20
	}
	inputTypes := recipeInputTypes(bt)
	if len(bt.AcceptedResources) > 0 {
		inputTypes = bt.AcceptedResources
	}
	if len(inputTypes) > 0 {
		DrawText(screen, t.InputLabel+":", float64(x), float64(y))
		y += 18
		for _, rt := range inputTypes {
			DrawText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.InputBuffer[rt], building.BufferCapacity), float64(x+8), float64(y))
			y += 18
		}
	}
	if bt.Recipe.TicksToProduce > 0 {
		DrawText(screen, t.OutputLabel+":", float64(x), float64(y))
		y += 18
		DrawText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[bt.Recipe.Output], b.OutputBuffer[bt.Recipe.Output], b.OutputLimit()), float64(x+8), float64(y))
		y += 18
	}
	roadState := t.Disconnected
	if connected || b.Kind == building.Warehouse || b.Kind == building.Road {
		roadState = t.Connected
	}
	DrawText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, roadState), float64(x), float64(y))
}

func recipeInputTypes(bt building.Type) []resource.Type {
	if len(bt.Recipe.Inputs) == 0 {
		return nil
	}
	out := make([]resource.Type, 0, len(bt.Recipe.Inputs))
	for _, rt := range resource.AllTypes() {
		if _, ok := bt.Recipe.Inputs[rt]; ok {
			out = append(out, rt)
		}
	}
	return out
}

func drawSerfInspector(screen *ebiten.Image, x, y int, s *logistics.Serf) {
	t := i18n.T()
	DrawText(screen, t.UnitSerf, float64(x), float64(y))
	y += 24
	state := t.StateIdle
	if s.Dismissing() {
		state = t.StateLeaving
	} else if s.Eating() {
		state = t.StateEating
	} else {
		switch s.State() {
		case logistics.SerfToPickup:
			state = t.StateWalking
		case logistics.SerfToDropoff:
			state = t.StateDelivering
		}
	}
	if s.Starving {
		state += " (" + t.StateStarving + ")"
	}
	DrawText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20

	rt, amount := s.Cargo()
	cargo := t.NoCargo
	if amount > 0 {
		cargo = fmt.Sprintf("%s × %d", t.ResourceName[rt], amount)
	}
	DrawText(screen, fmt.Sprintf("%s: %s", t.CargoLabel, cargo), float64(x), float64(y))
	y += 20

	from, to := s.PickupBuilding(), s.DropoffBuilding()
	if from != nil || to != nil {
		fromName, toName := t.NoRoute, t.NoRoute
		if from != nil {
			fromName = t.BuildingName[from.Kind]
		}
		if to != nil {
			toName = t.BuildingName[to.Kind]
		}
		DrawText(screen, fmt.Sprintf("%s: %s → %s", t.RouteLabel, fromName, toName), float64(x), float64(y))
		y += 20
	}
	DrawText(screen, fmt.Sprintf("%s: %d%%", t.HungerLabel, s.SatietyPercent()), float64(x), float64(y))
}

func drawVillagerInspector(screen *ebiten.Image, x, y int, v *villagers.Villager) {
	t := i18n.T()
	profession := t.UnitFarmer
	switch v.Profession {
	case villagers.Baker:
		profession = t.UnitBaker
	case villagers.Winemaker:
		profession = t.UnitWinemaker
	case villagers.Swineherd:
		profession = t.UnitSwineherd
	case villagers.Butcher:
		profession = t.UnitButcher
	}
	DrawText(screen, profession, float64(x), float64(y))
	y += 24

	state := t.StateWorking
	switch v.State() {
	case villagers.VillagerToTavern, villagers.VillagerToHome:
		state = t.StateWalking
	}
	if v.Starving {
		state += " (" + t.StateStarving + ")"
	}
	DrawText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20
	if v.HomeBuilding() != nil {
		DrawText(screen, fmt.Sprintf("%s: %s", t.HomeLabel, t.BuildingName[v.HomeBuilding().Kind]), float64(x), float64(y))
		y += 20
	}
	DrawText(screen, fmt.Sprintf("%s: %d%%", t.HungerLabel, v.SatietyPercent()), float64(x), float64(y))
}

func drawLumberjackInspector(screen *ebiten.Image, x, y int, j *lumberjack.Lumberjack) {
	t := i18n.T()
	DrawText(screen, t.UnitLumberjack, float64(x), float64(y))
	y += 24

	state := t.StateIdle
	switch j.State() {
	case lumberjack.StateToTree:
		state = t.StateWalking
	case lumberjack.StateChopping:
		state = t.StateChopping
	case lumberjack.StateToHome:
		state = t.StateDelivering
	case lumberjack.StateUnloading:
		state = t.StateUnloading
	case lumberjack.StateToTavern, lumberjack.StateToHomeAfterMeal:
		state = t.StateWalking
	}
	if j.Starving {
		state += " (" + t.StateStarving + ")"
	}
	DrawText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20

	_, amount := j.Cargo()
	cargo := t.NoCargo
	if amount > 0 {
		cargo = fmt.Sprintf("%s × %d", t.ResourceName[resource.Log], amount)
	}
	DrawText(screen, fmt.Sprintf("%s: %s", t.CargoLabel, cargo), float64(x), float64(y))
	y += 20

	target := t.NoRoute
	if j.TargetTree() != nil {
		target = t.BuildingName[building.Tree]
	}
	DrawText(screen, fmt.Sprintf("%s: %s → %s", t.RouteLabel, target, t.BuildingName[building.LumberjackHut]), float64(x), float64(y))
	y += 20
	DrawText(screen, fmt.Sprintf("%s: %d/%d", t.HungerLabel, min(j.HungerTicks(), lumberjack.HungerInterval), lumberjack.HungerInterval), float64(x), float64(y))
}

func drawFishermanInspector(screen *ebiten.Image, x, y int, f *fishing.Fisherman) {
	t := i18n.T()
	DrawText(screen, t.UnitFisherman, float64(x), float64(y))
	y += 24

	state := t.StateIdle
	switch f.State() {
	case fishing.StateToFish, fishing.StateToHome, fishing.StateToTavern, fishing.StateToHomeAfterMeal:
		state = t.StateWalking
	case fishing.StateFishing:
		state = t.StateFishing
	case fishing.StateUnloading:
		state = t.StateUnloading
	}
	if f.Starving {
		state += " (" + t.StateStarving + ")"
	}
	DrawText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20

	_, amount := f.Cargo()
	cargo := t.NoCargo
	if amount > 0 {
		cargo = fmt.Sprintf("%s × %d", t.ResourceName[resource.Fish], amount)
	}
	DrawText(screen, fmt.Sprintf("%s: %s", t.CargoLabel, cargo), float64(x), float64(y))
	y += 20

	target := t.NoRoute
	if f.TargetFish() != nil {
		target = t.BuildingName[building.Fish]
	}
	DrawText(screen, fmt.Sprintf("%s: %s → %s", t.RouteLabel, target, t.BuildingName[building.FisherHut]), float64(x), float64(y))
	y += 20
	DrawText(screen, fmt.Sprintf("%s: %d/%d", t.HungerLabel, min(f.HungerTicks(), fishing.HungerInterval), fishing.HungerInterval), float64(x), float64(y))
}

// DrawSelectionMarker draws a warm outline under the selected object so the
// player can connect the inspector to the world even when sprites overlap.
func DrawSelectionMarker(screen *ebiten.Image, cam *render.Camera, selection Selection) {
	var x, y float64
	size := cam.TilePixels()
	if selection.Kind == SelectionBuilding && selection.Building != nil {
		x, y = cam.TileToScreen(selection.Building.X, selection.Building.Y)
		size *= float64(building.Types[selection.Building.Kind].Footprint)
	} else if selection.Kind == SelectionSerf && selection.Serf != nil {
		x, y = cam.TileToScreen(selection.Serf.X, selection.Serf.Y)
	} else if selection.Kind == SelectionVillager && selection.Villager != nil {
		x, y = cam.TileToScreen(selection.Villager.X, selection.Villager.Y)
	} else if selection.Kind == SelectionLumberjack && selection.Lumberjack != nil {
		x, y = cam.TileToScreen(selection.Lumberjack.X, selection.Lumberjack.Y)
	} else if selection.Kind == SelectionFisherman && selection.Fisherman != nil {
		x, y = cam.TileToScreen(selection.Fisherman.X, selection.Fisherman.Y)
	} else {
		return
	}

	line := color.RGBA{R: 245, G: 201, B: 72, A: 255}
	thickness := float32(2)
	vector.FillRect(screen, float32(x), float32(y), float32(size), thickness, line, false)
	vector.FillRect(screen, float32(x), float32(y+size-2), float32(size), thickness, line, false)
	vector.FillRect(screen, float32(x), float32(y), thickness, float32(size), line, false)
	vector.FillRect(screen, float32(x+size-2), float32(y), thickness, float32(size), line, false)
}

// DrawAccessMarker marks the only tile where a road can serve a building.
// Green means a road path to the Warehouse exists; red means the building is
// currently disconnected from the town.
func DrawAccessMarker(screen *ebiten.Image, cam *render.Camera, b *building.Building, connected bool) {
	p := b.AccessPoint()
	DrawAccessMarkerAt(screen, cam, p.X, p.Y, connected)
}

// DrawAccessMarkerAt is the placement-preview variant of DrawAccessMarker.
func DrawAccessMarkerAt(screen *ebiten.Image, cam *render.Camera, x, y int, connected bool) {
	marker := color.RGBA{R: 214, G: 63, B: 55, A: 230}
	if connected {
		marker = color.RGBA{R: 76, G: 205, B: 112, A: 230}
	}
	drawAccessMarker(screen, cam, x, y, marker)
}

// DrawPlacementAccessMarker marks the door/access tile of a building preview.
// Yellow means the location is legal, red means the footprint itself is
// invalid. A legal preview is intentionally not green: it is not connected
// to the road network until the player builds a road there.
func DrawPlacementAccessMarker(screen *ebiten.Image, cam *render.Camera, x, y int, valid bool) {
	marker := color.RGBA{R: 235, G: 181, B: 54, A: 235}
	if !valid {
		marker = color.RGBA{R: 214, G: 63, B: 55, A: 230}
	}
	drawAccessMarker(screen, cam, x, y, marker)
}

func drawAccessMarker(screen *ebiten.Image, cam *render.Camera, x, y int, marker color.RGBA) {
	sx, sy := cam.TileToScreen(x, y)
	scale := cam.TilePixels() / render.TileSize
	radius := float32(3 * scale)
	if radius < 2 {
		radius = 2
	}
	if radius > 5 {
		radius = 5
	}
	// A small door/connection dot is intentionally distinct from the worker
	// +/- marker. The old large plus looked like a green person on roofs.
	vector.FillCircle(screen, float32(sx+cam.TilePixels()/2), float32(sy+cam.TilePixels()-6*scale), radius, marker, false)
}

func DrawSpeedPanel(screen *ebiten.Image, layout Layout, speed economy.Speed) {
	r := layout.BottomPanel()
	vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), panelColor, false)
	vector.FillRect(screen, float32(layout.LeftWidth), float32(r.Min.Y+5), float32(layout.Width-layout.LeftWidth-5), 1, panelEdgeColor, false)

	startX := layout.speedStartX()
	DrawText(screen, i18n.T().SpeedTitle, float64(startX), float64(r.Min.Y+7))
	labels := []string{i18n.T().SpeedPaused, i18n.T().SpeedHalf, i18n.T().SpeedNormal, i18n.T().SpeedDouble, i18n.T().SpeedQuadruple}
	for i, label := range labels {
		x, y := startX+i*62, r.Min.Y+24
		fill := panelInnerColor
		if economy.Speed(i) == speed {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(x), float32(y), 58, 30, fill, false)
		DrawText(screen, label, float64(x+8), float64(y+8))
	}
}

// DrawUnitControls renders the small population action area beside the
// speed controls. Hiring currently has no resource cost; the visible button
// gives the action a discoverable mouse target while H remains a shortcut.
func DrawUnitControls(screen *ebiten.Image, layout Layout, serfCount int) {
	x := layout.LeftWidth + 16
	y := layout.Height - layout.BottomHeight + 20
	vector.FillRect(screen, float32(x), float32(y), 180, 34, panelInnerColor, false)
	vector.FillRect(screen, float32(x), float32(y+31), 180, 3, panelEdgeColor, false)
	DrawText(screen, i18n.T().HireSerf, float64(x+8), float64(y+8))
	DrawText(screen, fmt.Sprintf("%s: %d", i18n.T().UnitSerf, serfCount), float64(x+8), float64(y+22))
}

func drawBuildingIcon(screen *ebiten.Image, kind building.Kind, x, y, size int) {
	var img *ebiten.Image
	switch kind {
	case building.Farm:
		img = assets.FarmHouse
	case building.Mill:
		img = assets.MillFrames[0]
	case building.Bakery:
		img = assets.Bakery
	case building.Winery:
		img = assets.Winery
	case building.PigFarm:
		img = assets.PigFarm
	case building.MeatWorkshop:
		img = assets.MeatWorkshop
	case building.Tavern:
		img = assets.Tavern
	case building.Road:
		img = assets.Road
	case building.Warehouse:
		img = assets.Warehouse
	case building.LumberjackHut:
		img = assets.LumberjackHut
	case building.FisherHut:
		img = assets.FisherHutFrames[0]
	}
	if img == nil {
		return
	}

	b := img.Bounds()
	scale := float64(size) / float64(b.Dy())
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	op.Blend = ebiten.BlendSourceOver
	screen.DrawImage(img, op)
}
