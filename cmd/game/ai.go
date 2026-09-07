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

// factionOwners returns every faction's Owner currently in the match --
// 0 (the player) first, then each g.ais entry in slice order (never a
// map: iteration order must be deterministic, both for this session and
// for a future networked match). Empty g.ais (ordinary single-player
// "free map" games) still returns [0].
func (g *Game) factionOwners() []int {
	owners := make([]int, 0, 1+len(g.ais))
	owners = append(owners, 0)
	for _, f := range g.ais {
		owners = append(owners, f.owner)
	}
	return owners
}

// opposingOwners returns every faction's Owner except owner itself --
// generalizes the old two-faction opponentOwner (which was simply
// 1-owner) to however many opponents an "N против ИИ" match actually
// has, per the user's explicit "все против всех".
func (g *Game) opposingOwners(owner int) []int {
	var out []int
	for _, o := range g.factionOwners() {
		if o != owner {
			out = append(out, o)
		}
	}
	return out
}

// factionByOwner returns the AI faction with the given Owner, or nil if
// none matches -- owner 0 (the player) always returns nil here, since
// the player's own controllers live directly on Game, not in g.ais.
func (g *Game) factionByOwner(owner int) *faction {
	for _, f := range g.ais {
		if f.owner == owner {
			return f
		}
	}
	return nil
}

// logiFor/buildersFor return owner's own logistics/builder controller --
// the player's for owner 0, an AI faction's for any owner in g.ais, or
// nil if owner matches neither (already-defeated/nonexistent faction).
// Used by pruneDestroyedBuildings to tell the right controller to forget
// a Warehouse that combat just destroyed.
func (g *Game) logiFor(owner int) *logistics.Controller {
	if owner == 0 {
		return g.logi
	}
	if f := g.factionByOwner(owner); f != nil {
		return f.logi
	}
	return nil
}

func (g *Game) buildersFor(owner int) *builder.Controller {
	if owner == 0 {
		return g.builders
	}
	if f := g.factionByOwner(owner); f != nil {
		return f.builders
	}
	return nil
}

func (g *Game) stockFor(owner int) *resource.Stockpile {
	if owner == 0 {
		return g.stock
	}
	if f := g.factionByOwner(owner); f != nil {
		return f.stock
	}
	return nil
}

// ownerOfSoldiers/ownerOfSentries identify which faction a live
// controller pointer belongs to -- the opposing*For dispatchers below
// need this to know whose "everyone else" to gather.
func (g *Game) ownerOfSoldiers(c *soldier.Controller) (int, bool) {
	if c == g.soldiers {
		return 0, true
	}
	for _, f := range g.ais {
		if f.soldiers == c {
			return f.owner, true
		}
	}
	return 0, false
}

func (g *Game) ownerOfSentries(c *sentry.Controller) (int, bool) {
	if c == g.sentries {
		return 0, true
	}
	for _, f := range g.ais {
		if f.sentries == c {
			return f.owner, true
		}
	}
	return 0, false
}

// opposingBuildingsFor/opposingSoldiersFor return c's cross-faction
// combat candidates -- see soldier.Controller.Tick's opposingBuildings/
// opposingSoldiers parameters. Both are nil (no cross-faction combat at
// all) outside "N против ИИ" mode, i.e. whenever g.ais is empty.
func (g *Game) opposingBuildingsFor(c *soldier.Controller) []*building.Building {
	owner, ok := g.ownerOfSoldiers(c)
	if !ok {
		return nil
	}
	opposing := g.opposingOwners(owner)
	if len(opposing) == 0 {
		return nil
	}
	// g.ownedBuildings(o) already includes every natural resource node
	// (Tree/Fish/deposit) regardless of o -- harmless for a single
	// opposing faction (the original two-faction case), but
	// concatenating it once per opposing owner here would duplicate
	// every one of those pointers once per extra opponent. Take
	// resources from the first owner queried, buildings-only from the
	// rest.
	out := g.ownedBuildings(opposing[0])
	for _, o := range opposing[1:] {
		for _, b := range g.ownedBuildings(o) {
			if !isNaturalResourceKind(b.Kind) {
				out = append(out, b)
			}
		}
	}
	return out
}

func (g *Game) opposingSoldiersFor(c *soldier.Controller) []*soldier.Soldier {
	owner, ok := g.ownerOfSoldiers(c)
	if !ok {
		return nil
	}
	var out []*soldier.Soldier
	for _, o := range g.opposingOwners(owner) {
		if o == 0 {
			out = append(out, g.soldiers.Soldiers...)
			continue
		}
		if f := g.factionByOwner(o); f != nil {
			out = append(out, f.soldiers.Soldiers...)
		}
	}
	return out
}

// intruderTargetsForOwner builds intruderTargetsFrom's argument list for
// whichever faction owns owner -- owner 0 reads the player's own
// controllers directly off Game, any other owner looks up its faction in
// g.ais. Shared by opposingIntruderTargetsFor and its Soldier twin below.
func (g *Game) intruderTargetsForOwner(owner int) []sentry.IntruderTarget {
	if owner == 0 {
		return intruderTargetsFrom(owner, g.logi, g.vills, g.jacks, g.fishers, g.quarry, g.builders, g.miners, g.soldiers)
	}
	f := g.factionByOwner(owner)
	if f == nil {
		return nil
	}
	return intruderTargetsFrom(owner, f.logi, f.vills, f.jacks, f.fishers, f.quarry, f.builders, f.miners, f.soldiers)
}

// opposingIntruderTargetsFor returns c's cross-faction Sentry targets --
// every living unit (any of the 7 civilian professions, or a soldier)
// EVERY other faction currently has, wrapped as sentry.IntruderTarget. A
// real gap found from an actual playtest report ("почему башня не убила
// его слуг"): a WatchTower's Sentry could only ever fire at the
// sandbox-only enemy.Enemy, with no way at all to target anything
// belonging to a "N против ИИ" opponent. nil outside that mode, matching
// opposingBuildingsFor/opposingSoldiersFor above.
func (g *Game) opposingIntruderTargetsFor(c *sentry.Controller) []sentry.IntruderTarget {
	owner, ok := g.ownerOfSentries(c)
	if !ok {
		return nil
	}
	var out []sentry.IntruderTarget
	for _, o := range g.opposingOwners(owner) {
		out = append(out, g.intruderTargetsForOwner(o)...)
	}
	return out
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
// only the owner-lookup needs its own copy since a *soldier.Controller
// and a *sentry.Controller are different types.
func (g *Game) opposingIntruderTargetsForSoldiers(c *soldier.Controller) []combat.IntruderTarget {
	owner, ok := g.ownerOfSoldiers(c)
	if !ok {
		return nil
	}
	var out []combat.IntruderTarget
	for _, o := range g.opposingOwners(owner) {
		out = append(out, g.intruderTargetsForOwner(o)...)
	}
	return out
}

// intruderTargetsFrom builds one sentry.IntruderTarget per currently
// living unit across every controller a faction owns. Each Alive/Kill
// closure captures its own unit pointer directly (not an index), so it
// stays correct even though every one of these controllers may reorder
// or shrink its own slice on its next Tick.
func intruderTargetsFrom(
	owner int,
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
		out = append(out, sentry.IntruderTarget{X: s.X, Y: s.Y, Position: func() (int, int) { return s.X, s.Y }, Alive: s.Alive, Kill: s.Kill, Owner: owner})
	}
	for _, v := range vills.Villagers {
		v := v
		out = append(out, sentry.IntruderTarget{X: v.X, Y: v.Y, Position: func() (int, int) { return v.X, v.Y }, Alive: v.Alive, Kill: v.Kill, Owner: owner})
	}
	for _, j := range jacks.Lumberjacks {
		j := j
		out = append(out, sentry.IntruderTarget{X: j.X, Y: j.Y, Position: func() (int, int) { return j.X, j.Y }, Alive: j.Alive, Kill: j.Kill, Owner: owner})
	}
	for _, f := range fishers.Fishermen {
		f := f
		out = append(out, sentry.IntruderTarget{X: f.X, Y: f.Y, Position: func() (int, int) { return f.X, f.Y }, Alive: f.Alive, Kill: f.Kill, Owner: owner})
	}
	for _, q := range quarry.Quarrymen {
		q := q
		out = append(out, sentry.IntruderTarget{X: q.X, Y: q.Y, Position: func() (int, int) { return q.X, q.Y }, Alive: q.Alive, Kill: q.Kill, Owner: owner})
	}
	for _, b := range builders.Builders {
		b := b
		out = append(out, sentry.IntruderTarget{X: b.X, Y: b.Y, Position: func() (int, int) { return b.X, b.Y }, Alive: b.Alive, Kill: b.Kill, Owner: owner})
	}
	for _, m := range miners.Miners {
		m := m
		out = append(out, sentry.IntruderTarget{X: m.X, Y: m.Y, Position: func() (int, int) { return m.X, m.Y }, Alive: m.Alive, Kill: m.Kill, Owner: owner})
	}
	for _, sd := range soldiers.Soldiers {
		sd := sd
		if !sd.Alive() {
			continue
		}
		out = append(out, sentry.IntruderTarget{
			X: sd.X, Y: sd.Y,
			Position: func() (int, int) { return sd.X, sd.Y },
			Alive:    sd.Alive,
			// Instant kill regardless of the soldier's current HP -- the
			// user's own explicit combat rebalance request ("удар башни
			// по юниту - 100% хп"), same one-hit rule a WatchTower's
			// stone already applies to every unarmed intruder via its
			// own Kill(). Safe to make this unconditional even though
			// this same wrapped IntruderTarget list is also handed to a
			// rival Soldier's own opposingIntruders (see
			// opposingIntruderTargetsForSoldiers): a live rival soldier
			// always also appears in that Soldier's direct
			// opposingSoldiers list at the identical tile distance, and
			// nearestFactionTarget's tie-break (strictly-less, first
			// match wins) always prefers that direct entry -- soldier-vs-
			// soldier combat never actually reaches this Kill closure in
			// practice, only a WatchTower's engage() does.
			Kill:  func() { sd.HP = 0 },
			Owner: owner,
		})
	}
	return out
}

// tickAIFaction advances f's own economy by exactly one simulation tick
// -- a trimmed mirror of Update()'s player tick block (same controllers,
// same Reserve/Tick/event-handling shape), scoped to f's own buildings/
// stock/population instead of the player's. Called once per AI faction
// in g.ais (see Update's tick loop), each with its own fresh reservation
// ledger (factions never compete for the same Tavern/warehouse, so there
// is nothing to share). Deliberately simpler than the player's block in
// two ways: no debug enemy.Enemy interaction at all (that tool is
// sandbox/free-map only), and Sentries do not yet fight cross-faction
// (only soldiers do, see soldier.Controller.Tick's opposing* params) --
// a noted, deliberate scope cut for this first pass, not an oversight.
func (g *Game) tickAIFaction(f *faction, grid *world.Grid) {
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

	// g.buildings (the WHOLE map) for the new obstacles param specifically
	// -- see logistics.Controller.Tick's doc comment: a soldier-delivery
	// or construction-delivery serf's off-road leg can legitimately need
	// to walk into an opposing faction's territory (feeding a soldier
	// mid-attack there), and that walk must actually be blocked by a
	// wall that isn't this faction's own, the same real bug and fix as
	// soldiers.Tick's own g.buildings argument just below.
	serfResult := f.logi.Tick(grid, buildings, g.buildings, f.stock, ledger, f.soldiers.Soldiers)
	villagerDeaths := f.vills.Tick(buildings, ledger)
	jackEvents := f.jacks.Tick(grid, buildings, g.buildings, ledger)
	fishEvents := f.fishers.Tick(grid, buildings, ledger)
	quarryEvents := f.quarry.Tick(grid, buildings, g.buildings, ledger)
	builderEvents := f.builders.Tick(grid, buildings, g.buildings, f.stock, ledger)
	minerEvents := f.miners.Tick(grid, buildings, g.buildings, ledger)
	sentryResult := f.sentries.Tick(buildings, g.opposingIntruderTargetsFor(f.sentries), ledger)
	// g.buildings (the WHOLE map), not the per-faction buildings above, for
	// the obstacle-avoidance param specifically -- see soldier.Controller.
	// Tick's doc comment and the identical choice in Update's player tick
	// block for why: an attacking faction's soldiers must actually be
	// blocked by an opponent's walls/gates, which means those need to be
	// in the obstacle list at all.
	soldierResult := f.soldiers.Tick(grid, g.buildings, g.opposingBuildingsFor(f.soldiers), g.opposingSoldiersFor(f.soldiers), g.opposingIntruderTargetsForSoldiers(f.soldiers))
	g.recordLastAttacker(f.owner, soldierResult.KillOwners)
	g.recordLastAttacker(f.owner, sentryResult.KillOwners)

	f.pop.Deaths += serfResult.Deaths + villagerDeaths + sentryResult.Deaths + soldierResult.Deaths
	f.pop.UnitsDismissed += serfResult.Dismissed
	// Real cross-faction kills/building destructions -- see cmd/game's
	// player tick block for the same aggregation and the playtest report
	// this fixes.
	f.pop.Kills += soldierResult.Kills + sentryResult.Kills
	f.pop.EnemyBuildingsDestroyed += soldierResult.BuildingsDestroyed
	// The map's one shared, profession-free death animation (see
	// internal/render's DeathEffect) -- per the user's own report ("у
	// меня есть спрайт 3х кадровый смерти любого юнита... не вижу его в
	// последних коммитах"): real combat deaths/kills never triggered it
	// before, only the (now removed) sandbox debug enemy did.
	for _, p := range soldierResult.DeathPositions {
		g.addDeathEffect(p.X, p.Y)
	}
	for _, p := range soldierResult.KillPositions {
		g.addDeathEffect(p.X, p.Y)
	}
	for _, p := range sentryResult.KillPositions {
		g.addDeathEffect(p.X, p.Y)
	}

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
