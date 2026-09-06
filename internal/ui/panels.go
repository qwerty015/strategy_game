package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/enemy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/i18n"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/miner"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/quarry"
	"strategy_game/internal/render"
	"strategy_game/internal/resource"
	"strategy_game/internal/sentry"
	"strategy_game/internal/soldier"
	"strategy_game/internal/villagers"
)

var (
	panelColor      = color.RGBA{R: 30, G: 25, B: 26, A: 238}
	panelInnerColor = color.RGBA{R: 54, G: 43, B: 38, A: 245}
	panelEdgeColor  = color.RGBA{R: 151, G: 111, B: 64, A: 255}
	selectedColor   = color.RGBA{R: 179, G: 126, B: 48, A: 255}
	mutedTextColor  = color.RGBA{R: 201, G: 190, B: 168, A: 255}
	// unavailableCardColor marks a card the player can't act on right now --
	// a hire option with too little gold, or (see DrawBuildPanel) a building
	// whose production chain isn't ready yet (building.Unlocked).
	unavailableCardColor = color.RGBA{R: 69, G: 50, B: 48, A: 245}
	// emptyWorkplaceTextColor flags a hire card whose Limit exceeds
	// Current -- a real building of this profession stands empty, per the
	// user's explicit request ("во вкладке 'юниты' выделяй красным
	// юнитов которых нет (пустые здания)"). See drawHireCardInfo.
	emptyWorkplaceTextColor = color.RGBA{R: 235, G: 80, B: 70, A: 255}
	// lockedCardWashColor darkens a locked build card's icon and text too,
	// not just its background -- see DrawBuildPanel's doc comment on why
	// unavailableCardColor alone read as barely different from an ordinary
	// card at a glance.
	lockedCardWashColor = color.RGBA{R: 10, G: 8, B: 8, A: 150}
)

// buildLockTooltip mirrors resourceTooltip's pattern (see resource_icons.go):
// collected while DrawBuildPanel renders, drawn once on top of everything
// else at the end of the frame via DrawBuildLockTooltip.
var buildLockTooltip struct {
	active bool
	text   string
	x, y   int
}

// DrawBuildLockTooltip renders the reason a locked build card can't be
// placed yet under the cursor -- see DrawBuildPanel's hover detection.
func DrawBuildLockTooltip(screen *ebiten.Image, layout Layout) {
	if !buildLockTooltip.active || buildLockTooltip.text == "" {
		return
	}
	label := buildLockTooltip.text
	width := 20 + len([]rune(label))*7
	if width < 92 {
		width = 92
	}
	const height = 22
	x, y := buildLockTooltip.x+14, buildLockTooltip.y+14
	if x+width > layout.Width-4 {
		x = layout.Width - width - 4
	}
	if y+height > layout.Height-4 {
		y = buildLockTooltip.y - height - 8
	}
	if x < 4 {
		x = 4
	}
	if y < 4 {
		y = 4
	}
	fillIconRect(screen, x, y, width, height, panelColor)
	fillIconRect(screen, x, y, width, 1, panelEdgeColor)
	fillIconRect(screen, x, y+height-1, width, 1, panelEdgeColor)
	fillIconRect(screen, x, y, 1, height, panelEdgeColor)
	fillIconRect(screen, x+width-1, y, 1, height, panelEdgeColor)
	DrawInspectorText(screen, label, float64(x+8), float64(y+4))
}

func drawPanel(screen *ebiten.Image, r imageRect, title string) {
	vector.FillRect(screen, float32(r.x), float32(r.y), float32(r.w), float32(r.h), panelColor, false)
	vector.FillRect(screen, float32(r.x+5), float32(r.y+5), float32(r.w-10), 34, panelInnerColor, false)
	DrawMenuText(screen, title, float64(r.x+14), float64(r.y+13))
}

// imageRect is a tiny local rectangle type to keep panel drawing independent
// from image.Rectangle arithmetic and make the intended pixel layout clear.
type imageRect struct{ x, y, w, h int }

// DrawBuildPanel renders the two always-available world actions: construction
// and NPC hiring. Pause/options, saves and speed live behind Esc, so map input
// remains focused on the settlement itself.
func DrawBuildPanel(screen *ebiten.Image, layout Layout, p *Palette, tab LeftTab, demolitionMode bool, options []HireOption, builtCounts map[building.Kind]int, scrollBuild, scrollHire int, unlocked map[building.Kind]bool, lockReasons map[building.Kind]string) {
	r := layout.LeftPanel()
	drawPanel(screen, imageRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, i18n.T().BuildMenuTitle)

	drawMenuTabs(screen, layout, tab)
	buildLockTooltip.active = false
	if tab == HireTab {
		drawHireCards(screen, layout, options, scrollHire)
		return
	}
	drawDemolitionModeButton(screen, layout, demolitionMode)
	stride, cardH, start, visible := layout.leftListWindow(leftBuildCardsStartY, len(p.Kinds), scrollBuild)
	for row := 0; row < visible; row++ {
		i := start + row
		kind := p.Kinds[i]
		x, y := 12, leftBuildCardsStartY+row*stride
		w, h := layout.LeftWidth-24, cardH
		fill := panelInnerColor
		if !unlocked[kind] {
			fill = unavailableCardColor
		} else if i == p.Selected {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)
		vector.FillRect(screen, float32(x), float32(y+h-3), float32(w), 3, panelEdgeColor, false)

		// The building portrait identifies the card and its counter; the compact
		// resource line below shows the same exact construction cost used by the
		// placement code, without making the player open a separate tooltip.
		iconSize := min(34, cardH-6)
		iconX := x + 8
		iconY := y + (cardH-iconSize)/2
		drawBuildingIcon(screen, kind, iconX, iconY, iconSize)
		label := i18n.T().BuildingName[kind]
		if n := builtCounts[kind]; n > 0 {
			label = fmt.Sprintf("%s (%d)", label, n)
		}
		labelX := iconX + iconSize + 10
		DrawMenuText(screen, label, float64(labelX), float64(y+4))
		drawBuildCost(screen, kind, labelX, y+20)

		// A locked card was previously only distinguished by a subtle
		// background-fill difference -- a real playtest report ("визуально
		// нельзя отличить то можно построить сейчас от того что нельзя")
		// found that too easy to miss. A dark wash over the whole card
		// (icon and text included, not just the background peeking out
		// around them) reads unmistakably at a glance; hovering it explains
		// exactly what's missing via lockReasons, the same tooltip
		// convention BeginResourceTooltips/DrawResourceTooltip already use.
		if !unlocked[kind] {
			vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), lockedCardWashColor, false)
			mx, my := ebiten.CursorPosition()
			if mx >= x && mx < x+w && my >= y && my < y+h {
				buildLockTooltip.active = true
				buildLockTooltip.text = lockReasons[kind]
				buildLockTooltip.x, buildLockTooltip.y = mx, my
			}
		}
	}
	drawLeftScrollbar(screen, layout, leftBuildCardsStartY, len(p.Kinds), start, visible)
}

// drawLeftScrollbar draws a thin scroll indicator on the left panel's
// right edge when a card list is actually scrolled (visible < count) --
// see Layout.leftListWindow. Nothing is drawn when the whole list fits.
func drawLeftScrollbar(screen *ebiten.Image, layout Layout, startY, count, start, visible int) {
	if visible >= count || count <= 0 {
		return
	}
	r := layout.LeftPanel()
	trackX := float32(r.Max.X - 6)
	trackTop := float32(startY)
	trackHeight := float32(r.Max.Y - startY - 8)
	vector.FillRect(screen, trackX, trackTop, 3, trackHeight, panelInnerColor, false)

	thumbHeight := trackHeight * float32(visible) / float32(count)
	if thumbHeight < 16 {
		thumbHeight = 16
	}
	maxThumbTravel := trackHeight - thumbHeight
	thumbY := trackTop
	if count > visible {
		thumbY += maxThumbTravel * float32(start) / float32(count-visible)
	}
	vector.FillRect(screen, trackX, thumbY, 3, thumbHeight, selectedColor, false)
}

// drawDemolitionModeButton arms or disarms continuous removal from the
// construction panel. One confirmation still happens before this state is set.
func drawDemolitionModeButton(screen *ebiten.Image, layout Layout, active bool) {
	r := layout.DemolitionModeRect()
	fill := panelInnerColor
	label := i18n.T().DemolitionMode
	if active {
		fill = color.RGBA{R: 126, G: 53, B: 45, A: 255}
		label = i18n.T().DemolitionModeActive
	}
	vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), fill, false)
	vector.FillRect(screen, float32(r.Min.X), float32(r.Max.Y-3), float32(r.Dx()), 3, panelEdgeColor, false)
	DrawMenuText(screen, label, float64(r.Min.X+8), float64(r.Min.Y+8))
}

// drawMenuTabs renders only the two world-action tabs. Settings, saves and
// speed deliberately live in the centered Esc pause menu rather than here.
func drawMenuTabs(screen *ebiten.Image, layout Layout, active LeftTab) {
	labels := []string{i18n.T().BuildTab, i18n.T().HireTab}
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
func drawHireCards(screen *ebiten.Image, layout Layout, options []HireOption, scroll int) {
	stride, cardH, start, visible := layout.leftListWindow(leftCardsStartY, len(options), scroll)
	for row := 0; row < visible; row++ {
		option := options[start+row]
		x, y := 12, leftCardsStartY+row*stride
		w, h := layout.LeftWidth-24, cardH
		fill := panelInnerColor
		if !option.Available {
			fill = unavailableCardColor
		}
		vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)
		vector.FillRect(screen, float32(x), float32(y+h-3), float32(w), 3, panelEdgeColor, false)

		iconSize := min(34, cardH-6)
		iconX := x + 8
		iconY := y + (cardH-iconSize)/2
		drawHireIcon(screen, option.Kind, iconX, iconY, iconSize)
		labelX := iconX + iconSize + 10
		DrawMenuText(screen, hireName(option.Kind), float64(labelX), float64(y+4))
		drawHireCardInfo(screen, option, labelX, y+20)
	}
	drawLeftScrollbar(screen, layout, leftCardsStartY, len(options), start, visible)
}

// drawBuildCost draws the authoritative price directly below a construction
// card's name. Resource glyphs make the line readable even at a glance.
func drawBuildCost(screen *ebiten.Image, kind building.Kind, x, y int) {
	cost := building.Types[kind]
	if cost.PlankCost > 0 {
		drawResourceIcon(screen, resource.Plank, x, y)
		DrawCompactMenuText(screen, fmt.Sprintf("%d", cost.PlankCost), float64(x+resourceIconSize+4), float64(y))
		x += 32
	}
	if cost.StoneCost > 0 {
		drawResourceIcon(screen, resource.StoneBlock, x, y)
		DrawCompactMenuText(screen, fmt.Sprintf("%d", cost.StoneCost), float64(x+resourceIconSize+4), float64(y))
		x += 32
	}
	if cost.IronCost > 0 {
		drawResourceIcon(screen, resource.Iron, x, y)
		DrawCompactMenuText(screen, fmt.Sprintf("%d", cost.IronCost), float64(x+resourceIconSize+4), float64(y))
	}
}

// drawHireCardInfo shows both numbers carried by a hiring card: GoldCost is
// the payment and the second portrait is the current unit count/limit.
func drawHireCardInfo(screen *ebiten.Image, option HireOption, x, y int) {
	if option.GoldCost > 0 {
		drawResourceIcon(screen, resource.Gold, x, y)
		DrawCompactMenuText(screen, fmt.Sprintf("%d", option.GoldCost), float64(x+resourceIconSize+4), float64(y))
		x += 32
	}

	drawHireIcon(screen, option.Kind, x, y, resourceIconSize)
	count := fmt.Sprintf("%d", option.Current)
	if option.Limit > 0 {
		count = fmt.Sprintf("%d/%d", option.Current, option.Limit)
	}
	if option.Recommended > 0 {
		count += " (" + fmt.Sprintf(i18n.T().RecommendedServeCountLabel, option.Recommended) + ")"
	}
	if option.HasEmptyWorkplace() {
		DrawCompactMenuTextColor(screen, count, float64(x+resourceIconSize+4), float64(y), emptyWorkplaceTextColor)
		return
	}
	DrawCompactMenuText(screen, count, float64(x+resourceIconSize+4), float64(y))
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
	case HireWeaponsmith:
		return t.UnitWeaponsmith
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
	case HireWeaponsmith:
		img = assets.Weaponsmith[0]
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
	case SelectionOpposingBuilding:
		if selection.Building != nil {
			drawBuildingIcon(screen, selection.Building.Kind, x, y, size)
		}
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
	case villagers.Weaponsmith:
		return HireWeaponsmith
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
// BarracksHireAvailability bundles whether each of the three Barracks-hired
// units can currently be hired -- see cmd/game's canHireSentry/
// canHireArcher/canHireSwordsman. Passed as one value instead of three
// separate bool parameters so a future recruit doesn't have to touch every
// call site's positional args.
type BarracksHireAvailability struct {
	Sentry, Archer, Swordsman bool
}

// ArmoryProductionState is prepared by the simulation and shown next to a
// selected Armory. The UI therefore never guesses whether its Weaponsmith is
// actually at work or why a queued item is paused.
type ArmoryProductionState struct {
	Item                resource.Type
	Queued              bool
	Producing           bool
	WaitingForWorker    bool
	WaitingForMaterials bool
	WaitingForOutput    bool
	Progress, Total     int
}

func DrawInspectorPanel(screen *ebiten.Image, layout Layout, selection Selection, connected bool, stock *resource.Stockpile, pop *economy.Population, townBuildings, playedFrames, occupants int, showPriority bool, priorityLevel int, dialog DialogKind, trimServesPrompt string, canHire BarracksHireAvailability, armoryState ArmoryProductionState, formationLines, score int, leaderboard []LeaderboardEntry) {
	r := layout.RightPanel()
	drawPanel(screen, imageRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, i18n.T().InspectorTitle)
	if selection.Kind == SelectionNone {
		drawTownSummary(screen, r.Min.X, r.Dx(), stock, pop, townBuildings, playedFrames, score, leaderboard)
		if dialog == DialogConfirmDemolitionMode {
			drawConfirmDemolitionModeDialog(screen, layout)
		}
		if dialog == DialogConfirmTrimServes {
			drawConfirmTrimServesDialog(screen, layout, trimServesPrompt)
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
		drawBuildingInspector(screen, r.Min.X+18, inspectorBodyY, selection.Building, connected, stock, occupants, armoryState)
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
	case SelectionEnemy:
		drawEnemyInspector(screen, r.Min.X+18, inspectorBodyY, selection.Enemy)
	case SelectionSoldierGroup:
		drawSoldierGroupInspector(screen, r.Min.X+18, inspectorBodyY, selection.SoldierGroup)
	case SelectionOpposingBuilding:
		drawOpposingBuildingInspector(screen, r.Min.X+18, inspectorBodyY, selection.Building)
	case SelectionOpposingUnit:
		drawOpposingUnitInspector(screen, r.Min.X+18, inspectorBodyY, selection)
	}
	if selection.Kind == SelectionBuilding && selection.Building != nil &&
		selection.Building.Kind == building.Gate && selection.Building.ConstructionStage == building.ConstructionNone {
		drawGateControls(screen, layout, selection.Building)
	}
	if selection.Kind == SelectionBuilding && selection.Building != nil &&
		selection.Building.Kind == building.Barracks && selection.Building.ConstructionStage == building.ConstructionNone {
		drawBarracksHireControls(screen, layout, canHire)
	}
	if selection.Kind == SelectionBuilding && selection.Building != nil &&
		selection.Building.Kind == building.Armory && selection.Building.ConstructionStage == building.ConstructionNone {
		drawArmoryQueueControls(screen, layout, selection.Building, armoryState)
	}
	if selection.Kind == SelectionSoldierGroup && len(selection.SoldierGroup) > 0 {
		drawFormationLinesControl(screen, layout, formationLines)
		drawSoldierGroupActions(screen, layout)
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

// drawConfirmDemolitionModeDialog asks once before continuous map removal is
// armed. It deliberately appears in the empty inspector: no individual
// object has been selected or changed yet.
func drawConfirmDemolitionModeDialog(screen *ebiten.Image, layout Layout) {
	r := layout.InspectorConfirmRemoveRect()
	vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), panelInnerColor, false)
	vector.StrokeRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 2, panelEdgeColor, false)
	DrawInspectorText(screen, i18n.T().ConfirmDemolitionModePrompt, float64(r.Min.X+10), float64(r.Min.Y+12))

	confirm, cancel := layout.InspectorConfirmRemoveButtons()
	vector.FillRect(screen, float32(confirm.Min.X), float32(confirm.Min.Y), float32(confirm.Dx()), float32(confirm.Dy()), color.RGBA{R: 126, G: 53, B: 45, A: 255}, false)
	vector.FillRect(screen, float32(cancel.Min.X), float32(cancel.Min.Y), float32(cancel.Dx()), float32(cancel.Dy()), panelColor, false)
	DrawInspectorText(screen, i18n.T().ConfirmDemolitionModeButton, float64(confirm.Min.X+8), float64(confirm.Min.Y+8))
	DrawInspectorText(screen, i18n.T().SlotCancelButton, float64(cancel.Min.X+8), float64(cancel.Min.Y+8))
}

// drawConfirmTrimServesDialog asks, on a right-click of the Serf card when
// there are more serfs than recommended, whether to dismiss all the way
// down to the recommendation in one go or just one -- see cmd/game's
// handleConfirmTrimServesInput for what each button actually does. prompt
// is pre-formatted by cmd/game (with the current/recommended numbers) so
// this package stays as unaware of i18n formatting args as every other
// dialog here.
func drawConfirmTrimServesDialog(screen *ebiten.Image, layout Layout, prompt string) {
	r := layout.InspectorConfirmRemoveRect()
	vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), panelInnerColor, false)
	vector.StrokeRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 2, panelEdgeColor, false)
	DrawInspectorText(screen, prompt, float64(r.Min.X+10), float64(r.Min.Y+12))

	yes, no := layout.InspectorConfirmRemoveButtons()
	vector.FillRect(screen, float32(yes.Min.X), float32(yes.Min.Y), float32(yes.Dx()), float32(yes.Dy()), selectedColor, false)
	vector.FillRect(screen, float32(no.Min.X), float32(no.Min.Y), float32(no.Dx()), float32(no.Dy()), panelColor, false)
	DrawInspectorText(screen, i18n.T().ConfirmYesButton, float64(yes.Min.X+8), float64(yes.Min.Y+8))
	DrawInspectorText(screen, i18n.T().ConfirmNoButton, float64(no.Min.X+8), float64(no.Min.Y+8))
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
	rowY := layout.rightPanelUsableBottom() - priorityRowHeight - priorityBottomGap
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
func drawGateControls(screen *ebiten.Image, layout Layout, gate *building.Building) {
	t := i18n.T()
	toggle, auto := layout.GateControlRects()
	toggleLabel := t.GateOpenButton
	if gate.GateOpen {
		toggleLabel = t.GateCloseButton
	}
	autoLabel := t.GateAutoOnButton
	if gate.GateAuto {
		autoLabel = t.GateAutoOffButton
	}
	fill := panelColor
	if gate.GateOpen {
		fill = selectedColor
	}
	vector.FillRect(screen, float32(toggle.Min.X), float32(toggle.Min.Y), float32(toggle.Dx()), float32(toggle.Dy()), fill, false)
	vector.FillRect(screen, float32(auto.Min.X), float32(auto.Min.Y), float32(auto.Dx()), float32(auto.Dy()), panelColor, false)
	if gate.GateAuto {
		vector.FillRect(screen, float32(auto.Min.X), float32(auto.Max.Y-3), float32(auto.Dx()), 3, selectedColor, false)
	}
	DrawInspectorText(screen, toggleLabel, float64(toggle.Min.X+8), float64(toggle.Min.Y+8))
	DrawInspectorText(screen, autoLabel, float64(auto.Min.X+8), float64(auto.Min.Y+8))
}

// drawBarracksHireControls draws the three hire buttons for a selected,
// finished Barracks -- Sentry, Archer, Swordsman, the only way to get any
// of them, per the user's explicit request ("Найм будет осуществляться
// только при выборе казармы") extended to the two new units. canHire is
// computed by cmd/game -- this function only draws, it never decides
// availability itself.
func drawBarracksHireControls(screen *ebiten.Image, layout Layout, canHire BarracksHireAvailability) {
	t := i18n.T()
	sentryRect, archerRect, swordsmanRect := layout.BarracksHireRects()
	drawHireButton(screen, sentryRect, t.BarracksHireSentryButton, canHire.Sentry, assets.Sentry[0])
	drawHireButton(screen, archerRect, t.BarracksHireArcherButton, canHire.Archer, assets.Archer[0])
	drawHireButton(screen, swordsmanRect, t.BarracksHireSwordsmanButton, canHire.Swordsman, assets.Swordsman[0])
}

// drawHireButton is one row of drawBarracksHireControls: a filled/outline
// button with a label and, on the right, a small static icon.
func drawHireButton(screen *ebiten.Image, r image.Rectangle, label string, canHire bool, img *ebiten.Image) {
	fill := panelColor
	if canHire {
		fill = selectedColor
	}
	vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), fill, false)

	// The recruit portrait gets its own right-hand column, so long Russian
	// resource requirements remain readable instead of drawing beneath it.
	iconSize := r.Dy() - 8
	if img != nil {
		bounds := img.Bounds()
		scale := float64(iconSize) / float64(bounds.Dy())
		options := &ebiten.DrawImageOptions{}
		options.GeoM.Scale(scale, scale)
		options.GeoM.Translate(float64(r.Max.X-6-iconSize), float64(r.Min.Y+4))
		options.Blend = ebiten.BlendSourceOver
		screen.DrawImage(img, options)
	}
	DrawInspectorText(screen, label, float64(r.Min.X+10), float64(r.Min.Y+11))
}

// armoryQueueItems is the fixed display order for drawArmoryQueueControls
// and Layout.ArmoryQueueRowRects -- must match cmd/game's armoryOrder so a
// row's button always affects the item it visually shows.
var armoryQueueItems = [3]resource.Type{resource.Bow, resource.LeatherArmor, resource.Sword}

// armoryQueueCosts is the material bill for one queued item. It mirrors the
// simulation recipe in cmd/game/armory.go, but is intentionally display-only:
// changing an order never consumes anything until a Weaponsmith completes it.
var armoryQueueCosts = map[resource.Type][]resource.Type{
	resource.Bow:          {resource.Plank},
	resource.LeatherArmor: {resource.Hide},
	resource.Sword:        {resource.Iron, resource.Coal},
}

// drawArmoryQueueControls draws the three production-queue rows for a
// selected, finished Armory -- one per item (Bow/LeatherArmor/Sword), each
// showing the queued count and a -/+ button. Per the user's explicit
// "количество в очереди '-' количество в очереди '+' клик ПКМ +/- 10 штук
// в очередь, ЛКМ +/- 1 в очередь", the +/- buttons only draw the ±1 (LMB)
// affordance -- the ±10 RMB behavior lives entirely in cmd/game's click
// handling, this is just the visual target for both.
func drawArmoryQueueControls(screen *ebiten.Image, layout Layout, armory *building.Building, state ArmoryProductionState) {
	t := i18n.T()
	names := t.ResourceName
	firstRow, _, _ := layout.ArmoryQueueRowRects(0)
	DrawInspectorText(screen, t.ProductionQueueLabel, float64(firstRow.Min.X), float64(firstRow.Min.Y-18))

	for i, item := range armoryQueueItems {
		row, minus, plus := layout.ArmoryQueueRowRects(i)
		countRect := layout.ArmoryQueueCountRect(i)
		fill := panelInnerColor
		if state.Queued && state.Item == item && state.Producing {
			fill = panelColor
		}
		vector.FillRect(screen, float32(row.Min.X), float32(row.Min.Y), float32(row.Dx()), float32(row.Dy()), fill, false)
		drawResourceIcon(screen, item, row.Min.X+5, row.Min.Y+7)
		DrawInspectorText(screen, names[item], float64(row.Min.X+resourceIconSize+12), float64(row.Min.Y+9))
		drawArmoryQueueCost(screen, item, row, countRect)

		// Queue amount has a dedicated cell between the item name and controls:
		// no value can overlap the +/- hit targets any more.
		vector.FillRect(screen, float32(countRect.Min.X), float32(countRect.Min.Y), float32(countRect.Dx()), float32(countRect.Dy()), panelColor, false)
		count := fmt.Sprintf("%d", armory.ProductionQueue[item])
		DrawInspectorText(screen, count, float64(countRect.Min.X+countRect.Dx()/2-4), float64(countRect.Min.Y+9))

		vector.FillRect(screen, float32(minus.Min.X), float32(minus.Min.Y), float32(minus.Dx()), float32(minus.Dy()), panelColor, false)
		DrawInspectorText(screen, "-", float64(minus.Min.X+minus.Dx()/2-3), float64(minus.Min.Y+7))
		vector.FillRect(screen, float32(plus.Min.X), float32(plus.Min.Y), float32(plus.Dx()), float32(plus.Dy()), panelColor, false)
		DrawInspectorText(screen, "+", float64(plus.Min.X+plus.Dx()/2-3), float64(plus.Min.Y+7))
	}
}

func drawArmoryQueueCost(screen *ebiten.Image, item resource.Type, row, countRect image.Rectangle) {
	const costStride = resourceIconSize + 9
	costs := armoryQueueCosts[item]
	// Right-align the bill immediately before the count cell. It can never
	// collide with the +/- buttons, even if the inspector becomes narrower.
	x := countRect.Min.X - 5 - len(costs)*costStride
	for _, material := range costs {
		drawResourceIcon(screen, material, x, row.Min.Y+9)
		DrawInspectorText(screen, "1", float64(x+resourceIconSize+3), float64(row.Min.Y+9))
		x += costStride
	}
}

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
		drawResourceRow(screen, x, y, resource.Plank, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.Plank], b.InputBuffer[resource.Plank], bt.PlankCost))
		y += 18
	}
	if bt.StoneCost > 0 {
		drawResourceRow(screen, x, y, resource.StoneBlock, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.StoneBlock], b.InputBuffer[resource.StoneBlock], bt.StoneCost))
		y += 18
	}
	if bt.IronCost > 0 {
		drawResourceRow(screen, x, y, resource.Iron, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.Iron], b.InputBuffer[resource.Iron], bt.IronCost))
	}
}

func drawBuildingInspector(screen *ebiten.Image, x, y int, b *building.Building, connected bool, stock *resource.Stockpile, occupants int, armoryState ArmoryProductionState) {
	t := i18n.T()
	bt := building.Types[b.Kind]
	DrawInspectorText(screen, t.BuildingName[b.Kind], float64(x), float64(y))
	y += 24
	if b.ConstructionStage != building.ConstructionNone {
		drawConstructionInspector(screen, x, y, b, bt)
		return
	}
	// Only shown while actually damaged -- a permanent "100%" line on
	// every single building in the game would be noise for the vast
	// majority that can never be hit yet (see AGENTS.md: unit combat and
	// real attackers are a later pass).
	if b.HP > 0 && b.HP < building.MaxHP {
		DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HPLabel, b.HP), float64(x), float64(y))
		y += 20
	}
	if b.Kind == building.StoneWall {
		return
	}
	if b.Kind == building.Gate {
		state := t.GateClosedLabel
		if b.GateOpen {
			state = t.GateOpenLabel
		}
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.GateStateLabel, state), float64(x), float64(y))
		y += 20
		auto := t.GateAutoOffButton
		if b.GateAuto {
			auto = t.GateAutoOnButton
		}
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.GateAutoLabel, auto), float64(x), float64(y))
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
		drawResourceRow(screen, x, y, resource.Log, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.Log], b.OutputBuffer[resource.Log], building.BufferCapacity))
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
		drawResourceRow(screen, x, y, resource.Fish, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.Fish], b.OutputBuffer[resource.Fish], building.BufferCapacity))
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
		drawResourceRow(screen, x, y, resource.StoneBlock, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.StoneBlock], b.OutputBuffer[resource.StoneBlock], building.BufferCapacity))
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
			drawResourceRow(screen, x, y, rt, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.OutputBuffer[rt], building.BufferCapacity))
			y += 18
		}
		routeState := t.Disconnected
		if connected {
			routeState = t.Connected
		}
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, routeState), float64(x), float64(y))
		return
	}
	if b.Kind == building.WatchTower {
		DrawInspectorText(screen, t.ContentsLabel, float64(x), float64(y))
		y += 20
		drawResourceRow(screen, x, y, resource.StoneBlock, fmt.Sprintf("%s: %d/%d", t.ResourceName[resource.StoneBlock], b.InputBuffer[resource.StoneBlock], building.BufferCapacity))
		y += 20
		DrawInspectorText(screen, fmt.Sprintf("%s: %d", t.WatchTowerRangeLabel, sentry.WatchTowerRange), float64(x), float64(y))
		y += 20
		routeState := t.Disconnected
		if connected {
			routeState = t.Connected
		}
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, routeState), float64(x), float64(y))
		return
	}
	if b.Kind == building.Barracks {
		DrawInspectorText(screen, t.ContentsLabel, float64(x), float64(y))
		y += 20
		for _, rt := range []resource.Type{resource.Gold, resource.Bow, resource.LeatherArmor, resource.Sword} {
			drawResourceRow(screen, x, y, rt, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.InputBuffer[rt], building.BufferCapacity))
			y += 18
		}
		routeState := t.Disconnected
		if connected {
			routeState = t.Connected
		}
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, routeState), float64(x), float64(y))
		return
	}
	if b.Kind == building.Armory {
		drawArmoryProductionStatus(screen, x, y, t, armoryState)
		y += 20
		DrawInspectorText(screen, t.InputLabel+":", float64(x), float64(y))
		y += 18
		for _, rt := range []resource.Type{resource.Plank, resource.Hide, resource.Iron, resource.Coal} {
			drawResourceRow(screen, x+8, y, rt, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.InputBuffer[rt], building.BufferCapacity))
			y += 18
		}
		DrawInspectorText(screen, t.OutputLabel+":", float64(x), float64(y))
		y += 18
		for _, rt := range []resource.Type{resource.Bow, resource.LeatherArmor, resource.Sword} {
			drawResourceRow(screen, x+8, y, rt, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.OutputBuffer[rt], b.OutputLimit()))
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
			drawResourceRow(screen, x+8, y, rt, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.InputBuffer[rt], building.BufferCapacity))
			y += 18
		}
		DrawInspectorText(screen, t.OutputLabel+":", float64(x), float64(y))
		y += 18
		for _, rt := range []resource.Type{resource.Gold, resource.Iron} {
			drawResourceRow(screen, x+8, y, rt, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.OutputBuffer[rt], b.OutputLimit()))
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
			drawResourceRow(screen, x+8, y, rt, fmt.Sprintf("%s: %d/%d", t.ResourceName[rt], b.InputBuffer[rt], building.BufferCapacity))
			y += 18
		}
	}
	if bt.Recipe.TicksToProduce > 0 {
		DrawInspectorText(screen, t.OutputLabel+":", float64(x), float64(y))
		y += 18
		drawResourceRow(screen, x+8, y, bt.Recipe.Output, fmt.Sprintf("%s: %d/%d", t.ResourceName[bt.Recipe.Output], b.OutputBuffer[bt.Recipe.Output], b.OutputLimit()))
		y += 18
	}
	roadState := t.Disconnected
	if connected || b.Kind == building.Warehouse || b.Kind == building.Road {
		roadState = t.Connected
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.RoadLabel, roadState), float64(x), float64(y))
}

func drawArmoryProductionStatus(screen *ebiten.Image, x, y int, t i18n.Catalog, state ArmoryProductionState) {
	switch {
	case !state.Queued:
		DrawInspectorText(screen, t.ArmoryQueueEmptyLabel, float64(x), float64(y))
	case state.WaitingForWorker:
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.ArmoryWaitingWorkerLabel, t.ResourceName[state.Item]), float64(x), float64(y))
	case state.WaitingForMaterials:
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.ArmoryWaitingMaterialsLabel, t.ResourceName[state.Item]), float64(x), float64(y))
	case state.WaitingForOutput:
		DrawInspectorText(screen, fmt.Sprintf("%s: %s", t.ArmoryOutputFullLabel, t.ResourceName[state.Item]), float64(x), float64(y))
	default:
		DrawInspectorText(screen, fmt.Sprintf("%s: %s %d/%d", t.ArmoryProducingLabel, t.ResourceName[state.Item], state.Progress, state.Total), float64(x), float64(y))
	}
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

// drawOpposingBuildingInspector/drawOpposingUnitInspector are read-only
// summaries for an opposing faction's building/unit -- per the user's
// explicit request ("разреши клик на юнитов противника, и отображай в
// правом окне информацию о нем, но без управления им"): kind, owning
// faction, and HP/construction progress if there's any to show, nothing
// else -- no stockpile, no contents, no action controls (those all
// belong to the player-owned SelectionBuilding/SelectionSoldierGroup/...
// cases above, which this deliberately does not touch).
func drawOpposingBuildingInspector(screen *ebiten.Image, x, y int, b *building.Building) {
	if b == nil {
		return
	}
	t := i18n.T()
	DrawInspectorText(screen, t.BuildingName[b.Kind], float64(x), float64(y))
	y += 24
	DrawInspectorText(screen, factionOwnerLabel(b.Owner), float64(x), float64(y))
	y += 20
	if b.ConstructionStage != building.ConstructionNone {
		DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.ConstructionProgressLabel, int(b.ConstructionProgress()*100)), float64(x), float64(y))
		return
	}
	DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HPLabel, b.HP), float64(x), float64(y))
}

func drawOpposingUnitInspector(screen *ebiten.Image, x, y int, selection Selection) {
	t := i18n.T()
	DrawInspectorText(screen, selection.OpposingUnitKind, float64(x), float64(y))
	y += 24
	DrawInspectorText(screen, factionOwnerLabel(selection.OpposingUnitOwner), float64(x), float64(y))
	if selection.OpposingUnitMaxHP > 0 {
		y += 20
		DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HPLabel, selection.OpposingUnitHP), float64(x), float64(y))
	}
}

// factionOwnerLabel names owner using the same per-faction display name
// the development leaderboard already uses -- "Вы" for the player,
// falling back to a plain number for a faction FactionColorNominative
// doesn't know about (outside "N против ИИ", or an owner value nothing
// else recognizes either).
func factionOwnerLabel(owner int) string {
	t := i18n.T()
	if name, ok := t.FactionColorNominative[owner]; ok {
		return name
	}
	return fmt.Sprintf("%d", owner)
}

// drawEnemyInspector is deliberately minimal: the debug test-attacker
// (package enemy) has no profession, no hunger, nothing to report besides
// confirmation of what's selected and how much health it has left --
// enough for the player to see the right-click-move order landed.
func drawEnemyInspector(screen *ebiten.Image, x, y int, e *enemy.Enemy) {
	t := i18n.T()
	DrawInspectorText(screen, t.UnitEnemy, float64(x), float64(y))
	y += 24
	DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HPLabel, e.HP), float64(x), float64(y))
}

// drawSoldierGroupInspector shows the selected squad's composition (per
// profession -- a Shift+click merge, see cmd/game's mergeSoldierGroups,
// can mix Archers and Swordsmen in one group) and, as a representative
// sample (a full per-unit breakdown isn't worth the panel space for a
// squad-level control), the first member's own HP/hunger.
func drawSoldierGroupInspector(screen *ebiten.Image, x, y int, group []*soldier.Soldier) {
	if len(group) == 0 {
		return
	}
	t := i18n.T()
	archers, swordsmen := 0, 0
	for _, sd := range group {
		if sd.Profession == soldier.Swordsman {
			swordsmen++
		} else {
			archers++
		}
	}
	var parts []string
	if archers > 0 {
		parts = append(parts, fmt.Sprintf("%s ×%d", t.UnitArcher, archers))
	}
	if swordsmen > 0 {
		parts = append(parts, fmt.Sprintf("%s ×%d", t.UnitSwordsman, swordsmen))
	}
	DrawInspectorText(screen, strings.Join(parts, ", "), float64(x), float64(y))
	y += 24
	DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HPLabel, group[0].HP), float64(x), float64(y))
	y += 20
	DrawInspectorText(screen, fmt.Sprintf("%s: %d%%", t.HungerLabel, group[0].SatietyPercent()), float64(x), float64(y))
}

// drawSoldierGroupActions draws the "разъединить"/"отряд по виду" buttons
// -- see Layout.SoldierGroupActionRects.
func drawSoldierGroupActions(screen *ebiten.Image, layout Layout) {
	t := i18n.T()
	split, byProfession := layout.SoldierGroupActionRects()
	vector.FillRect(screen, float32(split.Min.X), float32(split.Min.Y), float32(split.Dx()), float32(split.Dy()), panelColor, false)
	DrawInspectorText(screen, t.SoldierGroupSplitButton, float64(split.Min.X+6), float64(split.Min.Y+8))
	vector.FillRect(screen, float32(byProfession.Min.X), float32(byProfession.Min.Y), float32(byProfession.Dx()), float32(byProfession.Dy()), panelColor, false)
	DrawInspectorText(screen, t.SoldierGroupByProfessionButton, float64(byProfession.Min.X+6), float64(byProfession.Min.Y+8))
}

// drawFormationLinesControl draws the "1/2/3 ranks" buttons for a selected
// soldier group -- see Layout.FormationLinesRects.
func drawFormationLinesControl(screen *ebiten.Image, layout Layout, current int) {
	t := i18n.T()
	one, two, three := layout.FormationLinesRects()
	DrawInspectorText(screen, t.FormationLinesLabel, float64(one.Min.X), float64(one.Min.Y-18))
	for i, r := range [3]image.Rectangle{one, two, three} {
		n := i + 1
		fill := panelColor
		if current == n {
			fill = selectedColor
		}
		vector.FillRect(screen, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), fill, false)
		DrawInspectorText(screen, fmt.Sprintf("%d", n), float64(r.Min.X+r.Dx()/2-4), float64(r.Min.Y+7))
	}
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
	case SelectionEnemy:
		if selection.Enemy == nil {
			return 0, 0, nil, false
		}
		return selection.Enemy.X, selection.Enemy.Y, selection.Enemy.RemainingPath(), true
	default:
		return 0, 0, nil, false
	}
}

// DrawSelectionMarker draws a warm outline under the selected object so the
// player can connect the inspector to the world even when sprites overlap.
// A SelectionSoldierGroup is the one multi-object case -- every living
// member of the group gets its own outline, not just a single tile (see
// selectedUnitTile's "one tile, one path" doc comment for why it can't
// represent a group itself).
func DrawSelectionMarker(screen *ebiten.Image, cam *render.Camera, selection Selection) {
	if selection.Kind == SelectionSoldierGroup {
		size := cam.TilePixels()
		for _, sd := range selection.SoldierGroup {
			if sd == nil || !sd.Alive() {
				continue
			}
			x, y := cam.TileToScreen(sd.X, sd.Y)
			drawSelectionBox(screen, x, y, size)
		}
		return
	}

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
	drawSelectionBox(screen, x, y, size)
}

// drawSelectionBox draws the actual warm-yellow outline square DrawSelectionMarker
// uses, factored out so both the single-object path and the per-soldier
// SelectionSoldierGroup loop share one drawing implementation.
func drawSelectionBox(screen *ebiten.Image, x, y, size float64) {
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
	if selection.Kind == SelectionSoldierGroup {
		for _, sd := range selection.SoldierGroup {
			if sd == nil || !sd.Alive() {
				continue
			}
			drawRouteFrom(screen, cam, sd.X, sd.Y, sd.RemainingPath())
		}
		return
	}
	tx, ty, path, ok := selectedUnitTile(selection)
	if !ok || len(path) == 0 {
		return
	}
	drawRouteFrom(screen, cam, tx, ty, path)
}

// drawRouteFrom draws the actual route line DrawSelectedRoute uses, from
// (tx, ty) through every point in path, factored out so both the
// single-object path and the per-soldier SelectionSoldierGroup loop share
// one drawing implementation.
func drawRouteFrom(screen *ebiten.Image, cam *render.Camera, tx, ty int, path []pathfind.Point) {
	if len(path) == 0 {
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
	case building.StoneWall:
		img = assets.StoneWallHorizontal
	case building.Gate:
		img = assets.GateHorizontalClosed
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
	case building.WatchTower:
		img = assets.WatchTower
	case building.Barracks:
		img = assets.Barracks
	case building.Armory:
		img = assets.Armory
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
