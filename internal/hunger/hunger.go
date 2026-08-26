// Package hunger defines the shared satiety clock used by every living unit.
// Keeping the thresholds in one pure-Go package prevents professions from
// silently drifting to different life spans as more workers are added.
package hunger

const (
	// MaxTicks is a full life reserve: one percent of satiety is consumed
	// every ten simulation ticks, so a newly fed unit reaches zero after
	// 1000 ticks (about eight minutes and twenty seconds at normal speed).
	MaxTicks = 1000

	// MealThresholdTicks is the elapsed-hunger value at 20% satiety. Units
	// request food from a Tavern at this point, leaving 200 ticks for travel
	// and any remaining work before starvation becomes fatal.
	MealThresholdTicks = 800
)

// Percent converts elapsed ticks since the last meal to the player-facing
// 0-100 satiety scale. Satiety changes in whole percent steps every ten ticks.
func Percent(elapsed int) int {
	if elapsed <= 0 {
		return 100
	}
	percent := 100 - elapsed/10
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
