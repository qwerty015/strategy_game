package main

import (
	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

// armoryTicksToProduce is how long one Bow/LeatherArmor/Sword takes once
// its raw material is on hand -- picked to match the Smeltery/Bakery scale
// (60 ticks), the closest existing "workshop converts one input into one
// finished good" comparison.
const armoryTicksToProduce = 60

// armoryOrder is the fixed priority an Armory works its three queues in:
// whichever of these comes first with a nonempty ProductionQueue entry is
// what's actively being crafted this tick. Simple and predictable for the
// player, matching how a queue reads top-to-bottom, rather than a fairness
// rotation across the three lines.
var armoryOrder = []resource.Type{resource.Bow, resource.LeatherArmor, resource.Sword}

// armoryCost is each queued item's raw-material cost, per the user's
// explicit "Производство лука: 1 ед доска. Кожаный доспех: 1 единица
// шкура. Меч: 1 железо, 1 уголь." Not a building.Recipe on Types[Armory]
// itself -- see building.go's Armory doc comment for why three parallel
// queues don't fit that model.
var armoryCost = map[resource.Type]map[resource.Type]int{
	resource.Bow:          {resource.Plank: 1},
	resource.LeatherArmor: {resource.Hide: 1},
	resource.Sword:        {resource.Iron: 1, resource.Coal: 1},
}

// tickArmories advances every finished, staffed Armory's production queue by
// one simulation tick. inactive is the same worker-presence map economy uses:
// an empty Armory, or one whose Weaponsmith left to eat, must pause exactly
// like every other worker building.
func tickArmories(buildings []*building.Building, inactive map[*building.Building]bool) {
	for _, b := range buildings {
		if b == nil || b.Kind != building.Armory || b.ConstructionStage != building.ConstructionNone || inactive[b] {
			continue
		}
		tickOneArmory(b)
	}
}

// armoryHasMaterial reports whether b's InputBuffer currently holds every
// raw material armoryCost[item] requires.
func armoryHasMaterial(b *building.Building, item resource.Type) bool {
	for rt, n := range armoryCost[item] {
		if b.InputBuffer[rt] < n {
			return false
		}
	}
	return true
}

func tickOneArmory(b *building.Building) {
	item, ok := activeArmoryItem(b)
	if !ok {
		b.ProgressTicks = 0
		return
	}
	if !armoryHasMaterial(b, item) {
		// Waiting on delivery -- the queue entry stays put, no progress is
		// lost, it just doesn't advance this tick.
		return
	}
	b.ProgressTicks++
	if b.ProgressTicks < armoryTicksToProduce {
		return
	}
	b.ProgressTicks = 0
	if !armoryHasMaterial(b, item) {
		return // ran out exactly at completion -- retry once resupplied
	}
	if b.AddOutput(item, 1) == 0 {
		// OutputBuffer is full (a serf hasn't cleared it yet) -- hold this
		// cycle's completion rather than silently discard the item.
		b.ProgressTicks = armoryTicksToProduce
		return
	}
	for rt, n := range armoryCost[item] {
		b.TakeInput(rt, n)
	}
	b.ProductionQueue[item]--
	if b.ProductionQueue[item] <= 0 {
		delete(b.ProductionQueue, item)
	}
}

// activeArmoryItem returns the first item (in armoryOrder) with a positive
// queue count, if any.
func activeArmoryItem(b *building.Building) (resource.Type, bool) {
	for _, item := range armoryOrder {
		if b.ProductionQueue[item] > 0 {
			return item, true
		}
	}
	return 0, false
}

// adjustArmoryQueue changes row's queued count by delta (already ±1 for a
// left-click, or the caller's ±10 for a right-click -- see the user's
// explicit "клик ПКМ +/- 10 штук в очередь, ЛКМ +/- 1 в очередь"), clamped
// at zero. row indexes armoryQueueItems in internal/ui/panels.go, which
// must stay in the same Bow/LeatherArmor/Sword order as armoryOrder here.
func (g *Game) adjustArmoryQueue(armory *building.Building, row, delta int) {
	if armory == nil || row < 0 || row >= len(armoryOrder) {
		return
	}
	item := armoryOrder[row]
	if armory.ProductionQueue == nil {
		armory.ProductionQueue = map[resource.Type]int{}
	}
	n := armory.ProductionQueue[item] + delta
	if n <= 0 {
		delete(armory.ProductionQueue, item)
		return
	}
	armory.ProductionQueue[item] = n
}
