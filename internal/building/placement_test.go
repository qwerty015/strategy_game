package building

import (
	"testing"

	"strategy_game/internal/world"
)

func testGrid() *world.Grid {
	g := world.NewGrid(10, 10)
	g.Set(2, 2, world.Tile{Terrain: world.Fertile})
	g.Set(3, 2, world.Tile{Terrain: world.Fertile})
	g.Set(2, 3, world.Tile{Terrain: world.Fertile})
	g.Set(3, 3, world.Tile{Terrain: world.Fertile})
	g.Set(8, 8, world.Tile{Terrain: world.Water})
	return g
}

func TestCanPlace(t *testing.T) {
	tests := []struct {
		name      string
		kind      Kind
		x, y      int
		existing  []*Building
		wantValid bool
	}{
		{"farm on fertile land", Farm, 2, 2, nil, true},
		{"farm on plain grass", Farm, 0, 0, nil, false}, // Farm requires Fertile
		{"mill on plain grass", Mill, 0, 0, nil, true},  // Mill has no terrain restriction
		{"out of bounds", Mill, 10, 10, nil, false},     // off the 10x10 grid entirely
		{"on water", Mill, 8, 8, nil, false},
		{
			name: "overlaps existing building",
			kind: Mill, x: 0, y: 0,
			existing:  []*Building{{Kind: Mill, X: 0, Y: 0}},
			wantValid: false,
		},
		{
			name: "adjacent, no overlap",
			kind: Mill, x: 1, y: 0,
			existing:  []*Building{{Kind: Mill, X: 0, Y: 0}},
			wantValid: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CanPlace(testGrid(), tc.existing, tc.kind, tc.x, tc.y)
			if got != tc.wantValid {
				t.Errorf("CanPlace(%v, x=%d, y=%d) = %v, want %v", tc.kind, tc.x, tc.y, got, tc.wantValid)
			}
		})
	}
}
