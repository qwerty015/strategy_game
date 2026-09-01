package render

import "testing"

func TestAdvanceDeathEffectsKeepsThenExpiresEffect(t *testing.T) {
	effects := []DeathEffect{NewDeathEffect(7, 9)}
	for range deathEffectLifetimeFrames - 1 {
		effects = AdvanceDeathEffects(effects)
	}
	if len(effects) != 1 || effects[0].Age != deathEffectLifetimeFrames-1 {
		t.Fatalf("effect before expiry = %#v, want one effect at age %d", effects, deathEffectLifetimeFrames-1)
	}
	effects = AdvanceDeathEffects(effects)
	if len(effects) != 0 {
		t.Fatalf("effects after expiry = %#v, want none", effects)
	}
}

func TestDeathFrameTimeline(t *testing.T) {
	cases := []struct {
		age, want int
	}{
		{0, 0},
		{15, 0},
		{16, 1},
		{49, 1},
		{50, 2},
	}
	for _, tc := range cases {
		if got := deathFrame(tc.age); got != tc.want {
			t.Errorf("deathFrame(%d) = %d, want %d", tc.age, got, tc.want)
		}
	}
}
