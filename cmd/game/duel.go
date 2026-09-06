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

// duelMapWidth/duelMapHeight are the "N против ИИ" map dimensions.
// Originally sized for a 2-territory map ("общая площадь ×2" of the
// free-map size); kept as-is for the 4-quadrant map too, per the user's
// explicit choice to use ONE map shape "вне зависимости от количества
// игроков" -- a quadrant here is roughly the size the old 2-territory
// map gave each half, not cramped further.
const (
	duelMapWidth  = 71
	duelMapHeight = 54

	// duelWaterStripWidth/duelIsthmusWidth size the water cross carved by
	// growQuadrantWaterCross -- wide enough to read as a real sea channel,
	// with narrow dry crossings between neighbouring quadrants.
	duelWaterStripWidth = 6
	duelIsthmusWidth    = 4

	// duelXxxDepositTiles are the exact per-quadrant tile counts for each
	// mineral on a duel map, per the user's explicit request ("оставь
	// только по 1 клетке золота, 3 клетки угля, 2 клетки камня, и 1
	// железо"). Each count is placed once, canonically, then
	// mirrorNaturalResourcesForFairness folds it into all four quadrants
	// -- so the map ends up with up to 4x this many of each mineral in
	// total, one cluster per quadrant. Replaces the single-player
	// percentage-based seedStoneDeposits/seedOreDeposits for the duel map
	// specifically (see growFixedDepositRegion) -- those scale with the
	// whole map's area, which on this much bigger 4-quadrant map produced
	// a wildly overabundant result a real playtest report called out.
	duelGoldOreDepositTiles = 1
	duelIronOreDepositTiles = 1
	duelCoalDepositTiles    = 3
	duelStoneDepositTiles   = 2

	// duelTreeTilesPerQuadrant is 8 percent of ONE quadrant's own area --
	// first set to 0.5% per the user's own initial request, raised to 2%
	// once that read as too sparse in an actual playtest, doubled to 4%
	// per a later playtest report ("увелич в 2 раза спавн деревьев, их
	// не хватает"), then doubled again to 8%. See seedTreesWithCount's
	// doc comment for why the plain single-player seedTrees (one percent
	// of THIS grid's own area) massively overshot on a duel map, whose
	// grid is the whole 4-quadrant map, not one quadrant.
	duelTreeTilesPerQuadrant = duelMapWidth * duelMapHeight * 8 / (4 * 100)

	// maxDepositDistanceFromWarehouse caps how far a mineral cluster's
	// canonical region may land from the player's own warehouse, per the
	// user's explicit "переделай спавн ресурсов чтоб они появлялись рядом
	// а не раскиданые на карте" -- see growFixedDepositRegion/
	// regionWithinDistance. minDepositDistanceFromWarehouse (10) already
	// keeps it from landing right on top of the warehouse; this keeps it
	// from landing clear across the quadrant instead.
	maxDepositDistanceFromWarehouse = 20
)

// quadrant identifies one of the four territories a duel map is divided
// into. Values double as an index into arrays keyed by quadrant (see
// quadrantWarehouseTargets) -- deliberately NOT the same thing as a
// faction's Owner (a 1×1 match only uses quadrantNW/quadrantSE, say, and
// still assigns Owner 0/1 to whichever factions actually exist).
type quadrant int

const (
	quadrantNW quadrant = iota
	quadrantNE
	quadrantSW
	quadrantSE
	quadrantCount
)

// mirrorX/mirrorY reflect a coordinate across the map's vertical/
// horizontal center line -- the transform a duel map needs to guarantee
// every faction's starting point is equidistant from the map's center,
// per the user's explicit "респы противников должны быть равно удалены
// друг от друга", now generalized to two axes for four quadrants.
func mirrorX(width, x int) int  { return width - 1 - x }
func mirrorY(height, y int) int { return height - 1 - y }

// growQuadrantWaterCross divides a duel map into four quadrants (NW/NE/
// SW/SE) with a vertical and a horizontal water band crossing at the
// map's center, per the user's explicit "карту разделенную на 4 участка"
// -- used for every duel match regardless of how many factions actually
// occupy it (1, 2 or 3 opponents), not a separate simpler map for the
// 1×1 case.
//
// Four dry crossings, one per pair of ADJACENT quadrants (NW-NE, SW-SE
// across the vertical band; NW-SW, NE-SE across the horizontal band),
// arranged in mirrored pairs (the NW-NE crossing's exact Y-mirror is the
// SW-SE crossing; the NW-SW crossing's exact X-mirror is the NE-SE
// crossing) -- this keeps the whole water layout itself symmetric under
// mirrorX/mirrorY/both, not just the resources placed on top of it (see
// mirrorNaturalResourcesForFairness), and forms a 4-cycle (NW-NE-SE-SW-NW)
// so every quadrant reaches every other one, never leaving one isolated.
//
// Deliberately simpler than the old growCenterWaterStrip's randomly
// seeded single isthmus: each crossing sits at a fixed offset (half of
// its own margin) rather than a random one. A random offset risks two
// independently-placed crossings colliding or leaving a gap depending on
// where they land; a fixed, geometry-derived offset is exactly
// reproducible and trivially proven connected once, rather than needing
// to re-verify connectivity for every random seed.
func growQuadrantWaterCross(g *world.Grid) []image.Rectangle {
	centerX := g.Width / 2
	centerY := g.Height / 2
	left := centerX - duelWaterStripWidth/2
	right := g.Width - left // see mirrorX symmetry note on the old version
	if left < 1 {
		left = 1
	}
	if right > g.Width-1 {
		right = g.Width - 1
	}
	top := centerY - duelWaterStripWidth/2
	bottom := g.Height - top
	if top < 1 {
		top = 1
	}
	if bottom > g.Height-1 {
		bottom = g.Height - 1
	}

	// North/south isthmus rows on the vertical band -- confined to
	// y<top / y>=bottom respectively, so they never overlap the
	// horizontal band's own rectangle (rows [top,bottom)) and the two
	// passes below never fight over the same tile.
	northMidY := top / 2
	northMinY := northMidY - duelIsthmusWidth/2
	northMaxY := northMinY + duelIsthmusWidth
	southMinY := g.Height - northMaxY
	southMaxY := g.Height - northMinY

	// West/east isthmus columns on the horizontal band -- confined to
	// x<left / x>=right, same reasoning.
	westMidX := left / 2
	westMinX := westMidX - duelIsthmusWidth/2
	westMaxX := westMinX + duelIsthmusWidth
	eastMinX := g.Width - westMaxX
	eastMaxX := g.Width - westMinX

	for y := 0; y < g.Height; y++ {
		if (y >= northMinY && y < northMaxY) || (y >= southMinY && y < southMaxY) {
			continue // NW-NE or SW-SE crossing
		}
		for x := left; x < right; x++ {
			g.Set(x, y, world.Tile{Terrain: world.Water})
		}
	}
	for x := 0; x < g.Width; x++ {
		if (x >= westMinX && x < westMaxX) || (x >= eastMinX && x < eastMaxX) {
			continue // NW-SW or NE-SE crossing
		}
		for y := top; y < bottom; y++ {
			g.Set(x, y, world.Tile{Terrain: world.Water})
		}
	}

	return []image.Rectangle{
		image.Rect(left, northMinY, right, northMaxY),
		image.Rect(left, southMinY, right, southMaxY),
		image.Rect(westMinX, top, westMaxX, bottom),
		image.Rect(eastMinX, top, eastMaxX, bottom),
	}
}

// quadrantWarehouseTargets returns one search-anchor point per quadrant,
// symmetric under mirrorX/mirrorY -- findDuelWarehouseSpot expands
// outward from each to find the actual placeable tile. Mirrors the old
// duelMapWidth/4-style anchor (a quarter of the way in from the edge on
// each axis) into all four quadrants at once.
func quadrantWarehouseTargets(width, height int) [quadrantCount]gridPoint {
	x0, y0 := width/4, height/4
	return [quadrantCount]gridPoint{
		quadrantNW: {x0, y0},
		quadrantNE: {mirrorX(width, x0), y0},
		quadrantSW: {x0, mirrorY(height, y0)},
		quadrantSE: {mirrorX(width, x0), mirrorY(height, y0)},
	}
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

// quadrantAssignmentOrder picks which quadrant each faction starts in,
// player first -- NW then the diagonally opposite SE for a 1-opponent
// match (as far apart as the map allows, matching the original
// 2-territory design's "opposite shores" intent), then NE/SW filling in
// as more opponents join. Index 0 is always the player.
var quadrantAssignmentOrder = [quadrantCount]quadrant{quadrantNW, quadrantSE, quadrantNE, quadrantSW}

// newDuelGame builds a fresh "N против ИИ" game: a symmetric,
// four-quadrant water-crossed map (see growQuadrantWaterCross) used
// regardless of how many opponents this particular match has, a player
// Warehouse in one quadrant and one AI Warehouse (each its own fully
// independent faction, see faction/newFaction) in as many of the
// remaining three quadrants as len(difficulties) calls for, and
// identical starting resources for every faction on any difficulty.
//
// Deliberately reuses the ordinary single-player generation functions for
// Trees/Thickets/Fish (seedTrees/seedThickets/seedFish) called once across
// the whole map rather than mirroring one quadrant's output into the
// other three -- a real simplification from the original design (see
// AGENTS.md's "1×1 против ИИ" notes): every quadrant still gets comparable
// wood/food at the same density the single-player map already uses, just
// not a pixel-exact mirror image of each other before
// mirrorNaturalResourcesForFairness folds them into one.
//
// Mineral deposits (Stone/Coal/GoldOre/IronOre) do NOT reuse the
// single-player percentage-based seedStoneDeposits/seedOreDeposits --
// those scale with the WHOLE map's area, which on this much bigger
// 4-quadrant map produced a wildly overabundant duel map (hundreds of
// tiles of each) that a real playtest report called out by name
// ("деревьев просто какое-то нереальное количество создалось" -- filed
// against ore/stone specifically, see the fixed duelXxxDepositTiles
// constants below and growFixedDepositRegion). Each mineral gets exactly
// that many tiles placed once, then mirrorNaturalResourcesForFairness
// folds the same canonical region into all four quadrants below, same as
// everything else natural.
func newDuelGame(difficulties []aiDifficulty) *Game {
	grid := world.NewGrid(duelMapWidth, duelMapHeight)
	isthmuses := growQuadrantWaterCross(grid)

	targets := quadrantWarehouseTargets(duelMapWidth, duelMapHeight)
	var points [quadrantCount]gridPoint
	for q := quadrant(0); q < quadrantCount; q++ {
		p, ok := findDuelWarehouseSpot(grid, targets[q].x, targets[q].y)
		if !ok {
			p = targets[q]
		}
		points[q] = p
	}

	// HP is set explicitly on every building below (building.MaxHP) --
	// the zero value means "not yet migrated" elsewhere in this codebase
	// (see save.migrateBuildingHP's doc comment) and, now that
	// pruneDestroyedBuildings actually removes a building whose HP
	// reaches 0, an unset HP would make a building vanish the instant
	// this game's very first tick runs.
	var buildings []*building.Building
	var warehouses [quadrantCount]*building.Building
	activeCount := 1 + len(difficulties)
	if activeCount > int(quadrantCount) {
		activeCount = int(quadrantCount)
	}
	for i := 0; i < activeCount; i++ {
		q := quadrantAssignmentOrder[i]
		p := points[q]
		owner := i // 0 = player, 1..N = bots, in quadrantAssignmentOrder
		wh := &building.Building{Kind: building.Warehouse, X: p.x, Y: p.y, Owner: owner, HP: building.MaxHP}
		road := &building.Building{Kind: building.Road, X: p.x, Y: p.y + 1, Owner: owner, HP: building.MaxHP}
		warehouses[q] = wh
		buildings = append(buildings, wh, road)
	}

	playerPoint := points[quadrantAssignmentOrder[0]]
	// seedTreesWithCount, not seedTrees/seedThickets -- a real playtest
	// report ("деревьев очень много спавнится на карте... что-то явно
	// сломалось в генерации"): seedTrees' own one-percent rule already
	// scales with THIS grid's area, and this grid is the whole 4-quadrant
	// duel map, much bigger than a single-player map -- seedThickets'
	// dense clusters piled on top scaled the same way. Per the user's
	// explicit request ("по деревьем... по 0.5% клеток в зоне юзера" +
	// "разбросаны, просто реже", not clustered like a mineral deposit):
	// half a percent of ONE quadrant's own area, scattered the same way
	// seedTrees always has, with no separate thicket-cluster mechanic on
	// the duel map at all.
	buildings = seedTreesWithCount(grid, buildings, duelTreeTilesPerQuadrant)
	buildings = seedFish(grid, buildings)
	buildings = pruneNaturalResourcesFromIsthmus(buildings, isthmuses)
	// Trees/Thickets/Fish are mirrored into their own final, stable
	// four-quadrant layout BEFORE any mineral is seeded -- a real bug
	// found testing this exact ordering (see
	// TestNewDuelGame_MineralDepositCountsAreFixedNotPercentage):
	// growFixedDepositRegion's mirror-clearance check (see
	// regionMirrorTargetsClear) can only see what's actually in
	// buildings at the time it runs. Trees are seeded per-quadrant
	// independently, not mirrored, so seeding minerals BEFORE this step
	// meant every tree's own still-pending mirror copy (about to be
	// inserted into a quadrant where no tree existed yet) was invisible
	// to that check -- a mineral tile could look perfectly clear, then
	// get silently wiped anyway once mirrorNaturalResourcesForFairness's
	// own atomic all-4 check ran into one of those just-inserted tree
	// copies. Doing this fold+mirror pass here first, then seeding
	// minerals against its now-fixed output, removes that moving target
	// entirely.
	buildings = mirrorNaturalResourcesForFairness(grid, buildings, points)

	buildings = growFixedDepositRegion(grid, buildings, building.StoneDeposit, duelStoneDepositTiles, defaultStoneSeed, playerPoint, minDepositDistanceFromWarehouse, maxDepositDistanceFromWarehouse, isthmuses, points)
	buildings = growFixedDepositRegion(grid, buildings, building.CoalDeposit, duelCoalDepositTiles, defaultCoalSeed, playerPoint, minDepositDistanceFromWarehouse, maxDepositDistanceFromWarehouse, isthmuses, points)
	buildings = growFixedDepositRegion(grid, buildings, building.GoldOreDeposit, duelGoldOreDepositTiles, defaultGoldOreSeed, playerPoint, minDepositDistanceFromWarehouse, maxDepositDistanceFromWarehouse, isthmuses, points)
	buildings = growFixedDepositRegion(grid, buildings, building.IronOreDeposit, duelIronOreDepositTiles, defaultIronOreSeed, playerPoint, minDepositDistanceFromWarehouse, maxDepositDistanceFromWarehouse, isthmuses, points)
	buildings = pruneNaturalResourcesFromIsthmus(buildings, isthmuses)
	// Idempotent for the already-symmetric trees/thickets/fish (folding a
	// symmetric set back to canonical and re-mirroring it just recreates
	// the same positions); this pass is what actually mirrors the
	// minerals just seeded above.
	buildings = mirrorNaturalResourcesForFairness(grid, buildings, points)

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

	var ais []*faction
	for i, difficulty := range difficulties {
		owner := i + 1
		q := quadrantAssignmentOrder[owner]
		ais = append(ais, newFaction(owner, warehouses[q], difficulty))
	}

	playerWarehouse := warehouses[quadrantAssignmentOrder[0]]
	game := &Game{
		grid:           grid,
		buildings:      buildings,
		stock:          stock,
		pop:            &economy.Population{},
		sim:            economy.NewSimulator(framesPerSimTick),
		logi:           logistics.NewController(playerWarehouse, startingSerfs),
		vills:          villagers.NewController(),
		jacks:          lumberjack.NewController(),
		fishers:        fishing.NewController(),
		quarry:         quarry.NewController(),
		builders:       builder.NewController(),
		miners:         miner.NewController(),
		sentries:       sentry.NewController(),
		soldiers:       soldier.NewController(),
		ais:            ais,
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

// isthmusApproachBuffer widens each isthmus's no-resource zone beyond its
// own strict dry-crossing rectangle -- a real connectivity bug found
// while testing this exact map: the tree/fish/deposit density right at
// the crossing's own immediate edge (still legitimately outside the
// crossing rectangle, so the original exact-rectangle check let it
// stand) could still wall off the single row/column that actually leads
// into it, sealing a quadrant off with a solid tree line one tile short
// of the isthmus itself. A pathfind.FindLandPath connectivity test
// between two quadrant warehouses is what actually caught this --
// nothing sat ON the isthmus, but nothing could reach it either.
const isthmusApproachBuffer = 5

// maxFixedDepositAttempts bounds growFixedDepositRegion's retry loop.
// Isthmus crossings cover a tiny fraction of a duel map's total area, so a
// handful of differently-seeded attempts is overwhelmingly likely to land
// a small (1-3 tile) region entirely clear of all four on the first try or
// two; this just guards against a pathological seed run.
const maxFixedDepositAttempts = 25

// growFixedDepositRegion places exactly target deposit tiles of kind
// (building.StoneDeposit or one of the ore kinds), retrying with a
// different seed until the whole region both survives isthmus pruning AND
// has every mirrorX/mirrorY/mirrorXY counterpart free (see
// regionSurvivesMirroring) -- i.e. a region that will actually still be
// there, whole, after mirrorNaturalResourcesForFairness runs.
//
// Unlike the single-player, percentage-based seedStoneDeposits/
// seedOreDeposits -- where a total spanning dozens of tiles is split
// across 2-5 regions, so losing one region to the isthmus (or to a
// mirror-target collision) barely moves the total -- a duel map's fixed,
// small per-quadrant count (as low as a single tile, see
// duelXxxDepositTiles) can't afford to lose its only region and end up
// with zero of that resource map-wide.
//
// Two real bugs found testing this exact change (see
// TestNewDuelGame_MineralDepositCountsAreFixedNotPercentage):
//
//  1. The isthmus check alone isn't enough. findStoneStart/findOreStart
//     pick the best-scoring tile across the WHOLE map, with no awareness
//     that mirrorNaturalResourcesForFairness will later require all four
//     of a tile's mirrored positions to be simultaneously free (the same
//     all-or-nothing atomicity that already protects a percentage-based
//     region, see that function's own doc comment). Fixed by seeding
//     minerals only after trees/thickets/fish have already been mirrored
//     into their own final, stable layout (see newDuelGame) -- otherwise
//     a tile could look perfectly clear, then still get wiped once a
//     tree's own about-to-be-inserted mirror copy landed on it.
//  2. avoid/minDistance here only ever kept the region away from ONE
//     point (the player's own warehouse) -- but a canonical tile can land
//     in ANY quadrant, and mirrorNaturalResourcesForFairness's own
//     distance check (canOK) tests each mirrored copy against THAT
//     quadrant's own warehouse, not the player's. A tile placed 30+ tiles
//     from the player's base could still be well within
//     minDepositDistanceFromWarehouse of a DIFFERENT quadrant's warehouse
//     entirely, silently failing the real check later. Fixed by
//     replicating that same per-quadrant distance test here too, against
//     all four real warehouse points (see regionSurvivesMirroring).
//
// maxDistance (see regionWithinDistance) is the separate, later addition
// answering the user's explicit "переделай спавн ресурсов чтоб они
// появлялись рядом а не раскиданые на карте": findStoneStart/findOreStart
// pick the single best-scoring tile across the WHOLE map with no
// awareness of avoid's own position beyond "not too close", so a
// perfectly valid region could still land clear across the quadrant from
// the player's own base. maxDistance <= 0 disables the constraint.
func growFixedDepositRegion(grid *world.Grid, buildings []*building.Building, kind building.Kind, target int, seed uint32, avoid gridPoint, minDistance, maxDistance int, isthmuses []image.Rectangle, warehousePoints [quadrantCount]gridPoint) []*building.Building {
	if target <= 0 {
		return buildings
	}
	grow := func(attemptSeed uint32) []*building.Building {
		if kind == building.StoneDeposit {
			return growStoneRegion(grid, buildings, target, attemptSeed, avoid, minDistance)
		}
		return growOreRegion(grid, buildings, kind, target, attemptSeed, avoid, minDistance)
	}
	var lastAttempt []*building.Building
	for attempt := uint32(0); attempt < maxFixedDepositAttempts; attempt++ {
		grown := grow(seed ^ attempt*0x9e3779b9)
		lastAttempt = grown
		added := grown[len(buildings):]
		if len(added) < target {
			continue // this attempt's region came up short -- try another seed
		}
		if !regionWithinDistance(added, avoid, maxDistance) {
			continue // too far from the player's own warehouse -- try another seed
		}
		if !regionClearOfIsthmuses(added, isthmuses) {
			continue
		}
		if !regionSurvivesMirroring(grid, buildings, kind, added, minDistance, warehousePoints) {
			continue
		}
		return grown
	}
	// Every attempt either came up short, touched an isthmus, or would
	// collide with something once mirrored -- fall back to the last
	// attempt so the map isn't silently left with zero of this resource;
	// pruneNaturalResourcesFromIsthmus and mirrorNaturalResourcesForFairness's
	// own atomic all-or-nothing check still run afterward as a safety net
	// for whatever this leaves behind (possibly nothing, in the worst
	// case -- exceedingly unlikely given maxFixedDepositAttempts tries).
	return lastAttempt
}

// regionClearOfIsthmuses reports whether every tile in added sits outside
// every isthmus (widened by isthmusApproachBuffer, same as
// pruneNaturalResourcesFromIsthmus).
func regionClearOfIsthmuses(added []*building.Building, isthmuses []image.Rectangle) bool {
	for _, b := range added {
		p := image.Point{X: b.X, Y: b.Y}
		for _, isthmus := range isthmuses {
			if p.In(isthmus.Inset(-isthmusApproachBuffer)) {
				return false
			}
		}
	}
	return true
}

// regionWithinDistance reports whether every tile in added sits within
// maxDistance of avoid (maxDistance <= 0 disables the check) -- see
// growFixedDepositRegion's own doc comment on maxDistance for why this
// exists (findStoneStart/findOreStart's best-scoring pick otherwise has
// no notion of "nearby", only "not too close").
func regionWithinDistance(added []*building.Building, avoid gridPoint, maxDistance int) bool {
	if maxDistance <= 0 {
		return true
	}
	maxDistSq := float64(maxDistance * maxDistance)
	for _, b := range added {
		dx, dy := float64(b.X-avoid.x), float64(b.Y-avoid.y)
		if dx*dx+dy*dy > maxDistSq {
			return false
		}
	}
	return true
}

// regionSurvivesMirroring reports whether every tile in added will
// actually still be placed, in every quadrant, once
// mirrorNaturalResourcesForFairness runs -- replicating that function's
// own canonical-fold + per-quadrant candidate + canOK logic exactly (see
// its doc comment), rather than the narrower single-point distance check
// growStoneRegion/growOreRegion apply during the initial search. Checked
// against buildings (the map's state before this region existed), same
// as that function's own kept starts without any natural resource in it.
func regionSurvivesMirroring(grid *world.Grid, buildings []*building.Building, kind building.Kind, added []*building.Building, minDistance int, warehousePoints [quadrantCount]gridPoint) bool {
	centerX := duelMapWidth / 2
	centerY := duelMapHeight / 2
	for _, b := range added {
		foldedX, foldedY := b.X, b.Y
		if foldedX >= centerX {
			foldedX = mirrorX(duelMapWidth, foldedX)
		}
		if foldedY >= centerY {
			foldedY = mirrorY(duelMapHeight, foldedY)
		}
		mx, my := mirrorX(duelMapWidth, foldedX), mirrorY(duelMapHeight, foldedY)

		type candidate struct {
			x, y  int
			avoid gridPoint
		}
		seen := map[[2]int]bool{}
		var candidates []candidate
		for _, c := range []candidate{
			{foldedX, foldedY, warehousePoints[quadrantNW]},
			{mx, foldedY, warehousePoints[quadrantNE]},
			{foldedX, my, warehousePoints[quadrantSW]},
			{mx, my, warehousePoints[quadrantSE]},
		} {
			pos := [2]int{c.x, c.y}
			if seen[pos] {
				continue
			}
			seen[pos] = true
			candidates = append(candidates, c)
		}

		for _, c := range candidates {
			if tooCloseToPoint(c.x, c.y, c.avoid, minDistance) {
				return false
			}
			if c.x == b.X && c.y == b.Y {
				continue // this tile's own already-placed self
			}
			if !building.CanPlace(grid, buildings, kind, c.x, c.y) {
				return false
			}
		}
	}
	return true
}

// pruneNaturalResourcesFromIsthmus removes any natural resource node
// (isNaturalResourceKind -- Tree/Fish/StoneDeposit/CoalDeposit/
// GoldOreDeposit/IronOreDeposit) that landed inside any of isthmuses
// (widened by isthmusApproachBuffer -- see its own doc comment), the
// duel map's dry land crossings between quadrants (see
// growQuadrantWaterCross's doc comment). Called right after the last
// seed*/before mirrorNaturalResourcesForFairness: a resource pruned here
// never gets a mirrored counterpart created for it either, so this
// can't introduce any quadrant imbalance of its own.
func pruneNaturalResourcesFromIsthmus(buildings []*building.Building, isthmuses []image.Rectangle) []*building.Building {
	kept := buildings[:0]
	for _, b := range buildings {
		if !isNaturalResourceKind(b.Kind) {
			kept = append(kept, b)
			continue
		}
		p := image.Point{X: b.X, Y: b.Y}
		blocked := false
		for _, isthmus := range isthmuses {
			if p.In(isthmus.Inset(-isthmusApproachBuffer)) {
				blocked = true
				break
			}
		}
		if !blocked {
			kept = append(kept, b)
		}
	}
	return kept
}

// mirrorNaturalResourcesForFairness replaces every natural resource node
// (Tree/Fish/StoneDeposit/CoalDeposit/GoldOreDeposit/IronOreDeposit) with
// a four-quadrant-symmetric layout: only the NW quadrant's (x<centerX,
// y<centerY) share of what the ordinary single-player seed functions
// generated is kept as canonical, and each surviving node gets an exact
// mirrorX/mirrorY/mirrorXY counterpart placed in the other three
// quadrants -- generalizes the original two-territory (mirrorX-only)
// version to two axes, per the user's explicit "карту разделенную на 4
// участка с зеркальным распределением ресурсов", placed regardless of
// how many of the 4 quadrants an actual faction occupies this match (an
// empty quadrant still gets its share of resources -- neutral,
// contestable territory).
//
// A real fairness bug found from an actual playtest report, in the
// original two-territory version, still guarded against here: naively
// discarding "the other half" instead of folding to canonical can zero
// out a resource type entirely if generation happened to place all of it
// on the discarded side (confirmed: 76/76 stone in one recorded run).
// Folding first (instead of discarding) keeps whatever generation
// actually produced, wherever it landed, and only repositions it.
func mirrorNaturalResourcesForFairness(grid *world.Grid, buildings []*building.Building, warehousePoints [quadrantCount]gridPoint) []*building.Building {
	centerX := duelMapWidth / 2
	centerY := duelMapHeight / 2
	isDeposit := func(kind building.Kind) bool {
		switch kind {
		case building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
			return true
		default:
			return false
		}
	}

	kept := make([]*building.Building, 0, len(buildings))
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
		foldedX, foldedY := b.X, b.Y
		if foldedX >= centerX {
			foldedX = mirrorX(duelMapWidth, foldedX)
		}
		if foldedY >= centerY {
			foldedY = mirrorY(duelMapHeight, foldedY)
		}
		key := canonicalKey{b.Kind, foldedX, foldedY}
		if _, seen := canonical[key]; seen {
			continue
		}
		canonical[key] = b
		order = append(order, key)
	}

	// canOK/place split, and every quadrant's copy in a group is checked
	// with canOK against a single, shared, not-yet-mutated kept snapshot
	// before ANY of them is committed -- a real bug found in the
	// original two-quadrant version: independent, unconditional place()
	// calls let one quadrant's copy commit a tile that then blocks
	// another quadrant's copy in the SAME group (or a later group) from
	// ever placing, silently drifting the per-quadrant counts apart
	// (confirmed: 69 vs 88 Fish in one run) even though the fold itself
	// is exactly symmetric. Checking the whole group atomically keeps
	// every quadrant's share equal: either all of them land, or none do.
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
		mx, my := mirrorX(duelMapWidth, key.x), mirrorY(duelMapHeight, key.y)

		// Up to 4 candidate positions, one per quadrant -- deduplicated
		// (a resource sitting exactly on one or both mirror axes yields
		// fewer than 4 distinct tiles) and using that quadrant's own
		// warehouse point for the deposit min-distance check.
		type candidate struct {
			x, y  int
			avoid gridPoint
		}
		seen := map[[2]int]bool{}
		var candidates []candidate
		for _, c := range []candidate{
			{key.x, key.y, warehousePoints[quadrantNW]},
			{mx, key.y, warehousePoints[quadrantNE]},
			{key.x, my, warehousePoints[quadrantSW]},
			{mx, my, warehousePoints[quadrantSE]},
		} {
			pos := [2]int{c.x, c.y}
			if seen[pos] {
				continue
			}
			seen[pos] = true
			candidates = append(candidates, c)
		}

		allOK := true
		for _, c := range candidates {
			if !canOK(src.Kind, c.x, c.y, c.avoid) {
				allOK = false
				break
			}
		}
		if !allOK {
			continue
		}
		for _, c := range candidates {
			place(src.Kind, c.x, c.y, src.Reserve, src.GrowthTicks, src.GrowthTargetTicks)
		}
	}
	return kept
}

// factionDefeated reports whether owner has been fully wiped out --
// every building except Road/StoneWall/Gate destroyed, and every unit
// dead -- per the user's explicit win condition ("все здания и юниты
// противника уничтожены (дорога и стены не в счет)"). Meaningful for the
// player (owner 0) or any AI faction's owner.
// duelResult is the outcome of an "N против ИИ" match -- see Game's own
// duelResult field doc comment.
type duelResult int

const (
	duelResultNone duelResult = iota
	duelResultVictory
	duelResultDefeat
)

// checkDuelResult sets g.duelResult once the match is actually decided --
// per the user's explicit "все против всех": the player loses the
// instant their OWN faction is defeated (it doesn't matter which bot
// eventually wins the rest), and wins only once every single AI faction
// is defeated too, however many there are. A no-op outside "N против ИИ"
// (g.ais empty) or once a result is already set (a finished match's
// simulation is frozen -- see Update's early return on g.duelResult --
// so this would never re-fire anyway, but a defensive check costs
// nothing and documents the intent).
func (g *Game) checkDuelResult() {
	if len(g.ais) == 0 || g.duelResult != duelResultNone {
		return
	}
	if g.factionDefeated(0) {
		g.duelResult = duelResultDefeat
		return
	}
	for _, f := range g.ais {
		if !g.factionDefeated(f.owner) {
			return // at least one bot still stands -- the match continues
		}
	}
	g.duelResult = duelResultVictory
}

// factionDefeated's isNaturalResourceKind exclusion matters far more here
// than it first looks: every Tree/Fish/StoneDeposit/CoalDeposit/
// GoldOreDeposit/IronOreDeposit on the whole map is built via a plain
// &building.Building{Kind: ..., X: ..., Y: ...} literal with no Owner
// field set at all -- meaning its Owner is the zero value, 0, the same
// as the player's. Without this exclusion, factionDefeated(0) (the
// player) would hit the very first tree anywhere on the map and return
// false immediately, no matter how thoroughly the player's own buildings
// were actually destroyed: a real bug found while adding a test for the
// player's own defeat specifically (every existing test up to that point
// only ever exercised the AI's defeat, factionDefeated(1), which was
// never at risk of this -- nothing seeds a natural resource with Owner:
// 1). Left the player's own defeat condition silently unreachable until
// caught.
func (g *Game) factionDefeated(owner int) bool {
	for _, b := range g.buildings {
		if b.Owner != owner || isNaturalResourceKind(b.Kind) {
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
	f := g.factionByOwner(owner)
	if f == nil {
		return 0
	}
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
