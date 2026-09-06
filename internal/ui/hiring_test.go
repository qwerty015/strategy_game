package ui

import "testing"

// TestHireOption_HasEmptyWorkplace is the regression for the user's
// explicit request ("во вкладке 'юниты' выделяй красным юнитов которых
// нет (пустые здания)"): a card must flag as an empty workplace exactly
// when a real building of that profession stands unstaffed -- never for
// serfs (Limit 0, no dedicated building to be missing a worker in), and
// never once every matching building is staffed.
func TestHireOption_HasEmptyWorkplace(t *testing.T) {
	cases := []struct {
		name   string
		option HireOption
		want   bool
	}{
		{"no buildings of this kind at all", HireOption{Limit: 0, Current: 0}, false},
		{"one empty building", HireOption{Limit: 1, Current: 0}, true},
		{"some staffed, one still empty", HireOption{Limit: 3, Current: 2}, true},
		{"fully staffed", HireOption{Limit: 3, Current: 3}, false},
		{"serf: unlimited, never flagged", HireOption{Limit: 0, Current: 5}, false},
	}
	for _, c := range cases {
		if got := c.option.HasEmptyWorkplace(); got != c.want {
			t.Errorf("%s: HasEmptyWorkplace() = %v, want %v", c.name, got, c.want)
		}
	}
}
