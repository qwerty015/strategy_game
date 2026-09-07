package soldier

import (
	"strategy_game/internal/combat"
	"strategy_game/internal/world"
	"testing"
)

func TestAttackFollowsMovingCivilian(t *testing.T) {
	grid := world.NewGrid(30, 10)
	c := &Controller{}
	s := &Soldier{Profession: Swordsman, X: 1, Y: 3, HP: 100}
	c.Soldiers = []*Soldier{s}
	x, y, alive := 2, 3, true
	target := combat.IntruderTarget{X: x, Y: y,
		Position: func() (int, int) { return x, y },
		Alive:    func() bool { return alive }, Kill: func() { alive = false }}
	s.AttackFactionIntruderOrder(target)
	x = 20 // Move after the order; the original tile is still in melee range.
	c.Tick(grid, nil, nil, nil, nil)
	if !alive {
		t.Fatal("killed a civilian at its old position")
	}
	for i := 0; i < 100 && alive; i++ {
		c.Tick(grid, nil, nil, nil, nil)
	}
	if alive {
		t.Fatal("never caught the moving target")
	}
	if !inRange(s.X, s.Y, x, y, s.AttackRange()) {
		t.Fatal("killed outside actual range")
	}
}
