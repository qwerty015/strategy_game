package render

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
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
	b := img.Bounds()
	native := float64(b.Dy())
	scale := (tilesTall * tilePixels) / native
	drawnW := float64(b.Dx()) * scale
	drawnH := native * scale

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(sx+tilePixels/2-drawnW/2, sy+tilePixels-drawnH)
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
