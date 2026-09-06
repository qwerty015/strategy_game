package render

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/building"
	"strategy_game/internal/world"
)

// minimapSampleStep is the size (in minimap pixels) of one sampled terrain
// cell. Sampling at a coarse, fixed pixel step -- rather than drawing one
// rectangle per world tile -- keeps the minimap's draw-call count bounded
// by its own on-screen size instead of by how large the map happens to be
// (the title showcase map alone is 168x74 = over 12,000 tiles).
const minimapSampleStep = 3

var minimapTerrainColor = map[world.TerrainType]color.RGBA{
	world.Grass:   {R: 74, G: 108, B: 58, A: 255},
	world.Fertile: {R: 107, G: 90, B: 52, A: 255},
	world.Forest:  {R: 43, G: 74, B: 41, A: 255},
	world.Water:   {R: 46, G: 78, B: 112, A: 255},
	world.Stone:   {R: 120, G: 118, B: 112, A: 255},
}

// minimapBuildingColor buckets every building kind into a small readable
// palette rather than 21 near-identical dots -- the minimap is meant for
// at-a-glance orientation, not for telling a Mill from a Bakery.
//
// A road/tree/fish/deposit tile keeps its neutral bucket color regardless
// of owner (same convention as drawOwnerOutline/isNaturalResourceKind
// elsewhere -- these aren't faction territory markers). Everything else
// owned by another faction (owner != 0) uses that faction's own color
// (see colorForOwner) instead of the generic orange bucket, per the
// user's explicit request ("раскрашивай постройки противников на
// миникарте согласно цвету игрока") -- so an opponent's base reads as a
// distinct blob of their own color at a glance, the same way it already
// does on the main map via drawOwnerOutline.
func minimapBuildingColor(k building.Kind, owner int) color.RGBA {
	switch k {
	case building.Road:
		return color.RGBA{R: 176, G: 168, B: 150, A: 255}
	case building.Tree:
		return color.RGBA{R: 58, G: 122, B: 52, A: 255}
	case building.Fish:
		return color.RGBA{R: 130, G: 190, B: 210, A: 255}
	case building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit:
		return color.RGBA{R: 150, G: 150, B: 150, A: 255}
	}
	if owner != 0 {
		return colorForOwner(owner)
	}
	switch k {
	case building.Warehouse, building.Tavern:
		return color.RGBA{R: 224, G: 168, B: 68, A: 255}
	default:
		return color.RGBA{R: 214, G: 96, B: 62, A: 255}
	}
}

// DrawMinimap renders a small overview of the whole map into rect: sampled
// terrain, one dot per building and the camera's current viewport as an
// outlined rectangle. Clicking inside rect is handled by the caller (see
// cmd/game's minimap click handling) via MinimapWorldPoint, which shares
// this function's exact coordinate mapping.
func DrawMinimap(screen *ebiten.Image, grid *world.Grid, buildings []*building.Building, cam *Camera, rect image.Rectangle) {
	if rect.Dx() <= 0 || rect.Dy() <= 0 || grid == nil || grid.Width <= 0 || grid.Height <= 0 {
		return
	}
	vector.FillRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), color.RGBA{R: 20, G: 22, B: 18, A: 255}, false)

	for py := rect.Min.Y; py < rect.Max.Y; py += minimapSampleStep {
		for px := rect.Min.X; px < rect.Max.X; px += minimapSampleStep {
			tx, ty := minimapPixelToTile(rect, grid, px, py)
			if !grid.InBounds(tx, ty) {
				continue
			}
			c := minimapTerrainColor[grid.At(tx, ty).Terrain]
			w := float32(minimapSampleStep)
			h := float32(minimapSampleStep)
			if px+minimapSampleStep > rect.Max.X {
				w = float32(rect.Max.X - px)
			}
			if py+minimapSampleStep > rect.Max.Y {
				h = float32(rect.Max.Y - py)
			}
			vector.FillRect(screen, float32(px), float32(py), w, h, c, false)
		}
	}

	scaleX := float64(rect.Dx()) / float64(grid.Width)
	scaleY := float64(rect.Dy()) / float64(grid.Height)
	for _, b := range buildings {
		if b == nil {
			continue
		}
		x := float32(rect.Min.X) + float32(float64(b.X)*scaleX)
		y := float32(rect.Min.Y) + float32(float64(b.Y)*scaleY)
		vector.FillRect(screen, x, y, maxPixel(scaleX), maxPixel(scaleY), minimapBuildingColor(b.Kind, b.Owner), false)
	}

	drawMinimapViewport(screen, grid, cam, rect)
	vector.StrokeRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), 2, color.RGBA{R: 151, G: 111, B: 64, A: 255}, false)
}

func drawMinimapViewport(screen *ebiten.Image, grid *world.Grid, cam *Camera, rect image.Rectangle) {
	_, _, vw, vh := cam.Viewport()
	if vw <= 0 || vh <= 0 || cam.Scale <= 0 {
		return
	}
	worldLeft := cam.X / TileSize
	worldTop := cam.Y / TileSize
	worldWidth := float64(vw) / cam.Scale / TileSize
	worldHeight := float64(vh) / cam.Scale / TileSize

	scaleX := float64(rect.Dx()) / float64(grid.Width)
	scaleY := float64(rect.Dy()) / float64(grid.Height)

	x := float32(rect.Min.X) + float32(worldLeft*scaleX)
	y := float32(rect.Min.Y) + float32(worldTop*scaleY)
	w := float32(worldWidth * scaleX)
	h := float32(worldHeight * scaleY)
	vector.StrokeRect(screen, x, y, w, h, 2, color.RGBA{R: 255, G: 240, B: 200, A: 230}, false)
}

// minimapPixelToTile maps a screen pixel inside rect back to a world tile,
// the exact inverse of the scaleX/scaleY mapping DrawMinimap's building
// dots and viewport box use.
func minimapPixelToTile(rect image.Rectangle, grid *world.Grid, px, py int) (int, int) {
	scaleX := float64(rect.Dx()) / float64(grid.Width)
	scaleY := float64(rect.Dy()) / float64(grid.Height)
	tx := int(float64(px-rect.Min.X) / scaleX)
	ty := int(float64(py-rect.Min.Y) / scaleY)
	return tx, ty
}

// MinimapWorldPoint converts a click at (px, py) inside rect into a world
// PIXEL coordinate (not a tile), ready for Camera.CenterOn. It clamps to
// the map bounds so a click right on the minimap's edge can never resolve
// just outside the grid.
func MinimapWorldPoint(grid *world.Grid, rect image.Rectangle, px, py int) (worldX, worldY float64, ok bool) {
	if grid == nil || grid.Width <= 0 || grid.Height <= 0 || rect.Dx() <= 0 || rect.Dy() <= 0 {
		return 0, 0, false
	}
	point := image.Pt(px, py)
	if !point.In(rect) {
		return 0, 0, false
	}
	tx, ty := minimapPixelToTile(rect, grid, px, py)
	if tx < 0 {
		tx = 0
	}
	if tx >= grid.Width {
		tx = grid.Width - 1
	}
	if ty < 0 {
		ty = 0
	}
	if ty >= grid.Height {
		ty = grid.Height - 1
	}
	return float64(tx)*TileSize + TileSize/2, float64(ty)*TileSize + TileSize/2, true
}
