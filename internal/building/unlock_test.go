package building

import (
	"slices"
	"testing"

	"strategy_game/internal/resource"
)

// noneProduced/noneStocked are the "nothing at all yet" baseline every
// test starts from, then narrows with its own producer/stock set.
func noneProduced(resource.Type) bool { return false }
func noneStocked(resource.Type) bool  { return false }

func TestUnlocked_NoInputRecipeIsAlwaysUnlocked(t *testing.T) {
	for _, kind := range []Kind{Farm, Winery, FisherHut, LumberjackHut, QuarryHut, MinerHut, Warehouse, Road, Tavern, StoneWall, WatchTower, Barracks, Armory} {
		if !Unlocked(kind, noneProduced, noneStocked) {
			t.Errorf("Unlocked(%v) = false with nothing built or stocked, want true (no recipe inputs)", kind)
		}
	}
}

func TestUnlocked_PigFarmNeedsFarmOrWheatInStock(t *testing.T) {
	if Unlocked(PigFarm, noneProduced, noneStocked) {
		t.Fatal("Pig Farm unlocked with no Farm and no Wheat in stock")
	}
	producedByFarm := func(r resource.Type) bool { return r == resource.Wheat }
	if !Unlocked(PigFarm, producedByFarm, noneStocked) {
		t.Fatal("Pig Farm still locked with a Farm (Wheat producer) built")
	}
	wheatInStock := func(r resource.Type) bool { return r == resource.Wheat }
	if !Unlocked(PigFarm, noneProduced, wheatInStock) {
		t.Fatal("Pig Farm still locked with Wheat already on the stockpile, no Farm needed")
	}
}

func TestUnlocked_MillThenBakeryChain(t *testing.T) {
	if Unlocked(Mill, noneProduced, noneStocked) {
		t.Fatal("Mill unlocked with no Farm and no Wheat in stock")
	}
	if Unlocked(Bakery, noneProduced, noneStocked) {
		t.Fatal("Bakery unlocked with no Mill and no Flour in stock")
	}
	millBuilt := func(r resource.Type) bool { return r == resource.Flour }
	if !Unlocked(Bakery, millBuilt, noneStocked) {
		t.Fatal("Bakery still locked with a Mill (Flour producer) built")
	}
}

func TestUnlocked_CarpentryWorkshopViaLogInStockAlone(t *testing.T) {
	// The user's explicit example: enough Log already banked, no
	// Lumberjack Hut needed at all.
	logInStock := func(r resource.Type) bool { return r == resource.Log }
	if !Unlocked(CarpentryWorkshop, noneProduced, logInStock) {
		t.Fatal("Carpentry Workshop still locked with Log already on the stockpile")
	}
	if Unlocked(CarpentryWorkshop, noneProduced, noneStocked) {
		t.Fatal("Carpentry Workshop unlocked with neither a Lumberjack Hut nor Log in stock")
	}
}

func TestUnlocked_SmelteryEitherOreRecipeUnlocksIt(t *testing.T) {
	if Unlocked(Smeltery, noneProduced, noneStocked) {
		t.Fatal("Smeltery unlocked with no Miner Hut and nothing in stock")
	}
	// Gold recipe's inputs (GoldOre + Coal) covered by stock alone -- the
	// user's explicit example, no Miner Hut ever built.
	goldPathInStock := func(r resource.Type) bool { return r == resource.GoldOre || r == resource.Coal }
	if !Unlocked(Smeltery, noneProduced, goldPathInStock) {
		t.Fatal("Smeltery still locked with GoldOre+Coal already on the stockpile")
	}
	// Iron recipe's inputs (IronOre + Coal) covered instead -- AltRecipes
	// must be checked too, not just the primary Recipe.
	ironPathInStock := func(r resource.Type) bool { return r == resource.IronOre || r == resource.Coal }
	if !Unlocked(Smeltery, noneProduced, ironPathInStock) {
		t.Fatal("Smeltery still locked with IronOre+Coal already on the stockpile (AltRecipes not checked?)")
	}
	// Half a recipe (ore but no Coal, or vice versa) must NOT unlock it.
	oreOnly := func(r resource.Type) bool { return r == resource.GoldOre || r == resource.IronOre }
	if Unlocked(Smeltery, noneProduced, oreOnly) {
		t.Fatal("Smeltery unlocked with ore in stock but no Coal at all")
	}
	// A Miner Hut alone (produces GoldOre, IronOre and Coal) unlocks it
	// with nothing in stock.
	minerHutBuilt := func(r resource.Type) bool {
		return r == resource.GoldOre || r == resource.IronOre || r == resource.Coal
	}
	if !Unlocked(Smeltery, minerHutBuilt, noneStocked) {
		t.Fatal("Smeltery still locked with a Miner Hut built and nothing in stock")
	}
}

func TestMissingInputs_NilWhenUnlockedOrNoRecipe(t *testing.T) {
	if got := MissingInputs(Farm, noneProduced, noneStocked); got != nil {
		t.Fatalf("Farm (no inputs at all) MissingInputs = %v, want nil", got)
	}
	wheatInStock := func(r resource.Type) bool { return r == resource.Wheat }
	if got := MissingInputs(PigFarm, noneProduced, wheatInStock); got != nil {
		t.Fatalf("Pig Farm already unlocked via stock, MissingInputs = %v, want nil", got)
	}
}

func TestMissingInputs_NamesTheActualMissingResource(t *testing.T) {
	got := MissingInputs(PigFarm, noneProduced, noneStocked)
	want := []resource.Type{resource.Wheat}
	if !slices.Equal(got, want) {
		t.Fatalf("Pig Farm MissingInputs = %v, want %v", got, want)
	}
}

func TestMissingInputs_SmelteryPicksTheCloserAltRecipe(t *testing.T) {
	// Coal alone leaves both recipes needing exactly one more thing each
	// (GoldOre or IronOre) -- either is a valid "closest" answer, but it
	// must be exactly one resource, not both ores combined.
	coalOnly := func(r resource.Type) bool { return r == resource.Coal }
	got := MissingInputs(Smeltery, noneProduced, coalOnly)
	if len(got) != 1 || (got[0] != resource.GoldOre && got[0] != resource.IronOre) {
		t.Fatalf("Smeltery MissingInputs with Coal already in stock = %v, want exactly one ore type", got)
	}

	// Nothing at all -- both recipes are equally (un)ready, but the result
	// must still be exactly one recipe's worth, not four resources merged.
	got = MissingInputs(Smeltery, noneProduced, noneStocked)
	if len(got) != 2 {
		t.Fatalf("Smeltery MissingInputs with nothing at all = %v, want exactly 2 (one recipe's worth)", got)
	}
}

func TestUnlocked_MeatWorkshopNeedsPigFarmOrCarcassInStock(t *testing.T) {
	if Unlocked(MeatWorkshop, noneProduced, noneStocked) {
		t.Fatal("Meat Workshop unlocked with no Pig Farm and no Carcass in stock")
	}
	carcassInStock := func(r resource.Type) bool { return r == resource.Carcass }
	if !Unlocked(MeatWorkshop, noneProduced, carcassInStock) {
		t.Fatal("Meat Workshop still locked with Carcass already on the stockpile")
	}
}
