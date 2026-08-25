package economy

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

func TestTick_FarmProducesIntoOwnOutputBuffer(t *testing.T) {
	farm := &building.Building{Kind: building.Farm}
	recipe := building.Types[building.Farm].Recipe

	for i := 0; i < recipe.TicksToProduce-1; i++ {
		Tick([]*building.Building{farm}, nil, nil)
		if got := farm.OutputBuffer[recipe.Output]; got != 0 {
			t.Fatalf("tick %d: OutputBuffer[%s] = %d, want 0 (not done yet)", i, recipe.Output, got)
		}
	}

	Tick([]*building.Building{farm}, nil, nil)

	if got, want := farm.OutputBuffer[recipe.Output], recipe.OutputAmount; got != want {
		t.Fatalf("after %d ticks: OutputBuffer[%s] = %d, want %d", recipe.TicksToProduce, recipe.Output, got, want)
	}
	if farm.ProgressTicks != 0 {
		t.Fatalf("ProgressTicks after producing = %d, want 0 (reset)", farm.ProgressTicks)
	}
}

func TestTick_MillHoldsWhenInputBufferEmpty(t *testing.T) {
	mill := &building.Building{Kind: building.Mill}
	recipe := building.Types[building.Mill].Recipe

	for i := 0; i < recipe.TicksToProduce+3; i++ {
		Tick([]*building.Building{mill}, nil, nil)
	}

	if got := mill.OutputBuffer[recipe.Output]; got != 0 {
		t.Fatalf("OutputBuffer[%s] = %d with no inputs available, want 0", recipe.Output, got)
	}
	if mill.ProgressTicks != recipe.TicksToProduce {
		t.Fatalf("ProgressTicks = %d, want %d (held at 100%% waiting on inputs)", mill.ProgressTicks, recipe.TicksToProduce)
	}
}

func TestTick_MillProducesAsSoonAsInputBufferIsFilled(t *testing.T) {
	mill := &building.Building{Kind: building.Mill}
	recipe := building.Types[building.Mill].Recipe

	for i := 0; i < recipe.TicksToProduce; i++ {
		Tick([]*building.Building{mill}, nil, nil)
	}
	if mill.OutputBuffer[recipe.Output] != 0 {
		t.Fatal("mill produced with an empty InputBuffer")
	}

	for rt, n := range recipe.Inputs {
		mill.AddInput(rt, n)
	}
	Tick([]*building.Building{mill}, nil, nil)

	if got, want := mill.OutputBuffer[recipe.Output], recipe.OutputAmount; got != want {
		t.Fatalf("after InputBuffer filled: OutputBuffer[%s] = %d, want %d", recipe.Output, got, want)
	}
	for rt := range recipe.Inputs {
		if got := mill.InputBuffer[rt]; got != 0 {
			t.Fatalf("InputBuffer[%s] not consumed: got %d left, want 0", rt, got)
		}
	}
}

func TestTick_ProductionHoldsWhenOutputBufferFull(t *testing.T) {
	farm := &building.Building{Kind: building.Farm}
	recipe := building.Types[building.Farm].Recipe
	farm.AddOutput(recipe.Output, building.BufferCapacity) // pre-fill to capacity

	for i := 0; i < recipe.TicksToProduce+3; i++ {
		Tick([]*building.Building{farm}, nil, nil)
	}

	if got := farm.OutputBuffer[recipe.Output]; got != building.BufferCapacity {
		t.Fatalf("OutputBuffer[%s] = %d, want unchanged %d (no room, held)", recipe.Output, got, building.BufferCapacity)
	}
}

func TestTick_SkipsNonProducingBuildings(t *testing.T) {
	warehouse := &building.Building{Kind: building.Warehouse}
	road := &building.Building{Kind: building.Road}

	// Should not panic and should not advance progress at all.
	for range 10 {
		Tick([]*building.Building{warehouse, road}, nil, nil)
	}

	if warehouse.ProgressTicks != 0 || road.ProgressTicks != 0 {
		t.Fatal("non-producing buildings accumulated ProgressTicks, want untouched")
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
