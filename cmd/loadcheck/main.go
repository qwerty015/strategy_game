package main

import (
	"fmt"
	"os"

	"strategy_game/internal/building"
	"strategy_game/internal/save"
)

func main() {
	state, err := save.Load(os.Args[1])
	if err != nil {
		fmt.Println("LOAD ERROR:", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded OK: Name=%q Version=%d Grid=%dx%d Buildings=%d StoneSeeded=%v OreSeeded=%v\n",
		state.Name, state.Version, state.GridWidth, state.GridHeight, len(state.Buildings), state.StoneSeeded, state.OreSeeded)

	counts := map[building.Kind]int{}
	for _, b := range state.Buildings {
		counts[b.Kind]++
	}
	for _, k := range []building.Kind{building.StoneDeposit, building.CoalDeposit, building.GoldOreDeposit, building.IronOreDeposit} {
		fmt.Printf("  %v: %d\n", k, counts[k])
	}

	// Also exercise the same warehouse-lookup and grid reconstruction the
	// real cmd/game.loadGame path does, so a structurally-broken save
	// (e.g. no warehouse) fails here instead of only inside the real game.
	var warehouseFound bool
	for _, b := range state.Buildings {
		if b.Kind == building.Warehouse {
			warehouseFound = true
			break
		}
	}
	fmt.Println("Warehouse present:", warehouseFound)
}
