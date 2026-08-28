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
	"image/color"
	"image/draw"
	"image/png"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed tiles/*.png units/*.png generated/*.png
var files embed.FS

// TileSize is the pixel width/height of every source sprite's canvas in
// this package, before the render package scales it up to the screen's
// on-screen tile size.
const TileSize = 64

var (
	Grass   = mustLoadGround("generated/terrain_grass.png")
	Fertile = mustLoadGround("generated/terrain_fertile.png") // tilled farmland
	Forest  = mustLoadGround("generated/terrain_forest.png")  // grass + trees, one tile
	Stone   = mustLoadGround("generated/terrain_stone.png")
	// StoneDeposit is the mineable boulder cluster placed on top of stone terrain.
	StoneDeposit = mustLoad("generated/terrain_stone_deposit.png")
	// The water export has the same bright top-edge fringe as several ground
	// tiles. Load it through the one-time repair pass so zoomed ponds stay
	// seamless instead of gaining horizontal white stripes.
	Water = mustLoadGround("generated/terrain_water.png")
	Road  = mustLoad("generated/terrain_road_stone.png") // cobblestone path

	// MillFrames are flattened once on the CPU from the mill body and three
	// blade positions. The base is the older, more detailed mill sprite; its
	// baked-in sail pixels are masked before the new animated sail layer is
	// attached.
	MillFrames = [3]*ebiten.Image{
		mustCompositeLegacyMill("tiles/mill_blades1.png"),
		mustCompositeLegacyMill("tiles/mill_blades2.png"),
		mustCompositeLegacyMill("tiles/mill_blades3.png"),
	}

	Bakery            = mustLoad("generated/building_bakery.png")
	Warehouse         = mustLoad("generated/building_warehouse.png")
	FarmHouse         = mustLoad("generated/building_farm.png") // stands on one corner of the Farm's field
	Tavern            = mustLoad("generated/building_tavern.png")
	LumberjackHut     = mustLoad("generated/building_lumberjack_hut.png")
	Winery            = mustLoad("generated/building_winery.png")
	PigFarm           = mustLoad("generated/building_pig_farm.png")
	MeatWorkshop      = mustLoad("generated/building_meat_workshop.png")
	CarpentryWorkshop = mustLoad("generated/building_carpentry_workshop.png")
	QuarryHut         = mustLoad("generated/building_quarry_hut.png")
	// Construction art is deliberately generic: the same site can scale from
	// a one-tile road to a 3×3 farm without previewing the finished building.
	ConstructionFoundation  = mustLoad("generated/construction_foundation.png")
	ConstructionScaffolding = mustLoad("generated/construction_scaffolding.png")
	// The source fishing hut's pier points south. CPU-rotated variants let the
	// renderer orient it toward whichever cardinal water tile borders the hut.
	FisherHutFrames = [4]*ebiten.Image{
		mustLoad("generated/building_fisher_hut.png"),
		mustLoadRotated("generated/building_fisher_hut.png", 1),
		mustLoadRotated("generated/building_fisher_hut.png", 2),
		mustLoadRotated("generated/building_fisher_hut.png", 3),
	}

	// TreeFrames are the three growth stages from one transparent horizontal
	// sprite sheet. They are sliced once at startup and then drawn with nearest
	// neighbour scaling by the renderer.
	TreeFrames = [3]*ebiten.Image{
		mustLoadAtlasFrame("generated/tree_stages.png", 0),
		mustLoadAtlasFrame("generated/tree_stages.png", 1),
		mustLoadAtlasFrame("generated/tree_stages.png", 2),
	}

	// FishFrames contains the three transparent growth stages of a fish. A
	// sprite replaces the former procedural marker, keeping mature fish legible
	// while letting fry remain a quiet detail of the water surface.
	FishFrames = [3]*ebiten.Image{
		mustLoadAtlasFrame("generated/fish_stages.png", 0),
		mustLoadAtlasFrame("generated/fish_stages.png", 1),
		mustLoadAtlasFrame("generated/fish_stages.png", 2),
	}

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
	Lumberjack = [3]*ebiten.Image{
		mustLoad("generated/unit_lumberjack.png"),
		mustLoad("generated/unit_lumberjack.png"),
		mustLoad("generated/unit_lumberjack.png"),
	}
	Winemaker = [3]*ebiten.Image{
		mustLoad("generated/unit_winemaker.png"),
		mustLoad("generated/unit_winemaker.png"),
		mustLoad("generated/unit_winemaker.png"),
	}
	Fisherman = [3]*ebiten.Image{
		mustLoad("generated/unit_fisherman.png"),
		mustLoad("generated/unit_fisherman.png"),
		mustLoad("generated/unit_fisherman.png"),
	}
	Swineherd = [3]*ebiten.Image{
		mustLoad("generated/unit_swineherd.png"),
		mustLoad("generated/unit_swineherd.png"),
		mustLoad("generated/unit_swineherd.png"),
	}
	Butcher = [3]*ebiten.Image{
		mustLoad("generated/unit_butcher.png"),
		mustLoad("generated/unit_butcher.png"),
		mustLoad("generated/unit_butcher.png"),
	}
	Carpenter = [3]*ebiten.Image{
		mustLoad("generated/unit_carpenter.png"),
		mustLoad("generated/unit_carpenter.png"),
		mustLoad("generated/unit_carpenter.png"),
	}
	Quarryman = [3]*ebiten.Image{
		mustLoad("generated/unit_quarryman.png"),
		mustLoad("generated/unit_quarryman.png"),
		mustLoad("generated/unit_quarryman.png"),
	}
	Builder = [3]*ebiten.Image{
		mustLoad("generated/unit_builder.png"),
		mustLoad("generated/unit_builder.png"),
		mustLoad("generated/unit_builder.png"),
	}
	FishingBoat = mustLoad("generated/unit_fishing_boat.png")
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

// mustLoadRotated makes a nearest-neighbour 90-degree rotation once during
// startup. It keeps a waterside building's pier attached to its selected
// launch tile without adding per-frame image transformations.
func mustLoadRotated(name string, turns int) *ebiten.Image {
	src := mustDecode(name)
	turns %= 4
	if turns < 0 {
		turns += 4
	}
	if turns == 0 {
		return ebiten.NewImageFromImage(src)
	}
	b := src.Bounds()
	width, height := b.Dx(), b.Dy()
	if turns%2 == 0 {
		out := image.NewNRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				out.Set(width-1-x, height-1-y, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return ebiten.NewImageFromImage(out)
	}
	out := image.NewNRGBA(image.Rect(0, 0, height, width))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if turns == 1 {
				out.Set(height-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
			} else {
				out.Set(y, width-1-x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
	}
	return ebiten.NewImageFromImage(out)
}

func mustLoadAtlasFrame(name string, index int) *ebiten.Image {
	src := mustDecode(name)
	b := src.Bounds()
	const frameCount = 3
	if index < 0 || index >= frameCount || b.Dx()%frameCount != 0 {
		panic("assets: invalid three-frame sprite sheet")
	}
	frameWidth := b.Dx() / frameCount
	out := image.NewNRGBA(image.Rect(0, 0, frameWidth, b.Dy()))
	draw.Draw(out, out.Bounds(), src, image.Point{X: index * frameWidth, Y: 0}, draw.Src)
	return ebiten.NewImageFromImage(out)
}

// mustLoadGround repairs a one-pixel white export fringe present in a few of
// the generated terrain PNGs. It is fixed once at startup rather than hidden
// by a larger tile overlap, because the fringe otherwise becomes a bright
// grid line whenever the camera is zoomed.
func mustLoadGround(name string) *ebiten.Image {
	src := mustDecode(name)
	b := src.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, src, b.Min, draw.Src)
	if b.Dy() > 1 {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := out.At(x, b.Min.Y).RGBA()
			r, g, bl, _ := out.At(x, b.Min.Y).RGBA()
			if a > 0 && r > 0xe800 && g > 0xe800 && bl > 0xe800 {
				out.Set(x, b.Min.Y, out.At(x, b.Min.Y+1))
			}
		}
	}
	return ebiten.NewImageFromImage(out)
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
	return compositeImages(baseImg, overlayImg)
}

// mustCompositeScaled shrinks an oversized pixel-art overlay around the
// canvas centre before compositing it. The source mill sails nearly fill the
// 64x64 canvas; at the game's tall-building scale that reads as a giant X, so
// a smaller sail silhouette keeps the mill body and hub readable.
func mustCompositeScaled(base, overlay string, scale float64) *ebiten.Image {
	baseImg := mustDecode(base)
	overlayImg := mustDecode(overlay)
	return compositeImages(baseImg, scaleImageNearest(overlayImg, scale))
}

// mustCompositeLegacyMill keeps the detailed generated mill body that was
// used before the animation pass, but replaces its baked static sails with
// one of the separate rotating sail frames. The old sprite did not expose a
// clean body layer, so a narrow diagonal colour/shape mask removes only the
// wooden arms while leaving the roof, stone tower and door intact.
func mustCompositeLegacyMill(overlay string) *ebiten.Image {
	base := legacyMillBody()
	sails := scaleImageNearestAt(mustDecode(overlay), 0.55, 31, 24)
	return compositeImages(base, sails)
}

func legacyMillBody() image.Image {
	src := mustDecode("generated/building_mill.png")
	b := src.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, src, b.Min, draw.Src)

	const centerX, centerY = 31, 24
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := out.At(x, y).RGBA()
			if a < 0x8000 {
				continue
			}
			dx, dy := x-centerX, y-centerY
			diagonalDistance := minInt(absInt(dy-dx), absInt(dy+dx))
			distance := dx*dx + dy*dy
			if diagonalDistance <= 3 && distance > 70 && y < 43 && isWoodSail(r>>8, g>>8, bl>>8) {
				out.Set(x, y, color.Transparent)
			}
		}
	}
	return out
}

func scaleImageNearestAt(src image.Image, scale float64, centerX, centerY int) image.Image {
	b := src.Bounds()
	out := image.NewNRGBA(b)
	if scale <= 0 {
		return out
	}
	sourceCX := float64(b.Min.X+b.Max.X-1) / 2
	sourceCY := float64(b.Min.Y+b.Max.Y-1) / 2
	destCX, destCY := float64(centerX), float64(centerY)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			srcX := int(math.Round((float64(x)-destCX)/scale + sourceCX))
			srcY := int(math.Round((float64(y)-destCY)/scale + sourceCY))
			if srcX < b.Min.X || srcX >= b.Max.X || srcY < b.Min.Y || srcY >= b.Max.Y {
				continue
			}
			out.Set(x, y, src.At(srcX, srcY))
		}
	}
	return out
}

func isWoodSail(r, g, b uint32) bool {
	return r > 130 && g > 85 && b > 35 && r > g+15 && g > b+20
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func scaleImageNearest(src image.Image, scale float64) image.Image {
	b := src.Bounds()
	out := image.NewNRGBA(b)
	if scale <= 0 {
		return out
	}
	cx := float64(b.Min.X+b.Max.X-1) / 2
	cy := float64(b.Min.Y+b.Max.Y-1) / 2
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			srcX := int(math.Round((float64(x)-cx)/scale + cx))
			srcY := int(math.Round((float64(y)-cy)/scale + cy))
			if srcX < b.Min.X || srcX >= b.Max.X || srcY < b.Min.Y || srcY >= b.Max.Y {
				continue
			}
			out.Set(x, y, src.At(srcX, srcY))
		}
	}
	return out
}

func compositeImages(baseImg image.Image, overlayImg image.Image) *ebiten.Image {

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
