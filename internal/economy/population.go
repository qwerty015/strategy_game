package economy

// Population is a display-only headcount for the HUD ("Население: N").
// It is no longer independently simulated: real individual hunger now
// lives on the actual units (package logistics' Serf, package
// villagers' Villager), each of which walks to a Tavern to eat -- see
// AGENTS.md. The caller (cmd/game) sets Count each frame to the number
// of actual units in play.
type Population struct {
	Count  int
	Deaths int // starvation deaths across all living unit types

	// BuildingsRemoved counts player-demolished structures, including every
	// individual road tile. UnitsDismissed counts workers/serfs who completed
	// a player-requested dismissal. Keeping them separate makes the town
	// summary describe what actually happened instead of one mixed total.
	BuildingsRemoved int
	UnitsDismissed   int

	// Removed is the legacy combined counter saved by formats through v3.
	// New code does not increment it; save migration moves its best-known
	// total into BuildingsRemoved because old saves cannot reconstruct the
	// original split after the fact.
	Removed int

	// Kills counts every enemy this faction's own forces have actually
	// finished off, regardless of whether a Sentry's stone or a soldier's
	// blow landed it: the sandbox-only debug enemy.Enemy (see cmd/game's
	// pruneDeadEnemies) AND, since a later playtest report ("счетчик
	// убито врагов не считает юнитов, нужно считать убитых с помощью
	// башни или убитых боевыми юнитами") found the counter never actually
	// counted a real opponent, every opposing-faction unit killed in "1×1
	// против ИИ" cross-faction combat too (see soldier.Controller.
	// TickResult.Kills and sentry.Controller.TickResult.Kills). Shown on
	// the empty-selection town summary panel alongside a derived
	// development score.
	Kills int

	// EnemyBuildingsDestroyed counts opposing-faction buildings this
	// faction's own soldiers have destroyed in cross-faction combat (see
	// soldier.Controller.TickResult.BuildingsDestroyed) -- a new counter
	// added alongside the Kills fix above, per the same report ("введи
	// новый счетчик: 'Разрушено построек'"). Distinct from
	// BuildingsRemoved above, which counts this faction's OWN buildings
	// demolished by its own player/AI, not an opponent's buildings
	// destroyed by force.
	EnemyBuildingsDestroyed int
}
