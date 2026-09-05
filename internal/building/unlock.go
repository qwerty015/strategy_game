package building

import (
	"slices"

	"strategy_game/internal/resource"
)

// Unlocked reports whether kind's production chain is actually ready to
// be built -- a real playtest gap: the palette used to offer every
// building at once regardless of the economy's actual state, so a Pig
// Farm (needs Wheat) could be placed with no Farm anywhere, or a
// Smeltery (needs ore + Coal) with no Miner Hut ever built.
//
// A kind with no recipe inputs at all (Farm, Winery, FisherHut, Lumberjack
// Hut, Quarry Hut, Miner Hut, and every building with no Recipe/AltRecipes
// at all -- Warehouse, Road, Tavern, StoneWall, Gate, WatchTower, Barracks,
// Armory) is always unlocked; there is nothing to wait for.
//
// A kind with input requirements is unlocked once at least one of its
// recipes (see Type.AllRecipes -- the Smeltery has two, gold and iron) has
// every input resource covered by hasProducer (a completed building that
// already outputs it) OR hasStock (already banked on the stockpile, with
// no producer needed at all). The stock half of that check matters on its
// own: per an explicit user report, a player who already gathered enough
// ore/coal/logs onto the stockpile -- say the gathering hut was later
// demolished, or was never actually needed for that much material -- can
// still build the Smeltery or Carpentry Workshop straight away, without
// being forced to keep (or rebuild) a Miner Hut/Lumberjack Hut they no
// longer need just to satisfy this check.
func Unlocked(kind Kind, hasProducer, hasStock func(resource.Type) bool) bool {
	recipes := Types[kind].AllRecipes()
	if len(recipes) == 0 {
		return true
	}
	for _, r := range recipes {
		if recipeInputsCovered(r, hasProducer, hasStock) {
			return true
		}
	}
	return false
}

// MissingInputs explains WHY kind is currently locked -- a real follow-up
// gap from a playtest report: greying out a card wasn't visible enough on
// its own ("визуально нельзя отличить то можно построить сейчас от того
// что нельзя"), so a locked card's tooltip needs to say what's actually
// missing (the user's own example: "отсутствует построенная ферма или
// пшеница на складе").
//
// Returns the missing resource types for whichever of kind's recipes is
// closest to ready (fewest resources still missing) -- for the Smeltery's
// two alt-recipes, this avoids combining "no gold ore" and "no iron ore"
// into one confusing list when only one of the two recipes actually needs
// to become satisfiable. Returns nil once kind is Unlocked (there is
// nothing to explain), and nil for a no-input kind (nothing was ever
// missing). Order is deterministic (sorted by resource.Type) since
// Recipe.Inputs is a map -- a tooltip listing the same two resources in a
// different order every frame would be distracting.
func MissingInputs(kind Kind, hasProducer, hasStock func(resource.Type) bool) []resource.Type {
	recipes := Types[kind].AllRecipes()
	var best []resource.Type
	for _, r := range recipes {
		var missing []resource.Type
		for resType := range r.Inputs {
			if !hasProducer(resType) && !hasStock(resType) {
				missing = append(missing, resType)
			}
		}
		if len(missing) == 0 {
			return nil // this recipe alone already makes kind Unlocked
		}
		if best == nil || len(missing) < len(best) {
			best = missing
		}
	}
	slices.Sort(best)
	return best
}

// recipeInputsCovered reports whether every input resource in r.Inputs is
// individually satisfied -- see Unlocked's doc comment for what "covered"
// means. A recipe with no Inputs at all (a Farm's, say) is vacuously
// covered: the loop below never runs, so this returns true.
func recipeInputsCovered(r Recipe, hasProducer, hasStock func(resource.Type) bool) bool {
	for resType := range r.Inputs {
		if !hasProducer(resType) && !hasStock(resType) {
			return false
		}
	}
	return true
}
