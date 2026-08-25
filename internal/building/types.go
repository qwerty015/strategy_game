package building

import (
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// Types is the registry of every building kind in the game. This is the
// data that defines the whole economy: to add a new production chain
// later (winery, sawmill, ...), add entries here -- no other package
// needs to change.
var Types = map[Kind]Type{
	Farm: {
		Kind:           Farm,
		Name:           "Farm",
		Footprint:      2,
		AllowedTerrain: []world.TerrainType{world.Fertile},
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
		Footprint: 2,
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
		Footprint: 2,
		Recipe: Recipe{
			Inputs:         map[resource.Type]int{resource.Flour: 5},
			Output:         resource.Bread,
			OutputAmount:   5,
			TicksToProduce: 3,
		},
	},
}
