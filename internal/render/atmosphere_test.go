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

func TestAtmosphericTwilightIsLimitedToEvening(t *testing.T) {
	original := animFrame
	t.Cleanup(func() { animFrame = original })

	animFrame = 0
	if got := atmosphericTwilight(); got != 0 {
		t.Fatalf("daylight twilight=%f, want 0", got)
	}
	animFrame = 12420 // 69% of the 18,000-frame visual cycle
	if got := atmosphericTwilight(); got < 0.98 {
		t.Fatalf("evening twilight=%f, want near 1", got)
	}
	animFrame = 17100 // 95% of the 18,000-frame visual cycle
	if got := atmosphericTwilight(); got != 0 {
		t.Fatalf("next daylight twilight=%f, want 0", got)
	}
}
