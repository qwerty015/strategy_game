package economy

import "strategy_game/internal/resource"

// Population tracks how many villagers the town supports and their food
// need. Kept intentionally simple for the MVP: a running headcount and a
// meal timer, no individual villager entities or growth mechanic yet --
// enough to make Bread matter.
type Population struct {
	Count          int
	TicksPerMeal   int // simulation ticks between meals
	TicksSinceMeal int
}

// NewPopulation creates a population of count villagers who each eat one
// unit of Bread every ticksPerMeal simulation ticks.
func NewPopulation(count, ticksPerMeal int) *Population {
	return &Population{Count: count, TicksPerMeal: ticksPerMeal}
}

// Tick should be called once per simulation tick. Every TicksPerMeal
// ticks, the whole population eats one unit of Bread per villager from
// the stockpile; if there isn't enough Bread to feed everyone, the town
// loses one villager to starvation instead.
func (p *Population) Tick(stock *resource.Stockpile) {
	if p.Count <= 0 {
		return
	}
	p.TicksSinceMeal++
	if p.TicksSinceMeal < p.TicksPerMeal {
		return
	}
	p.TicksSinceMeal = 0

	if stock.Remove(resource.Bread, p.Count) {
		return
	}
	p.Count--
}
