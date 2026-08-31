package ui

import "testing"

func TestFormatPlayedFrames(t *testing.T) {
	for _, tc := range []struct {
		frames int
		want   string
	}{
		{0, "0:00:00"},
		{59, "0:00:00"},
		{60, "0:00:01"},
		{60*65 + 12, "0:01:05"},
		{60 * (2*3600 + 3*60 + 4), "2:03:04"},
		{-60, "0:00:00"},
	} {
		if got := formatPlayedFrames(tc.frames); got != tc.want {
			t.Errorf("formatPlayedFrames(%d) = %q, want %q", tc.frames, got, tc.want)
		}
	}
}
