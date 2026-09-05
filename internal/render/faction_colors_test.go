package render

import "testing"

// TestColorForOwner_EachKnownOwnerIsDistinct locks in the user's explicit
// request ("реализуй разные цвета разным ботам"): once a duel match can
// hold more than one AI faction, each bot needs its own marker color, not
// the single shared "any opponent" red the original two-faction version
// used -- see drawOwnerOutline/DrawOpponentUnitMarker, which both key off
// this same lookup.
func TestColorForOwner_EachKnownOwnerIsDistinct(t *testing.T) {
	seen := map[[4]uint8]int{}
	for owner := 1; owner <= 3; owner++ {
		c := colorForOwner(owner)
		key := [4]uint8{c.R, c.G, c.B, c.A}
		if other, ok := seen[key]; ok {
			t.Fatalf("owner %d and owner %d resolved to the same color %v", owner, other, c)
		}
		seen[key] = owner
	}
}

// TestColorForOwner_OwnerOneMatchesTheOriginalTwoFactionMarker keeps a
// 1-opponent match looking exactly as it always did -- red, the same
// color the pre-multi-opponent version hardcoded as opponentOutlineColor.
func TestColorForOwner_OwnerOneMatchesTheOriginalTwoFactionMarker(t *testing.T) {
	c := colorForOwner(1)
	if c.R < c.G || c.R < c.B {
		t.Fatalf("owner 1's color = %v, want a red-dominant color (unchanged from the original marker)", c)
	}
}
