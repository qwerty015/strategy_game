package render

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/assets"
	"strategy_game/internal/world"
)

// DrawGrid renders every tile currently visible in the camera's viewport.
// Off-screen tiles are skipped so panning stays cheap on a big map.
func DrawGrid(screen *ebiten.Image, g *world.Grid, cam *Camera) {
	visible := cam.VisibleTileBounds(0)
	tilePixels := cam.TilePixels()

	for ty := visible.MinY; ty <= visible.MaxY; ty++ {
		for tx := visible.MinX; tx <= visible.MaxX; tx++ {
			if !g.InBounds(tx, ty) {
				continue
			}
			sx, sy := cam.TileToScreen(tx, ty)
			terrain := g.At(tx, ty).Terrain
			drawGround(screen, terrainImage(terrain), sx, sy, tilePixels)
			if terrain == world.Grass {
				drawGrassSway(screen, sx, sy, tx, ty, tilePixels)
			}
			blendTerrainEdges(screen, g, terrain, tx, ty, sx, sy, tilePixels)
			drawGroundDecor(screen, g, terrain, tx, ty, sx, sy, tilePixels)
		}
	}
}

// terrainEdgeBands shapes blendTerrainEdges' feathered shoreline, from
// outermost (right at the tile boundary) to innermost (deepest into the
// tile): each band redraws a shrinking crop of the *neighbouring* tile's
// own texture, cropped from the strip of it nearest the shared edge, at
// low alpha. Layered together via ordinary alpha-over, the strongest
// blending lands right at the boundary and tapers off by depthFraction,
// turning a hard one-pixel terrain swap (e.g. grass meeting water) into a
// soft multi-pixel gradient -- without needing dedicated shoreline art or
// a custom shader.
var terrainEdgeBands = [...]struct {
	depthFraction float64 // how deep into the tile this band's crop reaches, as a fraction of TileSize
	alpha         float32
}{
	{1.00, 0.16},
	{0.70, 0.18},
	{0.40, 0.22},
	{0.18, 0.26},
}

// blendTerrainEdges softens the boundary of tile (tx,ty) against any of
// its 4 cardinal neighbours that has a *different* terrain type -- see
// terrainEdgeBands' doc comment. A tile with no differing neighbour (the
// overwhelming majority, away from any coastline) draws nothing extra.
func blendTerrainEdges(screen *ebiten.Image, g *world.Grid, terrain world.TerrainType, tx, ty int, sx, sy, tilePixels float64) {
	type dir struct {
		dx, dy int
		edge   edgeSide
	}
	for _, d := range [...]dir{{0, -1, edgeN}, {0, 1, edgeS}, {-1, 0, edgeW}, {1, 0, edgeE}} {
		nx, ny := tx+d.dx, ty+d.dy
		if !g.InBounds(nx, ny) {
			continue
		}
		neighborTerrain := g.At(nx, ny).Terrain
		if neighborTerrain == terrain {
			continue
		}
		neighborImg := terrainImage(neighborTerrain)
		for _, band := range terrainEdgeBands {
			drawEdgeStrip(screen, neighborImg, sx, sy, tilePixels, d.edge, band.depthFraction*assets.TileSize, band.alpha)
		}
	}
}

// edgeSide names which of a tile's 4 sides an edge-strip effect (a road
// connector's brightening fade, or a terrain boundary's feathered blend)
// belongs to.
type edgeSide int

const (
	edgeN edgeSide = iota
	edgeE
	edgeS
	edgeW
)

// drawEdgeStrip crops a depthPx-deep band from img's own edge nearest the
// shared boundary (e.g. for edgeN, img's own bottom rows -- the part of
// the neighbour actually touching this tile) and draws it at alpha,
// flush against this tile's matching edge. Used by blendTerrainEdges to
// stack several such crops at decreasing alpha for a soft gradient.
func drawEdgeStrip(screen *ebiten.Image, img *ebiten.Image, sx, sy, tilePixels float64, edge edgeSide, depthPx float64, alpha float32) {
	b := img.Bounds()
	d := int(depthPx)
	if d < 1 {
		d = 1
	}
	if d > b.Dy() {
		d = b.Dy()
	}
	var src image.Rectangle
	switch edge {
	case edgeN:
		src = image.Rect(b.Min.X, b.Max.Y-d, b.Max.X, b.Max.Y)
	case edgeS:
		src = image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+d)
	case edgeW:
		src = image.Rect(b.Max.X-d, b.Min.Y, b.Max.X, b.Max.Y)
	case edgeE:
		src = image.Rect(b.Min.X, b.Min.Y, b.Min.X+d, b.Max.Y)
	}
	sub, ok := img.SubImage(src).(*ebiten.Image)
	if !ok {
		return
	}

	s := tilePixels / assets.TileSize
	destDepth := depthPx * s
	dx, dy := sx, sy
	switch edge {
	case edgeS:
		dy = sy + tilePixels - destDepth
	case edgeE:
		dx = sx + tilePixels - destDepth
	}

	op := &ebiten.DrawImageOptions{}
	// SubImage keeps the parent's coordinate space (its Bounds() is src
	// itself, not reset to the origin) -- normalize it to (0,0) before
	// scaling/positioning, or the crop would be drawn offset by its own
	// position within the source texture.
	op.GeoM.Translate(-float64(src.Min.X), -float64(src.Min.Y))
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(dx, dy)
	op.ColorScale.ScaleAlpha(alpha)
	op.Blend = ebiten.BlendSourceOver
	screen.DrawImage(sub, op)
}

// drawGround uses integer destination edges and a tiny one-pixel overlap.
// Fractional camera zoom otherwise lets independently scaled PNG tiles leave
// bright hairline seams between them due to rounding/filtering.
func drawGround(screen *ebiten.Image, img *ebiten.Image, sx, sy, tilePixels float64) {
	b := img.Bounds()
	x0 := math.Floor(sx)
	y0 := math.Floor(sy)
	x1 := math.Ceil(sx+tilePixels) + 1
	y1 := math.Ceil(sy+tilePixels) + 1
	if x1 <= x0 || y1 <= y0 {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale((x1-x0)/float64(b.Dx()), (y1-y0)/float64(b.Dy()))
	op.GeoM.Translate(x0, y0)
	op.Blend = ebiten.BlendSourceOver
	screen.DrawImage(img, op)
}

// drawGrassSway is a tiny two-frame ambient animation. Only a deterministic
// subset of grass tiles gets a few one-pixel blades, so the whole map does not
// flicker and the effect remains cheap at the prototype's scale.
func drawGrassSway(screen *ebiten.Image, sx, sy float64, tx, ty int, tilePixels float64) {
	seed := (tx*92821 + ty*68917) & 7
	if seed > 1 {
		return
	}
	phase := (animFrame/10 + tx + ty) & 1
	xOffset := float32(phase) * float32(tilePixels/TileSize)
	baseX := float32(sx) + float32(5+seed*7)*float32(tilePixels/TileSize)
	baseY := float32(sy) + float32(19)*float32(tilePixels/TileSize)
	blade := color.RGBA{R: 91, G: 157, B: 61, A: 190}
	width := float32(tilePixels / TileSize)
	if width < 1 {
		width = 1
	}
	vector.FillRect(screen, baseX+xOffset, baseY-4*width, width, 4*width, blade, false)
	vector.FillRect(screen, baseX+width-xOffset, baseY-3*width, width, 3*width, blade, false)
}

// terrainImage returns the ground sprite for a tile. Tree world objects are
// rendered separately above the ground, so a map does not need a reserved
// forest area for them to appear.
func terrainImage(t world.TerrainType) *ebiten.Image {
	switch t {
	case world.Fertile:
		return assets.Fertile
	case world.Forest:
		return assets.Forest
	case world.Water:
		return assets.Water
	case world.Stone:
		return assets.Stone
	default: // world.Grass
		return assets.Grass
	}
}
