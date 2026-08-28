package building

import (
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// Standard construction cost/pace for an ordinary building: 10 planks + 4
// stone, ~15s digging the foundation and ~30s finishing once materials
// arrive (60 simulation ticks/2 = 30s at the normal 2 ticks/sec pace). See
// AGENTS.md's construction section for the three exceptions (Winery,
// PigFarm, FisherHut need extra planks for a fence/boat) and Road (cheaper
// and much faster, since the player lays many of them).
const (
	standardPlankCost = 10
	standardStoneCost = 4

	fencedPlankCost = 15 // Winery, PigFarm, FisherHut: fence and/or boat

	standardFoundationTicks = 30
	standardBuildTicks      = 60

	roadStoneCost       = 1
	roadFoundationTicks = 10
	roadBuildTicks      = 10
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
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
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
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
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
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
	Warehouse: {
		Kind:      Warehouse,
		Name:      "Warehouse",
		Footprint: 1,
		// No Recipe: it produces nothing, it's the logistics hub serfs
		// move goods through. See Building's doc comment.
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
	Road: {
		Kind:      Road,
		Name:      "Road",
		Footprint: 1,
		// No AllowedTerrain restriction: a road can be laid on any
		// buildable tile. Cheaper and quicker than a building -- the
		// player lays many of these -- and needs no planks at all.
		StoneCost:                   roadStoneCost,
		ConstructionFoundationTicks: roadFoundationTicks,
		ConstructionBuildTicks:      roadBuildTicks,
	},
	Tavern: {
		Kind:                        Tavern,
		Name:                        "Tavern",
		Footprint:                   1,
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
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
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
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
		// Extra planks: the vineyard's wooden fence around all eight crop
		// cells.
		PlankCost:                   fencedPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
	FisherHut: {
		Kind:           FisherHut,
		Name:           "Fisher Hut",
		Footprint:      1,
		RequiresWorker: true,
		// The fisherman places caught Fish in OutputBuffer; ordinary serfs
		// collect it along the hut's normal road access point.
		// Extra planks: the boat.
		PlankCost:                   fencedPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
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
		// Extra planks: the pen fence.
		PlankCost:                   fencedPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
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
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
	CarpentryWorkshop: {
		Kind:           CarpentryWorkshop,
		Name:           "Carpentry Workshop",
		Footprint:      1,
		RequiresWorker: true,
		Recipe: Recipe{
			// One log becomes two planks. Input buffer stays at the default
			// BufferCapacity (6), same as every other consumer -- no override.
			Inputs:         map[resource.Type]int{resource.Log: 1},
			Output:         resource.Plank,
			OutputAmount:   2,
			TicksToProduce: 72,
		},
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
	StoneDeposit: {
		Kind:      StoneDeposit,
		Name:      "Stone Deposit",
		Footprint: 1,
		// Stone deposits are placed by map generation as one region, not
		// player-built or offered in the palette. Ordinary occupancy rules
		// still apply: nothing else can be built on one while it still has
		// Reserve left (see CanPlace/footprintsOverlap). No construction
		// cost/pace: it's never placed through the Builder flow.
	},
	QuarryHut: {
		Kind:           QuarryHut,
		Name:           "Quarry Hut",
		Footprint:      1,
		RequiresWorker: true,
		// No recipe: the quarryman physically walks to a stone deposit,
		// mines it, and deposits already-processed Stone Blocks into
		// OutputBuffer (1 mined stone -> 2 blocks, applied on unload -- see
		// package quarry). Serfs collect them through the hut's normal road
		// access point, same as a Lumberjack Hut's Logs.
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
	CoalDeposit: {
		Kind:      CoalDeposit,
		Name:      "Coal Deposit",
		Footprint: 1,
		// Placed by map generation as several regions, not player-built.
		// Deliberately the most abundant of the three ore-family deposits
		// -- see cmd/game's seedDepositRegions calls.
	},
	GoldOreDeposit: {
		Kind:      GoldOreDeposit,
		Name:      "Gold Ore Deposit",
		Footprint: 1,
		// Deliberately the rarest of the three -- gold ore smelts directly
		// into the game's hiring currency.
	},
	IronOreDeposit: {
		Kind:      IronOreDeposit,
		Name:      "Iron Ore Deposit",
		Footprint: 1,
	},
	MinerHut: {
		Kind:           MinerHut,
		Name:           "Miner Hut",
		Footprint:      1,
		RequiresWorker: true,
		// No recipe: the miner physically walks to whichever deposit the
		// quota currently points at (see package miner) and deposits the
		// raw ore/coal into OutputBuffer -- unlike the Quarryman, nothing
		// is processed here; that's the Smeltery's job for the two ores
		// (Coal itself needs no further processing).
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
	Smeltery: {
		Kind:           Smeltery,
		Name:           "Smeltery",
		Footprint:      1,
		RequiresWorker: true,
		Recipe: Recipe{
			// One recipe smelts gold, the other iron -- see Type.AltRecipes
			// and economy.pickRecipe for how the building chooses between
			// them each cycle. Coal is common to both, so it's the one
			// input that can make either recipe wait even when its own ore
			// is on hand -- exactly the kind of contention the existing
			// supply-priority slider already handles.
			Inputs:         map[resource.Type]int{resource.GoldOre: 1, resource.Coal: 1},
			Output:         resource.Gold,
			OutputAmount:   1,
			TicksToProduce: 60,
		},
		AltRecipes: []Recipe{
			{
				Inputs:         map[resource.Type]int{resource.IronOre: 1, resource.Coal: 1},
				Output:         resource.Iron,
				OutputAmount:   1,
				TicksToProduce: 60,
			},
		},
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
}
