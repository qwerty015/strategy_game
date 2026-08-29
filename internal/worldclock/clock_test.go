package worldclock

import "testing"

func hourTicks(hour float64) int {
	return int(hour / 24 * TicksPerDay)
}

// TestAtPhaseBoundaries locks in the fix for a real bug the user caught
// live in-game ("у тебя 16 часов ночи"): the first version anchored
// Sunrise's start at midnight instead of near dawn, which left most of the
// actual afternoon (16:00 included) classified as Night. Every case here is
// expressed as a real clock hour so a wrong boundary is obvious on sight.
func TestAtPhaseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hour  float64
		phase Phase
	}{
		{"midnight", 0, Night},
		{"3am", 3, Night},
		{"just before sunrise", sunriseStartHour - 0.01, Night},
		{"sunrise starts", sunriseStartHour, Sunrise},
		{"just before day", sunriseEndHour - 0.01, Sunrise},
		{"day starts", sunriseEndHour, Day},
		{"9am", 9, Day},
		{"noon", 12, Day},
		{"4pm -- the exact hour the user reported as wrongly Night", 16, Day},
		{"just before sunset", dayEndHour - 0.01, Day},
		{"sunset starts", dayEndHour, Sunset},
		{"just before night", sunsetEndHour - 0.01, Sunset},
		{"night starts", sunsetEndHour, Night},
		{"10pm", 22, Night},
		{"just before midnight", 23.99, Night},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := At(hourTicks(tc.hour)).Phase
			if got != tc.phase {
				t.Errorf("At(hour=%v).Phase = %v, want %v", tc.hour, got, tc.phase)
			}
		})
	}
}

func TestAtWrapsToNextDay(t *testing.T) {
	if got := At(TicksPerDay).Phase; got != Night {
		t.Errorf("At(TicksPerDay).Phase = %v, want Night (same as ticks=0)", got)
	}
	if got := At(TicksPerDay + hourTicks(16)).Phase; got != Day {
		t.Errorf("At(TicksPerDay + 16h).Phase = %v, want Day", got)
	}
}

func TestAtHourWrapsWithinADay(t *testing.T) {
	if got := At(0).Hour; got != 0 {
		t.Errorf("At(0).Hour = %f, want 0", got)
	}
	if got := At(TicksPerDay / 2).Hour; got != 12 {
		t.Errorf("At(TicksPerDay/2).Hour = %f, want 12", got)
	}
	if got := At(TicksPerDay).Hour; got != 0 {
		t.Errorf("At(TicksPerDay).Hour = %f, want 0 (wraps)", got)
	}
}

func TestAtNegativeClampsToZero(t *testing.T) {
	if got := At(-500).Phase; got != Night {
		t.Errorf("At(-500).Phase = %v, want Night (clamped to ticks=0)", got)
	}
}
