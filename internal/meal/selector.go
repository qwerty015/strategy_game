// Package meal provides the deterministic random choice used when a hungry
// unit chooses one of several foods available in a Tavern.
package meal

import "strategy_game/internal/resource"

const fallbackSeed uint32 = 0x9e3779b9

// Selector chooses uniformly from the foods offered to it. Its small
// xorshift state is exposed for save/load, so loading a game does not replace
// the simulation's choice sequence with wall-clock randomness.
type Selector struct {
	seed uint32
}

// NewSelector creates a selector with seed. Zero is replaced with a stable
// non-zero seed because xorshift has an all-zero fixed point.
func NewSelector(seed uint32) Selector {
	var s Selector
	s.SetSeed(seed)
	return s
}

// Seed returns the state that must be persisted to resume future choices.
func (s *Selector) Seed() uint32 { return s.seed }

// SetSeed restores a previously saved selector state.
func (s *Selector) SetSeed(seed uint32) {
	if seed == 0 {
		seed = fallbackSeed
	}
	s.seed = seed
}

// Pick returns one uniformly pseudo-random food from available. It advances
// only after a real choice, so a missing Tavern or an empty menu has no hidden
// effect on the sequence.
func (s *Selector) Pick(available []resource.Type) (resource.Type, bool) {
	if len(available) == 0 {
		return 0, false
	}
	pick := available[int(s.seed%uint32(len(available)))]
	s.seed = nextSeed(s.seed)
	return pick, true
}

func nextSeed(seed uint32) uint32 {
	seed ^= seed << 13
	seed ^= seed >> 17
	seed ^= seed << 5
	return seed
}
