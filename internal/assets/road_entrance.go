package assets

import (
	"image"
	"image/draw"

	"github.com/hajimehoshi/ebiten/v2"
)

// roadEntranceCornerRadius is intentionally pronounced: the entrance keeps a
// broad cobblestone centre while exposed outer corners yield visibly to grass.
const roadEntranceCornerRadius = 22

// RoadEntranceVariant returns the original cobblestone tile with only the
// requested exposed corners softened. Bits are NW=1, NE=2, SW=4, SE=8. The
// renderer selects a variant only for a building's single entrance road.
func RoadEntranceVariant(exposedCorners uint8) *ebiten.Image {
	return RoadEntranceVariants[exposedCorners&0x0f]
}

// RoadEntranceVariants are generated once from the existing authored road
// sprite. A corner becomes transparent only when it borders open grass; there
// are no new road colours or bright seam overlays.
var RoadEntranceVariants = mustLoadRoadEntranceVariants("generated/terrain_road_stone.png")

func mustLoadRoadEntranceVariants(name string) [16]*ebiten.Image {
	source := normalizeSprite(mustDecode(name))
	var variants [16]*ebiten.Image
	for mask := range variants {
		out := image.NewNRGBA(image.Rect(0, 0, TileSize, TileSize))
		draw.Draw(out, out.Bounds(), source, source.Bounds().Min, draw.Src)
		for y := 0; y < TileSize; y++ {
			for x := 0; x < TileSize; x++ {
				alpha := roundedRoadCornerAlpha(uint8(mask), x, y)
				if alpha == 255 {
					continue
				}
				pixel := out.NRGBAAt(x, y)
				pixel.A = uint8(uint16(pixel.A) * uint16(alpha) / 255)
				out.SetNRGBA(x, y, pixel)
			}
		}
		variants[mask] = ebiten.NewImageFromImage(out)
	}
	return variants
}

func roundedRoadCornerAlpha(exposedCorners uint8, x, y int) uint8 {
	const (
		northWest uint8 = 1 << iota
		northEast
		southWest
		southEast
	)
	r := roadEntranceCornerRadius
	corner := func(dx, dy int) uint8 {
		distanceSq := dx*dx + dy*dy
		if distanceSq > r*r {
			return 0
		}
		if distanceSq > (r-1)*(r-1) {
			return 128
		}
		return 255
	}
	switch {
	case x < r && y < r && exposedCorners&northWest != 0:
		return corner(r-1-x, r-1-y)
	case x >= TileSize-r && y < r && exposedCorners&northEast != 0:
		return corner(x-(TileSize-r), r-1-y)
	case x < r && y >= TileSize-r && exposedCorners&southWest != 0:
		return corner(r-1-x, y-(TileSize-r))
	case x >= TileSize-r && y >= TileSize-r && exposedCorners&southEast != 0:
		return corner(x-(TileSize-r), y-(TileSize-r))
	default:
		return 255
	}
}
