package render

import (
	"image/color"

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
	soilColor      = color.RGBA{R: 92, G: 66, B: 38, A: 255}   // freshly tilled earth (tints assets.Fertile)
	ripeWheatColor = color.RGBA{R: 231, G: 196, B: 84, A: 255} // golden, ready to harvest
	vineyardSoil   = color.RGBA{R: 80, G: 61, B: 38, A: 255}   // darker soil for grape rows
	unstaffedTint  = color.RGBA{R: 214, G: 63, B: 55, A: 90}   // translucent red over a workerless building
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
func DrawBuildings(screen *ebiten.Image, grid *world.Grid, buildings []*building.Building, cam *Camera, unstaffed map[*building.Building]bool) {
	tilePixels := cam.TilePixels()
	// Ground is drawn before this function. Roads are the bottom gameplay
	// layer, so render every road before any tree, field or standing building.
	for _, b := range buildings {
		if b.Kind != building.Road {
			continue
		}
		sx, sy := cam.TileToScreen(b.X, b.Y)
		drawStandingAtScale(screen, assets.Road, sx, sy, 1, tilePixels)
	}

	for _, b := range buildings {
		if b.Kind == building.Road {
			continue
		}
		bt := building.Types[b.Kind]
		sx, sy := cam.TileToScreen(b.X, b.Y)

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
			drawStandingAtScale(screen, assets.FarmHouse, sx, sy, buildingHeight, tilePixels)

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
			drawStandingAtScale(screen, assets.Winery, sx, sy, buildingHeight, tilePixels)

		case building.PigFarm:
			drawStandingAtScale(screen, assets.PigFarm, sx, sy, buildingHeight, tilePixels)

		case building.MeatWorkshop:
			drawStandingAtScale(screen, assets.MeatWorkshop, sx, sy, buildingHeight, tilePixels)

		case building.Mill:
			drawStandingAtScale(screen, assets.MillFrames[(animFrame/12)%len(assets.MillFrames)], sx, sy, buildingHeight, tilePixels)

		case building.Bakery:
			drawStandingAtScale(screen, assets.Bakery, sx, sy, buildingHeight, tilePixels)
			drawFire(screen, sx+17*tilePixels/TileSize, sy+4*tilePixels/TileSize, tilePixels)

		case building.Warehouse:
			drawStandingAtScale(screen, assets.Warehouse, sx, sy, buildingHeight, tilePixels)

		case building.Tavern:
			drawStandingAtScale(screen, assets.Tavern, sx, sy, buildingHeight, tilePixels)

		case building.LumberjackHut:
			drawStandingAtScale(screen, assets.LumberjackHut, sx, sy, buildingHeight, tilePixels)

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
			drawStandingAtScale(screen, assets.FisherHutFrames[frame], sx, sy, buildingHeight, tilePixels)
		}

		if bt.Recipe.TicksToProduce > 0 {
			progress := float32(b.ProgressTicks) / float32(bt.Recipe.TicksToProduce)
			if progress > 1 {
				progress = 1
			}
			barWidth := float32(bt.Footprint) * float32(tilePixels)
			barY := float32(sy) + float32(bt.Footprint)*float32(tilePixels) - float32(3*tilePixels/TileSize)
			barHeight := float32(3 * tilePixels / TileSize)
			vector.FillRect(screen, float32(sx), barY, barWidth*progress, barHeight, color.RGBA{R: 255, G: 255, B: 0, A: 220}, false)
		}

		// A production building with no resident worker at all (as opposed
		// to one merely away eating) is tinted red across its whole
		// footprint, so an empty workplace reads at a glance instead of
		// only being discoverable by opening the inspector.
		if unstaffed[b] {
			size := float32(bt.Footprint) * float32(tilePixels)
			vector.FillRect(screen, float32(sx), float32(sy), size, size, unstaffedTint, false)
		}
	}
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
