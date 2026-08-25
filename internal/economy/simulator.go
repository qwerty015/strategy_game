// Package economy ticks buildings' production and the town's population
// forward. It knows nothing about ebiten or rendering -- see AGENTS.md.
package economy

import (
	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

// Simulator advances the economy on a fixed simulation step, independent
// of the render framerate, so production speed doesn't drift with FPS.
type Simulator struct {
	FramesPerTick int // render frames per simulation tick
	frameAccum    int
}

// NewSimulator creates a Simulator that runs one simulation tick every
// framesPerTick calls to Update.
func NewSimulator(framesPerTick int) *Simulator {
	return &Simulator{FramesPerTick: framesPerTick}
}

// Update should be called once per render frame. It runs zero or one
// simulation tick depending on accumulated frames.
func (s *Simulator) Update(buildings []*building.Building, stock *resource.Stockpile, pop *Population) {
	s.frameAccum++
	if s.frameAccum < s.FramesPerTick {
		return
	}
	s.frameAccum = 0
	Tick(buildings, stock, pop)
}

// Tick runs exactly one simulation step: every building advances its
// production progress and, once a cycle completes and its inputs are
// available, consumes them and deposits its output into the shared
// stockpile. The population then eats, if it's mealtime.
func Tick(buildings []*building.Building, stock *resource.Stockpile, pop *Population) {
	for _, b := range buildings {
		recipe := building.Types[b.Kind].Recipe

		if b.ProgressTicks < recipe.TicksToProduce {
			b.ProgressTicks++
		}
		if b.ProgressTicks < recipe.TicksToProduce {
			continue
		}

		// Production cycle complete; hold here until inputs are
		// available rather than restarting the timer, so a building
		// starved of raw materials produces the instant they arrive.
		if !hasAllInputs(stock, recipe.Inputs) {
			continue
		}
		for t, n := range recipe.Inputs {
			stock.Remove(t, n)
		}
		stock.Add(recipe.Output, recipe.OutputAmount)
		b.ProgressTicks = 0
	}

	if pop != nil {
		pop.Tick(stock)
	}
}

func hasAllInputs(stock *resource.Stockpile, inputs map[resource.Type]int) bool {
	for t, n := range inputs {
		if !stock.Has(t, n) {
			return false
		}
	}
	return true
}
