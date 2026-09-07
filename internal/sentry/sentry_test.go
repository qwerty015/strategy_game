package sentry

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
)

// tickControllerWithIntruders runs one simulation step exactly like
// cmd/game does: a fresh ledger, seeded from this controller's own
// in-flight Sentries, then Tick with a "1×1 против ИИ"
// intruders list -- see IntruderTarget's doc comment.
func tickControllerWithIntruders(c *Controller, buildings []*building.Building, intruders []IntruderTarget) {
	ledger := reservations.New()
	c.Reserve(ledger)
	c.Tick(buildings, intruders, ledger)
}

func straightRoad(fromX, toX, y int) []*building.Building {
	var roads []*building.Building
	for x := fromX; x < toX; x++ {
		roads = append(roads, &building.Building{Kind: building.Road, X: x, Y: y})
	}
	return roads
}

// TestSentry_FiresAtOpposingFactionIntruderAndKillsIt is a real gap found
// from an actual playtest report ("почему башня не убила его слуг"): a
// WatchTower could only ever fire at the sandbox-only enemy.Enemy, with
// no way at all to target a "1×1 против ИИ" opponent's unit -- an enemy
// serf could walk right past a tower unharmed. Uses a plain struct
// (standing in for e.g. a rival serf) with its own Alive/Kill closures,
// exactly the shape cmd/game's real intruderTargetsFrom builds from every
// actual worker/soldier controller.
func TestSentry_FiresAtOpposingFactionIntruderAndKillsIt(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)

	c := NewController()
	c.Spawn(tower)

	killed := false
	alive := true
	intruder := IntruderTarget{
		X: tower.X + 1, Y: tower.Y, // within WatchTowerRange
		Alive: func() bool { return alive },
		Kill:  func() { killed = true; alive = false },
	}

	tickControllerWithIntruders(c, []*building.Building{tower}, []IntruderTarget{intruder})

	if killed {
		t.Fatal("intruder died on the very tick it was fired at -- the kill must wait for the stone to visually arrive, same as against a debug enemy")
	}
	if tower.InputBuffer[resource.StoneBlock] != building.BufferCapacity-1 {
		t.Fatalf("tower stone = %d, want %d (one shot consumed immediately)", tower.InputBuffer[resource.StoneBlock], building.BufferCapacity-1)
	}

	for range shotVisualLifetime {
		tickControllerWithIntruders(c, []*building.Building{tower}, []IntruderTarget{intruder})
	}
	if !killed {
		t.Fatal("intruder never got killed after the stone's full flight time")
	}
}

// TestController_Tick_ReportsKillOnIntruderKill is the regression test for
// a real playtest report ("счетчик убито врагов не считает юнитов, нужно
// считать убитых с помощью башни или убитых боевыми юнитами"):
// Controller.Tick's TickResult must actually report a Kill once a
// WatchTower's stone visually lands on a real "1×1 против ИИ" opponent's
// unit, not just count the sandbox debug enemy.Enemy.
func TestController_Tick_ReportsKillOnIntruderKill(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)

	c := NewController()
	c.Spawn(tower)

	alive := true
	intruder := IntruderTarget{
		X: tower.X + 1, Y: tower.Y,
		Alive: func() bool { return alive },
		Kill:  func() { alive = false },
	}

	tick := func() TickResult {
		ledger := reservations.New()
		c.Reserve(ledger)
		return c.Tick([]*building.Building{tower}, []IntruderTarget{intruder}, ledger)
	}

	if result := tick(); result.Kills != 0 {
		t.Fatalf("TickResult.Kills on the throwing tick = %d, want 0 (the kill waits for the stone to visually arrive)", result.Kills)
	}

	var gotKill bool
	for range shotVisualLifetime {
		if result := tick(); result.Kills != 0 {
			gotKill = true
			if result.Kills != 1 {
				t.Fatalf("TickResult.Kills on the arrival tick = %d, want 1", result.Kills)
			}
		}
	}
	if !gotKill {
		t.Fatal("TickResult.Kills never reported 1 across the stone's full flight time")
	}
}

// TestSentry_KillNeverLandsBeforeTheStoneVisuallyArrives is a direct
// regression test for the user's exact bug report: "раньше было сперва
// противник погибает, а потом летит камень в него" -- while ShotVisual()
// still reports an in-flight stone (ok == true), the target must remain
// alive; only once ShotVisual() stops reporting one (the stone has
// arrived) may the target be dead.
func TestSentry_KillNeverLandsBeforeTheStoneVisuallyArrives(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)

	c := NewController()
	guard := c.Spawn(tower)
	alive := true
	intruder := IntruderTarget{
		X: tower.X + 1, Y: tower.Y,
		Alive: func() bool { return alive },
		Kill:  func() { alive = false },
	}

	tickControllerWithIntruders(c, []*building.Building{tower}, []IntruderTarget{intruder})

	for range shotVisualLifetime + 2 {
		_, _, _, _, _, stillInFlight := guard.ShotVisual()
		if stillInFlight && !alive {
			t.Fatal("intruder is dead while ShotVisual() still reports the stone in flight -- death happened before the stone visually arrived")
		}
		tickControllerWithIntruders(c, []*building.Building{tower}, []IntruderTarget{intruder})
	}

	if alive {
		t.Fatal("intruder is still alive well after the stone's flight time -- the kill never landed at all")
	}
}

func TestSentry_DoesNotFireOutOfRange(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)

	c := NewController()
	c.Spawn(tower)

	alive := true
	intruder := IntruderTarget{
		X: tower.X + WatchTowerRange + 1, Y: tower.Y, // just outside range
		Alive: func() bool { return alive },
		Kill:  func() { alive = false },
	}

	tickControllerWithIntruders(c, []*building.Building{tower}, []IntruderTarget{intruder})

	if !alive {
		t.Fatal("intruder was killed despite being out of range")
	}
	if got := tower.InputBuffer[resource.StoneBlock]; got != building.BufferCapacity {
		t.Fatalf("tower stone = %d, want unchanged %d (never fired)", got, building.BufferCapacity)
	}
}

func TestSentry_DoesNotFireWithoutStone(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	// No stone delivered yet.

	c := NewController()
	c.Spawn(tower)

	alive := true
	intruder := IntruderTarget{
		X: tower.X + 1, Y: tower.Y,
		Alive: func() bool { return alive },
		Kill:  func() { alive = false },
	}

	tickControllerWithIntruders(c, []*building.Building{tower}, []IntruderTarget{intruder})

	if !alive {
		t.Fatal("intruder was killed despite the tower having no stone to fire")
	}
}

// TestSentry_RespectsShotCooldown uses two separate targets so the
// cooldown itself is what's under test -- with the kill deferred until
// the stone lands, reusing a single target could otherwise pass for the
// wrong reason. ShotCooldownTicks (20) comfortably outlasts
// shotVisualLifetime, so the first stone has always landed well before
// the cooldown that same shot started could ever let a second one fly.
func TestSentry_RespectsShotCooldown(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)

	c := NewController()
	c.Spawn(tower)

	firstAlive, secondAlive := true, true
	intruders := []IntruderTarget{
		{X: tower.X + 1, Y: tower.Y, Alive: func() bool { return firstAlive }, Kill: func() { firstAlive = false }},
		{X: tower.X + 1, Y: tower.Y + 1, Alive: func() bool { return secondAlive }, Kill: func() { secondAlive = false }},
	}

	// First tick fires at one of the two (both equidistant); consumes the
	// stone and starts the cooldown, but the kill hasn't landed yet.
	tickControllerWithIntruders(c, []*building.Building{tower}, intruders)
	if got := tower.InputBuffer[resource.StoneBlock]; got != building.BufferCapacity-1 {
		t.Fatalf("tower stone after first tick = %d, want %d (one shot fired)", got, building.BufferCapacity-1)
	}

	// Let the first stone's flight time fully elapse. Exactly one of the
	// two dies; the cooldown from that same shot is still far from over,
	// so no second shot has gone out yet.
	for range shotVisualLifetime {
		tickControllerWithIntruders(c, []*building.Building{tower}, intruders)
	}
	deaths := 0
	if !firstAlive {
		deaths++
	}
	if !secondAlive {
		deaths++
	}
	if deaths != 1 {
		t.Fatalf("exactly one of the two intruders should be dead by now, got %d", deaths)
	}
	if got := tower.InputBuffer[resource.StoneBlock]; got != building.BufferCapacity-1 {
		t.Fatalf("tower stone right after the first kill landed = %d, want still %d (still cooling down, no second shot yet)", got, building.BufferCapacity-1)
	}

	// Once the cooldown (started at the first shot) fully elapses, the
	// Sentry fires again at whichever target is still alive, and it dies
	// once that second stone's own flight time elapses too. A generous
	// margin covers both: the remaining cooldown plus a full new flight.
	for range ShotCooldownTicks + shotVisualLifetime {
		tickControllerWithIntruders(c, []*building.Building{tower}, intruders)
	}
	if firstAlive || secondAlive {
		t.Fatal("both intruders should be dead after the cooldown elapsed and the second stone landed")
	}
}

func TestSentry_IgnoresDeadIntruders(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)

	c := NewController()
	c.Spawn(tower)

	intruder := IntruderTarget{
		X: tower.X + 1, Y: tower.Y,
		Alive: func() bool { return false }, // already dead
		Kill:  func() {},
	}

	tickControllerWithIntruders(c, []*building.Building{tower}, []IntruderTarget{intruder})

	if got := tower.InputBuffer[resource.StoneBlock]; got != building.BufferCapacity {
		t.Fatalf("tower stone = %d, want unchanged %d (target already dead)", got, building.BufferCapacity)
	}
}

func TestSentry_WalksToTavernWhenHungryAndBack(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 0, Y: 0}
	tavern := &building.Building{Kind: building.Tavern, X: 5, Y: 0}
	tavern.AddInput(resource.Bread, 3)

	buildings := append([]*building.Building{tower, tavern}, straightRoad(1, 5, 0)...)

	c := NewController()
	c.Spawn(tower)
	s := c.Sentries[0]

	for range HungerInterval + 200 {
		tickControllerWithIntruders(c, buildings, nil)
		if s.Working() && s.X == tower.X && s.Y == tower.Y {
			break
		}
	}

	if !s.Working() {
		t.Fatalf("sentry phase = %v, want back to working", s.ph)
	}
	if s.X != tower.X || s.Y != tower.Y {
		t.Fatalf("sentry position = (%d,%d), want back at home (%d,%d)", s.X, s.Y, tower.X, tower.Y)
	}
	if s.SatietyPercent() <= 0 {
		t.Fatalf("SatietyPercent() = %d after eating, want > 0", s.SatietyPercent())
	}
}

func TestController_HasHomeAndRemoveHome(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 0, Y: 0}
	c := NewController()
	if c.HasHome(tower) {
		t.Fatal("HasHome(tower) = true before any Spawn")
	}
	c.Spawn(tower)
	if !c.HasHome(tower) {
		t.Fatal("HasHome(tower) = false after Spawn")
	}
	c.RemoveHome(tower)
	if c.HasHome(tower) {
		t.Fatal("HasHome(tower) = true after RemoveHome")
	}
	if len(c.Sentries) != 0 {
		t.Fatalf("len(Sentries) = %d after RemoveHome, want 0", len(c.Sentries))
	}
}
