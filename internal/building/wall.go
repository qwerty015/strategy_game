package building

import "strategy_game/internal/resource"

// WallAxis describes the axis a one-cell wall or gate spans. It is saved on a
// gate because a later missing neighbour must not rotate its art or collision.
type WallAxis uint8

const (
	WallHorizontal WallAxis = iota
	WallVertical
)

// WallShape describes the visible connection pattern of a completed wall
// piece. Collision remains per-cell; this only selects the correct artwork.
type WallShape uint8

const (
	WallShapeIsolated WallShape = iota
	WallShapeHorizontal
	WallShapeVertical
	WallShapeCornerNE
	WallShapeCornerNW
	WallShapeCornerSE
	WallShapeCornerSW
	WallShapeTNorth
	WallShapeTSouth
	WallShapeTEast
	WallShapeTWest
	WallShapeCross
)

// IsWallKind reports whether kind is a player-built piece of the town wall.
func IsWallKind(kind Kind) bool {
	return kind == StoneWall || kind == Gate
}

// IsFinishedWallSegment reports whether b currently forms part of a completed
// wall. A gate is still a wall segment while closed; an automatic gate remains
// a segment for rendering even though pathfinding may plan through it.
func IsFinishedWallSegment(b *Building) bool {
	return b != nil && IsWallKind(b.Kind) && b.ConstructionStage == ConstructionNone
}

// GatePassable reports whether a finished gate can be used as a land or road
// tile. Auto gates are routeable while visually closed so an ally can approach
// and trigger their opening; manual closed gates are true barriers.
//
// This has no concept of who is walking through -- every existing caller
// (serfs, villagers, lumberjacks, fishermen, quarrymen, builders, miners,
// sentries) only ever pathfinds across its own faction's own buildings plus
// shared-neutral ones (Road, natural resources -- see
// Game.ownedBuildingsWithRoads), so the gates it can even encounter are
// always its own faction's. See GatePassableTo for the one caller (soldier
// movement) whose obstacle list can legitimately contain another faction's
// gate.
func GatePassable(b *Building) bool {
	return b != nil && b.Kind == Gate && b.ConstructionStage == ConstructionNone && (b.GateOpen || b.GateAuto)
}

// GatePassableTo is GatePassable restricted to a specific owner -- a real bug
// found from an actual playtest report ("юниты противника спокойно проходят
// через мои ворота"): GatePassable alone lets ANY unit through an Open/Auto
// gate regardless of whose it is, which in the FFA duel mode means a hostile
// AI faction's soldiers can simply walk through the player's walls the
// instant a single gate anywhere in them is left on Auto (its default state
// right after building one -- see cmd/game's gate-placement flow). A gate's
// own owner still passes exactly as GatePassable always allowed (Open or
// Auto); every other owner is blocked outright, the same as a plain
// StoneWall segment -- GateOpen/GateAuto only ever meant "convenient for my
// own traffic", never "anyone may enter".
func GatePassableTo(b *Building, owner int) bool {
	return b != nil && b.Owner == owner && GatePassable(b)
}

// FinishedWallSegments indexes completed walls and gates once. Renderers use
// this instead of scanning every building for every wall tile, which keeps a
// large fortress cheap to draw.
func FinishedWallSegments(existing []*Building) map[Point]*Building {
	segments := make(map[Point]*Building)
	for _, b := range existing {
		if IsFinishedWallSegment(b) {
			segments[Point{X: b.X, Y: b.Y}] = b
		}
	}
	return segments
}

// WallAxisAt finds the straight axis of a completed wall segment. Gates can
// only replace a segment with neighbours on both opposite sides, never a cap
// or a corner; that makes their orientation and placement unambiguous.
func WallAxisAt(existing []*Building, x, y int) (WallAxis, bool) {
	return wallAxisFromSegments(FinishedWallSegments(existing), x, y)
}

func wallAxisFromSegments(segments map[Point]*Building, x, y int) (WallAxis, bool) {
	_, west := segments[Point{X: x - 1, Y: y}]
	_, east := segments[Point{X: x + 1, Y: y}]
	_, north := segments[Point{X: x, Y: y - 1}]
	_, south := segments[Point{X: x, Y: y + 1}]
	horizontal := west && east
	vertical := north && south
	switch {
	case horizontal && !vertical:
		return WallHorizontal, true
	case vertical && !horizontal:
		return WallVertical, true
	default:
		return WallHorizontal, false
	}
}

// WallShapeAt is the convenient one-off version used by tests and tools.
func WallShapeAt(existing []*Building, x, y int) WallShape {
	return WallShapeFromSegments(FinishedWallSegments(existing), x, y)
}

// CanCreateWallTopology reports whether adding the proposed wall cells keeps
// every touched section as either an end, a straight run or a 90-degree turn.
// A section with three or four cardinal neighbours would require a T/cross
// module, which the game intentionally does not support.
func CanCreateWallTopology(existing []*Building, proposed []Point) bool {
	segments := make(map[Point]bool)
	for _, b := range existing {
		if b != nil && IsWallKind(b.Kind) {
			segments[Point{X: b.X, Y: b.Y}] = true
		}
	}
	touched := make(map[Point]bool)
	for _, point := range proposed {
		segments[point] = true
		touched[point] = true
		for _, neighbour := range [...]Point{
			{X: point.X, Y: point.Y - 1},
			{X: point.X + 1, Y: point.Y},
			{X: point.X, Y: point.Y + 1},
			{X: point.X - 1, Y: point.Y},
		} {
			touched[neighbour] = true
		}
	}
	for point := range touched {
		if !segments[point] {
			continue
		}
		neighbours := 0
		for _, neighbour := range [...]Point{
			{X: point.X, Y: point.Y - 1},
			{X: point.X + 1, Y: point.Y},
			{X: point.X, Y: point.Y + 1},
			{X: point.X - 1, Y: point.Y},
		} {
			if segments[neighbour] {
				neighbours++
			}
		}
		if neighbours > 2 {
			return false
		}
	}
	return true
}

// WallShapeFromSegments selects the exact cardinal connection topology from
// an already-indexed wall set. Diagonal neighbours intentionally do not affect
// the shape: walls are orthogonal structures, and diagonal crossing remains
// disallowed by pathfinding.
func WallShapeFromSegments(segments map[Point]*Building, x, y int) WallShape {
	_, north := segments[Point{X: x, Y: y - 1}]
	_, east := segments[Point{X: x + 1, Y: y}]
	_, south := segments[Point{X: x, Y: y + 1}]
	_, west := segments[Point{X: x - 1, Y: y}]
	mask := uint8(0)
	if north {
		mask |= 1
	}
	if east {
		mask |= 2
	}
	if south {
		mask |= 4
	}
	if west {
		mask |= 8
	}
	switch mask {
	case 0:
		return WallShapeIsolated
	case 1, 4, 5:
		return WallShapeVertical
	case 2, 8, 10:
		return WallShapeHorizontal
	case 3:
		return WallShapeCornerNE
	case 9:
		return WallShapeCornerNW
	case 6:
		return WallShapeCornerSE
	case 12:
		return WallShapeCornerSW
	case 11:
		return WallShapeTNorth
	case 14:
		return WallShapeTSouth
	case 7:
		return WallShapeTEast
	case 13:
		return WallShapeTWest
	default:
		return WallShapeCross
	}
}

// WallRenderAxis preserves the former small API for callers that only need an
// orientation. New rendering should use WallShapeFromSegments for corners.
func WallRenderAxis(existing []*Building, x, y int) WallAxis {
	shape := WallShapeAt(existing, x, y)
	if shape == WallShapeVertical || shape == WallShapeCornerNE || shape == WallShapeCornerNW ||
		shape == WallShapeCornerSE || shape == WallShapeCornerSW || shape == WallShapeTEast || shape == WallShapeTWest {
		return WallVertical
	}
	return WallHorizontal
}

// ConstructionMaterialTypes is the stable common material order used by
// reservations, direct delivery, inspectors and advisor messages.
func ConstructionMaterialTypes() [3]resource.Type {
	return [3]resource.Type{resource.Plank, resource.StoneBlock, resource.Iron}
}
