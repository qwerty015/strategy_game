package building

import "strategy_game/internal/resource"

// WallAxis describes the axis a one-cell wall or gate spans. It is saved on a
// gate because a later missing neighbour must not rotate its art or collision.
type WallAxis uint8

const (
	WallHorizontal WallAxis = iota
	WallVertical
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
func GatePassable(b *Building) bool {
	return b != nil && b.Kind == Gate && b.ConstructionStage == ConstructionNone && (b.GateOpen || b.GateAuto)
}

// WallAxisAt finds the straight axis of a completed wall segment. Gates can
// only replace a segment with neighbours on both opposite sides, never a cap
// or a corner; that makes their orientation and placement unambiguous.
func WallAxisAt(existing []*Building, x, y int) (WallAxis, bool) {
	at := func(x, y int) bool {
		for _, b := range existing {
			if IsFinishedWallSegment(b) && b.X == x && b.Y == y {
				return true
			}
		}
		return false
	}
	horizontal := at(x-1, y) && at(x+1, y)
	vertical := at(x, y-1) && at(x, y+1)
	switch {
	case horizontal && !vertical:
		return WallHorizontal, true
	case vertical && !horizontal:
		return WallVertical, true
	default:
		return WallHorizontal, false
	}
}

// WallRenderAxis selects a stable visual for any completed wall piece. A
// straight line gets its matching frame; corners and isolated segments fall
// back to the horizontal silhouette until dedicated corner frames are added.
func WallRenderAxis(existing []*Building, x, y int) WallAxis {
	at := func(x, y int) bool {
		for _, b := range existing {
			if IsFinishedWallSegment(b) && b.X == x && b.Y == y {
				return true
			}
		}
		return false
	}
	if at(x, y-1) || at(x, y+1) {
		return WallVertical
	}
	return WallHorizontal
}

// ConstructionMaterialTypes is the stable common material order used by
// reservations, direct delivery, inspectors and advisor messages.
func ConstructionMaterialTypes() [3]resource.Type {
	return [3]resource.Type{resource.Plank, resource.StoneBlock, resource.Iron}
}
