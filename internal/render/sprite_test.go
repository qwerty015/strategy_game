package render

import (
	"testing"

	"strategy_game/internal/pathfind"
)

// TestFacingLeft covers the sprite-mirroring rule used across all seven
// walking unit types (Serf, Villager, Lumberjack, Fisherman, Quarryman,
// Builder, Miner): flip only when the next tile on the route is strictly
// to the left, keep the source art's own default facing (right) for a
// unit with no path at all, and don't flip for a purely vertical step.
func TestFacingLeft(t *testing.T) {
	cases := []struct {
		name     string
		x        int
		path     []pathfind.Point
		wantFlip bool
	}{
		{"no path (idle)", 5, nil, false},
		{"next tile to the left", 5, []pathfind.Point{{X: 4, Y: 5}}, true},
		{"next tile to the right", 5, []pathfind.Point{{X: 6, Y: 5}}, false},
		{"next tile straight up (same X)", 5, []pathfind.Point{{X: 5, Y: 4}}, false},
		{"next tile straight down (same X)", 5, []pathfind.Point{{X: 5, Y: 6}}, false},
		{"only the first path point matters", 5, []pathfind.Point{{X: 6, Y: 5}, {X: 1, Y: 5}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := facingLeft(c.x, c.path); got != c.wantFlip {
				t.Errorf("facingLeft(%d,%v) = %v, want %v", c.x, c.path, got, c.wantFlip)
			}
		})
	}
}
