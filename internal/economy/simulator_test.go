package economy

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

func TestTick_FarmProducesAfterTicksToProduce(t *testing.T) {
	farm := &building.Building{Kind: building.Farm}
	recipe := building.Types[building.Farm].Recipe
	stock := resource.NewStockpile(100)

	for i := 0; i < recipe.TicksToProduce-1; i++ {
		Tick([]*building.Building{farm}, stock, nil)
		if got := stock.Amount(recipe.Output); got != 0 {
			t.Fatalf("tick %d: got %d %s in stockpile, want 0 (not done yet)", i, got, recipe.Output)
		}
	}

	Tick([]*building.Building{farm}, stock, nil)

	if got, want := stock.Amount(recipe.Output), recipe.OutputAmount; got != want {
		t.Fatalf("after %d ticks: got %d %s, want %d", recipe.TicksToProduce, got, recipe.Output, want)
	}
	if farm.ProgressTicks != 0 {
		t.Fatalf("ProgressTicks after producing = %d, want 0 (reset)", farm.ProgressTicks)
	}
}

func TestTick_MillHoldsWhenInputsMissing(t *testing.T) {
	mill := &building.Building{Kind: building.Mill}
	recipe := building.Types[building.Mill].Recipe
	stock := resource.NewStockpile(100) // no Wheat deposited

	for i := 0; i < recipe.TicksToProduce+3; i++ {
		Tick([]*building.Building{mill}, stock, nil)
	}

	if got := stock.Amount(recipe.Output); got != 0 {
		t.Fatalf("got %d %s produced with no inputs available, want 0", got, recipe.Output)
	}
	if mill.ProgressTicks != recipe.TicksToProduce {
		t.Fatalf("ProgressTicks = %d, want %d (held at 100%% waiting on inputs)", mill.ProgressTicks, recipe.TicksToProduce)
	}
}

func TestTick_MillProducesAssoonAsInputsArrive(t *testing.T) {
	mill := &building.Building{Kind: building.Mill}
	recipe := building.Types[building.Mill].Recipe
	stock := resource.NewStockpile(100)

	for i := 0; i < recipe.TicksToProduce; i++ {
		Tick([]*building.Building{mill}, stock, nil)
	}
	if stock.Amount(recipe.Output) != 0 {
		t.Fatal("mill produced with no inputs")
	}

	for rt, n := range recipe.Inputs {
		stock.Add(rt, n)
	}
	Tick([]*building.Building{mill}, stock, nil)

	if got, want := stock.Amount(recipe.Output), recipe.OutputAmount; got != want {
		t.Fatalf("after inputs arrived: got %d %s, want %d", got, recipe.Output, want)
	}
	for rt := range recipe.Inputs {
		if got := stock.Amount(rt); got != 0 {
			t.Fatalf("input %s not consumed: got %d left, want 0", rt, got)
		}
	}
}

func TestPopulation_EatsWhenFed(t *testing.T) {
	pop := NewPopulation(3, 2)
	stock := resource.NewStockpile(100)
	stock.Add(resource.Bread, 10)

	pop.Tick(stock) // tick 1: not mealtime yet
	pop.Tick(stock) // tick 2: mealtime

	if pop.Count != 3 {
		t.Fatalf("Count = %d, want 3 (fed, no starvation)", pop.Count)
	}
	if got, want := stock.Amount(resource.Bread), 10-3; got != want {
		t.Fatalf("Bread left = %d, want %d", got, want)
	}
}

func TestPopulation_StarvesWithoutBread(t *testing.T) {
	pop := NewPopulation(3, 1)
	stock := resource.NewStockpile(100) // no bread

	pop.Tick(stock)

	if pop.Count != 2 {
		t.Fatalf("Count = %d, want 2 (lost one villager to starvation)", pop.Count)
	}
}
