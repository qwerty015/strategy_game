package render

import (
	"strategy_game/internal/building"
	"testing"
)

func TestBuildingFireFollowsDamage(t *testing.T) {
	b := &building.Building{Kind: building.Warehouse, HP: 100}
	previous := 0.0
	for hp := 99; hp > 0; hp-- {
		b.HP = hp
		got := buildingFireIntensity(b)
		if got <= previous {
			t.Fatalf("fire did not increase at HP %d", hp)
		}
		previous = got
	}
	for _, hp := range []int{0, 100, 110} {
		b.HP = hp
		if buildingFireIntensity(b) != 0 {
			t.Fatalf("fire at HP %d", hp)
		}
	}
	b.HP = 50
	b.ConstructionStage = building.ConstructionFoundation
	if buildingFireIntensity(b) != 0 {
		t.Fatal("unfinished building on fire")
	}
	b.ConstructionStage = building.ConstructionNone
	for _, kind := range []building.Kind{building.Road, building.Tree, building.Fish, building.StoneDeposit} {
		b.Kind = kind
		if buildingFireIntensity(b) != 0 {
			t.Fatal("natural object or road on fire")
		}
	}
}
