package building

import (
	"strategy_game/internal/resource"
)

// Types is the registry of every building kind in the game. This is the
// data that defines the whole economy: to add a new production chain
// later (winery, sawmill, ...), add entries here -- no other package
// needs to change.
var Types = map[Kind]Type{
	Farm: {
		Kind:      Farm,
		Name:      "Farm",
		Footprint: 3, // one tile is the farmhouse, the rest is tilled field around it -- see render/buildings.go
		Recipe: Recipe{
			// No Inputs: a Farm gathers Wheat from the land itself.
			Output:         resource.Wheat,
			OutputAmount:   5,
			TicksToProduce: 4,
		},
	},
	Mill: {
		Kind:      Mill,
		Name:      "Mill",
		Footprint: 1,
		Recipe: Recipe{
			Inputs:         map[resource.Type]int{resource.Wheat: 5},
			Output:         resource.Flour,
			OutputAmount:   5,
			TicksToProduce: 3,
		},
	},
	Bakery: {
		Kind:      Bakery,
		Name:      "Bakery",
		Footprint: 1,
		Recipe: Recipe{
			Inputs:         map[resource.Type]int{resource.Flour: 5},
			Output:         resource.Bread,
			OutputAmount:   5,
			TicksToProduce: 3,
		},
	},
	Warehouse: {
		Kind:      Warehouse,
		Name:      "Warehouse",
		Footprint: 1,
		// No Recipe: it produces nothing, it's the logistics hub serfs
		// move goods through. See Building's doc comment.
	},
	Road: {
		Kind:      Road,
		Name:      "Road",
		Footprint: 1,
		// No AllowedTerrain restriction: a road can be laid on any
		// buildable tile.
	},
	Tavern: {
		Kind:      Tavern,
		Name:      "Tavern",
		Footprint: 1,
		Recipe: Recipe{
			// TicksToProduce: 0 means economy.Tick skips it (it makes
			// nothing) -- but logistics still reads Inputs to know the
			// Tavern wants to be kept stocked with Bread, the same way
			// it reads any real consumer's Inputs. See AGENTS.md.
			Inputs: map[resource.Type]int{resource.Bread: BufferCapacity},
		},
	},
}
