package main

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/world"
)

func TestSeedFishLimitsEveryConnectedWaterBody(t *testing.T) {
	grid := world.NewGrid(8, 4)
	// A ten-tile pond and a disconnected one-tile puddle exercise both the
	// 40% cap and the deliberate one-fish minimum for tiny ponds.
	for y := 0; y < 2; y++ {
		for x := 0; x < 5; x++ {
			grid.Set(x, y, world.Tile{Terrain: world.Water})
		}
	}
	grid.Set(7, 3, world.Tile{Terrain: world.Water})

	buildings := seedFish(grid, nil)
	large := waterBodyCells(grid, gridPoint{0, 0})
	small := waterBodyCells(grid, gridPoint{7, 3})
	if got, want := countFishInCells(buildings, large), 4; got != want {
		t.Fatalf("large pond fish = %d, want %d (40%% of 10)", got, want)
	}
	if got, want := countFishInCells(buildings, small), 1; got != want {
		t.Fatalf("small pond fish = %d, want %d (tiny-pond minimum)", got, want)
	}
}

func TestFishRegrowthRespectsBodyCap(t *testing.T) {
	grid := world.NewGrid(3, 1)
	for x := 0; x < grid.Width; x++ {
		grid.Set(x, 0, world.Tile{Terrain: world.Water})
	}
	// Three water tiles may hold one fish (floor(3 * 40 / 100)).
	existing := building.NewFish(0, 0)
	game := &Game{
		grid:      grid,
		buildings: []*building.Building{existing},
		fishRegrowth: []fishRegrowth{{
			waterX: 1,
			waterY: 0,
			ticks:  1,
			target: 1,
			seed:   1,
		}},
	}

	game.tickFishRegrowth()
	pond := waterBodyCells(grid, gridPoint{0, 0})
	if got, want := countFishInCells(game.buildings, pond), 1; got != want {
		t.Fatalf("fish after capped regrowth = %d, want %d", got, want)
	}
	if len(game.fishRegrowth) != 1 {
		t.Fatal("regrowth entry disappeared even though the pond is at its cap")
	}
}
