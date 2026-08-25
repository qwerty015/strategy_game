package world

import (
	"reflect"
	"testing"
)

func TestGrid_InBounds(t *testing.T) {
	g := NewGrid(5, 3)

	tests := []struct {
		x, y int
		want bool
	}{
		{0, 0, true},
		{4, 2, true},
		{5, 0, false},
		{0, 3, false},
		{-1, 0, false},
	}
	for _, tc := range tests {
		if got := g.InBounds(tc.x, tc.y); got != tc.want {
			t.Errorf("InBounds(%d, %d) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestGrid_SetAt(t *testing.T) {
	g := NewGrid(5, 5)
	g.Set(2, 3, Tile{Terrain: Stone})

	if got := g.At(2, 3).Terrain; got != Stone {
		t.Errorf("At(2, 3) = %v, want Stone", got)
	}
	if got := g.At(0, 0).Terrain; got != Grass {
		t.Errorf("At(0, 0) = %v, want Grass (default)", got)
	}
}

func TestGrid_TilesRoundTrip(t *testing.T) {
	original := NewTestGrid()

	rebuilt, err := NewGridFromTiles(original.Width, original.Height, original.Tiles())
	if err != nil {
		t.Fatalf("NewGridFromTiles() error = %v", err)
	}

	if !reflect.DeepEqual(original.Tiles(), rebuilt.Tiles()) {
		t.Error("rebuilt grid's tiles differ from the original")
	}
}

func TestNewGridFromTiles_RejectsWrongLength(t *testing.T) {
	if _, err := NewGridFromTiles(3, 3, make([]Tile, 5)); err == nil {
		t.Error("NewGridFromTiles with mismatched tile count succeeded, want error")
	}
}
