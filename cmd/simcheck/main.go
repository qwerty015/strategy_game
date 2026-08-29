// Temporary program, not part of the repo -- deleted after use. Traces
// per-BUILDING (not per-kind) backup to find whether the same single
// instance monopolizes/starves its siblings.
package main

import (
	"fmt"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/logistics"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/save"
	"strategy_game/internal/world"
)

func findWarehouse(buildings []*building.Building) *building.Building {
	for _, b := range buildings {
		if b.Kind == building.Warehouse {
			return b
		}
	}
	return nil
}

func main() {
	state, err := save.Load("saves/panel_slot_1.json")
	if err != nil {
		fmt.Println("load error:", err)
		return
	}
	grid := world.NewGrid(state.GridWidth, state.GridHeight)
	buildings := make([]*building.Building, len(state.Buildings))
	for i := range state.Buildings {
		b := state.Buildings[i]
		buildings[i] = &b
	}
	warehouse := findWarehouse(buildings)
	logi := logistics.NewController(warehouse, 55)
	stock := resource.NewStockpile(0)

	// Watch every building of these kinds individually.
	watch := map[building.Kind]bool{
		building.Winery: true, building.Farm: true, building.Bakery: true,
		building.PigFarm: true, building.MeatWorkshop: true, building.Mill: true,
	}
	type stats struct{ fullSamples, totalSamples, index int }
	tracked := map[*building.Building]*stats{}
	idx := map[building.Kind]int{}
	for _, b := range buildings {
		if b != nil && watch[b.Kind] {
			tracked[b] = &stats{index: idx[b.Kind]}
			idx[b.Kind]++
		}
	}

	const ticks = 20000
	for i := 0; i < ticks; i++ {
		economy.TickWithConnectivity(buildings, nil, nil)
		ledger := reservations.New()
		logi.Reserve(ledger)
		logi.Tick(grid, buildings, stock, ledger)

		if i%50 != 0 {
			continue
		}
		for b, st := range tracked {
			st.totalSamples++
			cap := building.BufferCapacity
			if oc := building.Types[b.Kind].OutputCapacity; oc > 0 {
				cap = oc
			}
			for _, amt := range b.OutputBuffer {
				if amt >= cap {
					st.fullSamples++
					break
				}
			}
		}
	}

	byKind := map[building.Kind][]struct {
		idx  int
		pct  float64
		x, y int
	}{}
	for b, st := range tracked {
		pct := 100 * float64(st.fullSamples) / float64(st.totalSamples)
		byKind[b.Kind] = append(byKind[b.Kind], struct {
			idx  int
			pct  float64
			x, y int
		}{st.index, pct, b.X, b.Y})
	}
	for kind, entries := range byKind {
		fmt.Printf("kind=%d:\n", kind)
		for _, e := range entries {
			fmt.Printf("  #%d at (%d,%d): backup=%.0f%%\n", e.idx, e.x, e.y, e.pct)
		}
	}
}
