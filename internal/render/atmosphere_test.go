package render

import "testing"

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

// TestAtmosphericTwilightIsFrozen locks in the user's explicit request
// ("убери смену освещения, пусть всегда будет статично и только при дожде -
// темнее"): lighting never cycles through the day any more, at any frame.
func TestAtmosphericTwilightIsFrozen(t *testing.T) {
	original := animFrame
	t.Cleanup(func() { animFrame = original })

	for _, frame := range []int{0, 12420, 17100, 999999} {
		animFrame = frame
		if got := atmosphericTwilight(); got != 0 {
			t.Fatalf("frame %d: twilight=%f, want 0 (lighting must stay static)", frame, got)
		}
	}
}
