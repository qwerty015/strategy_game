package resource

import "testing"

func TestUnlimitedStockpileDoesNotCapResource(t *testing.T) {
	stock := NewStockpile(0)

	if got := stock.Add(Wheat, 1000); got != 1000 {
		t.Fatalf("Add returned %d, want 1000", got)
	}
	if got := stock.Amount(Wheat); got != 1000 {
		t.Fatalf("Wheat amount = %d, want 1000", got)
	}
}

func TestPositiveStockpileCapacityRemainsBoundedForLogicTests(t *testing.T) {
	stock := NewStockpile(3)

	if got := stock.Add(Wheat, 10); got != 3 {
		t.Fatalf("bounded Add returned %d, want 3", got)
	}
}
