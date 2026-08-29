package economy

import "testing"

func TestSimulator_AdvanceRespectsSpeed(t *testing.T) {
	tests := []struct {
		name  string
		speed Speed
		want  int
	}{
		{name: "paused", speed: Paused, want: 0},
		{name: "half", speed: Half, want: 0},
		{name: "normal", speed: Normal, want: 1},
		{name: "double", speed: Double, want: 2},
		{name: "quadruple", speed: Quadruple, want: 4},
		{name: "octuple", speed: Octuple, want: 8},
		{name: "sixteenfold", speed: Sixteenfold, want: 16},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSimulator(30)
			s.SetSpeed(tc.speed)
			got := 0
			for range 30 {
				got += s.Advance()
			}
			if got != tc.want {
				t.Fatalf("Advance() over 30 frames = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSimulator_InvalidSpeedFallsBackToNormal(t *testing.T) {
	s := NewSimulator(30)
	s.SetSpeed(Speed(-1))
	if got := s.Speed(); got != Normal {
		t.Fatalf("Speed() = %v, want Normal", got)
	}
}
