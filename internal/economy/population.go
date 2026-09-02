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

	// Kills counts enemies whose death cmd/game has actually processed
	// (see pruneDeadEnemies) -- regardless of whether a Sentry's stone or
	// a soldier's blow finished them off. Shown on the empty-selection
	// town summary panel alongside a derived development score -- see
	// the user's explicit request for a kill counter and "some kind of
	// development score".
	Kills int
}
