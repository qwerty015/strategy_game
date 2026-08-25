package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/building"
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
// Road is one path tile. Mill/Bakery/Warehouse/Tavern each stand on
// their single tile taller than the tile itself (see buildingHeight),
// with a production-progress bar underneath. MillFrames contains three
// compact sail positions, switched periodically to animate the windmill.
func DrawBuildings(screen *ebiten.Image, buildings []*building.Building, cam *Camera) {
	tilePixels := cam.TilePixels()
	for _, b := range buildings {
		bt := building.Types[b.Kind]
		sx, sy := cam.TileToScreen(b.X, b.Y)

		switch b.Kind {
		case building.Road:
			drawStandingAtScale(screen, assets.Road, sx, sy, 1, tilePixels)
			continue

		case building.Tree:
			drawTree(screen, sx, sy, tilePixels, b.GrowthStage())
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

		case building.Mill:
			drawStandingAtScale(screen, assets.MillFrames[(animFrame/12)%len(assets.MillFrames)], sx, sy, buildingHeight, tilePixels)

		case building.Bakery:
			drawStandingAtScale(screen, assets.Bakery, sx, sy, buildingHeight, tilePixels)
			drawFire(screen, sx+17*tilePixels/TileSize, sy+4*tilePixels/TileSize, tilePixels)

		case building.Warehouse:
			drawStandingAtScale(screen, assets.Warehouse, sx, sy, buildingHeight, tilePixels)

		case building.Tavern:
			drawStandingAtScale(screen, assets.Tavern, sx, sy, buildingHeight, tilePixels)
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
	}
}

// drawTree is a deliberately small procedural sprite. The terrain already
// contains background forest art; this separate silhouette represents the
// persistent object that can grow and later become a lumberjack target.
func drawTree(screen *ebiten.Image, sx, sy, tilePixels float64, stage int) {
	if stage < 0 {
		stage = 0
	}
	if stage > 2 {
		stage = 2
	}
	scale := tilePixels / TileSize
	baseX := sx + tilePixels/2
	baseY := sy + tilePixels
	trunkWidth := float32(2+stage) * float32(scale)
	trunkHeight := float32(5+stage*3) * float32(scale)
	vector.FillRect(screen, float32(baseX)-trunkWidth/2, float32(baseY)-trunkHeight, trunkWidth, trunkHeight, color.RGBA{R: 106, G: 65, B: 35, A: 255}, false)

	if stage == 0 {
		vector.FillCircle(screen, float32(baseX), float32(baseY)-trunkHeight-float32(2*scale), float32(4*scale), color.RGBA{R: 91, G: 142, B: 53, A: 240}, false)
		return
	}

	radius := float32(5+stage*2) * float32(scale)
	foliage := color.RGBA{R: 55, G: 123, B: 54, A: 245}
	light := color.RGBA{R: 89, G: 157, B: 66, A: 235}
	vector.FillCircle(screen, float32(baseX), float32(baseY)-trunkHeight-radius, radius, foliage, false)
	vector.FillCircle(screen, float32(baseX)-radius*0.75, float32(baseY)-trunkHeight-radius*0.65, radius*0.72, light, false)
	vector.FillCircle(screen, float32(baseX)+radius*0.72, float32(baseY)-trunkHeight-radius*0.58, radius*0.68, foliage, false)
	if stage == 2 {
		vector.FillCircle(screen, float32(baseX), float32(baseY)-trunkHeight-radius*1.7, radius*0.72, light, false)
	}
}
