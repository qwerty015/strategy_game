package building

import (
	"testing"

	"strategy_game/internal/world"
)

func testGrid() *world.Grid {
	g := world.NewGrid(10, 10)
	for y := 2; y < 5; y++ {
		for x := 2; x < 5; x++ {
			g.Set(x, y, world.Tile{Terrain: world.Fertile})
		}
	}
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
		{"farm on plain grass", Farm, 0, 0, nil, true}, // Farm can be placed on any non-water land
		{"mill on plain grass", Mill, 0, 0, nil, true}, // Mill has no terrain restriction
		{"out of bounds", Mill, 10, 10, nil, false},    // off the 10x10 grid entirely
		{"on water", Mill, 8, 8, nil, false},
		{"farm on water", Farm, 8, 8, nil, false},
		{
			name: "overlaps existing building",
			kind: Mill, x: 0, y: 0,
			existing:  []*Building{{Kind: Mill, X: 0, Y: 0}},
			wantValid: false,
		},
		{
			name: "adjacent player buildings need a gap",
			kind: Mill, x: 1, y: 0,
			existing:  []*Building{{Kind: Mill, X: 0, Y: 0}},
			wantValid: false,
		},
		{
			name: "one empty tile between player buildings is valid",
			kind: Mill, x: 2, y: 0,
			existing:  []*Building{{Kind: Mill, X: 0, Y: 0}},
			wantValid: true,
		},
		{
			name: "diagonal player buildings need a gap",
			kind: Mill, x: 1, y: 1,
			existing:  []*Building{{Kind: Mill, X: 0, Y: 0}},
			wantValid: false,
		},
		{
			name: "only one tree per cell",
			kind: Tree, x: 6, y: 6,
			existing:  []*Building{{Kind: Tree, X: 6, Y: 6}},
			wantValid: false,
		},
		{
			name: "tree cannot grow on water",
			kind: Tree, x: 8, y: 8,
			wantValid: false,
		},
		{
			name:      "fisher hut beside cardinal water",
			kind:      FisherHut,
			x:         8,
			y:         7,
			wantValid: true,
		},
		{
			name:      "fisher hut cannot use diagonal water",
			kind:      FisherHut,
			x:         7,
			y:         7,
			wantValid: false,
		},
		{
			name:      "fish can occupy a water cell",
			kind:      Fish,
			x:         8,
			y:         8,
			wantValid: true,
		},
		{
			name:      "fish cannot occupy dry land",
			kind:      Fish,
			x:         7,
			y:         8,
			wantValid: false,
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

func TestFoundationRoad(t *testing.T) {
	g := world.NewGrid(8, 8)
	road, ok := FoundationRoad(g, nil, Mill, 4, 4)
	if !ok || road == nil {
		t.Fatal("FoundationRoad() did not create the entrance road")
	}
	if road.Kind != Road || road.X != 4 || road.Y != 5 {
		t.Fatalf("starter road = %#v, want finished Road at (4,5)", road)
	}
	if road.ConstructionStage != ConstructionNone {
		t.Fatalf("starter road stage = %v, want ConstructionNone", road.ConstructionStage)
	}
}

func TestFoundationRoadReusesExistingFinishedRoad(t *testing.T) {
	g := world.NewGrid(8, 8)
	existing := []*Building{{Kind: Road, X: 4, Y: 5}}
	road, ok := FoundationRoad(g, existing, Mill, 4, 4)
	if !ok {
		t.Fatal("FoundationRoad() rejected an existing entrance road")
	}
	if road != nil {
		t.Fatalf("FoundationRoad() = %#v, want nil because the existing road is reused", road)
	}
}
