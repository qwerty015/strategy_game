package assets

import (
	"strategy_game/internal/building"

	"github.com/hajimehoshi/ebiten/v2"
)

// Wall and gate frames stay external beside the executable, like the rest of
// the art pack. Dedicated corner modules keep an enclosure visually continuous
// while the logical wall remains one independently removable map cell.
var (
	StoneWallHorizontal = mustLoad("buildings/wall/horizontal.png")
	StoneWallVertical   = mustLoad("buildings/wall/vertical.png")
	StoneWallCornerNE   = mustLoad("buildings/wall/corner_ne.png")
	StoneWallCornerNW   = mustLoad("buildings/wall/corner_nw.png")
	StoneWallCornerSE   = mustLoad("buildings/wall/corner_se.png")
	StoneWallCornerSW   = mustLoad("buildings/wall/corner_sw.png")
	StoneWallTee        = mustLoad("buildings/wall/tee.png")
	StoneWallCross      = mustLoad("buildings/wall/cross.png")
	StoneWallPillar     = mustLoad("buildings/wall/pillar.png")

	GateHorizontalClosed = mustLoad("buildings/gate/horizontal_closed.png")
	GateVerticalClosed   = mustLoad("buildings/gate/vertical_closed.png")
	GateHorizontalOpen   = mustLoad("buildings/gate/horizontal_open.png")
	GateVerticalOpen     = mustLoad("buildings/gate/vertical_open.png")
)

// StoneWallFrame selects the module matching cardinally adjacent completed
// wall/gate cells. End caps reuse the straight module, whose corner posts
// already close the exposed end; junctions use their dedicated frames.
func StoneWallFrame(shape building.WallShape) *ebiten.Image {
	switch shape {
	case building.WallShapeVertical:
		return StoneWallVertical
	case building.WallShapeCornerNE:
		return StoneWallCornerNE
	case building.WallShapeCornerNW:
		return StoneWallCornerNW
	case building.WallShapeCornerSE:
		return StoneWallCornerSE
	case building.WallShapeCornerSW:
		return StoneWallCornerSW
	case building.WallShapeTNorth, building.WallShapeTSouth, building.WallShapeTEast, building.WallShapeTWest:
		return StoneWallTee
	case building.WallShapeCross:
		return StoneWallCross
	case building.WallShapeIsolated:
		return StoneWallPillar
	default:
		return StoneWallHorizontal
	}
}
