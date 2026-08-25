// Package economy ticks buildings' production forward. It knows nothing
// about ebiten or rendering -- see AGENTS.md.
package economy

import (
	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

// Simulator paces fixed simulation ticks against the render framerate,
// so production speed doesn't drift with FPS.
type Simulator struct {
	FramesPerTick int // render frames per simulation tick
	frameAccum    int
}

// NewSimulator creates a Simulator that allows one simulation tick every
// framesPerTick calls to ShouldTick.
func NewSimulator(framesPerTick int) *Simulator {
	return &Simulator{FramesPerTick: framesPerTick}
}

// ShouldTick should be called once per render frame. It returns true at
// most once every FramesPerTick frames, telling the caller to run one
// simulation step (economy.Tick, logistics.Controller.Tick, ...).
func (s *Simulator) ShouldTick() bool {
	s.frameAccum++
	if s.frameAccum < s.FramesPerTick {
		return false
	}
	s.frameAccum = 0
	return true
}

// Tick runs exactly one simulation step for every building's production.
// A building reads its recipe's inputs from its own InputBuffer and
// writes its output to its own OutputBuffer -- it never touches the
// shared warehouse stockpile directly, that's what serfs are for (see
// package logistics). Buildings with no Recipe (Warehouse, Road,
// Tavern) are skipped.
//
// starving marks buildings whose worker (package villagers) is off
// finding a meal and hasn't come back yet -- production pauses for
// those, same as it would if the building were simply short on raw
// materials. nil is fine (nothing is starving).
func Tick(buildings []*building.Building, starving map[*building.Building]bool) {
	for _, b := range buildings {
		recipe := building.Types[b.Kind].Recipe
		if recipe.TicksToProduce <= 0 || starving[b] {
			continue
		}

		if b.ProgressTicks < recipe.TicksToProduce {
			b.ProgressTicks++
		}
		if b.ProgressTicks < recipe.TicksToProduce {
			continue
		}

		// Production cycle complete; hold here (rather than restart the
		// timer) until inputs are available and there's room for the
		// output, so a building starved of raw materials or blocked by
		// a full OutputBuffer produces the instant a serf clears things.
		if !hasAllInputs(b, recipe.Inputs) {
			continue
		}
		if b.OutputBuffer[recipe.Output]+recipe.OutputAmount > building.BufferCapacity {
			continue
		}

		for t, n := range recipe.Inputs {
			b.TakeInput(t, n)
		}
		b.AddOutput(recipe.Output, recipe.OutputAmount)
		b.ProgressTicks = 0
	}
}

func hasAllInputs(b *building.Building, inputs map[resource.Type]int) bool {
	for t, n := range inputs {
		if b.InputBuffer[t] < n {
			return false
		}
	}
	return true
}
