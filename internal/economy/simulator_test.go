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
		Tick([]*building.Building{farm}, nil)
		if got := farm.OutputBuffer[recipe.Output]; got != 0 {
			t.Fatalf("tick %d: OutputBuffer[%s] = %d, want 0 (not done yet)", i, recipe.Output, got)
		}
	}

	Tick([]*building.Building{farm}, nil)

	if got, want := farm.OutputBuffer[recipe.Output], recipe.OutputAmount; got != want {
		t.Fatalf("after %d ticks: OutputBuffer[%s] = %d, want %d", recipe.TicksToProduce, recipe.Output, got, want)
	}
	if farm.ProgressTicks != 0 {
		t.Fatalf("ProgressTicks after producing = %d, want 0 (reset)", farm.ProgressTicks)
	}
}

func TestTick_WineryProducesWine(t *testing.T) {
	winery := &building.Building{Kind: building.Winery}
	recipe := building.Types[building.Winery].Recipe

	for i := 0; i < recipe.TicksToProduce; i++ {
		Tick([]*building.Building{winery}, nil)
	}

	if got, want := winery.OutputBuffer[resource.Wine], recipe.OutputAmount; got != want {
		t.Fatalf("after %d ticks: OutputBuffer[Wine] = %d, want %d", recipe.TicksToProduce, got, want)
	}
	if winery.ProgressTicks != 0 {
		t.Fatalf("Winery ProgressTicks after producing = %d, want 0", winery.ProgressTicks)
	}
}

func TestTick_MillHoldsWhenInputBufferEmpty(t *testing.T) {
	mill := &building.Building{Kind: building.Mill}
	recipe := building.Types[building.Mill].Recipe

	for i := 0; i < recipe.TicksToProduce+3; i++ {
		Tick([]*building.Building{mill}, nil)
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
		Tick([]*building.Building{mill}, nil)
	}
	if mill.OutputBuffer[recipe.Output] != 0 {
		t.Fatal("mill produced with an empty InputBuffer")
	}

	for rt, n := range recipe.Inputs {
		mill.AddInput(rt, n)
	}
	Tick([]*building.Building{mill}, nil)

	if got, want := mill.OutputBuffer[recipe.Output], recipe.OutputAmount; got != want {
		t.Fatalf("after InputBuffer filled: OutputBuffer[%s] = %d, want %d", recipe.Output, got, want)
	}
	for rt := range recipe.Inputs {
		if got := mill.InputBuffer[rt]; got != 0 {
			t.Fatalf("InputBuffer[%s] not consumed: got %d left, want 0", rt, got)
		}
	}
}

func TestTick_PigFarmConsumesFeedBeforeGrowth(t *testing.T) {
	pigFarm := &building.Building{Kind: building.PigFarm}
	recipe := building.Types[building.PigFarm].Recipe

	// Waiting without feed must not silently advance the animal's lifetime.
	for range 20 {
		Tick([]*building.Building{pigFarm}, nil)
	}
	if pigFarm.ProgressTicks != 0 {
		t.Fatalf("unfed pig farm progress = %d, want 0", pigFarm.ProgressTicks)
	}

	pigFarm.AddInput(resource.Wheat, recipe.Inputs[resource.Wheat])
	Tick([]*building.Building{pigFarm}, nil)
	if got := pigFarm.InputBuffer[resource.Wheat]; got != 0 {
		t.Fatalf("wheat after growth started = %d, want 0", got)
	}
	if got := pigFarm.ProgressTicks; got != 1 {
		t.Fatalf("progress after feeding = %d, want 1", got)
	}

	for range recipe.TicksToProduce - 1 {
		Tick([]*building.Building{pigFarm}, nil)
	}
	if got := pigFarm.OutputBuffer[resource.Carcass]; got != 1 {
		t.Fatalf("carcasses after %d fed growth ticks = %d, want 1", recipe.TicksToProduce, got)
	}
	// Per the user's explicit request, a pig gives up its hide the same
	// moment it gives up its carcass -- one animal, both products at once.
	if got := pigFarm.OutputBuffer[resource.Hide]; got != 1 {
		t.Fatalf("hides after %d fed growth ticks = %d, want 1 (SecondaryOutput)", recipe.TicksToProduce, got)
	}
	if pigFarm.ProgressTicks != 0 {
		t.Fatalf("progress after producing a carcass = %d, want 0", pigFarm.ProgressTicks)
	}
}

// TestTick_SecondaryOutputHeldWhenItsOwnBufferIsFull covers hasOutputRoom's
// half of SecondaryOutput: a full Hide buffer must block the cycle from
// even starting (PigFarm's prepaid recipe checks output room before
// spending the feed), exactly like a full Carcass buffer already would,
// not silently drop the hide while still growing/producing the carcass.
func TestTick_SecondaryOutputHeldWhenItsOwnBufferIsFull(t *testing.T) {
	pigFarm := &building.Building{Kind: building.PigFarm}
	recipe := building.Types[building.PigFarm].Recipe
	pigFarm.AddOutput(resource.Hide, building.BufferCapacity) // pre-fill to capacity

	pigFarm.AddInput(resource.Wheat, recipe.Inputs[resource.Wheat])
	for range recipe.TicksToProduce {
		Tick([]*building.Building{pigFarm}, nil)
	}

	if got := pigFarm.OutputBuffer[resource.Carcass]; got != 0 {
		t.Fatalf("carcasses while Hide buffer is full = %d, want 0 (held, not half-produced)", got)
	}
	if pigFarm.ProgressTicks != 0 {
		t.Fatalf("progress while blocked on a full Hide buffer = %d, want 0 (feed never spent, growth never started)", pigFarm.ProgressTicks)
	}
	if got := pigFarm.InputBuffer[resource.Wheat]; got != recipe.Inputs[resource.Wheat] {
		t.Fatalf("wheat while blocked = %d, want unchanged %d (never consumed)", got, recipe.Inputs[resource.Wheat])
	}
}

func TestTick_ProductionHoldsWhenOutputBufferFull(t *testing.T) {
	farm := &building.Building{Kind: building.Farm}
	recipe := building.Types[building.Farm].Recipe
	farm.AddOutput(recipe.Output, building.BufferCapacity) // pre-fill to capacity

	for i := 0; i < recipe.TicksToProduce+3; i++ {
		Tick([]*building.Building{farm}, nil)
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
		Tick([]*building.Building{warehouse, road}, nil)
	}

	if warehouse.ProgressTicks != 0 || road.ProgressTicks != 0 {
		t.Fatal("non-producing buildings accumulated ProgressTicks, want untouched")
	}
}

func TestTick_SkipsStarvingBuildings(t *testing.T) {
	farm := &building.Building{Kind: building.Farm}
	recipe := building.Types[building.Farm].Recipe
	starving := map[*building.Building]bool{farm: true}

	for i := 0; i < recipe.TicksToProduce+3; i++ {
		Tick([]*building.Building{farm}, starving)
	}

	if farm.ProgressTicks != 0 {
		t.Fatalf("ProgressTicks = %d, want 0 (starving worker, no progress)", farm.ProgressTicks)
	}
	if got := farm.OutputBuffer[recipe.Output]; got != 0 {
		t.Fatalf("OutputBuffer[%s] = %d, want 0 (starving worker never produced)", recipe.Output, got)
	}

	delete(starving, farm)
	for i := 0; i < recipe.TicksToProduce; i++ {
		Tick([]*building.Building{farm}, starving)
	}
	if got := farm.OutputBuffer[recipe.Output]; got != recipe.OutputAmount {
		t.Fatalf("after worker returned: OutputBuffer[%s] = %d, want %d", recipe.Output, got, recipe.OutputAmount)
	}
}

func TestTickWithConnectivity_SkipsDisconnectedProduction(t *testing.T) {
	farm := &building.Building{Kind: building.Farm}
	disconnected := map[*building.Building]bool{farm: true}

	for range 10 {
		TickWithConnectivity([]*building.Building{farm}, nil, disconnected)
	}

	if farm.ProgressTicks != 0 || farm.OutputBuffer[resource.Wheat] != 0 {
		t.Fatalf("disconnected farm progressed to %d with output %d, want no production", farm.ProgressTicks, farm.OutputBuffer[resource.Wheat])
	}

	for range building.Types[building.Farm].Recipe.TicksToProduce {
		TickWithConnectivity([]*building.Building{farm}, nil, nil)
	}
	if got := farm.OutputBuffer[resource.Wheat]; got != building.Types[building.Farm].Recipe.OutputAmount {
		t.Fatalf("reconnected farm output = %d, want %d", got, building.Types[building.Farm].Recipe.OutputAmount)
	}
}

// TestTickWithConnectivity_SmelteryWaitsWithNoOreOnHand covers the one
// behavior that deliberately differs from a single-recipe building: with
// neither GoldOre nor IronOre present, a Smeltery must not tick blindly the
// way a Mill ticks even without wheat -- there's no recipe to be blind
// about yet.
func TestTickWithConnectivity_SmelteryWaitsWithNoOreOnHand(t *testing.T) {
	smeltery := &building.Building{Kind: building.Smeltery}
	smeltery.AddInput(resource.Coal, 6) // plenty of coal, but no ore of either kind

	for range 100 {
		TickWithConnectivity([]*building.Building{smeltery}, nil, nil)
	}

	if smeltery.ProgressTicks != 0 {
		t.Fatalf("ProgressTicks with no ore on hand = %d, want 0 (nothing to be mid-cycle on)", smeltery.ProgressTicks)
	}
}

// TestTickWithConnectivity_SmelteryAlternatesGoldAndIron covers the
// round-robin fairness pickRecipe exists for: with both Gold ore and Iron
// ore continuously available, the Smeltery must not let Gold (recipe index
// 0) win every single cycle just because it comes first.
func TestTickWithConnectivity_SmelteryAlternatesGoldAndIron(t *testing.T) {
	smeltery := &building.Building{Kind: building.Smeltery}
	recipes := building.Types[building.Smeltery].AllRecipes()
	if len(recipes) != 2 {
		t.Fatalf("Smeltery recipe count = %d, want 2 (gold, iron)", len(recipes))
	}
	ticksPerCycle := recipes[0].TicksToProduce

	// Keep both ores and coal topped up every tick, as if serfs were
	// perfectly keeping pace -- isolates the recipe-choice fairness from
	// delivery timing.
	refill := func() {
		smeltery.AddInput(resource.GoldOre, 1)
		smeltery.AddInput(resource.IronOre, 1)
		smeltery.AddInput(resource.Coal, 2)
	}
	refill()

	for cycle := 0; cycle < 4; cycle++ {
		for range ticksPerCycle + 1 {
			TickWithConnectivity([]*building.Building{smeltery}, nil, nil)
			refill()
		}
	}

	gold := smeltery.OutputBuffer[resource.Gold]
	iron := smeltery.OutputBuffer[resource.Iron]
	if gold == 0 || iron == 0 {
		t.Fatalf("after 4 cycles with both ores always available: Gold=%d Iron=%d, want both > 0 (alternation, not one recipe starving the other)", gold, iron)
	}
}

// TestTickWithConnectivity_SmelteryProducesIronWhenOnlyIronIsAvailable
// proves the production side of the chain independently of delivery: coal
// plus IronOre must select the alternate recipe and create Iron even with no
// GoldOre buffered at all.
func TestTickWithConnectivity_SmelteryProducesIronWhenOnlyIronIsAvailable(t *testing.T) {
	smeltery := &building.Building{Kind: building.Smeltery}
	smeltery.AddInput(resource.IronOre, 1)
	smeltery.AddInput(resource.Coal, 1)

	ticks := building.Types[building.Smeltery].AltRecipes[0].TicksToProduce
	for range ticks {
		TickWithConnectivity([]*building.Building{smeltery}, nil, nil)
	}
	if got := smeltery.OutputBuffer[resource.Iron]; got != 1 {
		t.Fatalf("Smeltery Iron output = %d, want 1", got)
	}
	if got := smeltery.OutputBuffer[resource.Gold]; got != 0 {
		t.Fatalf("Smeltery Gold output = %d, want 0 without GoldOre", got)
	}
}
