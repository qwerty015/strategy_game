package advisor

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/hunger"
	"strategy_game/internal/resource"
)

func TestEvaluateFoodRunway(t *testing.T) {
	pop := &economy.Population{Count: 10}

	t.Run("plenty of food, no tip", func(t *testing.T) {
		stock := resource.NewStockpile(0)
		// Comfortably above the warning threshold: 10 residents eat
		// pop.Count units every MealThresholdTicks ticks, so this stock
		// alone lasts far past FoodRunwayWarningTicks.
		stock.Add(resource.Bread, 1000)
		tips := Evaluate(nil, stock, pop, nil, nil, nil, 0, 0, 0)
		for _, tip := range tips {
			if tip.Kind == KindFoodRunningOut {
				t.Fatalf("got a food-runway tip with a full stockpile: %+v", tip)
			}
		}
	})

	t.Run("low food triggers a tip with the right runway", func(t *testing.T) {
		stock := resource.NewStockpile(0)
		// 10 residents, MealThresholdTicks apart per meal: 1 unit of
		// food buys the whole population 1/10th of MealThresholdTicks
		// of runway. 30 units -> 30*1040/10 = 3120... too high, so pick
		// a stock that lands comfortably under the 350-tick threshold.
		stock.Add(resource.Bread, 3)
		wantTicksLeft := 3 * hunger.MealThresholdTicks / pop.Count
		if wantTicksLeft > FoodRunwayWarningTicks {
			t.Fatalf("test setup error: wantTicksLeft=%d is not below the warning threshold %d", wantTicksLeft, FoodRunwayWarningTicks)
		}
		tips := Evaluate(nil, stock, pop, nil, nil, nil, 0, 0, 0)
		found := false
		for _, tip := range tips {
			if tip.Kind == KindFoodRunningOut {
				found = true
				if tip.TicksLeft != wantTicksLeft {
					t.Fatalf("TicksLeft = %d, want %d", tip.TicksLeft, wantTicksLeft)
				}
			}
		}
		if !found {
			t.Fatal("expected a KindFoodRunningOut tip, got none")
		}
	})

	t.Run("sums every food type and every Tavern, not just the Warehouse", func(t *testing.T) {
		stock := resource.NewStockpile(0)
		stock.Add(resource.Wine, 1)
		tavern := &building.Building{Kind: building.Tavern}
		tavern.AddInput(resource.Fish, 1)
		tavern.AddInput(resource.Sausage, 1)
		buildings := []*building.Building{tavern}
		// 3 units total (1 warehouse Wine + 1 Tavern Fish + 1 Tavern
		// Sausage) at 10 population.
		wantTicksLeft := 3 * hunger.MealThresholdTicks / pop.Count
		tips := Evaluate(buildings, stock, pop, nil, nil, nil, 0, 0, 0)
		found := false
		for _, tip := range tips {
			if tip.Kind == KindFoodRunningOut {
				found = true
				if tip.TicksLeft != wantTicksLeft {
					t.Fatalf("TicksLeft = %d, want %d (should count Tavern buffers and every food type)", tip.TicksLeft, wantTicksLeft)
				}
			}
		}
		if !found {
			t.Fatal("expected a KindFoodRunningOut tip counting Tavern stock, got none")
		}
	})

	t.Run("no population, no tip even with zero food", func(t *testing.T) {
		empty := &economy.Population{Count: 0}
		tips := Evaluate(nil, resource.NewStockpile(0), empty, nil, nil, nil, 0, 0, 0)
		for _, tip := range tips {
			if tip.Kind == KindFoodRunningOut {
				t.Fatalf("got a food-runway tip with zero population: %+v", tip)
			}
		}
	})
}

func TestEvaluateIdleBuildings(t *testing.T) {
	mill := &building.Building{Kind: building.Mill}

	t.Run("recently idle is not reported yet", func(t *testing.T) {
		idleSince := map[*building.Building]int{mill: 100}
		tips := Evaluate(nil, resource.NewStockpile(0), nil, nil, idleSince, nil, 100+IdleBuildingWarningTicks-1, 0, 0)
		for _, tip := range tips {
			if tip.Kind == KindIdleBuilding {
				t.Fatalf("got an idle-building tip before the grace period elapsed: %+v", tip)
			}
		}
	})

	t.Run("idle past the threshold is reported, aggregated", func(t *testing.T) {
		bakery := &building.Building{Kind: building.Bakery}
		idleSince := map[*building.Building]int{mill: 100, bakery: 50}
		tips := Evaluate(nil, resource.NewStockpile(0), nil, nil, idleSince, nil, 100+IdleBuildingWarningTicks, 0, 0)
		found := false
		for _, tip := range tips {
			if tip.Kind == KindIdleBuilding {
				found = true
				if tip.Count != 2 {
					t.Fatalf("Count = %d, want 2 (both buildings past the threshold)", tip.Count)
				}
				if tip.Building == nil {
					t.Fatal("Building example is nil")
				}
			}
		}
		if !found {
			t.Fatal("expected a KindIdleBuilding tip, got none")
		}
	})
}

// TestEvaluateGatherWorkerStuck covers the user's explicit request: warn
// when a lumberjack/quarryman/miner can't find a tree/stone/ore deposit
// nearby, but only once it's been stuck that long (600-700 ticks), not
// the moment-to-moment idle every worker briefly passes through between
// jobs.
func TestEvaluateGatherWorkerStuck(t *testing.T) {
	hut := &building.Building{Kind: building.LumberjackHut}

	t.Run("briefly idle is not reported yet", func(t *testing.T) {
		gatherStuckSince := map[*building.Building]int{hut: 100}
		tips := Evaluate(nil, resource.NewStockpile(0), nil, nil, nil, gatherStuckSince, 100+GatherWorkerStuckWarningTicks-1, 0, 0)
		for _, tip := range tips {
			if tip.Kind == KindGatherWorkerStuck {
				t.Fatalf("got a gather-worker-stuck tip before the grace period elapsed: %+v", tip)
			}
		}
	})

	t.Run("stuck past the threshold is reported", func(t *testing.T) {
		gatherStuckSince := map[*building.Building]int{hut: 100}
		tips := Evaluate(nil, resource.NewStockpile(0), nil, nil, nil, gatherStuckSince, 100+GatherWorkerStuckWarningTicks, 0, 0)
		found := false
		for _, tip := range tips {
			if tip.Kind == KindGatherWorkerStuck {
				found = true
				if tip.Count != 1 || tip.Building != hut {
					t.Fatalf("tip = %+v, want Count=1 Building=hut", tip)
				}
			}
		}
		if !found {
			t.Fatal("expected a KindGatherWorkerStuck tip, got none")
		}
	})
}

func TestEvaluateDisconnectedBuildings(t *testing.T) {
	mill := &building.Building{Kind: building.Mill}
	bakery := &building.Building{Kind: building.Bakery}
	buildings := []*building.Building{mill, bakery}

	t.Run("none disconnected, no tip", func(t *testing.T) {
		tips := Evaluate(buildings, resource.NewStockpile(0), nil, map[*building.Building]bool{}, nil, nil, 0, 0, 0)
		for _, tip := range tips {
			if tip.Kind == KindDisconnectedBuilding {
				t.Fatalf("got a disconnected tip with nothing disconnected: %+v", tip)
			}
		}
	})

	t.Run("one disconnected building is reported", func(t *testing.T) {
		disconnected := map[*building.Building]bool{mill: true}
		tips := Evaluate(buildings, resource.NewStockpile(0), nil, disconnected, nil, nil, 0, 0, 0)
		found := false
		for _, tip := range tips {
			if tip.Kind == KindDisconnectedBuilding {
				found = true
				if tip.Count != 1 || tip.Building != mill {
					t.Fatalf("tip = %+v, want Count=1 Building=mill", tip)
				}
			}
		}
		if !found {
			t.Fatal("expected a KindDisconnectedBuilding tip, got none")
		}
	})
}

func TestEvaluateServeCount(t *testing.T) {
	for _, tc := range []struct {
		name        string
		recommended int
		current     int
		wantKind    Kind
		wantNoTip   bool
	}{
		{"comfortably within range", 30, 30, 0, true},
		{"slightly under is fine (deadband)", 30, 25, 0, true},
		{"far under recommendation", 30, 20, KindServeCountLow, false},
		{"slightly over is fine (big deadband)", 30, 45, 0, true},
		{"far over recommendation", 30, 70, KindServeCountHigh, false},
		{"no recommendation yet, no tip", 0, 5, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tips := Evaluate(nil, resource.NewStockpile(0), nil, nil, nil, nil, 0, tc.recommended, tc.current)
			var found *Tip
			for i := range tips {
				if tips[i].Kind == KindServeCountLow || tips[i].Kind == KindServeCountHigh {
					found = &tips[i]
				}
			}
			if tc.wantNoTip {
				if found != nil {
					t.Fatalf("got a serve-count tip, want none: %+v", found)
				}
				return
			}
			if found == nil {
				t.Fatal("expected a serve-count tip, got none")
			}
			if found.Kind != tc.wantKind {
				t.Fatalf("Kind = %v, want %v", found.Kind, tc.wantKind)
			}
			if found.Recommended != tc.recommended || found.Current != tc.current {
				t.Fatalf("tip = %+v, want Recommended=%d Current=%d", found, tc.recommended, tc.current)
			}
		})
	}
}
