package building

import (
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// Standard construction cost/pace for an ordinary building: a flat 5
// construction materials, ~15s digging the foundation and ~30s finishing once
// materials arrive (60 simulation ticks/2 = 30s at the normal 2 ticks/sec
// pace). Per the user's explicit request ("снизим стоимость зданий, все
// здания 5 доска, 5 каменный блок"), every building costs the same flat
// amount now -- Winery/PigFarm/FisherHut no longer pay extra planks for
// their fence/boat (see the removed fencedPlankCost). Road is deliberately
// unchanged (kept at its original, already-minimal roadStoneCost) --
// confirmed explicitly with the user, since the player lays many of them.
const (
	standardPlankCost = 5
	standardStoneCost = 5

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
			//
			// Per the user's explicit request ("чтобы за раз не сразу
			// восемь единиц продукции попадало... а условно давать по
			// два... цикл производства немного увеличится, но логика
			// изменится сильно"): the farmer brings in Wheat two units at
			// a time instead of the old single 5-unit harvest at the end
			// of the cycle, at a slightly slower overall rate (2/60 here
			// vs. the old 5/120) to reflect the overhead of several
			// shorter trips instead of one long one. This also fixes a
			// real side effect the user found by simulating their save
			// (see AGENTS.md): a batch that lands all at once, at or
			// above a building's OutputCapacity, sits reading "100% full"
			// for as long as it takes a serf to physically reach it --
			// small, frequent batches never create that all-or-nothing
			// spike in the first place.
			Output:         resource.Wheat,
			OutputAmount:   2,
			TicksToProduce: 60,
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
			//
			// Per the user's explicit request, see Farm's Recipe doc
			// comment for the full reasoning: the winemaker brings in Wine
			// two units at a time instead of the old single 8-unit harvest,
			// at a slightly slower overall rate (2/72 here vs. the old
			// 8/240).
			Output:         resource.Wine,
			OutputAmount:   2,
			TicksToProduce: 72,
		},
		OutputCapacity:              8,
		PlankCost:                   standardPlankCost,
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
		PlankCost:                   standardPlankCost,
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
			Inputs:       map[resource.Type]int{resource.Wheat: 3},
			Output:       resource.Carcass,
			OutputAmount: 1,
			// A pig gives up its hide the same moment it gives up its
			// carcass -- one animal, both products at once, per the
			// user's explicit request ("оба выхода с одной свиньи
			// одновременно").
			SecondaryOutput:       resource.Hide,
			SecondaryOutputAmount: 1,
			TicksToProduce:        600,
			ConsumeInputsAtStart:  true,
		},
		PlankCost:                   standardPlankCost,
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
	StoneWall: {
		Kind:      StoneWall,
		Name:      "Stone Wall",
		Footprint: 1,
		// A one-tile wall section intentionally costs only three processed
		// stone blocks. Short build phases keep long player-drawn runs from
		// becoming a construction queue that takes most of a game day.
		StoneCost:                   3,
		ConstructionFoundationTicks: roadFoundationTicks,
		ConstructionBuildTicks:      20,
	},
	Gate: {
		Kind:      Gate,
		Name:      "Gate",
		Footprint: 1,
		// Gates are a reinforced wall opening: timber leaves, stone posts and
		// three iron units for straps/latch. They are installed only over a
		// finished straight StoneWall segment by cmd/game, never placed on
		// bare ground.
		PlankCost:                   5,
		StoneCost:                   3,
		IronCost:                    3,
		ConstructionFoundationTicks: roadFoundationTicks,
		ConstructionBuildTicks:      40,
	},
	WatchTower: {
		Kind:           WatchTower,
		Name:           "Watch Tower",
		Footprint:      1,
		RequiresWorker: true,
		// No adjacency-to-wall requirement -- the player explicitly wants
		// to place a tower behind a wall, in front of one, or with no wall
		// at all. Range is a plain radius (package sentry), not a line of
		// sight, so it fires over a wall too.
		PassiveInputs:               map[resource.Type]int{resource.StoneBlock: BufferCapacity},
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
	Barracks: {
		Kind:      Barracks,
		Name:      "Barracks",
		Footprint: 1,
		// Deliberately not RequiresWorker: nobody lives here. It's a hire
		// point, not a workplace -- see cmd/game's Barracks inspector
		// button. Gold, Bow, LeatherArmor and Sword delivered here (up to
		// BufferCapacity each) are spent per hire, straight from this
		// building's own InputBuffer rather than the shared stockpile
		// every other hire draws from directly: a Sentry needs only gold,
		// an Archer needs gold + Bow + LeatherArmor, a Swordsman needs
		// gold + Sword + LeatherArmor.
		PassiveInputs: map[resource.Type]int{
			resource.Gold:         BufferCapacity,
			resource.Bow:          BufferCapacity,
			resource.LeatherArmor: BufferCapacity,
			resource.Sword:        BufferCapacity,
		},
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
	Armory: {
		Kind:           Armory,
		Name:           "Armory",
		Footprint:      1,
		RequiresWorker: true,
		// Raw material for all three queued products at once (Plank for
		// Bow, Hide for LeatherArmor, Iron+Coal for Sword) -- see
		// cmd/game's tickArmories, which is what actually turns these
		// into finished goods; there's no Recipe here at all (three
		// parallel player-queued lines don't fit the single-active-recipe
		// model economy.Tick assumes).
		PassiveInputs: map[resource.Type]int{
			resource.Plank: BufferCapacity,
			resource.Hide:  BufferCapacity,
			resource.Iron:  BufferCapacity,
			resource.Coal:  BufferCapacity,
		},
		PlankCost:                   standardPlankCost,
		StoneCost:                   standardStoneCost,
		ConstructionFoundationTicks: standardFoundationTicks,
		ConstructionBuildTicks:      standardBuildTicks,
	},
}
