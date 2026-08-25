package building

import "testing"

func TestTreeGrowthIsStableAndStopsAtMaturity(t *testing.T) {
	a := NewTree(7, 11)
	b := NewTree(7, 11)
	if a.GrowthTargetTicks != b.GrowthTargetTicks {
		t.Fatalf("same tree coordinates produced different targets: %d and %d", a.GrowthTargetTicks, b.GrowthTargetTicks)
	}
	for i := 0; i < a.GrowthTargetTicks+20; i++ {
		a.TickGrowth()
	}
	if a.GrowthTicks != a.GrowthTargetTicks {
		t.Fatalf("GrowthTicks = %d, want target %d", a.GrowthTicks, a.GrowthTargetTicks)
	}
	if got := a.GrowthProgress(); got != 1 {
		t.Fatalf("GrowthProgress = %v, want 1", got)
	}
	if got := a.GrowthStage(); got != 2 {
		t.Fatalf("GrowthStage = %d, want mature stage 2", got)
	}
}
