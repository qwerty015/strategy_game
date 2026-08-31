package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"strategy_game/internal/assets"
	"strategy_game/internal/building"
	"strategy_game/internal/pathfind"
)

// drawStanding draws img so its bottom edge sits on the bottom of tile
// (sx, sy) and it's horizontally centered on that tile, scaled so its
// height equals tilesTall tiles. tilesTall=1 makes it fill exactly one
// tile (used for ground tiles); tilesTall>1 makes it rise above the tile
// (used for buildings), matching the source pack's look: single-cell
// footprint, sprite taller than the cell (see AGENTS.md).
//
// Every sprite in package assets shares one 64x64 canvas convention, so
// calling this with the same (img varies, sx, sy, tilesTall) for two
// different images -- e.g. a building base and an overlay -- draws them
// perfectly aligned with no extra offset math.
func drawStanding(screen *ebiten.Image, img *ebiten.Image, sx, sy, tilesTall float64) {
	drawStandingScaled(screen, img, sx, sy, tilesTall, TileSize, color.White)
}

func drawStandingAtScale(screen *ebiten.Image, img *ebiten.Image, sx, sy, tilesTall, tilePixels float64) {
	drawStandingScaled(screen, img, sx, sy, tilesTall, tilePixels, color.White)
}

// drawFootprintAtScale fills a square footprint starting exactly at its
// upper-left map cell. It is intentionally separate from drawStanding:
// construction-site frames are ground objects spanning several cells, not
// tall sprites anchored to the first cell. Using drawStanding for a 3×3
// frame would centre it on that first cell and shift it one tile up-left.
func drawFootprintAtScale(screen *ebiten.Image, img *ebiten.Image, sx, sy, footprint, tilePixels float64) {
	drawFootprintTintedAtScale(screen, img, sx, sy, footprint, tilePixels, color.White)
}

// drawFootprintTintedAtScale is the footprint counterpart to
// drawStandingTintedAtScale. Construction art uses it for a future per-kind
// site sheet while retaining normal alpha compositing for transparent timber
// frames and ground markers.
func drawFootprintTintedAtScale(screen *ebiten.Image, img *ebiten.Image, sx, sy, footprint, tilePixels float64, clr color.Color) {
	if img == nil {
		return
	}
	b := img.Bounds()
	size := footprint * tilePixels
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(size/float64(b.Dx()), size/float64(b.Dy()))
	op.GeoM.Translate(sx, sy)
	op.ColorScale.ScaleWithColor(clr)
	op.Blend = ebiten.BlendSourceOver
	screen.DrawImage(img, op)
}

// drawVisualLayerAtScale places one layer from assets.BuildingVisual. The
// simulation owns the footprint; the art only decides whether it spans that
// footprint or rises from a particular tile pivot. This is the bridge from the
// current 64px sprites to future KaM-style, arbitrary-canvas art packs.
func drawVisualLayerAtScale(screen *ebiten.Image, layer assets.VisualLayer, sx, sy, footprint, tilePixels float64) {
	if layer.Image == nil {
		return
	}
	tint := layer.Tint
	if tint.A == 0 {
		tint = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	anchorX := sx + layer.AnchorX*tilePixels
	anchorY := sy + layer.AnchorY*tilePixels
	switch layer.Mode {
	case assets.LayerFootprint:
		drawFootprintTintedAtScale(screen, layer.Image, anchorX, anchorY, footprint, tilePixels, tint)
	default:
		tilesTall := layer.TilesTall
		if tilesTall <= 0 {
			tilesTall = 1
		}
		drawStandingTintedAtScale(screen, layer.Image, anchorX, anchorY, tilesTall, tilePixels, tint)
	}
}

// unitBob returns a four-step one-pixel gait offset. It is intentionally
// subtle: at 24 pixels per tile, a small vertical weight shift reads better
// than large sprite jumps until directional walk frames are available.
func unitBob() float64 {
	switch (animFrame / 6) % 4 {
	case 1:
		return -1
	case 3:
		return 1
	default:
		return 0
	}
}

// walkingFrame returns a neutral middle pose for a stationary unit and moves
// through the three atlas poses only while the unit actually has a route.
// offset de-synchronizes neighbours so a row of workers does not step as one.
func walkingFrame(path []pathfind.Point, offset int) int {
	const walkFrames = 3
	if len(path) == 0 {
		return 1
	}
	return (animFrame/7 + offset) % walkFrames
}

// drawStandingTinted is drawStanding with a color multiplied over the
// sprite -- used to sweep a Farm's fertile tiles from bare soil to
// golden wheat as its crop matures (see render/buildings.go).
func drawStandingTinted(screen *ebiten.Image, img *ebiten.Image, sx, sy, tilesTall float64, clr color.Color) {
	drawStandingScaled(screen, img, sx, sy, tilesTall, TileSize, clr)
}

func drawStandingTintedAtScale(screen *ebiten.Image, img *ebiten.Image, sx, sy, tilesTall, tilePixels float64, clr color.Color) {
	drawStandingScaled(screen, img, sx, sy, tilesTall, tilePixels, clr)
}

func drawStandingScaled(screen *ebiten.Image, img *ebiten.Image, sx, sy, tilesTall, tilePixels float64, clr color.Color) {
	drawStandingFacingScaled(screen, img, sx, sy, tilesTall, tilePixels, clr, false)
}

// facingLeft reports whether a walking unit's sprite should be
// horizontally mirrored to face its direction of travel, derived from
// its own current tile vs. the next tile on its route -- so it needs no
// new field on any of the seven profession structs (Serf, Villager,
// Lumberjack, Fisherman, Quarryman, Builder, Miner all already expose
// RemainingPath, added for the route-line overlay, and this reuses it).
// The source art's own default facing is treated as "right"; a unit with
// no path right now (idle, or working at its post rather than walking)
// keeps that default rather than flipping arbitrarily -- there's no
// direction of travel to face.
func facingLeft(x int, path []pathfind.Point) bool {
	if len(path) == 0 {
		return false
	}
	return path[0].X < x
}

// drawStandingFacingTintedAtScale is drawStandingTintedAtScale with an
// optional horizontal mirror -- see facingLeft.
func drawStandingFacingTintedAtScale(screen *ebiten.Image, img *ebiten.Image, sx, sy, tilesTall, tilePixels float64, clr color.Color, flip bool) {
	drawStandingFacingScaled(screen, img, sx, sy, tilesTall, tilePixels, clr, flip)
}

// visualBuildingHeight is the single source for visual-only building size. It
// deliberately differs from building.Types[kind].Footprint: changing art must
// never move an existing building, invalidate a road or corrupt a save file.
func visualBuildingHeight(kind building.Kind) float64 {
	if visual, ok := assets.BuildingVisualFor(kind); ok && visual.Body.TilesTall > 0 {
		return visual.Body.TilesTall
	}
	return buildingHeight
}

func drawStandingFacingScaled(screen *ebiten.Image, img *ebiten.Image, sx, sy, tilesTall, tilePixels float64, clr color.Color, flip bool) {
	b := img.Bounds()
	native := float64(b.Dy())
	scale := (tilesTall * tilePixels) / native
	drawnW := float64(b.Dx()) * scale
	drawnH := native * scale

	op := &ebiten.DrawImageOptions{}
	if flip {
		// A negative X scale mirrors the image around its own local
		// origin, which extends it leftward instead of rightward -- the
		// translate below has to land on the bounding box's *right*
		// edge instead of its left for the final on-screen position to
		// match the unflipped case exactly, just mirrored in place.
		op.GeoM.Scale(-scale, scale)
		op.GeoM.Translate(sx+tilePixels/2+drawnW/2, sy+tilePixels-drawnH)
	} else {
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(sx+tilePixels/2-drawnW/2, sy+tilePixels-drawnH)
	}
	op.ColorScale.ScaleWithColor(clr)
	// The zero-value CompositeMode is CompositeModeCustom (not
	// SourceOver!) with a zero-value Blend, which does NOT behave like
	// normal alpha compositing -- layering a second sprite (e.g. the
	// mill's blades over its base) would blank out the first one's
	// pixels wherever the second is transparent, instead of blending.
	// Setting this explicitly is required for any layered sprite draw.
	op.Blend = ebiten.BlendSourceOver
	screen.DrawImage(img, op)
}
