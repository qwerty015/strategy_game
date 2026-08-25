package economy

// Population is a display-only headcount for the HUD ("Население: N").
// It is no longer independently simulated: real individual hunger now
// lives on the actual units (package logistics' Serf, package
// villagers' Villager), each of which walks to a Tavern to eat -- see
// AGENTS.md. The caller (cmd/game) sets Count each frame to the number
// of actual units in play.
type Population struct {
	Count int
}
