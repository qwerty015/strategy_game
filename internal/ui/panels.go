package ui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
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

func DrawBuildPanel(screen *ebiten.Image, layout Layout, p *Palette) {
	r := layout.LeftPanel()
	drawPanel(screen, imageRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, i18n.T().BuildMenuTitle)

	for i, kind := range p.Kinds {
		x, y := 12, 56+i*58
		w, h := layout.LeftWidth-24, 52
		fill := panelInnerColor
		if i == p.Selected {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)
		vector.FillRect(screen, float32(x), float32(y+h-3), float32(w), 3, panelEdgeColor, false)

		drawBuildingIcon(screen, kind, x+8, y+10, 32)
		DrawText(screen, i18n.T().BuildingName[kind], float64(x+50), float64(y+10))
		DrawText(screen, fmt.Sprintf("%d×%d  [%d]", building.Types[kind].Footprint, building.Types[kind].Footprint, i+1), float64(x+50), float64(y+29))
	}
}

// DrawInspectorPanel renders the currently selected object. It reads only
// public accessors from the logic packages, keeping display formatting out of
// the simulation.
func DrawInspectorPanel(screen *ebiten.Image, layout Layout, selection Selection, connected bool, stock *resource.Stockpile) {
	r := layout.RightPanel()
	drawPanel(screen, imageRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, i18n.T().InspectorTitle)
	if selection.Kind == SelectionNone {
		DrawText(screen, i18n.T().InspectorHint, float64(r.Min.X+18), 62)
		return
	}

	switch selection.Kind {
	case SelectionBuilding:
		drawBuildingInspector(screen, r.Min.X+18, 62, selection.Building, connected, stock)
	case SelectionSerf:
		drawSerfInspector(screen, r.Min.X+18, 62, selection.Serf)
	case SelectionVillager:
		drawVillagerInspector(screen, r.Min.X+18, 62, selection.Villager)
	case SelectionLumberjack:
		drawLumberjackInspector(screen, r.Min.X+18, 62, selection.Lumberjack)
	}
}

func drawBuildingInspector(screen *ebiten.Image, x, y int, b *building.Building, connected bool, stock *resource.Stockpile) {
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
		DrawText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[bt.Recipe.Output], b.OutputBuffer[bt.Recipe.Output], building.BufferCapacity), float64(x+8), float64(y))
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
	if s.Eating() {
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
	DrawText(screen, fmt.Sprintf("%s: %d/%d", t.HungerLabel, s.HungerTicks(), logistics.HungerInterval), float64(x), float64(y))
}

func drawVillagerInspector(screen *ebiten.Image, x, y int, v *villagers.Villager) {
	t := i18n.T()
	profession := t.UnitFarmer
	if v.Profession == villagers.Baker {
		profession = t.UnitBaker
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
	DrawText(screen, fmt.Sprintf("%s: %d/%d", t.HungerLabel, v.HungerTicks(), villagers.HungerInterval), float64(x), float64(y))
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
	DrawText(screen, fmt.Sprintf("%s: %d/%d", t.HungerLabel, j.HungerTicks(), lumberjack.HungerInterval), float64(x), float64(y))
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
	case building.Tavern:
		img = assets.Tavern
	case building.Road:
		img = assets.Road
	case building.Warehouse:
		img = assets.Warehouse
	case building.LumberjackHut:
		img = assets.LumberjackHut
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
