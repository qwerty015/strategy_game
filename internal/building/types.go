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
		Footprint:      3, // one tile is the farmhouse, the rest is tilled field around it -- see render/buildings.go
		RequiresWorker: true,
		Recipe: Recipe{
			// No Inputs: a Farm gathers Wheat from the land itself.
			Output:       resource.Wheat,
			OutputAmount: 5,
			// At the normal speed (2 simulation ticks/sec) this is about
			// one minute per harvest: deliberately much slower than the
			// original prototype's two-second cycle.
			TicksToProduce: 120,
		},
	},
	Mill: {
		Kind:      Mill,
		Name:      "Mill",
		Footprint: 1,
		Recipe: Recipe{
			// One unit of Wheat is milled into one unit of Flour.
			Inputs:       map[resource.Type]int{resource.Wheat: 1},
			Output:       resource.Flour,
			OutputAmount: 1,
			// Milling is faster than growing, but remains a visible stage
			// in the chain instead of completing instantly.
			TicksToProduce: 48,
		},
	},
	Bakery: {
		Kind:           Bakery,
		Name:           "Bakery",
		Footprint:      1,
		RequiresWorker: true,
		Recipe: Recipe{
			// One unit of Flour bakes into two units of Bread.
			Inputs:       map[resource.Type]int{resource.Flour: 1},
			Output:       resource.Bread,
			OutputAmount: 2,
			// Baking takes roughly 36 seconds at normal speed.
			TicksToProduce: 72,
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
			// nothing) -- but logistics still reads the accepted menu to
			// keep the Tavern stocked. Every food type is equally valid.
			Inputs: map[resource.Type]int{
				resource.Bread:   BufferCapacity,
				resource.Fish:    BufferCapacity,
				resource.Wine:    BufferCapacity,
				resource.Sausage: BufferCapacity,
			},
		},
		AcceptedResources: []resource.Type{
			resource.Bread,
			resource.Fish,
			resource.Wine,
			resource.Sausage,
		},
	},
	Tree: {
		Kind:      Tree,
		Name:      "Tree",
		Footprint: 1,
		// Trees are spawned by map generation/regrowth, not offered in the
		// build palette. They still use normal occupancy rules.
	},
	LumberjackHut: {
		Kind:           LumberjackHut,
		Name:           "Lumberjack Hut",
		Footprint:      1,
		RequiresWorker: true,
		// The hut has no recipe: the lumberjack physically walks to a tree
		// and deposits finished Logs into OutputBuffer. Serfs collect them
		// through the hut's road access point.
	},
	Winery: {
		Kind:           Winery,
		Name:           "Winery",
		Footprint:      3,
		RequiresWorker: true,
		Recipe: Recipe{
			// The eight vineyard cells appear immediately with the building.
			// Progress is the shared grape-growing/harvest cycle; the raw
			// grapes stay internal and only finished Wine enters logistics.
			Output:         resource.Wine,
			OutputAmount:   8,
			TicksToProduce: 240, // about two minutes at normal speed
		},
		OutputCapacity: 8,
	},
	FisherHut: {
		Kind:           FisherHut,
		Name:           "Fisher Hut",
		Footprint:      1,
		RequiresWorker: true,
		// The fisherman places caught Fish in OutputBuffer; ordinary serfs
		// collect it along the hut's normal road access point.
	},
	Fish: {
		Kind:           Fish,
		Name:           "Fish",
		Footprint:      1,
		AllowedTerrain: []world.TerrainType{world.Water},
		// Fish are spawned by the water-population system, not player-built.
	},
	PigFarm: {
		Kind:           PigFarm,
		Name:           "Pig Farm",
		Footprint:      1,
		RequiresWorker: true,
		Recipe: Recipe{
			// Feed is consumed before growth begins: without all three units
			// of Wheat there is no pig being raised yet.
			Inputs:               map[resource.Type]int{resource.Wheat: 3},
			Output:               resource.Carcass,
			OutputAmount:         1,
			TicksToProduce:       600,
			ConsumeInputsAtStart: true,
		},
	},
	MeatWorkshop: {
		Kind:           MeatWorkshop,
		Name:           "Meat Workshop",
		Footprint:      1,
		RequiresWorker: true,
		Recipe: Recipe{
			// A single generic carcass becomes two sausage portions. Keeping
			// Carcass unified makes future animals add producers, not recipes.
			Inputs:         map[resource.Type]int{resource.Carcass: 1},
			Output:         resource.Sausage,
			OutputAmount:   2,
			TicksToProduce: 72,
		},
	},
}
