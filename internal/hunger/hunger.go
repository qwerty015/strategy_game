// Package hunger defines the shared satiety clock used by every living unit.
// Keeping the thresholds in one pure-Go package prevents professions from
// silently drifting to different life spans as more workers are added.
package hunger

const (
	// MaxTicks is a full life reserve: 30% longer than the original 1000
	// ticks, per the user's explicit request ("увелич на 30% время
	// голода, чтобы юниты жили дольше") -- one percent of satiety is now
	// consumed every thirteen simulation ticks, so a newly fed unit
	// reaches zero after 1300 ticks (about ten minutes and forty-three
	// seconds at normal speed, versus the previous eight minutes twenty).
	MaxTicks = 1300

	// MealThresholdTicks is the elapsed-hunger value at 20% satiety --
	// still the same 80%-of-MaxTicks point as before the 30% increase, so
	// a unit still has proportionally the same 20% cushion (now 260
	// ticks instead of 200) to reach a Tavern before starvation becomes
	// fatal.
	MealThresholdTicks = 1040
)

// Percent converts elapsed ticks since the last meal to the player-facing
// 0-100 satiety scale. Uses MaxTicks directly (not a hardcoded divisor) so
// satiety always spans exactly 0-100 regardless of how MaxTicks is tuned.
func Percent(elapsed int) int {
	if elapsed <= 0 {
		return 100
	}
	percent := 100 - elapsed*100/MaxTicks
	if percent < 0 {
		return 0
	}
	return percent
}

// NeedsMeal reports whether a unit has reached the 20% meal threshold.
func NeedsMeal(elapsed int) bool {
	return elapsed >= MealThresholdTicks
}

// Dead reports whether satiety is empty and the unit must be removed.
func Dead(elapsed int) bool {
	return elapsed >= MaxTicks
}
