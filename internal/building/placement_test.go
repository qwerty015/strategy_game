package building

import (
	"testing"

	"strategy_game/internal/resource"

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

// TestWallsHaveDistinctPlacementRules protects the intentionally asymmetric
// gap rule: wall segments can form a continuous run, but player buildings stay
// one tile away whichever of the two was placed first. Gates are upgrades, not
// bare-ground palette objects.
func TestWallsHaveDistinctPlacementRules(t *testing.T) {
	grid := testGrid()
	if !CanPlace(grid, nil, StoneWall, 6, 6) {
		t.Fatal("free stone-wall tile rejected")
	}
	existingWall := []*Building{{Kind: StoneWall, X: 6, Y: 6}}
	if !CanPlace(grid, existingWall, StoneWall, 7, 6) {
		t.Fatal("adjacent wall section rejected; continuous walls must be allowed")
	}
	if CanPlace(grid, []*Building{{Kind: Mill, X: 4, Y: 4}}, StoneWall, 5, 4) {
		t.Fatal("wall adjacent to an ordinary building was accepted")
	}
	if CanPlace(grid, existingWall, Mill, 7, 6) {
		t.Fatal("ordinary building adjacent to a wall was accepted")
	}
	if CanPlace(grid, nil, Gate, 2, 2) {
		t.Fatal("bare-ground gate was accepted")
	}
}

func TestGateConstructionCostIncludesIron(t *testing.T) {
	gate := NewConstructionSite(Gate, 3, 3)
	if got := gate.ConstructionMaterialCost(resource.Plank); got != 5 {
		t.Fatalf("gate plank cost = %d, want 5", got)
	}
	if got := gate.ConstructionMaterialCost(resource.StoneBlock); got != 3 {
		t.Fatalf("gate stone cost = %d, want 3", got)
	}
	if got := gate.ConstructionMaterialCost(resource.Iron); got != 3 {
		t.Fatalf("gate iron cost = %d, want 3", got)
	}
	gate.AddConstructionMaterial(resource.Plank, 5)
	gate.AddConstructionMaterial(resource.StoneBlock, 3)
	if gate.ConstructionMaterialsReady() {
		t.Fatal("gate was ready without its required iron")
	}
	gate.AddConstructionMaterial(resource.Iron, 3)
	if !gate.ConstructionMaterialsReady() {
		t.Fatal("fully supplied gate was not ready")
	}
}

func TestWallAxisAtRequiresStraightCompletedSegment(t *testing.T) {
	line := []*Building{
		{Kind: StoneWall, X: 2, Y: 3},
		{Kind: StoneWall, X: 3, Y: 3},
		{Kind: StoneWall, X: 4, Y: 3},
	}
	axis, ok := WallAxisAt(line, 3, 3)
	if !ok || axis != WallHorizontal {
		t.Fatalf("horizontal segment axis = %v, %v; want horizontal, true", axis, ok)
	}
	line = append(line, &Building{Kind: StoneWall, X: 3, Y: 2}, &Building{Kind: StoneWall, X: 3, Y: 4})
	if _, ok := WallAxisAt(line, 3, 3); ok {
		t.Fatal("corner/cross segment was accepted as a gate location")
	}
}

// TestWallShapeAtUsesDedicatedCorners makes the visual topology explicit:
// the logical per-cell wall can bend without pretending that a corner is a
// straight horizontal/vertical segment.
func TestWallShapeAtUsesDedicatedCorners(t *testing.T) {
	center := &Building{Kind: StoneWall, X: 5, Y: 5}
	cases := []struct {
		name   string
		around []*Building
		want   WallShape
	}{
		{"north-east", []*Building{{Kind: StoneWall, X: 5, Y: 4}, {Kind: StoneWall, X: 6, Y: 5}}, WallShapeCornerNE},
		{"north-west", []*Building{{Kind: StoneWall, X: 5, Y: 4}, {Kind: StoneWall, X: 4, Y: 5}}, WallShapeCornerNW},
		{"south-east", []*Building{{Kind: StoneWall, X: 6, Y: 5}, {Kind: StoneWall, X: 5, Y: 6}}, WallShapeCornerSE},
		{"south-west", []*Building{{Kind: StoneWall, X: 4, Y: 5}, {Kind: StoneWall, X: 5, Y: 6}}, WallShapeCornerSW},
		{"cross", []*Building{{Kind: StoneWall, X: 5, Y: 4}, {Kind: StoneWall, X: 6, Y: 5}, {Kind: StoneWall, X: 5, Y: 6}, {Kind: StoneWall, X: 4, Y: 5}}, WallShapeCross},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			walls := append([]*Building{center}, tc.around...)
			if got := WallShapeAt(walls, center.X, center.Y); got != tc.want {
				t.Fatalf("WallShapeAt() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCanCreateWallTopology keeps the wall art and construction model simple:
// paths may extend or turn, but no cell may become a T/cross junction.
func TestCanCreateWallTopology(t *testing.T) {
	cases := []struct {
		name     string
		existing []*Building
		proposed []Point
		want     bool
	}{
		{
			name:     "right angle remains allowed",
			existing: []*Building{{Kind: StoneWall, X: 4, Y: 4}},
			proposed: []Point{{X: 5, Y: 4}, {X: 5, Y: 5}},
			want:     true,
		},
		{
			name: "third branch is rejected",
			existing: []*Building{
				{Kind: StoneWall, X: 3, Y: 4},
				{Kind: StoneWall, X: 4, Y: 4},
				{Kind: StoneWall, X: 5, Y: 4},
			},
			proposed: []Point{{X: 4, Y: 5}},
			want:     false,
		},
		{
			name: "cross is rejected",
			existing: []*Building{
				{Kind: StoneWall, X: 4, Y: 3},
				{Kind: StoneWall, X: 5, Y: 4},
				{Kind: StoneWall, X: 4, Y: 5},
				{Kind: StoneWall, X: 3, Y: 4},
			},
			proposed: []Point{{X: 4, Y: 4}},
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanCreateWallTopology(tc.existing, tc.proposed); got != tc.want {
				t.Fatalf("CanCreateWallTopology() = %v, want %v", got, tc.want)
			}
		})
	}
}
