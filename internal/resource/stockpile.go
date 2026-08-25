package resource

// Stockpile is the shared warehouse inventory. A positive Capacity keeps the
// small logic-package tests useful for bounded stores; Capacity <= 0 means
// the actual town warehouse is unlimited.
type Stockpile struct {
	Capacity int // max units of any single resource type
	Amounts  map[Type]int
}

// NewStockpile creates an empty stockpile with the given per-resource
// capacity.
func NewStockpile(capacity int) *Stockpile {
	return &Stockpile{Capacity: capacity, Amounts: make(map[Type]int)}
}

// Amount returns how many units of t are currently stored.
func (s *Stockpile) Amount(t Type) int {
	return s.Amounts[t]
}

// Add deposits n units of t. Positive Capacity caps the amount; Capacity <= 0
// is unlimited and is used by the town warehouse.
func (s *Stockpile) Add(t Type, n int) int {
	if n <= 0 {
		return 0
	}
	if s.Amounts == nil {
		s.Amounts = make(map[Type]int)
	}
	if s.Capacity > 0 {
		room := s.Capacity - s.Amounts[t]
		if room <= 0 {
			return 0
		}
		if n > room {
			n = room
		}
	}
	s.Amounts[t] += n
	return n
}

// Has reports whether at least n units of t are available.
func (s *Stockpile) Has(t Type, n int) bool {
	return s.Amounts[t] >= n
}

// Remove deducts n units of t if available (all-or-nothing) and reports
// whether it succeeded. On failure, the stockpile is left unchanged.
func (s *Stockpile) Remove(t Type, n int) bool {
	if n <= 0 {
		return true
	}
	if s.Amounts[t] < n {
		return false
	}
	s.Amounts[t] -= n
	return true
}
