package main

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/resource"
	"strategy_game/internal/soldier"
	"strategy_game/internal/world"
)

func TestArmoryCraftsOnlyWhileWeaponsmithIsAtPost(t *testing.T) {
	armory := &building.Building{
		Kind:            building.Armory,
		InputBuffer:     map[resource.Type]int{resource.Plank: 1},
		OutputBuffer:    map[resource.Type]int{},
		ProductionQueue: map[resource.Type]int{resource.Bow: 1},
	}
	buildings := []*building.Building{armory}

	// The separate Armory queue used to bypass this condition and manufacture
	// a bow with no Weaponsmith at all. Keep that regression impossible.
	tickArmories(buildings, map[*building.Building]bool{armory: true})
	if armory.ProgressTicks != 0 || armory.OutputBuffer[resource.Bow] != 0 || armory.InputBuffer[resource.Plank] != 1 {
		t.Fatalf("unstaffed armory advanced: progress=%d bow=%d plank=%d", armory.ProgressTicks, armory.OutputBuffer[resource.Bow], armory.InputBuffer[resource.Plank])
	}

	for range armoryTicksToProduce {
		tickArmories(buildings, map[*building.Building]bool{armory: false})
	}
	if got := armory.OutputBuffer[resource.Bow]; got != 1 {
		t.Fatalf("staffed armory bow output = %d, want 1", got)
	}
	if got := armory.InputBuffer[resource.Plank]; got != 0 {
		t.Fatalf("staffed armory plank input = %d, want 0", got)
	}
	if got := armory.ProductionQueue[resource.Bow]; got != 0 {
		t.Fatalf("bow queue after completion = %d, want 0", got)
	}
}

func TestHireEquippedSoldiersConsumesExactlyTheirEquipment(t *testing.T) {
	grid := world.NewGrid(12, 12)
	barracks := &building.Building{
		Kind: building.Barracks,
		X:    5,
		Y:    5,
		InputBuffer: map[resource.Type]int{
			resource.Gold:         unitHireCost,
			resource.Bow:          1,
			resource.LeatherArmor: 1,
		},
	}
	game := &Game{grid: grid, buildings: []*building.Building{barracks}, soldiers: soldier.NewController()}

	game.hireArcher(barracks)
	if got := len(game.soldiers.Soldiers); got != 1 || game.soldiers.Soldiers[0].Profession != soldier.Archer {
		t.Fatalf("archer roster = %+v, want one Archer", game.soldiers.Soldiers)
	}
	for _, item := range []resource.Type{resource.Gold, resource.Bow, resource.LeatherArmor} {
		if got := barracks.InputBuffer[item]; got != 0 {
			t.Fatalf("archer hire left %v=%d, want 0", item, got)
		}
	}

	barracks.InputBuffer[resource.Gold] = unitHireCost
	barracks.InputBuffer[resource.Sword] = 1
	barracks.InputBuffer[resource.LeatherArmor] = 1
	game.hireSwordsman(barracks)
	if got := len(game.soldiers.Soldiers); got != 2 || game.soldiers.Soldiers[1].Profession != soldier.Swordsman {
		t.Fatalf("swordsman roster = %+v, want Archer plus Swordsman", game.soldiers.Soldiers)
	}
	for _, item := range []resource.Type{resource.Gold, resource.Sword, resource.LeatherArmor} {
		if got := barracks.InputBuffer[item]; got != 0 {
			t.Fatalf("swordsman hire left %v=%d, want 0", item, got)
		}
	}
}
