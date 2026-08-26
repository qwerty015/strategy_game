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
			{Kind: building.LumberjackHut, X: 8, Y: 4, OutputBuffer: map[resource.Type]int{resource.Log: 2}},
			{Kind: building.Tree, X: 10, Y: 4, GrowthTicks: 33, GrowthTargetTicks: 240},
		},
		Stockpile:  *stock,
		Population: economy.Population{Count: 4},
		Units: []UnitState{
			{Kind: UnitSerf, X: 7, Y: 8, HomeIndex: -1, HungerTicks: 12, Starving: true},
			{Kind: UnitFarmer, X: 4, Y: 5, HomeIndex: 0, HungerTicks: 21, State: 0},
			{Kind: UnitLumberjack, X: 9, Y: 4, HomeIndex: 2, HungerTicks: 44, State: 2, TargetIndex: 3, WorkTicks: 7, Cargo: resource.Log, CargoAmount: 1},
		},
		TreeRegrowth:       []TreeRegrowthState{{Ticks: 20, TargetTicks: 200, Seed: 42}},
		TreeSeed:           12345,
		SerfMealSeed:       11,
		VillagerMealSeed:   22,
		LumberjackMealSeed: 33,
		FishermanMealSeed:  44,
		CameraX:            128.5,
		CameraY:            64,
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
