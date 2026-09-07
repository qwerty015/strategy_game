package main

import "testing"

func TestLoadRestoresMatchHistory(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy})
	g.aiDefeatedAnnounced = map[int]bool{2: true}
	g.lastAttackerOwner = map[int]int{1: 0}
	path := t.TempDir() + "/match.json"
	if err := g.saveGame(path, "history"); err != nil {
		t.Fatal(err)
	}
	// Simulate playing further, then reloading an earlier save.
	g.aiDefeatedAnnounced[1] = true
	g.lastAttackerOwner[1] = 2
	if err := g.loadGame(path); err != nil {
		t.Fatal(err)
	}
	if g.aiDefeatedAnnounced[1] || !g.aiDefeatedAnnounced[2] {
		t.Fatalf("incorrect announcements: %v", g.aiDefeatedAnnounced)
	}
	if owner, ok := g.lastAttackerOwner[1]; !ok || owner != 0 {
		t.Fatalf("lost player attribution: %v", g.lastAttackerOwner)
	}
}

func TestLoadWithoutHistoryClearsPreviousMatch(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AIEasy})
	path := t.TempDir() + "/legacy.json"
	if err := g.saveGame(path, "legacy"); err != nil {
		t.Fatal(err)
	}
	g.aiDefeatedAnnounced = map[int]bool{1: true}
	g.lastAttackerOwner = map[int]int{1: 2}
	if err := g.loadGame(path); err != nil {
		t.Fatal(err)
	}
	if len(g.aiDefeatedAnnounced) != 0 || len(g.lastAttackerOwner) != 0 {
		t.Fatal("history leaked from previously loaded world")
	}
}
