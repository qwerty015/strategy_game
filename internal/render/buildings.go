package render

import (
	"image/color"
	"math"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/building"
	"strategy_game/internal/world"
)

// buildingHeight is how tall (in tiles) a standing building sprite is
// drawn -- deliberately more than 1, so it rises above its single-tile
// footprint the way the source art (see AGENTS.md) is meant to be used,
// rather than being squashed to fit exactly on its tile.
const buildingHeight = 1.7

// animFrame is a shared optional animation clock. The current generated art
// uses consistent single poses, but keeping the clock avoids changing the
// render API when directional or mill-blade frames are added later.
var animFrame int

// Tick advances the shared animation clock by one render frame. Call
// once per Draw.
func Tick() {
	animFrame++
}

var (
	soilColor      = color.RGBA{R: 92, G: 66, B: 38, A: 255}    // freshly tilled earth (tints assets.Fertile)
	ripeWheatColor = color.RGBA{R: 231, G: 196, B: 84, A: 255}  // golden, ready to harvest
	vineyardSoil   = color.RGBA{R: 80, G: 61, B: 38, A: 255}    // darker soil for grape rows
	unstaffedTint  = color.RGBA{R: 214, G: 63, B: 55, A: 90}    // translucent red over a workerless building
	stoneFullColor = color.RGBA{R: 150, G: 150, B: 150, A: 255} // freshly placed, full Reserve
	stoneWornColor = color.RGBA{R: 196, G: 189, B: 150, A: 255} // nearly spent, sun-bleached

	coalWornColor    = color.RGBA{R: 150, G: 150, B: 150, A: 255} // nearly spent, ashen
	goldOreWornColor = color.RGBA{R: 210, G: 200, B: 150, A: 255} // nearly spent, pale
	ironOreWornColor = color.RGBA{R: 200, G: 180, B: 160, A: 255} // nearly spent, pale rust

	constructionGroundColor = color.RGBA{R: 109, G: 79, B: 45, A: 180}  // exposed earth beneath a site
	constructionWaitColor   = color.RGBA{R: 214, G: 63, B: 55, A: 100}  // stalled, waiting on delivery
	constructionBarColor    = color.RGBA{R: 255, G: 205, B: 61, A: 235} // construction progress
)

// lerpColor blends from a to b as t goes from 0 to 1, clamped.
func lerpColor(a, b color.RGBA, t float32) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	lerp := func(a, b uint8) uint8 {
		return uint8(float32(a) + (float32(b)-float32(a))*t)
	}
	return color.RGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: 255}
}

// DrawBuildings renders every placed building.
//
// A Farm is its house sprite standing on one corner of its footprint,
// with tilled field on the rest of it -- the field's color sweeps from
// bare earth to golden wheat as the crop matures (see AGENTS.md), so the
// field itself shows the growth the player asked to be able to see. A
// Roads are drawn in a dedicated first pass, so a road can never cover the
// lower part of a building just because it appears later in the save/build
// slice. Mill/Bakery/Warehouse/Tavern each stand on their single tile taller
// than the tile itself (see buildingHeight), with a production-progress bar
// underneath. MillFrames contains three compact sail positions, switched
// periodically to animate the windmill.
func DrawBuildings(screen *ebiten.Image, grid *world.Grid, buildings []*building.Building, cam *Camera, unstaffed, disconnected map[*building.Building]bool) {
	tilePixels := cam.TilePixels()
	// Two tiles cover tall roofs, construction effects and the one-tile
	// prefetch ring while the camera pans. Objects outside this rectangle are
	// still simulated; they simply submit no draw calls this frame.
	visible := cam.VisibleTileBounds(2)
	roads := finishedRoadPositions(buildings)
	entranceRoads := buildingEntranceRoadPositions(buildings, roads)
	occupiedTiles := nonRoadBuildingPositions(buildings)
	entranceCorners := buildingEntranceCornerMasks(entranceRoads, roads, occupiedTiles)
	// Ground is drawn before this function. Roads and stone deposits are
	// both flat, terrain-scale ground decoration rather than standing
	// objects, so both render in this same bottom pass -- otherwise a
	// deposit drawn in the main loop below could land after (and so on top
	// of) a tall standing building it happens to be adjacent to, purely
	// because of where it sits in the save/build slice.
	for _, b := range buildings {
		if !visible.Intersects(b.X, b.Y, 1) {
			continue
		}
		sx, sy := cam.TileToScreen(b.X, b.Y)
		switch {
		case b.Kind == building.Road && b.ConstructionStage != building.ConstructionNone:
			// Not a real road yet -- see pathfind.roadSet. Drawing the
			// cobblestone texture here would visually claim otherwise.
			drawConstructionSite(screen, b, 1, sx, sy, tilePixels)
		case b.Kind == building.Road:
			drawOrganicRoad(screen, entranceCorners, b.X, b.Y, sx, sy, tilePixels)
		case b.Kind == building.StoneDeposit:
			drawStoneDeposit(screen, sx, sy, tilePixels, b.Reserve)
		case b.Kind == building.CoalDeposit || b.Kind == building.GoldOreDeposit || b.Kind == building.IronOreDeposit:
			drawOreDeposit(screen, sx, sy, tilePixels, b.Kind, b.Reserve)
		}
	}
	drawWalls(screen, buildings, cam)

	// Wildlife belongs over flat landscape detail: a fox or hare should not
	// vanish beneath cobblestones or a low boulder deposit. Keep it before the
	// tall-building pass below, though, so houses, fields and trees still hide
	// an animal that happens to cross their footprint.
	DrawAmbientGroundLife(screen, grid, cam)

	// Tall sprites are depth-sorted by the bottom of their footprint instead
	// of construction/save order. This prevents a building behind another one
	// from being painted over its roof or field when the map grows large.
	standingBuildings := make([]*building.Building, 0, len(buildings))
	for _, b := range buildings {
		if b == nil || b.Kind == building.Road || b.Kind == building.StoneDeposit ||
			b.Kind == building.CoalDeposit || b.Kind == building.GoldOreDeposit || b.Kind == building.IronOreDeposit ||
			((b.Kind == building.StoneWall || b.Kind == building.Gate) && b.ConstructionStage == building.ConstructionNone) {
			continue
		}
		if visible.Intersects(b.X, b.Y, building.Types[b.Kind].Footprint) {
			standingBuildings = append(standingBuildings, b)
		}
	}
	sort.SliceStable(standingBuildings, func(i, j int) bool {
		a, b := standingBuildings[i], standingBuildings[j]
		aBottom := a.Y + building.Types[a.Kind].Footprint
		bBottom := b.Y + building.Types[b.Kind].Footprint
		if aBottom != bBottom {
			return aBottom < bBottom
		}
		return a.X < b.X
	})
	for _, b := range standingBuildings {
		bt := building.Types[b.Kind]
		sx, sy := cam.TileToScreen(b.X, b.Y)

		if b.ConstructionStage != building.ConstructionNone {
			drawConstructionSite(screen, b, bt.Footprint, sx, sy, tilePixels)
			continue
		}

		// A natural world object (Tree/Fish) grounds itself through its
		// own multi-stage sprite, not a building's contact shadow.
		if b.Kind != building.Tree && b.Kind != building.Fish {
			drawBuildingShadow(screen, sx, sy, tilePixels, visualBuildingHeight(b.Kind))
		}

		switch b.Kind {
		case building.Tree:
			drawTree(screen, sx, sy, tilePixels, b.GrowthStage())
			continue
		case building.Fish:
			drawFish(screen, sx, sy, tilePixels, b.GrowthStage())
			continue

		case building.Farm:
			growth := float32(1)
			if bt.Recipe.TicksToProduce > 0 {
				growth = float32(b.ProgressTicks) / float32(bt.Recipe.TicksToProduce)
			}
			tint := lerpColor(soilColor, ripeWheatColor, growth)
			for dy := range bt.Footprint {
				for dx := range bt.Footprint {
					if dx == 0 && dy == 0 {
						continue // this corner is the farmhouse, drawn below
					}
					fieldX := sx + float64(dx)*tilePixels
					fieldY := sy + float64(dy)*tilePixels
					drawStandingTintedAtScale(screen, assets.Fertile, fieldX, fieldY, 1, tilePixels, tint)
					drawCropGrowth(screen, fieldX, fieldY, growth, dx, dy, tilePixels)
				}
			}
			drawBuildingBody(screen, b, assets.FarmHouse, sx, sy, tilePixels)

		case building.Winery:
			growth := float32(0)
			if bt.Recipe.TicksToProduce > 0 {
				growth = float32(b.ProgressTicks) / float32(bt.Recipe.TicksToProduce)
			}
			for dy := range bt.Footprint {
				for dx := range bt.Footprint {
					if dx == 0 && dy == 0 {
						continue // the winery sprite occupies this corner
					}
					fieldX := sx + float64(dx)*tilePixels
					fieldY := sy + float64(dy)*tilePixels
					drawStandingTintedAtScale(screen, assets.Fertile, fieldX, fieldY, 1, tilePixels, vineyardSoil)
					drawVineyardGrowth(screen, fieldX, fieldY, growth, dx, dy, tilePixels)
				}
			}
			drawBuildingBody(screen, b, assets.Winery, sx, sy, tilePixels)

		case building.PigFarm:
			drawBuildingBody(screen, b, assets.PigFarm, sx, sy, tilePixels)

		case building.MeatWorkshop:
			drawBuildingBody(screen, b, assets.MeatWorkshop, sx, sy, tilePixels)

		case building.CarpentryWorkshop:
			drawBuildingBody(screen, b, assets.CarpentryWorkshop, sx, sy, tilePixels)

		case building.Mill:
			drawStandingAtScale(screen, assets.MillFrames[(animFrame/12)%len(assets.MillFrames)], sx, sy, visualBuildingHeight(b.Kind), tilePixels)

		case building.Bakery:
			drawBuildingBody(screen, b, assets.Bakery, sx, sy, tilePixels)
			drawFire(screen, sx+17*tilePixels/TileSize, sy+4*tilePixels/TileSize, tilePixels)

		case building.Warehouse:
			drawBuildingBody(screen, b, assets.Warehouse, sx, sy, tilePixels)

		case building.Tavern:
			drawBuildingBody(screen, b, assets.Tavern, sx, sy, tilePixels)

		case building.LumberjackHut:
			drawBuildingBody(screen, b, assets.LumberjackHut, sx, sy, tilePixels)

		case building.QuarryHut:
			drawBuildingBody(screen, b, assets.QuarryHut, sx, sy, tilePixels)

		case building.MinerHut:
			drawBuildingBody(screen, b, assets.MinerHut, sx, sy, tilePixels)

		case building.Smeltery:
			drawBuildingBody(screen, b, assets.Smeltery, sx, sy, tilePixels)
			drawFire(screen, sx+17*tilePixels/TileSize, sy+4*tilePixels/TileSize, tilePixels)

		case building.FisherHut:
			frame := 0 // source sprite's pier points south
			if water, ok := building.WaterAccessPoint(grid, b); ok {
				switch {
				case water.X < b.X:
					frame = 1 // rotate south-facing pier clockwise to west
				case water.Y < b.Y:
					frame = 2
				case water.X > b.X:
					frame = 3
				}
			}
			drawStandingAtScale(screen, assets.FisherHutFrames[frame], sx, sy, visualBuildingHeight(b.Kind), tilePixels)
		}

		drawProductionWorkEffect(screen, b.Kind, b.ProgressTicks, sx, sy, tilePixels)
		if bt.Recipe.TicksToProduce > 0 {
			progress := float32(b.ProgressTicks) / float32(bt.Recipe.TicksToProduce)
			if progress > 1 {
				progress = 1
			}
			barSpan := math.Max(float64(bt.Footprint), visualBuildingHeight(b.Kind))
			barWidth := float32(barSpan * tilePixels)
			barX := float32(sx + (tilePixels-barSpan*tilePixels)/2)
			barY := float32(sy) + float32(bt.Footprint)*float32(tilePixels) - float32(3*tilePixels/TileSize)
			barHeight := float32(3 * tilePixels / TileSize)
			vector.FillRect(screen, barX, barY, barWidth*progress, barHeight, color.RGBA{R: 255, G: 255, B: 0, A: 220}, false)
		}

		// A production building with no resident worker at all (as opposed
		// to one merely away eating) is tinted red across its whole
		// footprint, so an empty workplace reads at a glance instead of
		// only being discoverable by opening the inspector.
		//
		// A *staffed* building that isn't producing -- no road connection,
		// short a raw material, or finished with nowhere to put the
		// result -- used to get the same red tint, and the user reported
		// exactly the problem that invites: an idle worker and an empty
		// building looked identical at a glance. It's now a small "Zzz"
		// sleep bubble over the roof instead of a footprint-wide wash,
		// deliberately less alarming than the red tint and legible next
		// to it without being confused for it -- "someone's here but
		// dozing" reads as a different, lesser problem than "no one's
		// here at all", which is exactly true. All three stalled reasons
		// share one bubble rather than three different indicators: the
		// inspector already explains *why* once clicked, the map-level
		// glance only needs to flag *that* something's stalled.
		size := float32(bt.Footprint) * float32(tilePixels)
		switch {
		case unstaffed[b]:
			vector.FillRect(screen, float32(sx), float32(sy), size, size, unstaffedTint, false)
		case disconnected[b], buildingStallReason(b) != stallNone:
			drawIdleBubble(screen, b.Kind, sx, sy, tilePixels)
		}
	}
}

// drawWalls paints completed wall pieces after the road/deposit ground pass and
// before wildlife and tall buildings. The dedicated horizontal/vertical art is
// selected from real neighbouring wall cells; gates retain their installation
// axis in Building.GateAxis even after a neighbouring segment is removed.
func drawWalls(screen *ebiten.Image, buildings []*building.Building, cam *Camera) {
	visible := cam.VisibleTileBounds(1)
	tilePixels := cam.TilePixels()
	for _, b := range buildings {
		if b == nil || b.ConstructionStage != building.ConstructionNone || !building.IsWallKind(b.Kind) || !visible.Intersects(b.X, b.Y, 1) {
			continue
		}
		axis := b.GateAxis
		if b.Kind == building.StoneWall {
			axis = building.WallRenderAxis(buildings, b.X, b.Y)
		}
		var art *ebiten.Image
		if b.Kind == building.StoneWall {
			art = assets.StoneWallHorizontal
			if axis == building.WallVertical {
				art = assets.StoneWallVertical
			}
		} else if b.GateOpen {
			art = assets.GateHorizontalOpen
			if axis == building.WallVertical {
				art = assets.GateVerticalOpen
			}
		} else {
			art = assets.GateHorizontalClosed
			if axis == building.WallVertical {
				art = assets.GateVerticalClosed
			}
		}
		sx, sy := cam.TileToScreen(b.X, b.Y)
		drawFootprintAtScale(screen, art, sx, sy, 1, tilePixels)
	}
}

// drawBuildingBody resolves a building's body from the art manifest. Existing
// PNG exports remain fallbacks, while a future buildings/<id>/body.png is used
// immediately without introducing a rendering switch case or changing saves.
func drawBuildingBody(screen *ebiten.Image, b *building.Building, fallback *ebiten.Image, sx, sy, tilePixels float64) {
	if visual, ok := assets.BuildingVisualFor(b.Kind); ok && visual.Body.Image != nil {
		drawVisualLayerAtScale(screen, visual.Body, sx, sy, float64(building.Types[b.Kind].Footprint), tilePixels)
		return
	}
	drawStandingAtScale(screen, fallback, sx, sy, buildingHeight, tilePixels)
}

// DrawBuildingForegrounds is the final world-object pass. A future art pack
// can give a building a front porch, low fence or roof eave without changing
// the simulation: that transparent layer is drawn here after every worker.
// Current base sprites deliberately have no Front layer, so adding this pass
// changes no established image until a dedicated foreground sheet is supplied.
func DrawBuildingForegrounds(screen *ebiten.Image, buildings []*building.Building, cam *Camera) {
	visible := cam.VisibleTileBounds(2)
	standing := make([]*building.Building, 0, len(buildings))
	for _, b := range buildings {
		if b == nil || b.ConstructionStage != building.ConstructionNone || b.Kind == building.Road {
			continue
		}
		visual, ok := assets.BuildingVisualFor(b.Kind)
		if !ok || visual.Front.Image == nil || !visible.Intersects(b.X, b.Y, building.Types[b.Kind].Footprint) {
			continue
		}
		standing = append(standing, b)
	}
	sort.SliceStable(standing, func(i, j int) bool {
		a, b := standing[i], standing[j]
		aBottom := a.Y + building.Types[a.Kind].Footprint
		bBottom := b.Y + building.Types[b.Kind].Footprint
		if aBottom != bBottom {
			return aBottom < bBottom
		}
		return a.X < b.X
	})
	for _, b := range standing {
		visual, _ := assets.BuildingVisualFor(b.Kind)
		sx, sy := cam.TileToScreen(b.X, b.Y)
		drawVisualLayerAtScale(screen, visual.Front, sx, sy, float64(building.Types[b.Kind].Footprint), cam.TilePixels())
	}
}

// drawIdleBubble draws a small "Zzz" sleep bubble above a staffed building
// that isn't currently producing (see the stall/disconnected check above)
// -- deliberately smaller and calmer than unstaffedTint's footprint-wide
// red wash, so "someone's here but stuck" doesn't read as the same
// problem as "nobody's here at all" (the user's own report: the two
// looked identical). Anchored above the building's own standing sprite
// at buildingHeight over its first tile, not centred on a multi-tile
// footprint -- for Farm/Winery that's where the actual roof is, not the
// empty field tiles beside it.
func drawIdleBubble(screen *ebiten.Image, kind building.Kind, sx, sy, tilePixels float64) {
	cx := float32(sx + tilePixels*0.5)
	// drawStandingScaled anchors a tilesTall sprite to the *bottom* of its
	// tile (translate.y = sy+tilePixels-drawnH), not the top -- this must
	// mirror that exactly or the marker ends up a whole tile too high,
	// floating well above the actual roof instead of sitting on it.
	top := float32(sy + tilePixels - tilePixels*visualBuildingHeight(kind))
	bob := float32(0)
	if (animFrame/20)%2 == 1 {
		bob = -float32(tilePixels) * 0.05
	}
	cy := top - float32(tilePixels)*0.06 + bob
	tp := float32(tilePixels)

	// A soft, low-alpha thought-cloud -- bigger and fainter than the
	// first version (a plain small ball) the user asked to redo it away
	// from: "не шарик, а облачко, и крупнее, слабо заметное". assets.
	// IdleCloud is one flat-alpha shape (see its own doc comment for
	// why), so this single draw fades it uniformly instead of
	// overlapping circles compounding into a more-opaque patch in the
	// middle -- low alpha keeps it a background hint rather than
	// competing with unstaffedTint's much bolder red for attention.
	cb := assets.IdleCloud.Bounds()
	cloudSize := tp * 0.85
	s := float64(cloudSize) / float64(cb.Dx())
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(float64(cx)-float64(cloudSize)/2, float64(cy)-float64(cloudSize)/2)
	op.ColorScale.ScaleAlpha(0.4)
	op.Blend = ebiten.BlendSourceOver
	screen.DrawImage(assets.IdleCloud, op)

	// "Zzz" over the cloud, the classic sleep motif -- drawn as plain
	// line segments rather than a font glyph, so render doesn't need a
	// text face of its own (importing package ui for one would cycle
	// back to render, which ui already imports for render.Camera).
	mark := color.RGBA{R: 70, G: 70, B: 80, A: 235}
	for i, scale := range [...]float32{0.14, 0.19, 0.24} {
		zcx := cx - tp*0.12 + float32(i)*tp*0.13
		zcy := cy - tp*0.02 - float32(i)*tp*0.12
		zw, zh := tp*scale, tp*scale*0.6
		width := tp * 0.04
		vector.StrokeLine(screen, zcx-zw/2, zcy-zh/2, zcx+zw/2, zcy-zh/2, width, mark, true)
		vector.StrokeLine(screen, zcx+zw/2, zcy-zh/2, zcx-zw/2, zcy+zh/2, width, mark, true)
		vector.StrokeLine(screen, zcx-zw/2, zcy+zh/2, zcx+zw/2, zcy+zh/2, width, mark, true)
	}
}

// drawBuildingShadow blits assets.BuildingShadow flush against the bottom
// of the tile at (sx,sy) -- the same bottom edge drawStandingScaled
// anchors a standing sprite's own feet to, so the shadow reads as
// underneath the building rather than floating at an unrelated offset.
func drawBuildingShadow(screen *ebiten.Image, sx, sy, tilePixels, visualHeight float64) {
	img := assets.BuildingShadow
	b := img.Bounds()
	s := visualHeight * tilePixels / float64(assets.TileSize)
	w := float64(b.Dx()) * s
	h := float64(b.Dy()) * s
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(sx+tilePixels/2-w/2, sy+tilePixels-h)
	op.Blend = ebiten.BlendSourceOver
	screen.DrawImage(img, op)
}

// drawTree uses the dedicated three-stage transparent sprite sheet. Keeping
// the growth stage in the building object means the visual survives save/load
// together with the tree's growth timer.
func drawTree(screen *ebiten.Image, sx, sy, tilePixels float64, stage int) {
	if stage < 0 {
		stage = 0
	}
	if stage > 2 {
		stage = 2
	}
	drawStandingAtScale(screen, assets.TreeFrames[stage], sx, sy, 1.75, tilePixels)
}

// roadTile is a lightweight map key for exact tile positions -- cheaper
// than a *building.Building lookup, used only to answer "is there a
// finished Road at this exact cell" in O(1) from drawRoadConnections.
type roadTile struct{ x, y int }

// finishedRoadPositions indexes every completed Road tile in buildings, so
// drawRoadConnections can test each of a road's four neighbours in O(1)
// instead of a linear scan per tile. A road still under construction does
// not count -- see the ConstructionStage check in the caller's switch,
// it isn't drawn as a real road yet either.
// buildingEntranceRoadPositions finds exactly one finished road at each
// building's marked access point. The preference order mirrors FoundationRoad
// so the rounded visual threshold always agrees with the starter road created
// during placement.
func buildingEntranceRoadPositions(buildings []*building.Building, roads map[roadTile]bool) map[roadTile]bool {
	entrances := make(map[roadTile]bool)
	for _, b := range buildings {
		if b == nil || b.Kind == building.Road || b.Kind == building.Tree || b.Kind == building.Fish ||
			b.Kind == building.StoneDeposit || b.Kind == building.CoalDeposit || b.Kind == building.GoldOreDeposit || b.Kind == building.IronOreDeposit {
			continue
		}
		access := b.AccessPoint()
		for _, d := range [...]roadTile{{x: 0, y: 1}, {x: 1, y: 0}, {x: 0, y: -1}, {x: -1, y: 0}} {
			tile := roadTile{x: access.X + d.x, y: access.Y + d.y}
			if roads[tile] {
				entrances[tile] = true
				break
			}
		}
	}
	return entrances
}

const (
	roadCornerNorthWest uint8 = 1 << iota
	roadCornerNorthEast
	roadCornerSouthWest
	roadCornerSouthEast
)

// nonRoadBuildingPositions indexes every occupied building tile. Deposits and
// trees count too: a rounded road corner must never visually cut into any
// solid map object.
func nonRoadBuildingPositions(buildings []*building.Building) map[roadTile]bool {
	occupied := make(map[roadTile]bool)
	for _, b := range buildings {
		if b == nil || b.Kind == building.Road {
			continue
		}
		size := building.Types[b.Kind].Footprint
		for dy := 0; dy < size; dy++ {
			for dx := 0; dx < size; dx++ {
				occupied[roadTile{x: b.X + dx, y: b.Y + dy}] = true
			}
		}
	}
	return occupied
}

// buildingEntranceCornerMasks exposes an entrance corner only when all three
// tiles around it (two cardinal and one diagonal) are open grass. Roads and
// buildings keep that corner square, producing a continuous stone edge at
// doorways, junctions and diagonal route contacts.
func buildingEntranceCornerMasks(entrances, roads, occupied map[roadTile]bool) map[roadTile]uint8 {
	masks := make(map[roadTile]uint8, len(entrances))
	for tile := range entrances {
		blocked := func(dx, dy int) bool {
			neighbor := roadTile{x: tile.x + dx, y: tile.y + dy}
			return roads[neighbor] || occupied[neighbor]
		}
		var mask uint8
		if !blocked(0, -1) && !blocked(-1, 0) && !blocked(-1, -1) {
			mask |= roadCornerNorthWest
		}
		if !blocked(0, -1) && !blocked(1, 0) && !blocked(1, -1) {
			mask |= roadCornerNorthEast
		}
		if !blocked(0, 1) && !blocked(-1, 0) && !blocked(-1, 1) {
			mask |= roadCornerSouthWest
		}
		if !blocked(0, 1) && !blocked(1, 0) && !blocked(1, 1) {
			mask |= roadCornerSouthEast
		}
		masks[tile] = mask
	}
	return masks
}
func finishedRoadPositions(buildings []*building.Building) map[roadTile]bool {
	roads := make(map[roadTile]bool)
	for _, b := range buildings {
		if b.Kind == building.Road && b.ConstructionStage == building.ConstructionNone {
			roads[roadTile{b.X, b.Y}] = true
		}
	}
	return roads
}

// drawRoadConnections layers a brightening correction over whichever edges
// of the Road tile at (x,y) face another finished Road, cancelling
// terrain_road_stone.png's own dark vignette there (see
// assets.RoadFadeN's doc comment) -- otherwise every road tile reads as a
// separately framed stone slab, and a straight street looks like a row of
// disconnected paving stones instead of one continuous path.
func drawRoadConnections(screen *ebiten.Image, roads map[roadTile]bool, x, y int, sx, sy, tilePixels float64) {
	if roads[roadTile{x, y - 1}] {
		drawRoadFade(screen, assets.RoadFadeN, sx, sy, tilePixels, false)
	}
	if roads[roadTile{x, y + 1}] {
		drawRoadFade(screen, assets.RoadFadeS, sx, sy, tilePixels, false)
	}
	if roads[roadTile{x - 1, y}] {
		drawRoadFade(screen, assets.RoadFadeW, sx, sy, tilePixels, true)
	}
	if roads[roadTile{x + 1, y}] {
		drawRoadFade(screen, assets.RoadFadeE, sx, sy, tilePixels, true)
	}
}

// drawRoadFade blits one of assets.RoadFadeN/E/S/W flush against its own
// edge of the tile at (sx,sy), scaled uniformly with the rest of the tile
// (img's native size already encodes which edge it belongs to -- a
// TileSize-wide, shallow band for N/S, or a TileSize-tall, narrow band for
// E/W). farEdge selects the translate branch: false positions the image at
// the tile's own origin (already correct for N, whose band starts at the
// top, and W, whose band starts at the left); true shifts it flush against
// the tile's bottom (S) or right (E) edge instead. The additive
// (ebiten.BlendLighter) blend is what makes this a brightening correction
// rather than an opaque patch -- the cobblestone texture underneath stays
// visible.
func drawRoadFade(screen *ebiten.Image, img *ebiten.Image, sx, sy, tilePixels float64, farEdge bool) {
	b := img.Bounds()
	s := tilePixels / float64(assets.TileSize)
	w := float64(b.Dx()) * s
	h := float64(b.Dy()) * s
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(s, s)
	dx, dy := sx, sy
	if farEdge && w < h {
		dx = sx + tilePixels - w // East: the narrow (depth) axis is X
	} else if farEdge {
		dy = sy + tilePixels - h // South: the narrow (depth) axis is Y
	}
	op.GeoM.Translate(dx, dy)
	op.Blend = ebiten.BlendLighter
	screen.DrawImage(img, op)
}

// drawConstructionSite renders foundation, waiting-for-materials and finishing
// stages. A building may provide three dedicated art layers; incomplete visual
// families safely retain the shared site frames plus that building's silhouette.
func drawConstructionSite(screen *ebiten.Image, b *building.Building, footprint int, sx, sy, tilePixels float64) {
	size := float32(footprint) * float32(tilePixels)
	vector.FillRect(screen, float32(sx), float32(sy), size, size, constructionGroundColor, false)
	if b.ConstructionStage == building.ConstructionWaitingMaterials {
		vector.FillRect(screen, float32(sx), float32(sy), size, size, constructionWaitColor, false)
	}

	// The visual manifest supplies independent layers for every building and
	// construction state. Until a bespoke PNG exists the manifest falls back
	// to the approved shared site sheet while retaining the selected building's
	// faint silhouette, so the map never shows the wrong type of construction.
	if visual, ok := assets.BuildingVisualFor(b.Kind); ok && b.ConstructionStage <= building.ConstructionFinishing {
		art := visual.Construction[b.ConstructionStage]
		drawVisualLayerAtScale(screen, art.Site, sx, sy, float64(footprint), tilePixels)
		drawVisualLayerAtScale(screen, art.Preview, sx, sy, float64(footprint), tilePixels)
	} else {
		siteArt := assets.ConstructionFoundation
		if b.ConstructionStage == building.ConstructionFinishing {
			siteArt = assets.ConstructionScaffolding
		}
		drawFootprintAtScale(screen, siteArt, sx, sy, float64(footprint), tilePixels)
	}

	drawConstructionSiteEffect(screen, b.ConstructionStage, sx, sy, float64(footprint), tilePixels)
	progress := float32(b.ConstructionProgress())
	barY := float32(sy) + size - float32(3*tilePixels/TileSize)
	barHeight := float32(3 * tilePixels / TileSize)
	vector.FillRect(screen, float32(sx), barY, size*progress, barHeight, constructionBarColor, false)
}

// drawStoneDeposit draws a ground-level boulder cluster rather than a standing
// object. The tint drifts from full-reserve gray toward a sun-bleached
// tone as it depletes, so a partly-worked deposit reads at a glance without
// needing dedicated multi-stage art.
func drawStoneDeposit(screen *ebiten.Image, sx, sy, tilePixels float64, reserve int) {
	fraction := float32(reserve) / float32(building.StoneDepositReserve)
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	tint := lerpColor(stoneWornColor, stoneFullColor, fraction)
	drawStandingTintedAtScale(screen, assets.StoneDeposit, sx, sy, 1, tilePixels, tint)
}

// drawOreDeposit renders each ore family with its own transparent boulder
// cluster. As a reserve is exhausted the cluster fades toward a pale worn
// tint, keeping depletion readable without disguising its material type.
func drawOreDeposit(screen *ebiten.Image, sx, sy, tilePixels float64, kind building.Kind, reserve int) {
	fraction := float32(reserve) / float32(building.OreDepositReserve)
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	full := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	var sprite *ebiten.Image
	var worn color.RGBA
	switch kind {
	case building.GoldOreDeposit:
		sprite, worn = assets.GoldOreDeposit, goldOreWornColor
	case building.IronOreDeposit:
		sprite, worn = assets.IronOreDeposit, ironOreWornColor
	default: // building.CoalDeposit
		sprite, worn = assets.CoalDeposit, coalWornColor
	}
	tint := lerpColor(worn, full, fraction)
	drawStandingTintedAtScale(screen, sprite, sx, sy, 1, tilePixels, tint)
}

// drawFish uses the dedicated three-stage transparent fish sprite sheet. The
// sprite is deliberately small against a water tile: fish should read as part
// of the pond, not as a rectangular world marker or a UI icon.
func drawFish(screen *ebiten.Image, sx, sy, tilePixels float64, stage int) {
	if stage < 0 {
		stage = 0
	}
	if stage > 2 {
		stage = 2
	}
	// A mature fish occupies at most about half a water cell. At the previous
	// standing-building scale it read as a giant creature rather than a quiet
	// population marker, especially after zooming in.
	drawStandingAtScale(screen, assets.FishFrames[stage], sx, sy, 0.55, tilePixels)
}
