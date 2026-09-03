package main

import (
	"strategy_game/internal/builder"
	"strategy_game/internal/building"
	"strategy_game/internal/combat"
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

// restoreFaction rebuilds a "1×1 против ИИ" save's AI faction -- unlike
// newFaction (a brand new match's starting economy), every controller
// starts genuinely empty: the actual roster is restored separately (see
// cmd/game's restoreUnits, which needs g.ai to already exist before it
// can dispatch any Owner: 1 unit into it), and stock/pop/brain come from
// the save instead of a fresh game's starting amounts. A real gap found
// from the user confirming they want duel saves at all ("Конечно нужно
// сохранение"): buildSaveState/loadGame never had a way to reconstruct
// the AI's own economy, so saveGame refused to save a duel game outright
// rather than lose it silently.
func restoreFaction(owner int, warehouse *building.Building, difficulty aiDifficulty, stock resource.Stockpile, pop economy.Population, brainCooldown, buildIndex, buildAttempts int) *faction {
	stock.Capacity = 0 // same normalization loadGame already applies to the player's own Stockpile
	f := &faction{
		owner:    owner,
		stock:    &stock,
		pop:      &pop,
		logi:     logistics.NewController(warehouse, 0),
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
	f.brain.cooldown = brainCooldown
	f.brain.buildIndex = buildIndex
	f.brain.buildAttempts = buildAttempts
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

// opposingIntruderTargetsFor returns c's cross-faction Sentry targets --
// every living unit (any of the 7 civilian professions, or a soldier) the
// OTHER faction currently has, wrapped as sentry.IntruderTarget. A real
// gap found from an actual playtest report ("почему башня не убила его
// слуг"): a WatchTower's Sentry could only ever fire at the sandbox-only
// enemy.Enemy, with no way at all to target anything belonging to the
// "1×1 против ИИ" opponent. nil outside that mode, matching
// opposingBuildingsFor/opposingSoldiersFor above.
func (g *Game) opposingIntruderTargetsFor(c *sentry.Controller) []sentry.IntruderTarget {
	if g.ai == nil {
		return nil
	}
	switch c {
	case g.sentries:
		return intruderTargetsFrom(g.ai.logi, g.ai.vills, g.ai.jacks, g.ai.fishers, g.ai.quarry, g.ai.builders, g.ai.miners, g.ai.soldiers)
	case g.ai.sentries:
		return intruderTargetsFrom(g.logi, g.vills, g.jacks, g.fishers, g.quarry, g.builders, g.miners, g.soldiers)
	default:
		return nil
	}
}

// opposingIntruderTargetsForSoldiers is opposingIntruderTargetsFor's twin
// for package soldier's Controller.Tick, whose opposingIntruders parameter
// takes the same []combat.IntruderTarget shape (sentry.IntruderTarget is a
// type alias for it, see internal/combat) but is keyed off *soldier.
// Controller rather than *sentry.Controller. A second playtest report --
// "боевые юниты могут уничтожать любых юнитов противника - это враги!" --
// wanted a Soldier able to engage ANY opposing unit, not only rival
// soldiers/buildings, exactly like the WatchTower fix above; the target
// list itself is identical (intruderTargetsFrom is controller-agnostic),
// only the dispatcher's switch needs its own copy since a *soldier.
// Controller and a *sentry.Controller are different types.
func (g *Game) opposingIntruderTargetsForSoldiers(c *soldier.Controller) []combat.IntruderTarget {
	if g.ai == nil {
		return nil
	}
	switch c {
	case g.soldiers:
		return intruderTargetsFrom(g.ai.logi, g.ai.vills, g.ai.jacks, g.ai.fishers, g.ai.quarry, g.ai.builders, g.ai.miners, g.ai.soldiers)
	case g.ai.soldiers:
		return intruderTargetsFrom(g.logi, g.vills, g.jacks, g.fishers, g.quarry, g.builders, g.miners, g.soldiers)
	default:
		return nil
	}
}

// intruderTargetsFrom builds one sentry.IntruderTarget per currently
// living unit across every controller a faction owns. Each Alive/Kill
// closure captures its own unit pointer directly (not an index), so it
// stays correct even though every one of these controllers may reorder
// or shrink its own slice on its next Tick.
func intruderTargetsFrom(
	logi *logistics.Controller,
	vills *villagers.Controller,
	jacks *lumberjack.Controller,
	fishers *fishing.Controller,
	quarry *quarry.Controller,
	builders *builder.Controller,
	miners *miner.Controller,
	soldiers *soldier.Controller,
) []sentry.IntruderTarget {
	var out []sentry.IntruderTarget
	for _, s := range logi.Serfs {
		s := s
		out = append(out, sentry.IntruderTarget{X: s.X, Y: s.Y, Alive: s.Alive, Kill: s.Kill})
	}
	for _, v := range vills.Villagers {
		v := v
		out = append(out, sentry.IntruderTarget{X: v.X, Y: v.Y, Alive: v.Alive, Kill: v.Kill})
	}
	for _, j := range jacks.Lumberjacks {
		j := j
		out = append(out, sentry.IntruderTarget{X: j.X, Y: j.Y, Alive: j.Alive, Kill: j.Kill})
	}
	for _, f := range fishers.Fishermen {
		f := f
		out = append(out, sentry.IntruderTarget{X: f.X, Y: f.Y, Alive: f.Alive, Kill: f.Kill})
	}
	for _, q := range quarry.Quarrymen {
		q := q
		out = append(out, sentry.IntruderTarget{X: q.X, Y: q.Y, Alive: q.Alive, Kill: q.Kill})
	}
	for _, b := range builders.Builders {
		b := b
		out = append(out, sentry.IntruderTarget{X: b.X, Y: b.Y, Alive: b.Alive, Kill: b.Kill})
	}
	for _, m := range miners.Miners {
		m := m
		out = append(out, sentry.IntruderTarget{X: m.X, Y: m.Y, Alive: m.Alive, Kill: m.Kill})
	}
	for _, sd := range soldiers.Soldiers {
		sd := sd
		if !sd.Alive() {
			continue
		}
		out = append(out, sentry.IntruderTarget{
			X: sd.X, Y: sd.Y,
			Alive: sd.Alive,
			// Soldier has no Kill() of its own (soldiers die from combat.
			// ApplyDamage reducing HP, not a single external kill call --
			// see combat.UnitDamagePerHit) -- a sling stone lethal to an
			// unarmed worker in one hit is not obviously also a clean
			// one-hit kill against an armoured Archer/Swordsman, so this
			// applies the same per-hit damage a rival soldier's own
			// factionTarget.hit() would, not an instant kill.
			Kill: func() { sd.HP = combat.ApplyDamage(sd.HP, combat.UnitDamagePerHit) },
		})
	}
	return out
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
	sentryDeaths := f.sentries.Tick(buildings, nil, g.opposingIntruderTargetsFor(f.sentries), ledger)
	soldierDeaths := f.soldiers.Tick(grid, buildings, nil, g.opposingBuildingsFor(f.soldiers), g.opposingSoldiersFor(f.soldiers), g.opposingIntruderTargetsForSoldiers(f.soldiers))

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
