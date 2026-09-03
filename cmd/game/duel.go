package main

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/i18n"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/miner"
	"strategy_game/internal/quarry"
	"strategy_game/internal/render"
	"strategy_game/internal/resource"
	"strategy_game/internal/sentry"
	"strategy_game/internal/soldier"
	"strategy_game/internal/ui"
	"strategy_game/internal/villagers"
	"strategy_game/internal/world"
)

// duelMapWidth/duelMapHeight are the "1×1 против ИИ" map dimensions --
// roughly double the ordinary mapWidth*mapHeight area (per the user's
// explicit "увеличим карту в 2 раза [по общей площади]"), scaled by
// sqrt(2) on each axis rather than doubling each dimension outright.
const (
	duelMapWidth  = 71
	duelMapHeight = 54

	// duelWaterStripWidth/duelIsthmusWidth size the vertical water divide
	// carved by growCenterWaterStrip -- wide enough to read as a real sea
	// channel, with one narrow dry crossing.
	duelWaterStripWidth = 6
	duelIsthmusWidth    = 4
)

// growCenterWaterStrip carves a vertical water divide down the middle of
// a duel map, with one narrow land isthmus left dry at a seeded row
// range -- per the user's explicit "вода разделяет 2 земли... земли
// соединяются небольшим перешейком на котором не спавнятся ресурсы".
// Unlike growSeaRegion (which hugs a single map edge with a random-walk
// depth, for the ordinary single-player map), this fills a fixed-width
// band around the vertical center line, uniform except for the isthmus
// gap -- deliberately simple and exactly reproducible, since both
// territories must end up geometrically identical in shape.
func growCenterWaterStrip(g *world.Grid, seed uint32) (isthmusMinY, isthmusMaxY int) {
	centerX := g.Width / 2
	left := centerX - duelWaterStripWidth/2
	// right is derived as g.Width-left, not centerX+duelWaterStripWidth/2,
	// so the band is exactly symmetric under mirrorX (mirrorX(w,x) =
	// w-1-x): a tile x is in [left,right) iff mirrorX(w,x) is too only
	// when left+right == w. A real bug found from an actual playtest
	// report: the old centerX+width/2 formula left left+right == w-1,
	// off by exactly one tile -- every mirrored natural resource whose
	// original sat right at that one edge column landed back on dry
	// land (or vice versa) instead of water, silently failing to place
	// and leaving the two sides' Fish counts visibly unequal (61 vs 41
	// in one recorded run) despite mirrorNaturalResourcesForFairness
	// otherwise mirroring everything correctly.
	right := g.Width - left
	if left < 1 {
		left = 1
	}
	if right > g.Width-1 {
		right = g.Width - 1
	}

	margin := 3
	span := g.Height - 2*margin - duelIsthmusWidth
	if span < 1 {
		span = 1
	}
	isthmusMinY = margin + int(seed%uint32(span))
	isthmusMaxY = isthmusMinY + duelIsthmusWidth

	for y := 0; y < g.Height; y++ {
		if y >= isthmusMinY && y < isthmusMaxY {
			continue // the one dry crossing
		}
		for x := left; x < right; x++ {
			g.Set(x, y, world.Tile{Terrain: world.Water})
		}
	}
	return isthmusMinY, isthmusMaxY
}

// mirrorX reflects x across the map's vertical center line -- the one
// coordinate transform a duel map needs to guarantee the two factions'
// starting points are exactly equidistant from the map's center, per the
// user's explicit "респы противников должны быть равно удалены друг от
// друга".
func mirrorX(width, x int) int {
	return width - 1 - x
}

// findDuelWarehouseSpot searches outward in growing square rings from
// (targetX, targetY) for the nearest tile that can actually hold a
// Warehouse with a buildable tile for its starting Road immediately
// south -- the same two constraints newGameWithSize's own
// findWarehouseSpot enforces for the ordinary single-player map, just
// searching from a fixed target point instead of scoring the whole map.
func findDuelWarehouseSpot(grid *world.Grid, targetX, targetY int) (gridPoint, bool) {
	valid := func(x, y int) bool {
		return building.CanPlace(grid, nil, building.Warehouse, x, y) &&
			grid.InBounds(x, y+1) && grid.At(x, y+1).Buildable()
	}
	if valid(targetX, targetY) {
		return gridPoint{targetX, targetY}, true
	}
	for radius := 1; radius <= 20; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if absInt(dx) != radius && absInt(dy) != radius {
					continue
				}
				x, y := targetX+dx, targetY+dy
				if !grid.InBounds(x, y) || !valid(x, y) {
					continue
				}
				return gridPoint{x, y}, true
			}
		}
	}
	return gridPoint{}, false
}

// newDuelGame builds a fresh "1×1 против ИИ" game: a symmetric
// water-divided map (see growCenterWaterStrip), a player Warehouse on
// the west shore and an AI Warehouse mirrored onto the east shore (so
// both starting points are exactly equidistant from the map's own
// center), identical starting resources for both sides, and a second,
// fully independent faction (see faction/newFaction) driving the AI's
// own economy and army every simulation tick (tickAIFaction).
//
// Deliberately reuses the ordinary single-player generation functions
// (seedTrees/seedThickets/seedFish/seedStoneDeposits/seedOreDeposits)
// called once across the whole map rather than mirroring one half's
// output into the other -- a real simplification from the original
// design (see AGENTS.md's "1×1 против ИИ" notes): both sides still get
// comparable resources at the same density the single-player map
// already uses, just not a pixel-exact mirror image of each other.
func newDuelGame(difficulty aiDifficulty) *Game {
	mapSeed := newMapSeed()
	grid := world.NewGrid(duelMapWidth, duelMapHeight)
	_, _ = growCenterWaterStrip(grid, mapSeed^0x9e3779b9)

	playerPoint, ok := findDuelWarehouseSpot(grid, duelMapWidth/4, duelMapHeight/2)
	if !ok {
		playerPoint = gridPoint{2, duelMapHeight / 2}
	}
	aiTargetX := mirrorX(duelMapWidth, playerPoint.x)
	aiPoint, ok := findDuelWarehouseSpot(grid, aiTargetX, playerPoint.y)
	if !ok {
		aiPoint = gridPoint{mirrorX(duelMapWidth, playerPoint.x), playerPoint.y}
	}

	// HP is set explicitly on every building below (building.MaxHP) --
	// the zero value means "not yet migrated" elsewhere in this codebase
	// (see save.migrateBuildingHP's doc comment) and, now that
	// pruneDestroyedBuildings actually removes a building whose HP
	// reaches 0, an unset HP would make a building vanish the instant
	// this game's very first tick runs.
	warehouse := &building.Building{Kind: building.Warehouse, X: playerPoint.x, Y: playerPoint.y, Owner: 0, HP: building.MaxHP}
	initialRoad := &building.Building{Kind: building.Road, X: playerPoint.x, Y: playerPoint.y + 1, Owner: 0, HP: building.MaxHP}
	aiWarehouse := &building.Building{Kind: building.Warehouse, X: aiPoint.x, Y: aiPoint.y, Owner: 1, HP: building.MaxHP}
	aiRoad := &building.Building{Kind: building.Road, X: aiPoint.x, Y: aiPoint.y + 1, Owner: 1, HP: building.MaxHP}

	buildings := []*building.Building{warehouse, initialRoad, aiWarehouse, aiRoad}
	buildings = seedTrees(grid, buildings)
	buildings = seedThickets(grid, buildings, mapSeed^0x27d4eb2f)
	buildings = seedFish(grid, buildings)
	buildings = seedStoneDeposits(grid, buildings, defaultStoneSeed, playerPoint, minDepositDistanceFromWarehouse)
	buildings = seedOreDeposits(grid, buildings, building.CoalDeposit, coalMinPercent, coalMaxPercent, defaultCoalSeed, playerPoint, minDepositDistanceFromWarehouse)
	buildings = seedOreDeposits(grid, buildings, building.GoldOreDeposit, goldOreMinPercent, goldOreMaxPercent, defaultGoldOreSeed, playerPoint, minDepositDistanceFromWarehouse)
	buildings = seedOreDeposits(grid, buildings, building.IronOreDeposit, ironOreMinPercent, ironOreMaxPercent, defaultIronOreSeed, playerPoint, minDepositDistanceFromWarehouse)
	buildings = mirrorNaturalResourcesForFairness(grid, buildings, playerPoint, aiPoint)

	stock := resource.NewStockpile(stockpileCapacity)
	stock.Add(resource.Plank, startingPlanks)
	stock.Add(resource.StoneBlock, startingStone)
	stock.Add(resource.Bread, startingBread)
	stock.Add(resource.Fish, startingFish)
	stock.Add(resource.Sausage, startingSausage)
	stock.Add(resource.Wine, startingWine)
	stock.Add(resource.Gold, startingGold)

	layout := ui.NewLayout(screenWidth, screenHeight)
	camera := render.NewCamera()
	mapRect := layout.MapRect()
	camera.SetViewport(mapRect.Min.X, mapRect.Min.Y, mapRect.Dx(), mapRect.Dy())

	game := &Game{
		grid:           grid,
		buildings:      buildings,
		stock:          stock,
		pop:            &economy.Population{},
		sim:            economy.NewSimulator(framesPerSimTick),
		logi:           logistics.NewController(warehouse, startingSerfs),
		vills:          villagers.NewController(),
		jacks:          lumberjack.NewController(),
		fishers:        fishing.NewController(),
		quarry:         quarry.NewController(),
		builders:       builder.NewController(),
		miners:         miner.NewController(),
		sentries:       sentry.NewController(),
		soldiers:       soldier.NewController(),
		ai:             newFaction(1, aiWarehouse, difficulty),
		formationLines: 2,
		treeSeed:       defaultTreeSeed,
		fishSeed:       defaultFishSeed,
		stoneSeeded:    true,
		oreSeeded:      true,
		camera:         camera,
		palette:        ui.NewPalette(),
		layout:         layout,
	}
	game.refreshPopulation()
	game.refreshSlotCache()
	return game
}

// mirrorNaturalResourcesForFairness replaces every natural resource node
// (Tree/Fish/StoneDeposit/CoalDeposit/GoldOreDeposit/IronOreDeposit) with
// a left/right-symmetric layout: only the left half (the player's side,
// x < duelMapWidth/2) of what the ordinary single-player seed functions
// generated is kept, and each surviving node gets an exact mirrorX
// counterpart placed on the AI's side.
//
// A real fairness bug found from an actual playtest report: the original
// design deliberately called the single-player seed functions once
// across the WHOLE map, unmirrored, as a documented simplification ("a
// real simplification from the original plan... both sides still get
// comparable resources at the same density... just not a pixel-exact
// mirror image"). In practice this meant no per-side balancing at all --
// stone/ore deposits only avoided the PLAYER's warehouse point (see
// seedStoneDeposits/seedOreDeposits' avoid parameter above), never the
// AI's, and each deposit type's 2-5 regions each start from one
// seed-derived point with no left/right balancing whatsoever. The
// reported outcome: one match had the AI's side holding effectively all
// the stone and iron ore, the player's holding almost none. Exact
// mirroring is the only approach that actually guarantees fairness
// rather than leaving it to chance.
func mirrorNaturalResourcesForFairness(grid *world.Grid, buildings []*building.Building, playerPoint, aiPoint gridPoint) []*building.Building {
	centerX := duelMapWidth / 2
	isDeposit := func(kind building.Kind) bool {
		switch kind {
		case building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
			return true
		default:
			return false
		}
	}

	kept := make([]*building.Building, 0, len(buildings))
	// canonical folds every natural resource to its left-half (x <
	// centerX) position, deduplicated by (kind, foldedX, y) -- not
	// simply "discard anything already on the right half", which was a
	// real, severe bug found from an actual playtest report ("камня - 0,
	// золота - 0"): seedStoneDeposits/seedOreDeposits' avoid parameter
	// only ever pushes a deposit AWAY from the player's own warehouse,
	// with nothing pulling it toward either half specifically -- given
	// growStoneRegion's deterministic, fixed-seed scoring and a
	// near-constant playerPoint, this consistently placed *100%* of
	// stone (confirmed: 76/76 across 5 independent runs) on the AI's
	// side. Discarding the right half then meant discarding ALL of it,
	// leaving nothing at all to mirror -- worse than the original
	// fairness bug, not better. Folding first (instead of discarding)
	// keeps the deposits that generation actually produced, wherever
	// they landed, and only repositions them into a symmetric layout.
	type canonicalKey struct {
		kind building.Kind
		x, y int
	}
	canonical := make(map[canonicalKey]*building.Building)
	var order []canonicalKey
	for _, b := range buildings {
		if !isNaturalResourceKind(b.Kind) {
			kept = append(kept, b)
			continue
		}
		foldedX := b.X
		if foldedX >= centerX {
			foldedX = mirrorX(duelMapWidth, foldedX)
		}
		key := canonicalKey{b.Kind, foldedX, b.Y}
		if _, seen := canonical[key]; seen {
			continue
		}
		canonical[key] = b
		order = append(order, key)
	}

	// canOK/place are split, and every pair below is checked with canOK
	// on BOTH the left and mirrored position before either is committed
	// -- a real bug found immediately after the fold rewrite above: with
	// two independent, unconditional place() calls, one side succeeding
	// could commit a tile that then blocks its own pair's other side (or
	// a LATER pair's side) from ever placing, silently drifting the
	// left/right counts apart (confirmed: 69 vs 88 Fish in one run) even
	// though the fold itself is exactly symmetric. Checking the whole
	// pair against a single, shared, not-yet-mutated kept snapshot
	// before adding either keeps every pair atomic: either both sides
	// land, or neither does.
	canOK := func(kind building.Kind, x, y int, avoid gridPoint) bool {
		if isDeposit(kind) && tooCloseToPoint(x, y, avoid, minDepositDistanceFromWarehouse) {
			return false
		}
		return building.CanPlace(grid, kept, kind, x, y)
	}
	place := func(kind building.Kind, x, y int, reserve, growthTicks, growthTarget int) {
		kept = append(kept, &building.Building{
			Kind:              kind,
			X:                 x,
			Y:                 y,
			Reserve:           reserve,
			GrowthTicks:       growthTicks,
			GrowthTargetTicks: growthTarget,
		})
	}
	for _, key := range order {
		src := canonical[key]
		mx := mirrorX(duelMapWidth, key.x)
		if mx == key.x {
			// The one self-mirroring column -- a single copy, no pair.
			if canOK(src.Kind, key.x, key.y, playerPoint) {
				place(src.Kind, key.x, key.y, src.Reserve, src.GrowthTicks, src.GrowthTargetTicks)
			}
			continue
		}
		if canOK(src.Kind, key.x, key.y, playerPoint) && canOK(src.Kind, mx, key.y, aiPoint) {
			place(src.Kind, key.x, key.y, src.Reserve, src.GrowthTicks, src.GrowthTargetTicks)
			place(src.Kind, mx, key.y, src.Reserve, src.GrowthTicks, src.GrowthTargetTicks)
		}
	}
	return kept
}

// factionDefeated reports whether owner has been fully wiped out --
// every building except Road/StoneWall/Gate destroyed, and every unit
// dead -- per the user's explicit win condition ("все здания и юниты
// противника уничтожены (дорога и стены не в счет)"). Only meaningful
// once g.ai != nil (a "1×1 против ИИ" game); owner 0 or 1.
// duelResult is the outcome of a "1×1 против ИИ" match -- see Game's own
// duelResult field doc comment.
type duelResult int

const (
	duelResultNone duelResult = iota
	duelResultVictory
	duelResultDefeat
)

// checkDuelResult sets g.duelResult the first tick either faction is
// fully defeated -- see factionDefeated. A no-op outside "1×1 против ИИ"
// (g.ai == nil) or once a result is already set (a finished match's
// simulation is frozen -- see Update's early return on g.duelResult --
// so this would never re-fire anyway, but a defensive check costs
// nothing and documents the intent).
func (g *Game) checkDuelResult() {
	if g.ai == nil || g.duelResult != duelResultNone {
		return
	}
	switch {
	case g.factionDefeated(1):
		g.duelResult = duelResultVictory
	case g.factionDefeated(0):
		g.duelResult = duelResultDefeat
	}
}

func (g *Game) factionDefeated(owner int) bool {
	for _, b := range g.buildings {
		if b.Owner != owner {
			continue
		}
		switch b.Kind {
		case building.Road, building.StoneWall, building.Gate:
			continue
		}
		return false
	}
	return g.factionUnitCount(owner) == 0
}

func (g *Game) factionUnitCount(owner int) int {
	if owner == 0 {
		return len(g.logi.Serfs) + len(g.vills.Villagers) + len(g.jacks.Lumberjacks) + len(g.fishers.Fishermen) + len(g.quarry.Quarrymen) + len(g.builders.Builders) + len(g.miners.Miners) + len(g.sentries.Sentries) + len(g.soldiers.Soldiers)
	}
	if g.ai == nil {
		return 0
	}
	f := g.ai
	return len(f.logi.Serfs) + len(f.vills.Villagers) + len(f.jacks.Lumberjacks) + len(f.fishers.Fishermen) + len(f.quarry.Quarrymen) + len(f.builders.Builders) + len(f.miners.Miners) + len(f.sentries.Sentries) + len(f.soldiers.Soldiers)
}

// duelResultBackRect is the one button a finished match's overlay shows
// -- reuses titleBackRect's exact geometry so it lands in the same,
// already-familiar screen position as every other "Назад"-shaped button.
func duelResultBackRect(width, height int) image.Rectangle {
	return titleBackRect(width, height)
}

// updateDuelResult handles input while g.duelResult != duelResultNone --
// the simulation itself is already frozen (Update's own early return
// routes here instead of the ordinary play loop). The only action
// available is leaving to the title screen; the finished match's own
// buildings/units stay exactly as they ended, simply no longer ticking,
// so nothing here needs to reset or clean up faction state itself.
func (g *Game) updateDuelResult() error {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return nil
	}
	mx, my := ebiten.CursorPosition()
	frontWidth, frontHeight := g.frontScreenDimensions()
	if duelResultBackClicked(mx, my, frontWidth, frontHeight) {
		g.screen = screenTitle
		g.duelResult = duelResultNone
	}
	return nil
}

// duelResultBackClicked is updateDuelResult's click resolution, pulled
// out as a pure function -- same "testable without live ebiten cursor
// state" shape as title.go's own titleActionAt/modeSelectActionAt.
func duelResultBackClicked(mx, my, width, height int) bool {
	return image.Pt(mx, my).In(duelResultBackRect(width, height))
}

// drawDuelResult overlays the match outcome on top of the frozen game
// world (already drawn by the ordinary Draw call this frame) -- the same
// "dim the world, show a panel on top" shape drawPauseMenu already uses,
// not a separate screen, so the player's final town stays visible behind
// the result instead of cutting straight to a blank menu.
func (g *Game) drawDuelResult(screen *ebiten.Image) {
	bounds := screen.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	vector.FillRect(screen, 0, 0, float32(width), float32(height), color.RGBA{R: 12, G: 12, B: 14, A: 190}, false)

	panel := image.Rect(width/2-220, height/2-110, width/2+220, height/2+110)
	drawTitlePanel(screen, panel)

	t := i18n.T()
	title, subtitle := t.DuelVictoryTitle, t.DuelVictorySubtitle
	if g.duelResult == duelResultDefeat {
		title, subtitle = t.DuelDefeatTitle, t.DuelDefeatSubtitle
	}
	ui.DrawTitleText(screen, title, float64(panel.Min.X+24), float64(panel.Min.Y+30), 2.6)
	ui.DrawMenuText(screen, subtitle, float64(panel.Min.X+24), float64(panel.Min.Y+78))
	drawTitleButton(screen, duelResultBackRect(width, height), t.DuelResultToTitle, false)
}
