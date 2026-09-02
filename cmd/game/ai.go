package main

import (
	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/fishing"
	"strategy_game/internal/logistics"
	"strategy_game/internal/lumberjack"
	"strategy_game/internal/miner"
	"strategy_game/internal/quarry"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
	"strategy_game/internal/sentry"
	"strategy_game/internal/soldier"
	"strategy_game/internal/villagers"
	"strategy_game/internal/world"
)

// faction is a second, independent "town" -- the AI opponent in "1×1
// против ИИ" mode. It deliberately mirrors the shape of Game's own
// player-side fields (g.stock, g.pop, g.logi, ...g.soldiers) rather than
// refactoring those into a shared type: the player's fields, and every
// existing function that reads them, stay completely untouched (zero
// regression risk to the whole rest of this file), and this is the one
// new place that needs to know two "towns" can exist at once. g.ai is
// nil in ordinary single-player "free map" games -- every AI-specific
// tick/draw/decision call is skipped whenever it's nil.
type faction struct {
	owner int

	stock *resource.Stockpile
	pop   *economy.Population

	logi     *logistics.Controller
	vills    *villagers.Controller
	jacks    *lumberjack.Controller
	fishers  *fishing.Controller
	quarry   *quarry.Controller
	builders *builder.Controller
	miners   *miner.Controller
	sentries *sentry.Controller
	soldiers *soldier.Controller

	brain *aiBrain
}

// newFaction builds a fresh AI faction rooted at warehouse (already
// placed and owned by owner in g.buildings), with the same starting
// stockpile and serf count the player itself starts with -- per the
// user's explicit "та же механика, что у игрока и те же стартовые
// ресурсы".
func newFaction(owner int, warehouse *building.Building, difficulty aiDifficulty) *faction {
	stock := resource.NewStockpile(stockpileCapacity)
	stock.Add(resource.Plank, startingPlanks)
	stock.Add(resource.StoneBlock, startingStone)
	stock.Add(resource.Bread, startingBread)
	stock.Add(resource.Fish, startingFish)
	stock.Add(resource.Sausage, startingSausage)
	stock.Add(resource.Wine, startingWine)
	stock.Add(resource.Gold, startingGold)

	f := &faction{
		owner:    owner,
		stock:    stock,
		pop:      &economy.Population{},
		logi:     logistics.NewController(warehouse, startingSerfs),
		vills:    villagers.NewController(),
		jacks:    lumberjack.NewController(),
		fishers:  fishing.NewController(),
		quarry:   quarry.NewController(),
		builders: builder.NewController(),
		miners:   miner.NewController(),
		sentries: sentry.NewController(),
		soldiers: soldier.NewController(),
	}
	f.brain = newAIBrain(owner, difficulty)
	return f
}

// opponentOwner is the other faction's Owner in a two-faction "1×1"
// game -- there are only ever two (0 = player, 1 = AI), so this is
// simply 1-owner.
func opponentOwner(owner int) int {
	return 1 - owner
}

// opposingBuildingsFor/opposingSoldiersFor return c's cross-faction
// combat candidates -- see soldier.Controller.Tick's opposingBuildings/
// opposingSoldiers parameters. Both are nil (no cross-faction combat at
// all) outside "1×1 против ИИ" mode, i.e. whenever g.ai is nil.
func (g *Game) opposingBuildingsFor(c *soldier.Controller) []*building.Building {
	if g.ai == nil {
		return nil
	}
	switch c {
	case g.soldiers:
		return g.ownedBuildings(g.ai.owner)
	case g.ai.soldiers:
		return g.ownedBuildings(0)
	default:
		return nil
	}
}

func (g *Game) opposingSoldiersFor(c *soldier.Controller) []*soldier.Soldier {
	if g.ai == nil {
		return nil
	}
	switch c {
	case g.soldiers:
		return g.ai.soldiers.Soldiers
	case g.ai.soldiers:
		return g.soldiers.Soldiers
	default:
		return nil
	}
}

// tickAIFaction advances the AI's own economy by exactly one simulation
// tick -- a trimmed mirror of Update()'s player tick block (same
// controllers, same Reserve/Tick/event-handling shape), scoped to the
// AI's own buildings/stock/population instead of the player's. Runs
// after the player's own tick block, using its own fresh reservation
// ledger (the two factions never compete for the same Tavern/warehouse,
// so there is nothing to share). Deliberately simpler than the player's
// block in two ways: no debug enemy.Enemy interaction at all (that tool
// is sandbox/free-map only), and Sentries do not yet fight cross-faction
// (only soldiers do, see soldier.Controller.Tick's opposing* params) --
// a noted, deliberate scope cut for this first pass, not an oversight.
func (g *Game) tickAIFaction(grid *world.Grid) {
	f := g.ai
	if f == nil {
		return
	}
	buildings := g.ownedBuildingsWithRoads(f.owner)

	f.brain.tick(g, f, grid)

	ledger := reservations.New()
	f.logi.Reserve(ledger)
	f.vills.Reserve(ledger)
	f.jacks.Reserve(ledger)
	f.fishers.Reserve(ledger)
	f.quarry.Reserve(ledger)
	f.builders.Reserve(ledger)
	f.miners.Reserve(ledger)
	f.sentries.Reserve(ledger)

	serfResult := f.logi.Tick(grid, buildings, f.stock, ledger, f.soldiers.Soldiers)
	villagerDeaths := f.vills.Tick(buildings, ledger)
	jackEvents := f.jacks.Tick(grid, buildings, ledger)
	fishEvents := f.fishers.Tick(grid, buildings, ledger)
	quarryEvents := f.quarry.Tick(grid, buildings, ledger)
	builderEvents := f.builders.Tick(grid, buildings, ledger)
	minerEvents := f.miners.Tick(grid, buildings, ledger)
	sentryDeaths := f.sentries.Tick(buildings, nil, ledger)
	soldierDeaths := f.soldiers.Tick(grid, buildings, nil, g.opposingBuildingsFor(f.soldiers), g.opposingSoldiersFor(f.soldiers))

	f.pop.Deaths += serfResult.Deaths + villagerDeaths + sentryDeaths + soldierDeaths
	f.pop.UnitsDismissed += serfResult.Dismissed

	for _, event := range jackEvents {
		switch event.Kind {
		case lumberjack.TreeCut:
			g.cutTree(event.Tree)
		case lumberjack.WorkerDied:
			f.pop.Deaths++
			if event.Cargo > 0 {
				f.stock.Add(resource.Log, event.Cargo)
			}
		}
	}
	for _, event := range fishEvents {
		switch event.Kind {
		case fishing.FishCaught:
			g.catchFish(event.Fish)
		case fishing.WorkerDied:
			f.pop.Deaths++
			if event.Cargo > 0 {
				f.stock.Add(resource.Fish, event.Cargo)
			}
		}
	}
	for _, event := range quarryEvents {
		switch event.Kind {
		case quarry.DepositExhausted:
			g.removeDeposit(event.Deposit)
		case quarry.WorkerDied:
			f.pop.Deaths++
			if event.Cargo > 0 {
				f.stock.Add(resource.StoneBlock, event.Cargo)
			}
		}
	}
	for _, event := range builderEvents {
		switch event.Kind {
		case builder.ConstructionComplete:
			g.finishConstruction(event.Building)
		case builder.WorkerDied:
			f.pop.Deaths++
		case builder.WorkerDismissed:
			f.pop.UnitsDismissed++
		}
	}
	for _, event := range minerEvents {
		switch event.Kind {
		case miner.DepositExhausted:
			g.removeDeposit(event.Deposit)
		case miner.WorkerDied:
			f.pop.Deaths++
			if event.Cargo > 0 {
				f.stock.Add(event.CargoResource, event.Cargo)
			}
		}
	}

	f.pop.Count = len(f.logi.Serfs) + len(f.vills.Villagers) + len(f.jacks.Lumberjacks) + len(f.fishers.Fishermen) + len(f.quarry.Quarrymen) + len(f.builders.Builders) + len(f.miners.Miners) + len(f.sentries.Sentries) + len(f.soldiers.Soldiers)
}
