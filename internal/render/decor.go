package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"strategy_game/internal/world"
)

// tileHash is a small, deterministic per-tile hash used to pick sparse,
// stable-across-frames decoration (which tiles get one, and which variant)
// without storing any extra per-tile state -- the same trick
// drawGrassSway already uses, just with different multipliers so the two
// effects don't always land on the same tiles.
func tileHash(tx, ty int, multiplierA, multiplierB uint32) uint32 {
	return uint32(tx)*multiplierA + uint32(ty)*multiplierB
}

// drawGroundDecor adds small, purely cosmetic detail on top of the plain
// terrain tile -- rocks/stumps/flowers/bushes scattered thinly across
// grass, reeds and pebbles along the shoreline. None of this touches
// world.Grid or building.CanPlace: it is drawn, not simulated, so it can
// never block or interfere with placement the way an actual world object
// (a Tree, a StoneDeposit) does.
//
// Every shape below is sized as a fraction of tilePixels (the tile's
// actual on-screen size), not of assets.TileSize (the 64px source-art
// canvas the sprite-scaling helpers in sprite.go key off) -- these are
// drawn directly with package vector, not scaled from a source image, so
// TileSize is the wrong reference and would render at a fraction of a
// pixel at the default 24px zoom.
func drawGroundDecor(screen *ebiten.Image, g *world.Grid, terrain world.TerrainType, tx, ty int, sx, sy, tilePixels float64) {
	switch terrain {
	case world.Grass:
		if shoreEdge, ok := findShoreEdge(g, tx, ty, world.Water); ok {
			drawPebbles(screen, sx, sy, tx, ty, tilePixels, shoreEdge)
			return // a shore tile keeps its pebbles; no rock/flower clutter on top
		}
		drawGrassDecor(screen, sx, sy, tx, ty, tilePixels)
	case world.Water:
		if shoreEdge, ok := findShoreEdge(g, tx, ty, world.Grass); ok {
			drawReeds(screen, sx, sy, tx, ty, tilePixels, shoreEdge)
		}
	}
}

// findShoreEdge reports the first cardinal direction (fixed N,E,S,W scan
// order -- decoration doesn't need to react to every edge of a concave
// coastline, just to read as "near the shore") where tile (tx,ty) borders
// a neighbour of the given terrain.
func findShoreEdge(g *world.Grid, tx, ty int, neighborTerrain world.TerrainType) (edgeSide, bool) {
	for _, d := range [...]struct {
		dx, dy int
		edge   edgeSide
	}{{0, -1, edgeN}, {1, 0, edgeE}, {0, 1, edgeS}, {-1, 0, edgeW}} {
		nx, ny := tx+d.dx, ty+d.dy
		if g.InBounds(nx, ny) && g.At(nx, ny).Terrain == neighborTerrain {
			return d.edge, true
		}
	}
	return 0, false
}

// edgeAnchor returns a point near the tile's edge in the given direction
// -- shared by drawReeds/drawPebbles to place their decoration flush
// against the shoreline rather than floating in the tile's centre.
func edgeAnchor(sx, sy, tilePixels float64, edge edgeSide) (x, y float64) {
	switch edge {
	case edgeN:
		return sx + tilePixels*0.5, sy + tilePixels*0.22
	case edgeS:
		return sx + tilePixels*0.5, sy + tilePixels*0.78
	case edgeW:
		return sx + tilePixels*0.22, sy + tilePixels*0.5
	default: // edgeE
		return sx + tilePixels*0.78, sy + tilePixels*0.5
	}
}

var (
	rockColor          = color.RGBA{R: 96, G: 95, B: 92, A: 255} // darker than the earlier 120/118/112 -- too close to grass's own shading to read as a distinct rock at normal zoom, especially inside a rocky decorZoneKind meant to look visually distinct
	rockHighlightColor = color.RGBA{R: 165, G: 163, B: 155, A: 255}
	stumpColor         = color.RGBA{R: 107, G: 74, B: 46, A: 255}
	stumpRingColor     = color.RGBA{R: 79, G: 53, B: 31, A: 255}
	bushColor          = color.RGBA{R: 58, G: 102, B: 44, A: 255}
	flowerStemColor    = color.RGBA{R: 79, G: 122, B: 56, A: 255}
	reedColor          = color.RGBA{R: 121, G: 168, B: 77, A: 255}
	pebbleColor        = color.RGBA{R: 186, G: 182, B: 168, A: 255}
)

// flowerColors are cycled by tile hash so a patch of flowers isn't every
// one the same colour.
var flowerColors = [...]color.RGBA{
	{R: 231, G: 196, B: 84, A: 255},  // yellow
	{R: 236, G: 236, B: 240, A: 255}, // white
	{R: 186, G: 130, B: 224, A: 255}, // violet
}

// decorZoneSize is the edge length, in tiles, of one decorative-zone cell
// -- see decorZoneKindAt.
const decorZoneSize = 12

// decorZoneKind is a coarse cosmetic "flavour" layered on top of
// drawGrassDecor's uniform per-tile scatter -- a meadow reads as
// noticeably flower-heavy, a rocky patch as noticeably rock-heavy,
// spanning many tiles instead of drawGrassDecor's usual scattered
// individual decorations. Purely visual: neither affects CanPlace or
// anything else in world.Grid.
type decorZoneKind int

const (
	zoneNone decorZoneKind = iota
	zoneMeadow
	zoneRocky
)

// decorZoneKindAt reports which decorative zone tile (tx,ty) falls in.
// Deterministic from tile coordinates alone, the same way tileHash's
// per-tile decorations already are -- nothing about a zone's shape or
// location is stored anywhere, so a save/reload reproduces the exact
// same zones without a single new save-file field. Most zone cells are
// plain (zoneNone, ~65%); a minority read as a distinct meadow or rocky
// patch (~17.5% each) -- the same spirit as seedThickets layering denser
// tree clusters on top of the uniform tree scatter, but computed at
// render time instead of placed as real world objects, since a patch of
// flowers has no gameplay weight to justify that.
func decorZoneKindAt(tx, ty int) decorZoneKind {
	zx, zy := tx/decorZoneSize, ty/decorZoneSize
	roll := tileHash(zx, zy, 668265263, 2246822519) % 100
	switch {
	case roll < 17:
		return zoneMeadow
	case roll < 34:
		return zoneRocky
	default:
		return zoneNone
	}
}

// grassDecorPick is drawGrassDecor's decision logic, pulled out as a pure
// function so the density/variant-bias rules a decorZoneKind applies can
// be checked with a plain go test instead of only by eye in a screenshot
// (a screenshot is still how the actual visual result was checked --
// individual dots at normal zoom are too small and too close to
// drawGrassSway's own texture noise to visually judge a 3x density
// change by eye with any confidence).
func grassDecorPick(tx, ty int) (variant int, ok bool) {
	hash := tileHash(tx, ty, 2654435761, 40503)
	zone := decorZoneKindAt(tx, ty)
	threshold := uint32(32)
	if zone != zoneNone {
		threshold = 10
	}
	if hash%threshold != 0 {
		return 0, false
	}

	variant = int((hash / 800) % 4)
	switch zone {
	case zoneMeadow:
		if (hash/800)%3 != 0 {
			variant = 2 // mostly flowers, ...
		} else {
			variant = 3 // ...occasionally a bush
		}
	case zoneRocky:
		if (hash/800)%3 != 0 {
			variant = 0 // mostly rocks, ...
		} else {
			variant = 1 // ...occasionally a stump
		}
	}
	return variant, true
}

// drawGrassDecor scatters one small decoration (rock/stump/flower/bush)
// onto grass tiles -- roughly 1 in 32 outside any named zone, sparse
// enough to read as detail, not clutter and independent of
// drawGrassSway's own tile selection; roughly 1 in 10 (and biased toward
// that zone's own flavour) inside a meadow or rocky decorZoneKind.
func drawGrassDecor(screen *ebiten.Image, sx, sy float64, tx, ty int, tilePixels float64) {
	variant, ok := grassDecorPick(tx, ty)
	if !ok {
		return
	}
	hash := tileHash(tx, ty, 2654435761, 40503)
	cx := float32(sx + tilePixels*(0.3+0.4*float64((hash/32)%5)/4))
	cy := float32(sy + tilePixels*(0.35+0.35*float64((hash/160)%5)/4))
	tp := float32(tilePixels)

	switch variant {
	case 0: // rock
		vector.FillCircle(screen, cx, cy, 0.16*tp, rockColor, false)
		vector.FillCircle(screen, cx-0.06*tp, cy-0.06*tp, 0.06*tp, rockHighlightColor, false)
	case 1: // stump
		vector.FillCircle(screen, cx, cy, 0.15*tp, stumpColor, false)
		vector.StrokeCircle(screen, cx, cy, 0.08*tp, 0.03*tp, stumpRingColor, false)
	case 2: // flower cluster
		vector.StrokeLine(screen, cx, cy+0.16*tp, cx, cy-0.02*tp, 0.025*tp, flowerStemColor, false)
		petal := flowerColors[(hash/3200)%uint32(len(flowerColors))]
		vector.FillCircle(screen, cx, cy-0.08*tp, 0.07*tp, petal, false)
	default: // bush
		vector.FillCircle(screen, cx-0.09*tp, cy, 0.11*tp, bushColor, false)
		vector.FillCircle(screen, cx+0.09*tp, cy, 0.11*tp, bushColor, false)
		vector.FillCircle(screen, cx, cy-0.09*tp, 0.12*tp, bushColor, false)
	}
}

// drawReeds marks roughly 1 in 3 water tiles that border grass with a
// couple of thin reed strokes near the shore edge.
func drawReeds(screen *ebiten.Image, sx, sy float64, tx, ty int, tilePixels float64, edge edgeSide) {
	hash := tileHash(tx, ty, 374761393, 668265263)
	if hash%3 != 0 {
		return
	}
	ax, ay := edgeAnchor(sx, sy, tilePixels, edge)
	tp := float32(tilePixels)
	for i, offset := range [...]float32{-0.22 * tp, 0, 0.22 * tp} {
		lean := 0.08 * tp * float32(i-1)
		var x0, y0, x1, y1 float32
		if edge == edgeN || edge == edgeS {
			x0, y0 = float32(ax)+offset, float32(ay)+0.14*tp
			x1, y1 = x0+lean, y0-0.4*tp
		} else {
			x0, y0 = float32(ax)+0.14*tp, float32(ay)+offset
			x1, y1 = x0-0.4*tp, y0+lean
		}
		vector.StrokeLine(screen, x0, y0, x1, y1, 0.07*tp, reedColor, true)
	}
}

// drawPebbles marks roughly 1 in 3 grass tiles that border water with a
// few small pebbles near the shore edge.
func drawPebbles(screen *ebiten.Image, sx, sy float64, tx, ty int, tilePixels float64, edge edgeSide) {
	hash := tileHash(tx, ty, 2246822519, 3266489917)
	if hash%3 != 0 {
		return
	}
	ax, ay := edgeAnchor(sx, sy, tilePixels, edge)
	tp := float32(tilePixels)
	for _, spread := range [...][2]float32{{-0.16, 0}, {0.08, 0.09}, {0.2, -0.08}} {
		vector.FillCircle(screen, float32(ax)+spread[0]*tp, float32(ay)+spread[1]*tp, 0.045*tp, pebbleColor, false)
	}
}
