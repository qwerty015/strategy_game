package sentry

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/enemy"
	"strategy_game/internal/reservations"
	"strategy_game/internal/resource"
)

// tickController runs one simulation step exactly like cmd/game does: a
// fresh ledger, seeded from this controller's own in-flight Sentries,
// then Tick.
func tickController(c *Controller, buildings []*building.Building, enemies []*enemy.Enemy) {
	ledger := reservations.New()
	c.Reserve(ledger)
	c.Tick(buildings, enemies, ledger)
}

func straightRoad(fromX, toX, y int) []*building.Building {
	var roads []*building.Building
	for x := fromX; x < toX; x++ {
		roads = append(roads, &building.Building{Kind: building.Road, X: x, Y: y})
	}
	return roads
}

func TestSentry_FiresAtEnemyInRangeAndConsumesStone(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)

	c := NewController()
	c.Spawn(tower)

	e := enemy.New(tower.X+1, tower.Y) // within WatchTowerRange

	tickController(c, []*building.Building{tower}, []*enemy.Enemy{e})

	// Per the user's explicit request, a stone hit is an instant kill --
	// unlike a building, which still only takes combat.DamagePerHit.
	if e.Alive() {
		t.Fatalf("enemy HP after one tick = %d, want dead (one hit kills)", e.HP)
	}
	if tower.InputBuffer[resource.StoneBlock] != building.BufferCapacity-1 {
		t.Fatalf("tower stone = %d, want %d (one shot consumed)", tower.InputBuffer[resource.StoneBlock], building.BufferCapacity-1)
	}
}

func TestSentry_DoesNotFireOutOfRange(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)

	c := NewController()
	c.Spawn(tower)

	e := enemy.New(tower.X+WatchTowerRange+1, tower.Y) // just outside range

	tickController(c, []*building.Building{tower}, []*enemy.Enemy{e})

	if e.HP != enemy.MaxHP {
		t.Fatalf("enemy HP = %d, want unchanged %d (out of range)", e.HP, enemy.MaxHP)
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

	e := enemy.New(tower.X+1, tower.Y)

	tickController(c, []*building.Building{tower}, []*enemy.Enemy{e})

	if e.HP != enemy.MaxHP {
		t.Fatalf("enemy HP = %d, want unchanged %d (no stone to fire)", e.HP, enemy.MaxHP)
	}
}

// TestSentry_RespectsShotCooldown uses two separate targets so the
// cooldown itself is what's under test -- with a one-hit kill, reusing a
// single already-dead target would pass for the wrong reason (nothing
// left alive to shoot), not because the cooldown blocked a second shot.
func TestSentry_RespectsShotCooldown(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)

	c := NewController()
	c.Spawn(tower)

	first := enemy.New(tower.X+1, tower.Y)
	second := enemy.New(tower.X+1, tower.Y+1)
	enemies := []*enemy.Enemy{first, second}

	// First tick kills the first target and starts the cooldown.
	tickController(c, []*building.Building{tower}, enemies)
	if first.Alive() {
		t.Fatal("first enemy still alive after one tick, want dead (one hit kills)")
	}
	if !second.Alive() {
		t.Fatal("second enemy died on the same tick as the first -- only one shot per tick")
	}

	// Immediately after, still on cooldown: no second shot this tick.
	tickController(c, []*building.Building{tower}, enemies)
	if !second.Alive() {
		t.Fatal("second enemy died while the Sentry should still be on cooldown")
	}

	// Once the cooldown fully elapses, the Sentry fires again.
	for range ShotCooldownTicks {
		tickController(c, []*building.Building{tower}, enemies)
	}
	if second.Alive() {
		t.Fatal("second enemy still alive after the cooldown elapsed, want dead")
	}
}

func TestSentry_IgnoresDeadEnemies(t *testing.T) {
	tower := &building.Building{Kind: building.WatchTower, X: 10, Y: 10, ConstructionStage: building.ConstructionNone}
	tower.AddInput(resource.StoneBlock, building.BufferCapacity)

	c := NewController()
	c.Spawn(tower)

	dead := enemy.New(tower.X+1, tower.Y)
	dead.HP = 0

	tickController(c, []*building.Building{tower}, []*enemy.Enemy{dead})

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
		tickController(c, buildings, nil)
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
