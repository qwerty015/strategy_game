package hunger

import "testing"

func TestPercent(t *testing.T) {
	cases := []struct {
		elapsed int
		want    int
	}{
		{elapsed: -5, want: 100}, // never fed yet clamps up, not down
		{elapsed: 0, want: 100},
		{elapsed: 10, want: 99},
		{elapsed: MealThresholdTicks, want: 20}, // 800 ticks elapsed -> 20% satiety
		{elapsed: MaxTicks, want: 0},
		{elapsed: MaxTicks + 500, want: 0}, // clamps at 0, never negative
	}
	for _, c := range cases {
		if got := Percent(c.elapsed); got != c.want {
			t.Errorf("Percent(%d) = %d, want %d", c.elapsed, got, c.want)
		}
	}
}

func TestNeedsMeal(t *testing.T) {
	if NeedsMeal(MealThresholdTicks - 1) {
		t.Error("NeedsMeal just below the threshold = true, want false")
	}
	if !NeedsMeal(MealThresholdTicks) {
		t.Error("NeedsMeal exactly at the threshold = false, want true")
	}
}

func TestDead(t *testing.T) {
	if Dead(MaxTicks - 1) {
		t.Error("Dead just below MaxTicks = true, want false")
	}
	if !Dead(MaxTicks) {
		t.Error("Dead exactly at MaxTicks = false, want true")
	}
}
