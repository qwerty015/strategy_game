package assets

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// BuildingShadow is a small precomputed soft-edged ellipse used by
// render.DrawBuildings to ground every standing building sprite. The
// source art is a mix of different generation batches (see CREDITS.md):
// some already bake in their own soft ground shadow (Warehouse, Bakery),
// most don't (Smeltery, LumberjackHut, CarpentryWorkshop, ...) -- relying
// on the source art alone left buildings looking inconsistently grounded,
// floating for some kinds and not others depending on which batch they
// happened to come from. This one procedural shadow, drawn under every
// standing building the same way, makes that consistent regardless of
// what the source art includes.
var BuildingShadow = buildBuildingShadow()

const (
	shadowWidth  = 64 // matches TileSize, so the usual tilePixels/TileSize scale applies directly
	shadowHeight = 26
)

func buildBuildingShadow() *ebiten.Image {
	out := image.NewNRGBA(image.Rect(0, 0, shadowWidth, shadowHeight))
	cx, cy := float64(shadowWidth)/2, float64(shadowHeight)/2
	for y := range shadowHeight {
		for x := range shadowWidth {
			dx := (float64(x) + 0.5 - cx) / cx
			dy := (float64(y) + 0.5 - cy) / cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist >= 1 {
				continue
			}
			// Solid-ish near the centre, fading to nothing at the
			// ellipse's edge -- a hard-edged ellipse would look like a
			// painted puddle instead of a soft contact shadow.
			alpha := (1 - dist) * (1 - dist) * 130
			out.SetNRGBA(x, y, color.NRGBA{R: 12, G: 9, B: 7, A: uint8(alpha)})
		}
	}
	return ebiten.NewImageFromImage(out)
}
