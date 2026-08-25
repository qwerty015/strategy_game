package building

import (
	"testing"

	"strategy_game/internal/resource"
)

func TestProductionRatios(t *testing.T) {
	mill := Types[Mill].Recipe
	if got := mill.Inputs[resource.Wheat]; got != 1 {
		t.Fatalf("mill Wheat input = %d, want 1", got)
	}
	if mill.OutputAmount != 1 || mill.Output != resource.Flour {
		t.Fatalf("mill recipe = %+v, want 1 Flour", mill)
	}

	bakery := Types[Bakery].Recipe
	if got := bakery.Inputs[resource.Flour]; got != 1 {
		t.Fatalf("bakery Flour input = %d, want 1", got)
	}
	if bakery.OutputAmount != 2 || bakery.Output != resource.Bread {
		t.Fatalf("bakery recipe = %+v, want 2 Bread", bakery)
	}
}
