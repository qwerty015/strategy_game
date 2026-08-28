package main

import (
	"testing"

	"strategy_game/internal/world"
)

func TestZZZStressWarehouseNearWater(t *testing.T) {
	limitSq := float64(maxWarehouseDistanceFromWater * maxWarehouseDistanceFromWater)
	for seed := uint32(0); seed < 200; seed++ {
		mapSeed := seed*2654435761 + 1
		grid := generateGrid(mapWidth, mapHeight, mapSeed)
		spot, ok := findWarehouseSpot(grid, mapSeed^0xc2b2ae35)
		if !ok {
			t.Errorf("seed %d: findWarehouseSpot found nothing", seed)
			continue
		}
		within := false
		for y := 0; y < grid.Height && !within; y++ {
			for x := 0; x < grid.Width; x++ {
				if grid.At(x, y).Terrain != world.Water {
					continue
				}
				dx, dy := float64(spot.x-x), float64(spot.y-y)
				if dx*dx+dy*dy <= limitSq {
					within = true
					break
				}
			}
		}
		if !within {
			t.Errorf("seed %d: warehouse spot (%d,%d) has no water within %d tiles", seed, spot.x, spot.y, maxWarehouseDistanceFromWater)
		}
	}
}
