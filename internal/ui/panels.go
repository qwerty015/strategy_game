package ui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/i18n"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/miner"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/quarry"
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
	DrawMenuText(screen, title, float64(r.x+14), float64(r.y+13))
}

// imageRect is a tiny local rectangle type to keep panel drawing independent
// from image.Rectangle arithmetic and make the intended pixel layout clear.
type imageRect struct{ x, y, w, h int }

// DrawBuildPanel renders construction, NPC hiring and settings in the same
// left panel. A professional card is muted when every matching workplace
// already has a resident, making the one-worker-per-building limit visible.
func DrawBuildPanel(screen *ebiten.Image, layout Layout, p *Palette, tab LeftTab, options []HireOption, builtCounts map[building.Kind]int, speed economy.Speed, slots []SaveSlotInfo, dialog DialogKind, dialogSlot int, dialogText string) {
	r := layout.LeftPanel()
	drawPanel(screen, imageRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, i18n.T().BuildMenuTitle)

	drawMenuTabs(screen, layout, tab)
	switch tab {
	case HireTab:
		drawHireCards(screen, layout, options)
		return
	case SettingsTab:
		drawSettingsContent(screen, layout, speed, slots, dialog, dialogSlot, dialogText)
		return
	}
	stride, cardH := layout.cardGeometry(len(p.Kinds))
	for i, kind := range p.Kinds {
		x, y := 12, leftCardsStartY+i*stride
		w, h := layout.LeftWidth-24, cardH
		fill := panelInnerColor
		if i == p.Selected {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)
		vector.FillRect(screen, float32(x), float32(y+h-3), float32(w), 3, panelEdgeColor, false)

		// The former footprint line made the construction list visually noisy.
		// A single centered label leaves room for a more readable icon and type.
		iconSize := min(34, cardH-6)
		iconX := x + 8
		iconY := y + (cardH-iconSize)/2
		drawBuildingIcon(screen, kind, iconX, iconY, iconSize)
		label := i18n.T().BuildingName[kind]
		if n := builtCounts[kind]; n > 0 {
			label = fmt.Sprintf("%s (%d)", label, n)
		}
		DrawMenuText(screen, label, float64(iconX+iconSize+10), float64(y+(cardH-12)/2))
	}
}

func drawMenuTabs(screen *ebiten.Image, layout Layout, active LeftTab) {
	labels := []string{i18n.T().BuildTab, i18n.T().HireTab, i18n.T().SettingsTab}
	for i, label := range labels {
		x, w := layout.tabRect(i)
		fill := panelInnerColor
		if LeftTab(i) == active {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(x), leftTabY, float32(w), leftTabHeight, fill, false)
		DrawMenuText(screen, label, float64(x+4), float64(leftTabY+9))
	}
}

// drawSettingsContent renders the settings tab: a live language switch, the
// game's single speed-control row, and the five named save slots -- or, while
// a modal is open, the naming/overwrite dialog in place of the slot list. See
// Layout's settings* constants for the shared geometry.
func drawSettingsContent(screen *ebiten.Image, layout Layout, speed economy.Speed, slots []SaveSlotInfo, dialog DialogKind, dialogSlot int, dialogText string) {
	t := i18n.T()
	x := 12
	w := layout.LeftWidth - 24

	langs := []struct {
		lang  i18n.Lang
		label string
	}{{i18n.RU, "Русский"}, {i18n.EN, "English"}}
	langSegW := w / 2
	for i, entry := range langs {
		bx := x + i*langSegW
		fill := panelInnerColor
		if i18n.Current() == entry.lang {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(bx), float32(settingsLangRowY), float32(langSegW-2), float32(settingsLangRowH), fill, false)
		DrawMenuText(screen, entry.label, float64(bx+6), float64(settingsLangRowY+5))
	}

	speedLabels := []string{t.SpeedPaused, t.SpeedHalf, t.SpeedNormal, t.SpeedDouble, t.SpeedQuadruple, t.SpeedOctuple}
	speedSegW := w / len(speedLabels)
	for i, label := range speedLabels {
		bx := x + i*speedSegW
		fill := panelInnerColor
		if economy.Speed(i) == speed {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(bx), float32(settingsSpeedRowY), float32(speedSegW-2), float32(settingsSpeedRowH), fill, false)
		DrawCompactMenuText(screen, label, float64(bx+4), float64(settingsSpeedRowY+8))
	}

	vector.FillRect(screen, float32(x), float32(settingsNewGameRowY), float32(w), float32(settingsNewGameRowH), panelInnerColor, false)
	DrawMenuText(screen, t.NewGameButton, float64(x+8), float64(settingsNewGameRowY+6))

	if IsSettingsDialog(dialog) {
		drawSettingsDialog(screen, layout, dialog, dialogSlot, dialogText)
		return
	}

	DrawMenuText(screen, t.SaveSlotsLabel, float64(x), float64(settingsSlotsLabelY))
	for i := 0; i < len(slots) && i < settingsSlotCount; i++ {
		slot := slots[i]
		rowY := settingsSlotsStartY + i*settingsSlotStride
		name := slot.Name
		if !slot.Occupied {
			name = t.SlotEmptyLabel
		}
		DrawMenuText(screen, fmt.Sprintf("%d. %s", i+1, name), float64(x), float64(rowY))

		autoX := x + w - settingsSlotAutoW
		autoFill := panelInnerColor
		if slot.Autosave {
			autoFill = selectedColor
		}
		vector.FillRect(screen, float32(autoX), float32(rowY), float32(settingsSlotAutoW-2), float32(settingsSlotNameH-2), autoFill, false)
		DrawCompactMenuText(screen, t.AutosaveToggle, float64(autoX+4), float64(rowY+4))

		btnY := rowY + settingsSlotNameH
		halfW := w / 2
		vector.FillRect(screen, float32(x), float32(btnY), float32(halfW-2), float32(settingsSlotButtonH), panelInnerColor, false)
		DrawMenuText(screen, t.SlotSaveButton, float64(x+4), float64(btnY+5))

		loadFill := panelInnerColor
		if !slot.Occupied {
			loadFill = color.RGBA{R: 69, G: 50, B: 48, A: 245}
		}
		vector.FillRect(screen, float32(x+halfW), float32(btnY), float32(halfW-2), float32(settingsSlotButtonH), loadFill, false)
		DrawMenuText(screen, t.SlotLoadButton, float64(x+halfW+4), float64(btnY+5))
	}
}

// drawSettingsDialog renders the modal that replaces the slot list while
// the player is naming a slot or confirming an overwrite. Both dialog kinds
// share the same button row geometry (see SettingsDialogButtonAt) -- only
// the title text and the presence of the text field differ.
func drawSettingsDialog(screen *ebiten.Image, layout Layout, dialog DialogKind, slot int, text string) {
	t := i18n.T()
	x := 12
	w := layout.LeftWidth - 24

	if dialog == DialogConfirmOverwrite {
		DrawMenuText(screen, fmt.Sprintf(t.SlotOverwritePrompt, slot, text), float64(x), float64(settingsSlotsLabelY))
		drawDialogButtons(screen, x, w, t.SlotOverwriteButton, t.SlotCancelButton)
		return
	}

	if dialog == DialogConfirmNewGame {
		DrawMenuText(screen, t.NewGameConfirmPrompt, float64(x), float64(settingsSlotsLabelY))
		drawDialogButtons(screen, x, w, t.NewGameConfirmButton, t.SlotCancelButton)
		return
	}

	DrawMenuText(screen, fmt.Sprintf(t.SlotNamePrompt, slot), float64(x), float64(settingsSlotsLabelY))
	vector.FillRect(screen, float32(x), float32(settingsDialogFieldY), float32(w), float32(settingsDialogFieldH), panelInnerColor, false)
	DrawMenuText(screen, text+"_", float64(x+6), float64(settingsDialogFieldY+7))
	drawDialogButtons(screen, x, w, t.SlotSaveButton, t.SlotCancelButton)
}

func drawDialogButtons(screen *ebiten.Image, x, w int, leftLabel, rightLabel string) {
	halfW := w / 2
	vector.FillRect(screen, float32(x), float32(settingsDialogButtonY), float32(halfW-2), float32(settingsDialogButtonH), selectedColor, false)
	DrawMenuText(screen, leftLabel, float64(x+8), float64(settingsDialogButtonY+7))
	vector.FillRect(screen, float32(x+halfW), float32(settingsDialogButtonY), float32(halfW-2), float32(settingsDialogButtonH), panelInnerColor, false)
	DrawMenuText(screen, rightLabel, float64(x+halfW+8), float64(settingsDialogButtonY+7))
}

func drawHireCards(screen *ebiten.Image, layout Layout, options []HireOption) {
	stride, cardH := layout.cardGeometry(len(options))
	for i, option := range options {
		x, y := 12, leftCardsStartY+i*stride
		w, h := layout.LeftWidth-24, cardH
		fill := panelInnerColor
		if !option.Available {
			fill = color.RGBA{R: 69, G: 50, B: 48, A: 245}
		}
		vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)
		vector.FillRect(screen, float32(x), float32(y+h-3), float32(w), 3, panelEdgeColor, false)

		iconSize := min(34, cardH-6)
		iconX := x + 8
		iconY := y + (cardH-iconSize)/2
		drawHireIcon(screen, option.Kind, iconX, iconY, iconSize)
		DrawMenuText(screen, hireName(option.Kind), float64(iconX+iconSize+10), float64(y+4))
		count := fmt.Sprintf("%d", option.Current)
		if option.Limit > 0 {
			count = fmt.Sprintf("%d/%d", option.Current, option.Limit)
		}
		DrawMenuText(screen, count, float64(iconX+iconSize+10), float64(y+21))
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
	case HireCarpenter:
		return t.UnitCarpenter
	case HireQuarryman:
		return t.UnitQuarryman
	case HireBuilder:
		return t.UnitBuilder
	case HireMiner:
		return t.UnitMiner
	case HireSmelter:
		return t.UnitSmelter
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
	case HireCarpenter:
		img = assets.Carpenter[0]
	case HireQuarryman:
		img = assets.Quarryman[0]
	case HireBuilder:
		img = assets.Builder[0]
	case HireMiner:
		img = assets.Miner[0]
	case HireSmelter:
		img = assets.Smelter[0]
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

// drawSelectionIcon reuses the map/palette art in the inspector, so a click
// always has both a readable name and an immediate visual identity.
func drawSelectionIcon(screen *ebiten.Image, selection Selection, x, y, size int) {
	switch selection.Kind {
	case SelectionBuilding:
		if selection.Building != nil {
			drawBuildingIcon(screen, selection.Building.Kind, x, y, size)
		}
	case SelectionSerf:
		drawHireIcon(screen, HireSerf, x, y, size)
	case SelectionVillager:
		if selection.Villager != nil {
			drawHireIcon(screen, hireKindForProfession(selection.Villager.Profession), x, y, size)
		}
	case SelectionLumberjack:
		drawHireIcon(screen, HireLumberjack, x, y, size)
	case SelectionFisherman:
		drawHireIcon(screen, HireFisherman, x, y, size)
	case SelectionQuarryman:
		drawHireIcon(screen, HireQuarryman, x, y, size)
	case SelectionBuilder:
		drawHireIcon(screen, HireBuilder, x, y, size)
	case SelectionMiner:
		drawHireIcon(screen, HireMiner, x, y, size)
	}
}

func hireKindForProfession(profession villagers.Profession) HireKind {
	switch profession {
	case villagers.Farmer:
		return HireFarmer
	case villagers.Baker:
		return HireBaker
	case villagers.Winemaker:
		return HireWinemaker
	case villagers.Swineherd:
		return HireSwineherd
	case villagers.Butcher:
		return HireButcher
	case villagers.Carpenter:
		return HireCarpenter
	case villagers.Smelter:
		return HireSmelter
	default:
		return HireSerf
	}
}

const (
	inspectorIconSize = 72
	inspectorDividerY = 138
	inspectorBodyY    = 152
)

// DrawInspectorPanel renders the currently selected object. It reads only
// public accessors from the logic packages, keeping display formatting out of
// the simulation. A large centered portrait separates the selected object
// from its data, while no selection becomes the compact town summary.
func DrawInspectorPanel(screen *ebiten.Image, layout Layout, selection Selection, connected bool, stock *resource.Stockpile, pop *economy.Population, occupants int, showPriority bool, priorityLevel int, dialog DialogKind) {
	r := layout.RightPanel()
	drawPanel(screen, imageRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, i18n.T().InspectorTitle)
	if selection.Kind == SelectionNone {
		x := float64(r.Min.X + 18)
		DrawInspectorText(screen, i18n.T().InspectorHint, x, 62)
		if pop != nil {
			DrawInspectorText(screen, fmt.Sprintf("%s: %d", i18n.T().Population, pop.Count), x, 92)
			DrawInspectorText(screen, fmt.Sprintf("%s: %d", i18n.T().DeathsLabel, pop.Deaths), x, 112)
			DrawInspectorText(screen, fmt.Sprintf("%s: %d", i18n.T().RemovedLabel, pop.Removed), x, 132)
		}
		return
	}

	// The icon is a real visual header now, rather than a small badge in the
	// corner: it makes a worker and his workplace recognisable before reading.
	iconX := r.Min.X + (r.Dx()-inspectorIconSize)/2
	drawSelectionIcon(screen, selection, iconX, 48, inspectorIconSize)
	vector.FillRect(screen, float32(r.Min.X+18), inspectorDividerY, float32(r.Dx()-36), 2, panelEdgeColor, false)

	switch selection.Kind {
	case SelectionBuilding:
		drawBuildingInspector(screen, r.Min.X+18, inspectorBodyY, selection.Building, connected, stock, occupants)
	case SelectionSerf:
		drawSerfInspector(screen, r.Min.X+18, inspectorBodyY, selection.Serf)
	case SelectionVillager:
		drawVillagerInspector(screen, r.Min.X+18, inspectorBodyY, selection.Villager)
	case SelectionLumberjack:
		drawLumberjackInspector(screen, r.Min.X+18, inspectorBodyY, selection.Lumberjack)
	case SelectionFisherman:
		drawFishermanInspector(screen, r.Min.X+18, inspectorBodyY, selection.Fisherman)
	case SelectionQuarryman:
		drawQuarrymanInspector(screen, r.Min.X+18, inspectorBodyY, selection.Quarryman)
	case SelectionBuilder:
		drawBuilderInspector(screen, r.Min.X+18, inspectorBodyY, selection.Builder)
	case SelectionMiner:
		drawMinerInspector(screen, r.Min.X+18, inspectorBodyY, selection.Miner)
	}
	if CanRemoveSelection(selection) {
		drawRemoveButton(screen, layout, selection, showPriority)
	}
	if showPriority {
		drawPriorityControl(screen, layout, priorityLevel)
	}
	if dialog == DialogConfirmRemoval {
		drawConfirmRemovalDialog(screen, layout, selection)
	}
}

// drawConfirmRemovalDialog overlays the current inspector instead of hiding
// the selected object. The same confirmation is used before changing any
// removable building or serf.
func drawConfirmRemovalDialog(screen *ebiten.Image, layout Layout, selection Selection) {
	r := layout.InspectorConfirmRemoveRect()
	vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), panelInnerColor, false)
	vector.StrokeRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 2, panelEdgeColor, false)

	name := i18n.T().UnitSerf
	if selection.Kind == SelectionBuilding && selection.Building != nil {
		name = i18n.T().BuildingName[selection.Building.Kind]
	}
	DrawInspectorText(screen, fmt.Sprintf(i18n.T().ConfirmRemovalPrompt, name), float64(r.Min.X+10), float64(r.Min.Y+12))

	confirm, cancel := layout.InspectorConfirmRemoveButtons()
	vector.FillRect(screen, float32(confirm.Min.X), float32(confirm.Min.Y), float32(confirm.Dx()), float32(confirm.Dy()), color.RGBA{R: 126, G: 53, B: 45, A: 255}, false)
	vector.FillRect(screen, float32(cancel.Min.X), float32(cancel.Min.Y), float32(cancel.Dx()), float32(cancel.Dy()), panelColor, false)
	DrawInspectorText(screen, i18n.T().ConfirmRemovalButton, float64(confirm.Min.X+8), float64(confirm.Min.Y+8))
	DrawInspectorText(screen, i18n.T().SlotCancelButton, float64(cancel.Min.X+8), float64(cancel.Min.Y+8))
}

// drawRemoveButton renders the selected object's removal action in the
// inspector. Serfs are dismissed safely after their current delivery; the
// actual behavior is implemented by cmd/game's removeSelected method.
func drawRemoveButton(screen *ebiten.Image, layout Layout, selection Selection, showPriority bool) {
	r := layout.InspectorRemoveRect(showPriority)
	fill := color.RGBA{R: 126, G: 53, B: 45, A: 255}
	vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), fill, false)
	vector.FillRect(screen, float32(r.Min.X), float32(r.Max.Y-3), float32(r.Dx()), 3, panelEdgeColor, false)

	label := i18n.T().RemoveSelected
	if selection.Kind == SelectionSerf {
		label = i18n.T().DismissSerf
	}
	DrawInspectorText(screen, label, float64(r.Min.X+8), float64(r.Min.Y+8))
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
	DrawInspectorText(screen, t.PriorityLabel, float64(r.Min.X+priorityMargin), float64(rowY-18))

	labels := [5]string{"--", "-", "•", "+", "++"}
	for i, label := range labels {
		level := i - 2
		x := startX + i*segW
		fill := panelInnerColor
		if level == current {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(x), float32(rowY), float32(segW-2), float32(priorityRowHeight), fill, false)
		DrawInspectorText(screen, label, float64(x+segW/2-6), float64(rowY+7))
	}
}

// drawConstructionInspector shows a placed-but-unfinished building or
// road's stage, delivered materials, and overall progress -- see package
// builder. Called instead of the rest of drawBuildingInspector for any
// building still under construction, regardless of what it will become.
func drawConstructionInspector(screen *ebiten.Image, x, y int, b *building.Building, bt building.Type) {
	t := i18n.T()
	stage := t.ConstructionFoundationLabel
	switch b.ConstructionStage {
	case building.ConstructionWaitingMaterials:
		stage = t.ConstructionWaitingLabel
	case building.ConstructionFinishing:
		stage = t.ConstructionFinishingLabel
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.StateLabel, stage), float64(x), float64(y))
	y += 20
	DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.ConstructionProgressLabel, int(b.ConstructionProgress()*100)), float64(x), float64(y))
	y += 20
	if bt.PlankCost > 0 {
		DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.Plank], b.InputBuffer[resource.Plank], bt.PlankCost), float64(x), float64(y))
		y += 18
	}
	if bt.StoneCost > 0 {
		DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.StoneBlock], b.InputBuffer[resource.StoneBlock], bt.StoneCost), float64(x), float64(y))
	}
}

func drawBuildingInspector(screen *ebiten.Image, x, y int, b *building.Building, connected bool, stock *resource.Stockpile, occupants int) {
	t := i18n.T()
	bt := building.Types[b.Kind]
	DrawInspectorText(screen, t.BuildingName[b.Kind], float64(x), float64(y))
	y += 24
	if b.ConstructionStage != building.ConstructionNone {
		drawConstructionInspector(screen, x, y, b, bt)
		return
	}
	if b.Kind == building.Tree {
		DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.GrowthLabel, int(b.GrowthProgress()*100)), float64(x), float64(y))
		y += 20
		DrawInspectorText(screen, t.HarvestableLabel, float64(x), float64(y))
		return
	}
	if b.Kind == building.Fish {
		DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.GrowthLabel, int(b.GrowthProgress()*100)), float64(x), float64(y))
		y += 20
		if b.GrowthStage() >= 2 {
			DrawInspectorText(screen, t.CatchableLabel, float64(x), float64(y))
		}
		return
	}
	if b.Kind == building.StoneDeposit {
		DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.StoneReserveLabel, b.Reserve, building.StoneDepositReserve), float64(x), float64(y))
		return
	}
	switch b.Kind {
	case building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
		DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.DepositReserveLabel, b.Reserve, building.OreDepositReserve), float64(x), float64(y))
		return
	}
	// Every real building, from here on: how many units currently stand
	// on its footprint. Not just an assigned resident -- a serf mid-drop
	// off or a hungry unit eating at the Tavern counts too, so it works
	// the same way on buildings that never have a dedicated resident
	// (Tavern, Warehouse) as it does on a workplace (Farm, Bakery, ...).
	DrawInspectorText(screen, fmt.Sprintf("%s: %d", t.PeopleInsideLabel, occupants), float64(x), float64(y))
	y += 20
	if b.Kind == building.LumberjackHut {
		DrawInspectorText(screen, t.ContentsLabel, float64(x), float64(y))
		y += 20
		DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.Log], b.OutputBuffer[resource.Log], building.BufferCapacity), float64(x), float64(y))
		y += 20
		routeState := t.Disconnected
		if connected {
			routeState = t.Connected
		}
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, routeState), float64(x), float64(y))
		return
	}
	if b.Kind == building.FisherHut {
		DrawInspectorText(screen, t.ContentsLabel, float64(x), float64(y))
		y += 20
		DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.Fish], b.OutputBuffer[resource.Fish], building.BufferCapacity), float64(x), float64(y))
		y += 20
		routeState := t.Disconnected
		if connected {
			routeState = t.Connected
		}
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, routeState), float64(x), float64(y))
		return
	}
	if b.Kind == building.QuarryHut {
		DrawInspectorText(screen, t.ContentsLabel, float64(x), float64(y))
		y += 20
		DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.StoneBlock], b.OutputBuffer[resource.StoneBlock], building.BufferCapacity), float64(x), float64(y))
		y += 20
		routeState := t.Disconnected
		if connected {
			routeState = t.Connected
		}
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, routeState), float64(x), float64(y))
		return
	}
	if b.Kind == building.MinerHut {
		DrawInspectorText(screen, t.ContentsLabel, float64(x), float64(y))
		y += 20
		for _, rt := range []resource.Type{resource.Coal, resource.GoldOre, resource.IronOre} {
			DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.OutputBuffer[rt], building.BufferCapacity), float64(x), float64(y))
			y += 18
		}
		routeState := t.Disconnected
		if connected {
			routeState = t.Connected
		}
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, routeState), float64(x), float64(y))
		return
	}
	if b.Kind == building.Smeltery {
		recipes := bt.AllRecipes()
		if len(recipes) > 0 {
			active := b.ActiveRecipe
			if active < 0 || active >= len(recipes) {
				active = 0
			}
			DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.StateLabel, b.ProgressTicks, recipes[active].TicksToProduce), float64(x), float64(y))
			y += 20
		}
		DrawInspectorText(screen, t.InputLabel+":", float64(x), float64(y))
		y += 18
		for _, rt := range []resource.Type{resource.GoldOre, resource.IronOre, resource.Coal} {
			DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.InputBuffer[rt], building.BufferCapacity), float64(x+8), float64(y))
			y += 18
		}
		DrawInspectorText(screen, t.OutputLabel+":", float64(x), float64(y))
		y += 18
		for _, rt := range []resource.Type{resource.Gold, resource.Iron} {
			DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.OutputBuffer[rt], b.OutputLimit()), float64(x+8), float64(y))
			y += 18
		}
		roadState := t.Disconnected
		if connected {
			roadState = t.Connected
		}
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, roadState), float64(x), float64(y))
		return
	}
	if b.Kind == building.Warehouse && stock != nil {
		DrawInspectorText(screen, t.ContentsLabel, float64(x), float64(y))
		y += 20
		for _, rt := range resource.AllTypes() {
			drawWarehouseResourceRow(screen, x, y, rt, stock.Amount(rt))
			y += 18
		}
	}
	if bt.Recipe.TicksToProduce > 0 {
		DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.StateLabel, b.ProgressTicks, bt.Recipe.TicksToProduce), float64(x), float64(y))
		y += 20
	}
	inputTypes := recipeInputTypes(bt)
	if len(bt.AcceptedResources) > 0 {
		inputTypes = bt.AcceptedResources
	}
	if len(inputTypes) > 0 {
		DrawInspectorText(screen, t.InputLabel+":", float64(x), float64(y))
		y += 18
		for _, rt := range inputTypes {
			DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.InputBuffer[rt], building.BufferCapacity), float64(x+8), float64(y))
			y += 18
		}
	}
	if bt.Recipe.TicksToProduce > 0 {
		DrawInspectorText(screen, t.OutputLabel+":", float64(x), float64(y))
		y += 18
		DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.ResourceName[bt.Recipe.Output], b.OutputBuffer[bt.Recipe.Output], b.OutputLimit()), float64(x+8), float64(y))
		y += 18
	}
	roadState := t.Disconnected
	if connected || b.Kind == building.Warehouse || b.Kind == building.Road {
		roadState = t.Connected
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, roadState), float64(x), float64(y))
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
	DrawInspectorText(screen, t.UnitSerf, float64(x), float64(y))
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
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20

	rt, amount := s.Cargo()
	cargo := t.NoCargo
	if amount > 0 {
		cargo = fmt.Sprintf("%s × %d", t.ResourceName[rt], amount)
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.CargoLabel, cargo), float64(x), float64(y))
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
		DrawInspectorText(screen, fmt.Sprintf("%s: %s → %s", t.RouteLabel, fromName, toName), float64(x), float64(y))
		y += 20
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HungerLabel, s.SatietyPercent()), float64(x), float64(y))
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
	case villagers.Carpenter:
		profession = t.UnitCarpenter
	}
	DrawInspectorText(screen, profession, float64(x), float64(y))
	y += 24

	state := t.StateWorking
	switch v.State() {
	case villagers.VillagerToTavern, villagers.VillagerToHome:
		state = t.StateWalking
	}
	if v.Starving {
		state += " (" + t.StateStarving + ")"
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20
	if v.HomeBuilding() != nil {
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.HomeLabel, t.BuildingName[v.HomeBuilding().Kind]), float64(x), float64(y))
		y += 20
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HungerLabel, v.SatietyPercent()), float64(x), float64(y))
}

func drawLumberjackInspector(screen *ebiten.Image, x, y int, j *lumberjack.Lumberjack) {
	t := i18n.T()
	DrawInspectorText(screen, t.UnitLumberjack, float64(x), float64(y))
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
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20

	_, amount := j.Cargo()
	cargo := t.NoCargo
	if amount > 0 {
		cargo = fmt.Sprintf("%s × %d", t.ResourceName[resource.Log], amount)
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.CargoLabel, cargo), float64(x), float64(y))
	y += 20

	target := t.NoRoute
	if j.TargetTree() != nil {
		target = t.BuildingName[building.Tree]
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s → %s", t.RouteLabel, target, t.BuildingName[building.LumberjackHut]), float64(x), float64(y))
	y += 20
	DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.HungerLabel, min(j.HungerTicks(), lumberjack.HungerInterval), lumberjack.HungerInterval), float64(x), float64(y))
}

func drawQuarrymanInspector(screen *ebiten.Image, x, y int, q *quarry.Quarryman) {
	t := i18n.T()
	DrawInspectorText(screen, t.UnitQuarryman, float64(x), float64(y))
	y += 24

	state := t.StateIdle
	switch q.State() {
	case quarry.StateToDeposit:
		state = t.StateWalking
	case quarry.StateMining:
		state = t.StateMining
	case quarry.StateToHome:
		state = t.StateDelivering
	case quarry.StateUnloading:
		state = t.StateUnloading
	case quarry.StateToTavern, quarry.StateToHomeAfterMeal:
		state = t.StateWalking
	}
	if q.Starving {
		state += " (" + t.StateStarving + ")"
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20

	rt, amount := q.Cargo()
	cargo := t.NoCargo
	if amount > 0 {
		cargo = fmt.Sprintf("%s × %d", t.ResourceName[rt], amount)
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.CargoLabel, cargo), float64(x), float64(y))
	y += 20

	target := t.NoRoute
	if q.TargetDeposit() != nil {
		target = t.BuildingName[building.StoneDeposit]
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s → %s", t.RouteLabel, target, t.BuildingName[building.QuarryHut]), float64(x), float64(y))
	y += 20
	DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HungerLabel, q.SatietyPercent()), float64(x), float64(y))
}

func drawBuilderInspector(screen *ebiten.Image, x, y int, bld *builder.Builder) {
	t := i18n.T()
	DrawInspectorText(screen, t.UnitBuilder, float64(x), float64(y))
	y += 24

	state := t.StateIdle
	switch bld.State() {
	case builder.StateToSite:
		state = t.StateWalking
	case builder.StateFoundation, builder.StateFinishing:
		state = t.StateBuilding
	case builder.StateWaitingMaterials:
		state = t.StateWaitingMaterials
	case builder.StateToTavern:
		state = t.StateWalking
	}
	if bld.Starving {
		state += " (" + t.StateStarving + ")"
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20

	target := t.NoRoute
	if site := bld.TargetSite(); site != nil {
		target = t.BuildingName[site.Kind]
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RouteLabel, target), float64(x), float64(y))
	y += 20
	DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HungerLabel, bld.SatietyPercent()), float64(x), float64(y))
}

func drawMinerInspector(screen *ebiten.Image, x, y int, m *miner.Miner) {
	t := i18n.T()
	DrawInspectorText(screen, t.UnitMiner, float64(x), float64(y))
	y += 24

	state := t.StateIdle
	switch m.State() {
	case miner.StateToDeposit:
		state = t.StateWalking
	case miner.StateMining:
		state = t.StateMining
	case miner.StateToHome:
		state = t.StateDelivering
	case miner.StateUnloading:
		state = t.StateUnloading
	case miner.StateToTavern, miner.StateToHomeAfterMeal:
		state = t.StateWalking
	}
	if m.Starving {
		state += " (" + t.StateStarving + ")"
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20

	rt, amount := m.Cargo()
	cargo := t.NoCargo
	if amount > 0 {
		cargo = fmt.Sprintf("%s × %d", t.ResourceName[rt], amount)
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.CargoLabel, cargo), float64(x), float64(y))
	y += 20

	target := t.NoRoute
	if deposit := m.TargetDeposit(); deposit != nil {
		target = t.BuildingName[deposit.Kind]
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s → %s", t.RouteLabel, target, t.BuildingName[building.MinerHut]), float64(x), float64(y))
	y += 20

	quotaIndex, quotaProgress := m.QuotaProgress()
	if quotaIndex >= 0 && quotaIndex < len(miner.DefaultQuota) {
		entry := miner.DefaultQuota[quotaIndex]
		DrawInspectorText(screen, fmt.Sprintf("%s: %s (%d/%d)", t.QuotaLabel, t.ResourceName[entry.Resource], quotaProgress, entry.Amount), float64(x), float64(y))
		y += 20
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HungerLabel, m.SatietyPercent()), float64(x), float64(y))
}

func drawFishermanInspector(screen *ebiten.Image, x, y int, f *fishing.Fisherman) {
	t := i18n.T()
	DrawInspectorText(screen, t.UnitFisherman, float64(x), float64(y))
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
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.StateLabel, state), float64(x), float64(y))
	y += 20

	_, amount := f.Cargo()
	cargo := t.NoCargo
	if amount > 0 {
		cargo = fmt.Sprintf("%s × %d", t.ResourceName[resource.Fish], amount)
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.CargoLabel, cargo), float64(x), float64(y))
	y += 20

	target := t.NoRoute
	if f.TargetFish() != nil {
		target = t.BuildingName[building.Fish]
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s → %s", t.RouteLabel, target, t.BuildingName[building.FisherHut]), float64(x), float64(y))
	y += 20
	DrawInspectorText(screen, fmt.Sprintf("%s: %d/%d", t.HungerLabel, min(f.HungerTicks(), fishing.HungerInterval), fishing.HungerInterval), float64(x), float64(y))
}

// selectedUnitTile returns the current grid tile of whichever *unit* (not
// building) is selected, plus the tiles still ahead of it on its current
// route (e.g. Serf.RemainingPath). Shared by DrawSelectionMarker's unit
// branches and DrawSelectedRoute, so both agree on where a unit "is" and
// "is going" from exactly the same lookup instead of two parallel type
// switches that could quietly drift apart.
func selectedUnitTile(selection Selection) (tx, ty int, path []pathfind.Point, ok bool) {
	switch selection.Kind {
	case SelectionSerf:
		if selection.Serf == nil {
			return 0, 0, nil, false
		}
		return selection.Serf.X, selection.Serf.Y, selection.Serf.RemainingPath(), true
	case SelectionVillager:
		if selection.Villager == nil {
			return 0, 0, nil, false
		}
		return selection.Villager.X, selection.Villager.Y, selection.Villager.RemainingPath(), true
	case SelectionLumberjack:
		if selection.Lumberjack == nil {
			return 0, 0, nil, false
		}
		return selection.Lumberjack.X, selection.Lumberjack.Y, selection.Lumberjack.RemainingPath(), true
	case SelectionFisherman:
		if selection.Fisherman == nil {
			return 0, 0, nil, false
		}
		return selection.Fisherman.X, selection.Fisherman.Y, selection.Fisherman.RemainingPath(), true
	case SelectionQuarryman:
		if selection.Quarryman == nil {
			return 0, 0, nil, false
		}
		return selection.Quarryman.X, selection.Quarryman.Y, selection.Quarryman.RemainingPath(), true
	case SelectionBuilder:
		if selection.Builder == nil {
			return 0, 0, nil, false
		}
		return selection.Builder.X, selection.Builder.Y, selection.Builder.RemainingPath(), true
	case SelectionMiner:
		if selection.Miner == nil {
			return 0, 0, nil, false
		}
		return selection.Miner.X, selection.Miner.Y, selection.Miner.RemainingPath(), true
	default:
		return 0, 0, nil, false
	}
}

// DrawSelectionMarker draws a warm outline under the selected object so the
// player can connect the inspector to the world even when sprites overlap.
func DrawSelectionMarker(screen *ebiten.Image, cam *render.Camera, selection Selection) {
	var x, y float64
	size := cam.TilePixels()
	if selection.Kind == SelectionBuilding && selection.Building != nil {
		x, y = cam.TileToScreen(selection.Building.X, selection.Building.Y)
		size *= float64(building.Types[selection.Building.Kind].Footprint)
	} else if tx, ty, _, ok := selectedUnitTile(selection); ok {
		x, y = cam.TileToScreen(tx, ty)
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

// DrawSelectedRoute draws the remaining path of the currently selected
// unit as a thin connected line from tile-centre to tile-centre, so a
// player can see *where* a walking unit is actually headed -- until now
// DrawSelectionMarker only boxed its current tile, per the user's own
// roadmap note ("сам выбранный объект уже подсвечивается... но не рисует
// линию маршрута"). A building selection, or a unit that's idle/has no
// path right now, draws nothing.
func DrawSelectedRoute(screen *ebiten.Image, cam *render.Camera, selection Selection) {
	tx, ty, path, ok := selectedUnitTile(selection)
	if !ok || len(path) == 0 {
		return
	}

	line := color.RGBA{R: 245, G: 201, B: 72, A: 150}
	thickness := float32(2)
	tp := cam.TilePixels()
	center := func(gx, gy int) (float32, float32) {
		sx, sy := cam.TileToScreen(gx, gy)
		return float32(sx + tp/2), float32(sy + tp/2)
	}

	px, py := center(tx, ty)
	for _, p := range path {
		qx, qy := center(p.X, p.Y)
		vector.StrokeLine(screen, px, py, qx, qy, thickness, line, true)
		px, py = qx, qy
	}
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
	case building.CarpentryWorkshop:
		img = assets.CarpentryWorkshop
	case building.Tavern:
		img = assets.Tavern
	case building.Road:
		img = assets.Road
	case building.Warehouse:
		img = assets.Warehouse
	case building.LumberjackHut:
		img = assets.LumberjackHut
	case building.QuarryHut:
		img = assets.QuarryHut
	case building.MinerHut:
		img = assets.MinerHut
	case building.Smeltery:
		img = assets.Smeltery
	case building.FisherHut:
		img = assets.FisherHutFrames[0]
	case building.Tree:
		img = assets.TreeFrames[2]
	case building.Fish:
		img = assets.FishFrames[2]
	case building.StoneDeposit:
		img = assets.StoneDeposit
	case building.CoalDeposit:
		img = assets.CoalDeposit
	case building.GoldOreDeposit:
		img = assets.GoldOreDeposit
	case building.IronOreDeposit:
		img = assets.IronOreDeposit
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
