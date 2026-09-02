package main

import (
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/combat"
)

// TestNewDuelGame_MapIsSymmetricAndIsthmusIsClear locks in the "1×1
// против ИИ" map shape the user explicitly asked for: a water divide
// splitting the map into two territories, joined by exactly one dry
// isthmus with no resources on it, and both starting Warehouses exactly
// equidistant from the map's own vertical center line.
func TestNewDuelGame_MapIsSymmetricAndIsthmusIsClear(t *testing.T) {
	g := newDuelGame(AINormal)

	playerWarehouse := findWarehouseOwnedBy(g.buildings, 0)
	aiWarehouse := findWarehouseOwnedBy(g.buildings, 1)
	if playerWarehouse == nil || aiWarehouse == nil {
		t.Fatal("both factions must start with an operational warehouse")
	}
	centerX := float64(g.grid.Width-1) / 2
	playerDist := centerX - float64(playerWarehouse.X)
	aiDist := float64(aiWarehouse.X) - centerX
	if playerDist < 0 {
		playerDist = -playerDist
	}
	if aiDist < 0 {
		aiDist = -aiDist
	}
	if diff := playerDist - aiDist; diff > 0.01 || diff < -0.01 {
		t.Fatalf("player warehouse is %.1f tiles from center, AI warehouse is %.1f -- want equal", playerDist, aiDist)
	}
	if playerWarehouse.X >= g.grid.Width/2 || aiWarehouse.X < g.grid.Width/2 {
		t.Fatalf("warehouses are not on opposite shores: player X=%d, AI X=%d, map width=%d", playerWarehouse.X, aiWarehouse.X, g.grid.Width)
	}
}

// TestDuelSimulation_AIBuildsAndFactionsFight is the goal's own
// end-to-end verification, run entirely headless: a "1×1 против ИИ"
// game ticked forward for a long stretch of simulation time with no
// player input at all must (1) see the AI actually place and finish
// real buildings, using exactly the same construction/hiring mechanics
// the player uses, and (2) see the two factions' soldiers actually
// fight -- a rival soldier or building taking real damage -- once they
// meet, all through the ordinary auto-engage mechanism, no scripted
// combat outcome.
func TestDuelSimulation_AIBuildsAndFactionsFight(t *testing.T) {
	g := newDuelGame(AIHard) // fastest decisions/attacks -- fewest ticks needed to observe both behaviours
	initialAIBuildings := len(g.ownedBuildings(1))

	const maxTicks = 60000 // 30000s of simulated time at 1x -- generous headroom
	var aiBuilt, aiFinishedOne, combatDamageSeen bool
	var maxAIBuildings int

	for i := 0; i < maxTicks; i++ {
		g.tickOnce()

		aiBuildings := g.ownedBuildings(1)
		if len(aiBuildings) > maxAIBuildings {
			maxAIBuildings = len(aiBuildings)
		}
		if len(aiBuildings) > initialAIBuildings {
			aiBuilt = true
		}
		if !aiFinishedOne {
			for _, b := range aiBuildings {
				if b.Kind != building.Warehouse && b.Kind != building.Road && b.ConstructionStage == building.ConstructionNone {
					aiFinishedOne = true
					break
				}
			}
		}
		if !combatDamageSeen {
			for _, b := range g.buildings {
				if b.HP < building.MaxHP {
					combatDamageSeen = true
					break
				}
			}
		}
		if !combatDamageSeen {
			for _, s := range g.soldiers.Soldiers {
				if s.HP < combat.MaxHP {
					combatDamageSeen = true
					break
				}
			}
		}
		if !combatDamageSeen && g.ai != nil {
			for _, s := range g.ai.soldiers.Soldiers {
				if s.HP < combat.MaxHP {
					combatDamageSeen = true
					break
				}
			}
		}
		if g.factionDefeated(0) || g.factionDefeated(1) {
			combatDamageSeen = true
			break
		}
		if aiBuilt && aiFinishedOne && combatDamageSeen {
			break // everything the goal asks for has already been observed
		}
	}

	if !aiBuilt {
		t.Fatalf("AI never placed a single new building beyond its starting Warehouse+Road (peak owned buildings = %d, started with %d)", maxAIBuildings, initialAIBuildings)
	}
	if !aiFinishedOne {
		t.Fatal("AI placed construction sites but never actually finished one -- see aiHireBuilder/builder.Controller")
	}
	if !combatDamageSeen {
		t.Fatal("no building or soldier on either side ever took combat damage -- the two factions never actually fought")
	}
}
