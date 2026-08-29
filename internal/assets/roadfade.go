package assets

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// RoadFadeN/E/S/W are precomputed correction overlays that cancel
// terrain_road_stone.png's own dark vignette border along one edge, so two
// adjacent Road tiles read as one continuous stone path instead of two
// separately framed slabs -- see render.DrawBuildings' road-connector
// pass, which draws whichever of these lines up with a connected
// neighbour on top of the ordinary Road tile using an additive
// (ebiten.BlendLighter) blend.
//
// The falloff below was fit to the texture's actual measured
// edge-to-interior brightness profile (sampled once during development,
// not eyeballed): luminance rises from about 61 right at the edge to a
// roughly flat ~105 by 14-16px into the 64px tile. A light warm-gray laid
// on with exponentially decaying alpha (steepest right at the edge)
// approximately cancels that gap back to the interior tone while leaving
// the cobblestone texture underneath visible, instead of stamping a flat
// patch over it.
var (
	RoadFadeN = buildRoadFadeH(false)
	RoadFadeS = buildRoadFadeH(true)
	RoadFadeW = buildRoadFadeV(false)
	RoadFadeE = buildRoadFadeV(true)
)

const (
	// roadFadeDepth is how many of the 64 source pixels the correction
	// reaches into the tile -- past this the measured vignette is already
	// within noise of the flat interior tone, so correcting any further
	// would just add a visible tint over normal cobblestone.
	roadFadeDepth = 16
	// roadFadeMaxAlpha/roadFadeTau shape the exponential decay
	// alpha(depth) = roadFadeMaxAlpha * exp(-depth/roadFadeTau): the
	// coefficients are fit so alpha(0) roughly cancels the ~44-luminance
	// gap measured right at the edge (given roadFadeColor's own
	// luminance around 145) and the curve decays to near-zero by
	// roadFadeDepth, matching how quickly the real vignette fades.
	roadFadeMaxAlpha = 0.42
	roadFadeTau      = 5.5
)

// roadFadeColor approximates the Road texture's own interior average tone
// (measured ~115,113,109, brightened a little since this is an additive
// correction, not a replacement) -- it only needs to read as a plausible
// stone-gray, not match the source pixel-for-pixel.
var roadFadeColor = color.NRGBA{R: 150, G: 145, B: 135, A: 255}

func roadFadeAlpha(depth int) uint8 {
	a := roadFadeMaxAlpha * math.Exp(-float64(depth)/roadFadeTau)
	if a > 1 {
		a = 1
	}
	return uint8(a * 255)
}

// buildRoadFadeH builds the top-edge correction band (TileSize wide x
// roadFadeDepth tall), strongest on row 0. reverse builds the
// bottom-edge version instead (strongest on the last row), for South.
func buildRoadFadeH(reverse bool) *ebiten.Image {
	out := image.NewNRGBA(image.Rect(0, 0, TileSize, roadFadeDepth))
	for y := range roadFadeDepth {
		depth := y
		if reverse {
			depth = roadFadeDepth - 1 - y
		}
		c := roadFadeColor
		c.A = roadFadeAlpha(depth)
		for x := range TileSize {
			out.SetNRGBA(x, y, c)
		}
	}
	return ebiten.NewImageFromImage(out)
}

// buildRoadFadeV is buildRoadFadeH transposed: a left-edge correction band
// (roadFadeDepth wide x TileSize tall), strongest on column 0. reverse
// builds the right-edge version instead, for East.
func buildRoadFadeV(reverse bool) *ebiten.Image {
	out := image.NewNRGBA(image.Rect(0, 0, roadFadeDepth, TileSize))
	for x := range roadFadeDepth {
		depth := x
		if reverse {
			depth = roadFadeDepth - 1 - x
		}
		c := roadFadeColor
		c.A = roadFadeAlpha(depth)
		for y := range TileSize {
			out.SetNRGBA(x, y, c)
		}
	}
	return ebiten.NewImageFromImage(out)
}
