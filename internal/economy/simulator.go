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
	Octuple
	Sixteenfold
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
	case Octuple:
		return 8
	case Sixteenfold:
		return 16
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
	if speed < Paused || speed > Sixteenfold {
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
		// A building still under construction (see package builder) isn't
		// a working producer yet -- its ProgressTicks/InputBuffer are
		// tracking construction progress and delivered materials, not
		// this recipe's normal meaning.
		if b.ConstructionStage != building.ConstructionNone {
			continue
		}
		recipes := building.Types[b.Kind].AllRecipes()
		if len(recipes) == 0 || starving[b] || disconnected[b] {
			continue
		}
		if len(recipes) > 1 {
			// The Smeltery, currently the only multi-recipe building --
			// see pickRecipe's doc comment for why it can't just reuse the
			// single-recipe path below unchanged.
			tickMultiRecipe(b, recipes)
			continue
		}
		recipe := recipes[0]
		if recipe.ConsumeInputsAtStart {
			tickPrepaidRecipe(b, recipe)
			continue
		}
		tickRecipe(b, recipe)
	}
}

// tickRecipe is the ordinary (not prepaid, single-recipe) production cycle:
// count up to TicksToProduce, then hold there -- rather than restart the
// timer -- until inputs are available and there's room for the output, so a
// building starved of raw materials or blocked by a full OutputBuffer
// produces the instant a serf clears things.
func tickRecipe(b *building.Building, recipe building.Recipe) {
	if b.ProgressTicks < recipe.TicksToProduce {
		b.ProgressTicks++
	}
	if b.ProgressTicks < recipe.TicksToProduce {
		return
	}
	if !hasAllInputs(b, recipe.Inputs) {
		return
	}
	if !hasOutputRoom(b, recipe) {
		return
	}
	consumeInputs(b, recipe.Inputs)
	addRecipeOutputs(b, recipe)
	b.ProgressTicks = 0
}

// tickMultiRecipe is tickRecipe generalized to more than one possible
// recipe (the Smeltery: smelt gold ore or smelt iron ore, whichever is on
// hand -- see Building.ActiveRecipe and Type.AltRecipes). Unlike a
// single-recipe building, progress only starts once a specific recipe's
// inputs are already satisfied: there's no sensible "spin blindly" default,
// the way an ordinary Mill ticks even without wheat, when it isn't yet
// known which recipe will even run this cycle.
func tickMultiRecipe(b *building.Building, recipes []building.Recipe) {
	if b.ProgressTicks == 0 {
		idx := pickRecipe(b, recipes)
		if idx < 0 {
			return
		}
		b.ActiveRecipe = idx
	}
	if b.ActiveRecipe < 0 || b.ActiveRecipe >= len(recipes) {
		b.ActiveRecipe = 0
	}
	recipe := recipes[b.ActiveRecipe]
	if b.ProgressTicks < recipe.TicksToProduce {
		b.ProgressTicks++
	}
	if b.ProgressTicks < recipe.TicksToProduce {
		return
	}
	if !hasAllInputs(b, recipe.Inputs) || !hasOutputRoom(b, recipe) {
		return
	}
	consumeInputs(b, recipe.Inputs)
	addRecipeOutputs(b, recipe)
	b.ProgressTicks = 0
}

// pickRecipe returns the index of the next recipe -- starting just after
// b.ActiveRecipe and wrapping around -- that currently has all its inputs
// available, or -1 if none do. Starting after the last one used, rather
// than always index 0, is what keeps a Smeltery from starving Iron every
// single cycle Gold ore also happens to be on hand: the two recipes take
// turns instead of Gold (recipe index 0) always winning ties.
func pickRecipe(b *building.Building, recipes []building.Recipe) int {
	for i := 1; i <= len(recipes); i++ {
		idx := (b.ActiveRecipe + i) % len(recipes)
		if hasAllInputs(b, recipes[idx].Inputs) {
			return idx
		}
	}
	return -1
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
	addRecipeOutputs(b, recipe)
	b.ProgressTicks = 0
}

// addRecipeOutputs deposits a completed cycle's product(s): always
// Output/OutputAmount, plus SecondaryOutput/SecondaryOutputAmount when
// present (currently only the PigFarm's Hide alongside its Carcass -- see
// building.Recipe.SecondaryOutput's doc comment). Room for both was
// already confirmed by hasOutputRoom before any caller reaches this.
func addRecipeOutputs(b *building.Building, recipe building.Recipe) {
	b.AddOutput(recipe.Output, recipe.OutputAmount)
	if recipe.SecondaryOutputAmount > 0 {
		b.AddOutput(recipe.SecondaryOutput, recipe.SecondaryOutputAmount)
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

func hasOutputRoom(b *building.Building, recipe building.Recipe) bool {
	if b.OutputBuffer[recipe.Output]+recipe.OutputAmount > b.OutputLimit() {
		return false
	}
	if recipe.SecondaryOutputAmount > 0 && b.OutputBuffer[recipe.SecondaryOutput]+recipe.SecondaryOutputAmount > b.OutputLimit() {
		return false
	}
	return true
}

func consumeInputs(b *building.Building, inputs map[resource.Type]int) {
	for t, n := range inputs {
		b.TakeInput(t, n)
	}
}
