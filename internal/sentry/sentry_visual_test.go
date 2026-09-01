package sentry

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/enemy"
	"strategy_game/internal/resource"
)

func TestSentryShotVisualStartsOnlyAfterSuccessfulShot(t *testing.T) {
	tower := &building.Building{
		Kind:        building.WatchTower,
		X:           10,
		Y:           12,
		InputBuffer: map[resource.Type]int{resource.StoneBlock: 1},
	}
	controller := NewController()
	guard := controller.Spawn(tower)
	target := enemy.New(11, 12)

	tickController(controller, []*building.Building{tower}, []*enemy.Enemy{target})
	fromX, fromY, targetX, targetY, progress, ok := guard.ShotVisual()
	if !ok {
		t.Fatal("ShotVisual() = unavailable after a successful stone shot")
	}
	if fromX != 10 || fromY != 12 || targetX != 11 || targetY != 12 {
		t.Fatalf("ShotVisual() tiles = (%d,%d) -> (%d,%d), want (10,12) -> (11,12)", fromX, fromY, targetX, targetY)
	}
	if progress <= 0 || progress > 1 {
		t.Fatalf("ShotVisual() progress = %v, want (0, 1]", progress)
	}
}
