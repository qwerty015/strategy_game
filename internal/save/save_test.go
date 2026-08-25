package save

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	grid := world.NewTestGrid()

	stock := resource.NewStockpile(200)
	stock.Add(resource.Wheat, 12)
	stock.Add(resource.Bread, 3)

	want := GameState{
		GridWidth:  grid.Width,
		GridHeight: grid.Height,
		Tiles:      grid.Tiles(),
		Buildings: []building.Building{
			{Kind: building.Farm, X: 3, Y: 4, ProgressTicks: 2},
			{Kind: building.Mill, X: 6, Y: 4, ProgressTicks: 0},
		},
		Stockpile:  *stock,
		Population: economy.Population{Count: 4, TicksPerMeal: 6, TicksSinceMeal: 3},
		CameraX:    128.5,
		CameraY:    64,
	}

	path := filepath.Join(t.TempDir(), "slot1.json")

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Load stamps Version on the struct it hands back via the same field
	// Save sets; align it before comparing the rest.
	want.Version = FormatVersion

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-tripped state differs:\ngot:  %+v\nwant: %+v", got, want)
	}
}

func TestLoad_RejectsWrongVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slot1.json")
	if err := Save(path, GameState{GridWidth: 1, GridHeight: 1, Tiles: []world.Tile{{}}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Corrupt the version field to simulate a save from a future/older format.
	writeVersion(t, path, FormatVersion+1)

	if _, err := Load(path); err == nil {
		t.Fatal("Load() with mismatched version succeeded, want error")
	}
}

// writeVersion patches the Version field of an already-saved file in
// place, to simulate loading a save from a mismatched format version.
func writeVersion(t *testing.T, path string, version int) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	raw["Version"] = version
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
