package main

import (
	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/enemy"
	"strategy_game/internal/i18n"
	"strategy_game/internal/pathfind"
	"strategy_game/internal/soldier"
)

// soldierGroupRadius is how far (Chebyshev) from the clicked soldier every
// same-profession soldier is swept into the group -- per the user's
// explicit "клик на лучника - выделяются и управляются сразу все лучники в
// радиусе 4 клеток от выбранного, с мечниками также" (letting the player
// keep 2-3 separate groups by spacing them out on the map).
const soldierGroupRadius = 4

// soldierGroupNear collects every living soldier of center's own
// Profession within soldierGroupRadius tiles of it, center included.
func (g *Game) soldierGroupNear(center *soldier.Soldier) []*soldier.Soldier {
	var group []*soldier.Soldier
	for _, sd := range g.soldiers.Soldiers {
		if !sd.Alive() || sd.Profession != center.Profession {
			continue
		}
		if absInt(sd.X-center.X) <= soldierGroupRadius && absInt(sd.Y-center.Y) <= soldierGroupRadius {
			group = append(group, sd)
		}
	}
	return group
}

// mergeSoldierGroups combines a and b, deduplicated by pointer -- used by
// a Shift+click on a second soldier group (see handleLeftClick).
func mergeSoldierGroups(a, b []*soldier.Soldier) []*soldier.Soldier {
	seen := make(map[*soldier.Soldier]bool, len(a)+len(b))
	merged := make([]*soldier.Soldier, 0, len(a)+len(b))
	for _, list := range [2][]*soldier.Soldier{a, b} {
		for _, sd := range list {
			if !seen[sd] {
				seen[sd] = true
				merged = append(merged, sd)
			}
		}
	}
	return merged
}

// soldierGroupOfProfession collects every living soldier of profession p
// game-wide -- the "отряд по виду" inspector button, per the user's
// "либо группировать их по виду" alternative to the radius-based pick.
func (g *Game) soldierGroupOfProfession(p soldier.Profession) []*soldier.Soldier {
	var group []*soldier.Soldier
	for _, sd := range g.soldiers.Soldiers {
		if sd.Alive() && sd.Profession == p {
			group = append(group, sd)
		}
	}
	return group
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// enemyDetectionRadius is how far (Chebyshev) an enemy can be from a
// soldier and still trigger checkEnemySightings' alert -- per the user's
// clarifying answer to "обнаружение на радиусе ~6-8 клеток", picked at
// the recommended midpoint.
const enemyDetectionRadius = 8

// checkEnemySightings alerts the player and drops the simulation speed to
// Normal (1x) the first time a live enemy comes within
// enemyDetectionRadius of any soldier -- per the user's explicit request
// ("при появлении... враг - советник говорит юзеру об этом, сбрасываем
// скорость в х1"). Edge-triggered per enemy (g.sightedEnemies) so it
// fires once per sighting, not every tick while the enemy lingers in
// range; the entry is cleared in pruneDeadEnemies once the enemy dies, so
// a later respawn/re-sighting alerts again.
func (g *Game) checkEnemySightings() {
	if len(g.enemies) == 0 || len(g.soldiers.Soldiers) == 0 {
		return
	}
	if g.sightedEnemies == nil {
		g.sightedEnemies = map[*enemy.Enemy]bool{}
	}
	for _, e := range g.enemies {
		if !e.Alive() || g.sightedEnemies[e] {
			continue
		}
		for _, sd := range g.soldiers.Soldiers {
			if !sd.Alive() {
				continue
			}
			if absInt(sd.X-e.X) > enemyDetectionRadius || absInt(sd.Y-e.Y) > enemyDetectionRadius {
				continue
			}
			g.sightedEnemies[e] = true
			g.sim.SetSpeed(economy.Normal)
			g.statusMsg = i18n.T().AdvisorTipEnemySighted
			break
		}
	}
}

// enemyAt returns the living enemy standing on (tx, ty), if any -- used by
// the soldier group's right-click handling to tell an attack order apart
// from a plain move order.
func (g *Game) enemyAt(tx, ty int) *enemy.Enemy {
	for i := len(g.enemies) - 1; i >= 0; i-- {
		e := g.enemies[i]
		if e.X == tx && e.Y == ty && e.Alive() {
			return e
		}
	}
	return nil
}

// commandSoldierGroupTo is a right-click on an empty map tile while a
// SelectionSoldierGroup is active: the click point becomes the center of a
// g.formationLines-rank formation (per the user's "чтобы юнитов можно было
// расположить в 1/2/3 линии"), and every soldier in the group gets a
// MoveTo to its own slot in that grid -- see soldierFormationPositions.
// Cancels any standing attack order the whole group had (Soldier.MoveTo
// already does this per-soldier) and clears the red attack marker.
func (g *Game) commandSoldierGroupTo(mx, my int) {
	tx, ty := g.camera.ScreenToTile(mx, my)
	group := g.selection.SoldierGroup
	positions := soldierFormationPositions(tx, ty, len(group), g.formationLines)
	for i, sd := range group {
		p := positions[i]
		sd.MoveTo(g.grid, g.buildings, p.X, p.Y)
	}
	g.attackMarkerTarget = nil
}

// commandSoldierGroupAttack is a right-click landing on a live enemy while
// a SelectionSoldierGroup is active: every soldier in the group gets a
// standing AttackOrder against it, and the red square marker (per the
// user's explicit "отмечает противника красной рамкой") tracks it until it
// dies -- see the tick loop's attackMarkerTarget liveness check.
func (g *Game) commandSoldierGroupAttack(target *enemy.Enemy) {
	for _, sd := range g.selection.SoldierGroup {
		sd.AttackOrder(g.grid, g.buildings, target)
	}
	g.attackMarkerTarget = target
}

// opposingBuildingAt returns the opposing faction's building (any tile of
// its footprint, matching buildingSelectionAt's own convention) standing
// at (tx, ty), if any -- used by the soldier group's right-click handling
// to tell a cross-faction attack order apart from a plain move order, the
// same way enemyAt already does for the sandbox debug enemy. Reuses
// opposingBuildingsFor(g.soldiers) -- the exact same candidate list
// automatic FactionEngageRange engagement already targets, so a manual
// click can never attack something auto-engage wouldn't have fought too.
func (g *Game) opposingBuildingAt(tx, ty int) *building.Building {
	for _, b := range g.opposingBuildingsFor(g.soldiers) {
		if b == nil || b.HP <= 0 {
			continue
		}
		footprint := building.Types[b.Kind].Footprint
		if tx >= b.X && tx < b.X+footprint && ty >= b.Y && ty < b.Y+footprint {
			return b
		}
	}
	return nil
}

// commandSoldierGroupAttackFaction is opposingBuildingAt's counterpart to
// commandSoldierGroupAttack: every soldier in the group gets a standing
// AttackFactionOrder against the clicked opposing building -- see that
// method's own doc comment for the real bug this fixes (a plain move
// order that never actually attacked anything).
func (g *Game) commandSoldierGroupAttackFaction(target *building.Building) {
	for _, sd := range g.selection.SoldierGroup {
		sd.AttackFactionOrder(target)
	}
}

// soldierFormationPositions lays out n points on a lines-rank grid
// centered on (cx, cy) -- ceil(n/lines) columns per rank, per the user's
// "юнитов можно было расположить в 1/2/3 линии (шеренги)". Deliberately no
// facing/orientation, just a plain grid around the click point (see the
// plan's "что сознательно не делаем" list).
func soldierFormationPositions(cx, cy, n, lines int) []pathfind.Point {
	if n <= 0 {
		return nil
	}
	if lines < 1 {
		lines = 1
	}
	if lines > 3 {
		lines = 3
	}
	cols := (n + lines - 1) / lines
	positions := make([]pathfind.Point, 0, n)
	for i := 0; i < n; i++ {
		row := i / cols
		col := i % cols
		dx := col - (cols-1)/2
		dy := row - (lines-1)/2
		positions = append(positions, pathfind.Point{X: cx + dx, Y: cy + dy})
	}
	return positions
}
