package assets

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// IdleCloud is a small precomputed cloud silhouette used by
// render.drawIdleBubble to mark a staffed building that isn't currently
// producing. It's rasterized once as a single flat-alpha shape (three
// overlapping circles merged by a plain union test, not three separate
// low-alpha DrawImage calls) specifically so the overlaps don't compound
// into a solid, more-opaque patch where the circles cross -- with one
// flat shape, the whole cloud can be faded uniformly by scaling its alpha
// once when it's drawn, and stays uniformly faint everywhere.
var IdleCloud = buildIdleCloud()

const idleCloudSize = 48

func buildIdleCloud() *ebiten.Image {
	out := image.NewNRGBA(image.Rect(0, 0, idleCloudSize, idleCloudSize))
	type circle struct{ cx, cy, r float64 }
	// Three lobes of a classic thought-cloud silhouette, sized and
	// offset within the idleCloudSize canvas by eye -- there's no "real"
	// measurement to fit here, unlike roadfade.go's vignette correction.
	circles := [...]circle{
		{24, 22, 15},
		{12, 29, 11},
		{36, 28, 12},
	}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for y := range idleCloudSize {
		for x := range idleCloudSize {
			for _, c := range circles {
				dx, dy := float64(x)-c.cx, float64(y)-c.cy
				if dx*dx+dy*dy <= c.r*c.r {
					out.SetNRGBA(x, y, white)
					break
				}
			}
		}
	}
	return ebiten.NewImageFromImage(out)
}
