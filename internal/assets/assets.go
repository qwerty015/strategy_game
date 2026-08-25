// Package assets embeds the game's placeholder art and decodes it into
// ready-to-draw *ebiten.Image values once at startup. Source: Kenney's
// "Medieval RTS" pack (CC0, see assets/CREDITS.md) -- picked per the
// project's stated approach of using free asset packs rather than
// custom-drawn or commissioned art (see AGENTS.md). Every sprite in this
// pack shares one convention: a uniform 64x64 canvas with the art
// positioned consistently within it, which is what lets multi-part
// sprites (e.g. the mill's base + rotating blades, see MillFrames) be
// composited by simply overlaying them with no offset math.
package assets

import (
	"bytes"
	"embed"
	"image"
	"image/draw"
	"image/png"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed tiles/*.png units/*.png
var files embed.FS

// TileSize is the pixel width/height of every source sprite's canvas in
// this package, before the render package scales it up to the screen's
// on-screen tile size.
const TileSize = 64

var (
	Grass   = mustLoad("tiles/grass.png")
	Fertile = mustLoad("tiles/fertile.png") // tilled farmland
	Forest  = mustLoad("tiles/forest.png")  // grass + trees, one combined tile
	Stone   = mustLoad("tiles/stone.png")
	Water   = mustLoad("tiles/water.png")
	Road    = mustLoad("tiles/road.png")

	// MillFrames are the windmill's sail-rotation animation frames, each
	// the base tower with one blade-angle sprite pre-composited on top
	// at load time (see mustComposite) rather than drawn as two separate
	// ebiten.DrawImage calls every frame -- cheaper, and it sidesteps
	// the blade sprite's baked-in drop shadow washing out the base
	// underneath it (see mustComposite's doc comment).
	MillFrames = [3]*ebiten.Image{
		mustComposite("tiles/mill_base.png", "tiles/mill_blades1.png"),
		mustComposite("tiles/mill_base.png", "tiles/mill_blades2.png"),
		mustComposite("tiles/mill_base.png", "tiles/mill_blades3.png"),
	}

	Bakery    = mustLoad("tiles/bakery.png")
	Warehouse = mustLoad("tiles/warehouse.png")

	// Serf holds walk-pose frames for the serf sprite -- not a real
	// walk cycle (the source pack has directional poses, not leg
	// animation), but cycling between them while a serf is moving
	// reads fine as motion at this sprite size.
	Serf = [3]*ebiten.Image{
		mustLoad("units/serf_a.png"),
		mustLoad("units/serf_b.png"),
		mustLoad("units/serf_c.png"),
	}
)

func mustDecode(name string) image.Image {
	data, err := files.ReadFile(name)
	if err != nil {
		panic(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return img
}

func mustLoad(name string) *ebiten.Image {
	return ebiten.NewImageFromImage(mustDecode(name))
}

// mustComposite flattens overlay onto base (both same-sized canvases) on
// the CPU and returns the result as one ebiten.Image. It intentionally
// does not use ordinary alpha-over compositing -- see the comment
// inside -- because these sprites carry their own soft ground shadow
// that a plain alpha blend would smear across whatever they're layered
// onto instead of just onto flat ground as intended.
func mustComposite(base, overlay string) *ebiten.Image {
	baseImg := mustDecode(base)
	overlayImg := mustDecode(overlay)

	out := image.NewNRGBA(baseImg.Bounds())
	draw.Draw(out, out.Bounds(), baseImg, image.Point{}, draw.Src)

	// The overlay sprite carries its own soft drop-shadow baked in as
	// low-alpha pixels across most of its canvas (it's meant to be
	// placed directly on flat ground, not stacked on another sprite).
	// A normal alpha-over composite would let that shadow wash out the
	// base underneath, so only copy overlay pixels that are solidly
	// opaque -- a clean silhouette instead of a faded one.
	const opaqueThreshold = 0x8000 // ~50% alpha, 16-bit color.Color scale
	b := out.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := overlayImg.At(x, y)
			if _, _, _, a := c.RGBA(); a > opaqueThreshold {
				out.Set(x, y, c)
			}
		}
	}

	return ebiten.NewImageFromImage(out)
}
