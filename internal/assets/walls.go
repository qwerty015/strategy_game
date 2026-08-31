package assets

// Wall and gate frames are deliberately external PNGs beside the executable,
// like the rest of the art pack. Horizontal/vertical variants are distinct
// source sprites instead of runtime rotations so stone highlights and gate
// hinges remain intentional in both directions.
var (
	StoneWallHorizontal = mustLoad("buildings/wall/horizontal.png")
	StoneWallVertical   = mustLoad("buildings/wall/vertical.png")

	GateHorizontalClosed = mustLoad("buildings/gate/horizontal_closed.png")
	GateVerticalClosed   = mustLoad("buildings/gate/vertical_closed.png")
	GateHorizontalOpen   = mustLoad("buildings/gate/horizontal_open.png")
	GateVerticalOpen     = mustLoad("buildings/gate/vertical_open.png")
)
