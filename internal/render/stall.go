package render

import (
	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

// stallReason is why a staffed, connected production building isn't
// currently advancing -- distinct from simply being unstaffed (see
// unstaffedTint) or mid-cycle (neither counts as "stalled", it just
// hasn't reached the end of its timer yet). The player picked both of
// these as states worth surfacing on the map, on top of the
// already-existing unstaffed/under-construction/disconnected tints.
type stallReason int

const (
	stallNone stallReason = iota
	// stallNoInput: the building's timer is done, but it's short a raw
	// material it needs to actually complete the cycle.
	stallNoInput
	// stallOutputFull: the building has everything it needs but nowhere
	// to put the result -- its OutputBuffer is already at capacity,
	// which means nobody is coming to collect it.
	stallOutputFull
)

// buildingStallReason mirrors economy.tickRecipe/tickPrepaidRecipe/
// tickMultiRecipe's own stall conditions closely enough that this tint
// always agrees with what TickWithConnectivity is actually doing --
// it deliberately reads the same building fields (ProgressTicks,
// InputBuffer, OutputBuffer, ActiveRecipe) those functions do, rather
// than inventing a separate approximate notion of "stalled". It does not
// import package economy itself: hasAllInputs/hasOutputRoom there are
// unexported one-line map lookups, cheaper to mirror than to justify a
// new render->economy dependency for.
//
// A building still under construction, with no recipe at all
// (Warehouse/Road/Tavern/...), or not yet past its first full cycle of
// this tick (ProgressTicks below TicksToProduce -- there's still time
// for a serf to deliver the missing input before the timer completes, so
// nothing is actually blocked yet) reports stallNone.
func buildingStallReason(b *building.Building) stallReason {
	if b == nil || b.ConstructionStage != building.ConstructionNone {
		return stallNone
	}
	recipes := building.Types[b.Kind].AllRecipes()
	if len(recipes) == 0 {
		return stallNone
	}
	if len(recipes) > 1 {
		return multiRecipeStall(b, recipes)
	}
	recipe := recipes[0]
	if recipe.ConsumeInputsAtStart {
		return prepaidRecipeStall(b, recipe)
	}
	return ordinaryRecipeStall(b, recipe)
}

// ordinaryRecipeStall mirrors economy.tickRecipe: the progress counter
// climbs regardless of input stock, and only the check right at
// TicksToProduce can actually block a cycle.
func ordinaryRecipeStall(b *building.Building, recipe building.Recipe) stallReason {
	if b.ProgressTicks < recipe.TicksToProduce {
		return stallNone
	}
	if !hasAllInputs(b, recipe.Inputs) {
		return stallNoInput
	}
	if !hasOutputRoom(b, recipe) {
		return stallOutputFull
	}
	return stallNone
}

// prepaidRecipeStall mirrors economy.tickPrepaidRecipe: inputs are only
// relevant at the very start of a cycle (ProgressTicks == 0) -- an empty
// InputBuffer while ProgressTicks > 0 is the normal "already fed, growing"
// state (see the PigFarm), not a stall.
func prepaidRecipeStall(b *building.Building, recipe building.Recipe) stallReason {
	if b.ProgressTicks == 0 {
		if !hasAllInputs(b, recipe.Inputs) {
			return stallNoInput
		}
		if !hasOutputRoom(b, recipe) {
			return stallOutputFull
		}
		return stallNone
	}
	if b.ProgressTicks >= recipe.TicksToProduce && !hasOutputRoom(b, recipe) {
		return stallOutputFull
	}
	return stallNone
}

// multiRecipeStall mirrors economy.tickMultiRecipe (currently only the
// Smeltery): stallNoInput only when *no* recipe has enough input to even
// start a cycle, matching pickRecipe's own scan.
func multiRecipeStall(b *building.Building, recipes []building.Recipe) stallReason {
	if b.ProgressTicks == 0 {
		for _, r := range recipes {
			if hasAllInputs(b, r.Inputs) {
				return stallNone
			}
		}
		return stallNoInput
	}
	idx := b.ActiveRecipe
	if idx < 0 || idx >= len(recipes) {
		idx = 0
	}
	recipe := recipes[idx]
	if b.ProgressTicks >= recipe.TicksToProduce && !hasOutputRoom(b, recipe) {
		return stallOutputFull
	}
	return stallNone
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
