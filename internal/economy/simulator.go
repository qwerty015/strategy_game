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
	frameAccum    float64
	speed         Speed
}

// Speed is the simulation speed selected by the player. Rendering and input
// continue while the simulation is paused.
type Speed int

const (
	Paused Speed = iota
	Half
	Normal
	Double
	Quadruple
)

func (s Speed) multiplier() float64 {
	switch s {
	case Paused:
		return 0
	case Half:
		return 0.5
	case Double:
		return 2
	case Quadruple:
		return 4
	default:
		return 1
	}
}

// NewSimulator creates a Simulator that allows one simulation tick every
// framesPerTick calls to ShouldTick.
func NewSimulator(framesPerTick int) *Simulator {
	return &Simulator{FramesPerTick: framesPerTick, speed: Normal}
}

// SetSpeed changes the simulation speed. Invalid values are treated as the
// normal speed so a corrupted UI state cannot stop the simulation forever.
func (s *Simulator) SetSpeed(speed Speed) {
	if speed < Paused || speed > Quadruple {
		speed = Normal
	}
	s.speed = speed
}

// Speed reports the currently selected simulation speed.
func (s *Simulator) Speed() Speed {
	return s.speed
}

// Advance returns how many fixed simulation ticks should run during this
// render frame. At high speeds it may return more than one; every returned
// tick must be passed through economy, logistics, and villagers together.
func (s *Simulator) Advance() int {
	if s.FramesPerTick <= 0 || s.speed == Paused {
		return 0
	}

	s.frameAccum += s.speed.multiplier()
	ticks := 0
	for s.frameAccum >= float64(s.FramesPerTick) {
		s.frameAccum -= float64(s.FramesPerTick)
		ticks++
	}
	return ticks
}

// ShouldTick is the backwards-compatible single-tick view of Advance. New
// callers should use Advance so high speed settings can run more than one
// fixed simulation tick during a render frame.
func (s *Simulator) ShouldTick() bool {
	return s.Advance() > 0
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
// materials. nil is fine (nothing is starving). This compatibility wrapper
// treats all buildings as connected; the game loop should use
// TickWithConnectivity.
func Tick(buildings []*building.Building, starving map[*building.Building]bool) {
	TickWithConnectivity(buildings, starving, nil)
}

// TickWithConnectivity is Tick plus the town-network rule: entries in
// disconnected are not allowed to start a production cycle. Existing output
// and input buffers remain intact, so reconnecting the road network resumes
// the building instead of destroying its progress.
func TickWithConnectivity(buildings []*building.Building, starving, disconnected map[*building.Building]bool) {
	for _, b := range buildings {
		recipe := building.Types[b.Kind].Recipe
		if recipe.TicksToProduce <= 0 || starving[b] || disconnected[b] {
			continue
		}
		if recipe.ConsumeInputsAtStart {
			tickPrepaidRecipe(b, recipe)
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
		if !hasOutputRoom(b, recipe) {
			continue
		}

		consumeInputs(b, recipe.Inputs)
		b.AddOutput(recipe.Output, recipe.OutputAmount)
		b.ProgressTicks = 0
	}
}

// tickPrepaidRecipe advances a recipe whose inputs must be spent before its
// timer runs. The PigFarm uses this for feed: the player sees an empty input
// buffer while the already-fed pig grows, rather than an implausible instant
// animal appearing when wheat arrives after 600 ticks of empty waiting.
func tickPrepaidRecipe(b *building.Building, recipe building.Recipe) {
	if b.ProgressTicks == 0 {
		if !hasAllInputs(b, recipe.Inputs) || !hasOutputRoom(b, recipe) {
			return
		}
		consumeInputs(b, recipe.Inputs)
	}

	if b.ProgressTicks < recipe.TicksToProduce {
		b.ProgressTicks++
	}
	if b.ProgressTicks < recipe.TicksToProduce || !hasOutputRoom(b, recipe) {
		return
	}
	b.AddOutput(recipe.Output, recipe.OutputAmount)
	b.ProgressTicks = 0
}

func hasAllInputs(b *building.Building, inputs map[resource.Type]int) bool {
	for t, n := range inputs {
		if b.InputBuffer[t] < n {
			return false
		}
	}
	return true
}

func hasOutputRoom(b *building.Building, recipe building.Recipe) bool {
	return b.OutputBuffer[recipe.Output]+recipe.OutputAmount <= b.OutputLimit()
}

func consumeInputs(b *building.Building, inputs map[resource.Type]int) {
	for t, n := range inputs {
		b.TakeInput(t, n)
	}
}
