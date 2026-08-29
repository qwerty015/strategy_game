package assets

import (
	"image"
	"image/draw"

	"github.com/hajimehoshi/ebiten/v2"
)

// roadEntranceCornerRadius is deliberately small: an entrance remains a full
// cobblestone cell, with only its four outer corners yielding to the grass.
// This matches the soft threshold seen around buildings in classic RTS maps
// without turning the road into a narrow, artificial-looking pipe.
const roadEntranceCornerRadius = 14

func mustLoadRoundedRoadEntrance(name string) *ebiten.Image {
	source := normalizeSprite(mustDecode(name))
	out := image.NewNRGBA(image.Rect(0, 0, TileSize, TileSize))
	draw.Draw(out, out.Bounds(), source, source.Bounds().Min, draw.Src)
	for y := 0; y < TileSize; y++ {
		for x := 0; x < TileSize; x++ {
			alpha := roundedRoadCornerAlpha(x, y)
			if alpha == 255 {
				continue
			}
			pixel := out.NRGBAAt(x, y)
			pixel.A = uint8(uint16(pixel.A) * uint16(alpha) / 255)
			out.SetNRGBA(x, y, pixel)
		}
	}
	return ebiten.NewImageFromImage(out)
}

func roundedRoadCornerAlpha(x, y int) uint8 {
	r := roadEntranceCornerRadius
	corner := func(dx, dy int) uint8 {
		distanceSq := dx*dx + dy*dy
		if distanceSq > r*r {
			return 0
		}
		// One half-transparent pixel keeps the round edge smooth at high zoom,
		// while the rest stays intentionally crisp pixel art.
		if distanceSq > (r-1)*(r-1) {
			return 128
		}
		return 255
	}
	switch {
	case x < r && y < r:
		return corner(r-1-x, r-1-y)
	case x >= TileSize-r && y < r:
		return corner(x-(TileSize-r), r-1-y)
	case x < r && y >= TileSize-r:
		return corner(r-1-x, y-(TileSize-r))
	case x >= TileSize-r && y >= TileSize-r:
		return corner(x-(TileSize-r), y-(TileSize-r))
	default:
		return 255
	}
}
