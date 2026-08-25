package resource

import "testing"

func TestStockpile_AddRespectsCapacity(t *testing.T) {
	tests := []struct {
		name      string
		capacity  int
		preload   int
		add       int
		wantAdded int
		wantTotal int
	}{
		{"room for all", 100, 0, 30, 30, 30},
		{"partial room", 100, 90, 30, 10, 100},
		{"already full", 100, 100, 30, 0, 100},
		{"zero amount", 100, 10, 0, 0, 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStockpile(tc.capacity)
			s.Add(Wheat, tc.preload)

			got := s.Add(Wheat, tc.add)
			if got != tc.wantAdded {
				t.Errorf("Add returned %d, want %d", got, tc.wantAdded)
			}
			if s.Amount(Wheat) != tc.wantTotal {
				t.Errorf("Amount = %d, want %d", s.Amount(Wheat), tc.wantTotal)
			}
		})
	}
}

func TestStockpile_RemoveIsAllOrNothing(t *testing.T) {
	s := NewStockpile(100)
	s.Add(Wheat, 5)

	if ok := s.Remove(Wheat, 10); ok {
		t.Fatal("Remove(10) on 5 available succeeded, want failure")
	}
	if got := s.Amount(Wheat); got != 5 {
		t.Fatalf("Amount after failed Remove = %d, want unchanged 5", got)
	}

	if ok := s.Remove(Wheat, 5); !ok {
		t.Fatal("Remove(5) on 5 available failed, want success")
	}
	if got := s.Amount(Wheat); got != 0 {
		t.Fatalf("Amount after successful Remove = %d, want 0", got)
	}
}

func TestStockpile_Has(t *testing.T) {
	s := NewStockpile(100)
	s.Add(Bread, 3)

	if !s.Has(Bread, 3) {
		t.Error("Has(3) with 3 available = false, want true")
	}
	if s.Has(Bread, 4) {
		t.Error("Has(4) with 3 available = true, want false")
	}
}
