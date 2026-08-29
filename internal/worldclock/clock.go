// Package worldclock is the game's day/night cycle: pure arithmetic over a
// simulation tick count, no ebiten dependency, so both render (atmosphere
// tint, ambient wildlife) and ui (the clock/minimap panel) can share one
// definition of "what time is it" without either depending on the other.
package worldclock

// TicksPerDay is how many simulation ticks make up one full in-game day.
// The simulation already runs a fixed tick regardless of render framerate
// (see economy.Simulator); tying the clock to ticks rather than render
// frames means pausing freezes the day/night cycle and changing game speed
// speeds it up or slows it down right along with everything else -- unlike
// rain/clouds (see internal/render/atmosphere.go), which deliberately stay
// on the render clock so weather never visibly jumps when the player pauses
// or changes speed. At Normal speed (2 ticks/sec) 2400 ticks is 20 minutes
// of real play per in-game day: slow enough to read as a real rhythm, fast
// enough that a player sees it turn over in a normal session.
const TicksPerDay = 2400

// Phase is the coarse segment of the day a given tick falls in.
type Phase int

const (
	Night Phase = iota
	Sunrise
	Day
	Sunset
)

// Boundaries as an in-game hour (0..24, midnight = 0). Real bug the user
// caught ("у тебя 16 часов ночи"): the first version of this file anchored
// Sunrise's *start* at hour 0 (midnight) instead of near dawn, which pushed
// every other boundary out of place and left most of the actual afternoon
// classified as Night. These are anchored to where each phase actually
// belongs on a 24-hour clock; Night correctly wraps across midnight.
const (
	sunriseStartHour = 5.0
	sunriseEndHour   = 7.0
	dayEndHour       = 19.0
	sunsetEndHour    = 21.0
	// Night covers [sunsetEndHour, 24) and [0, sunriseStartHour).
)

// State is the game clock at a given tick count.
type State struct {
	Phase Phase
	Hour  float64 // 0..24, midnight = 0, noon = 12
}

// At computes the day/night state for a given absolute simulation tick
// count. Negative ticks (should not happen, but save/load or a fresh game
// could hand this 0 or an unset value) are clamped to 0.
func At(ticks int) State {
	if ticks < 0 {
		ticks = 0
	}
	frac := float64(ticks%TicksPerDay) / float64(TicksPerDay)
	hour := frac * 24

	phase := Night
	switch {
	case hour < sunriseStartHour:
		phase = Night
	case hour < sunriseEndHour:
		phase = Sunrise
	case hour < dayEndHour:
		phase = Day
	case hour < sunsetEndHour:
		phase = Sunset
	default:
		phase = Night
	}
	return State{Phase: phase, Hour: hour}
}
