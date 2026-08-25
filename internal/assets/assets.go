// Package assets embeds the game's terrain, building and unit art and
// decodes it into ready-to-draw *ebiten.Image values once at startup. The
// generated sprites are deliberately small 64x64 pixel-art assets, so they
// stay readable at the game's 24-pixel tile scale without turning this pet
// project into a large art pipeline. See assets/CREDITS.md for provenance.
package assets

import (
	"bytes"
	"embed"
	"image"
	"image/draw"
	"image/png"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed tiles/*.png units/*.png generated/*.png
var files embed.FS

// TileSize is the pixel width/height of every source sprite's canvas in
// this package, before the render package scales it up to the screen's
// on-screen tile size.
const TileSize = 64

var (
	Grass   = mustLoad("generated/terrain_grass.png")
	Fertile = mustLoad("generated/terrain_fertile.png") // tilled farmland
	Forest  = mustLoad("generated/terrain_forest.png")  // grass + trees, one tile
	Stone   = mustLoad("generated/terrain_stone.png")
	Water   = mustLoad("generated/terrain_water.png")
	Road    = mustLoad("generated/terrain_road_stone.png") // cobblestone path

	// MillFrames keeps the renderer's animation interface stable. The current
	// generated windmill is a finished sprite, so the three frames are
	// identical until a dedicated blade animation set is added.
	MillFrames = [3]*ebiten.Image{
		mustLoad("generated/building_mill.png"),
		mustLoad("generated/building_mill.png"),
		mustLoad("generated/building_mill.png"),
	}

	Bakery    = mustLoad("generated/building_bakery.png")
	Warehouse = mustLoad("generated/building_warehouse.png")
	FarmHouse = mustLoad("generated/building_farm.png") // stands on one corner of the Farm's field
	Tavern    = mustLoad("generated/building_tavern.png")

	// There is one purpose-built silhouette per profession for now. The
	// renderer still exposes frame arrays so directional/walking variants can
	// be added without changing the simulation packages.
	Serf = [3]*ebiten.Image{
		mustLoad("generated/unit_serf.png"),
		mustLoad("generated/unit_serf.png"),
		mustLoad("generated/unit_serf.png"),
	}
	Farmer = [3]*ebiten.Image{
		mustLoad("generated/unit_farmer.png"),
		mustLoad("generated/unit_farmer.png"),
		mustLoad("generated/unit_farmer.png"),
	}
	Baker = [3]*ebiten.Image{
		mustLoad("generated/unit_baker.png"),
		mustLoad("generated/unit_baker.png"),
		mustLoad("generated/unit_baker.png"),
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
