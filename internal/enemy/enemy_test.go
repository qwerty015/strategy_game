package enemy

import (
	"testing"

	"strategy_game/internal/building"
)

func TestNewIsAliveAtFullHealth(t *testing.T) {
	e := New(3, 4)
	if e.X != 3 || e.Y != 4 {
		t.Fatalf("New(3, 4) position = (%d, %d), want (3, 4)", e.X, e.Y)
	}
	if e.HP != MaxHP {
		t.Fatalf("New(...).HP = %d, want %d", e.HP, MaxHP)
	}
	if !e.Alive() {
		t.Fatal("Alive() = false for a freshly created enemy")
	}
}

func TestAliveIsFalseAtZeroHP(t *testing.T) {
	e := New(0, 0)
	e.HP = 0
	if e.Alive() {
		t.Fatal("Alive() = true at HP 0")
	}
}

func TestAliveIsFalseOnNilEnemy(t *testing.T) {
	var e *Enemy
	if e.Alive() {
		t.Fatal("Alive() = true on a nil *Enemy")
	}
}

func TestTick_DamagesAnAdjacentFinishedBuilding(t *testing.T) {
	target := &building.Building{Kind: building.Farm, X: 5, Y: 5, ConstructionStage: building.ConstructionNone, HP: building.MaxHP}
	e := New(target.X+1, target.Y) // adjacent, within AttackRange=1

	Tick([]*Enemy{e}, []*building.Building{target})

	if target.HP != building.MaxHP-10 {
		t.Fatalf("target.HP = %d, want %d (one hit)", target.HP, building.MaxHP-10)
	}
}

func TestTick_IgnoresBuildingsOutOfRange(t *testing.T) {
	target := &building.Building{Kind: building.Farm, X: 5, Y: 5, ConstructionStage: building.ConstructionNone, HP: building.MaxHP}
	e := New(target.X+5, target.Y)

	Tick([]*Enemy{e}, []*building.Building{target})

	if target.HP != building.MaxHP {
		t.Fatalf("target.HP = %d, want unchanged %d (out of range)", target.HP, building.MaxHP)
	}
}

func TestTick_IgnoresUnfinishedConstructionSites(t *testing.T) {
	site := building.NewConstructionSite(building.Farm, 5, 5)
	e := New(site.X+1, site.Y)

	Tick([]*Enemy{e}, []*building.Building{site})

	if site.HP != building.MaxHP {
		t.Fatalf("site.HP = %d, want unchanged %d (still under construction)", site.HP, building.MaxHP)
	}
}

func TestTick_RespectsAttackCooldown(t *testing.T) {
	target := &building.Building{Kind: building.Farm, X: 5, Y: 5, ConstructionStage: building.ConstructionNone, HP: building.MaxHP}
	e := New(target.X+1, target.Y)

	Tick([]*Enemy{e}, []*building.Building{target})
	Tick([]*Enemy{e}, []*building.Building{target})

	if target.HP != building.MaxHP-10 {
		t.Fatalf("target.HP after two ticks = %d, want %d (second hit still cooling down)", target.HP, building.MaxHP-10)
	}
}
