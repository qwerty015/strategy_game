package meal

import (
	"testing"

	"strategy_game/internal/resource"
)

func TestSelectorChoosesFromWholeAvailableMenu(t *testing.T) {
	foods := []resource.Type{resource.Bread, resource.Fish, resource.Wine}
	selector := NewSelector(2)

	got, ok := selector.Pick(foods)
	if !ok || got != resource.Wine {
		t.Fatalf("Pick() = %v, %v; want Wine, true", got, ok)
	}
}

func TestSelectorResumesFromSavedSeed(t *testing.T) {
	foods := []resource.Type{resource.Bread, resource.Fish, resource.Wine}
	original := NewSelector(0x12345678)
	if _, ok := original.Pick(foods); !ok {
		t.Fatal("first Pick() failed")
	}
	saved := original.Seed()
	want, ok := original.Pick(foods)
	if !ok {
		t.Fatal("second Pick() failed")
	}

	restored := NewSelector(saved)
	got, ok := restored.Pick(foods)
	if !ok || got != want {
		t.Fatalf("restored Pick() = %v, %v; want %v, true", got, ok, want)
	}
}
