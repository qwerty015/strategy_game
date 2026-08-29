package render

import (
	"testing"

	"strategy_game/internal/worldclock"
)

func TestAtmosphericRainSchedule(t *testing.T) {
	original := animFrame
	t.Cleanup(func() { animFrame = original })

	for _, tc := range []struct {
		frame   int
		raining bool
	}{
		{atmosphereRainStartFrame - 1, false},
		{atmosphereRainStartFrame, true},
		{atmosphereRainEndFrame - 1, true},
		{atmosphereRainEndFrame, false},
	} {
		animFrame = tc.frame
		got, _ := atmosphericRain()
		if got != tc.raining {
			t.Errorf("frame %d: raining=%v, want %v", tc.frame, got, tc.raining)
		}
	}
}

// TestCurrentDayStateFollowsWorldTicksNotAnimFrame locks in the user's
// explicit request: the day/night cycle (and therefore ambient darkening,
// fireflies vs. butterflies, the night glow) follows the simulation tick
// count set via SetWorldTicks, not the render-frame clock rain/clouds use.
func TestCurrentDayStateFollowsWorldTicksNotAnimFrame(t *testing.T) {
	originalTicks, originalFrame := worldTicks, animFrame
	t.Cleanup(func() { worldTicks, animFrame = originalTicks, originalFrame })

	animFrame = 999999 // must have zero effect on the day/night phase
	SetWorldTicks(0)   // midnight
	if got := currentDayState().Phase; got != worldclock.Night {
		t.Fatalf("ticks=0 (midnight): phase=%v, want Night", got)
	}
	SetWorldTicks(worldclock.TicksPerDay / 2) // noon, well into Day
	if got := currentDayState().Phase; got != worldclock.Day {
		t.Fatalf("ticks=1/2 day (noon): phase=%v, want Day", got)
	}
}
