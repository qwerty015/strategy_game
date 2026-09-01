package save

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/hunger"
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
			{Kind: building.Gate, X: 11, Y: 4, GateOpen: true, GateAuto: true, GateAxis: building.WallVertical, GateReplacesWall: true, InputBuffer: map[resource.Type]int{resource.Iron: 3}},
		},
		Stockpile:    *stock,
		Population:   economy.Population{Count: 4},
		PlayedFrames: 54321,
		Units: []UnitState{
			{Kind: UnitSerf, X: 7, Y: 8, HomeIndex: -1, HungerTicks: 12, Starving: true},
			{Kind: UnitFarmer, X: 4, Y: 5, HomeIndex: 0, HungerTicks: 21, State: 0},
			{Kind: UnitLumberjack, X: 9, Y: 4, HomeIndex: 2, HungerTicks: 44, State: 2, TargetIndex: 3, WorkTicks: 7, Cargo: resource.Log, CargoAmount: 1},
		},
		BuildingPriority:   []BuildingPriorityState{{Kind: building.Mill, Level: -1}, {Kind: building.PigFarm, Level: 2}},
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

// TestLoad_MigratesV1HungerToSatietyScale covers the version-1 compatibility
// path: an old save's 180-tick hunger gauge must be rescaled onto the
// 1000-tick satiety model, not loaded as-is (which would misread, say, a
// unit at the old "just started walking to eat" point as already almost
// starved to death on the new scale).
func TestLoad_MigratesV1HungerToSatietyScale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slot1.json")
	state := GameState{
		GridWidth:  1,
		GridHeight: 1,
		Tiles:      []world.Tile{{}},
		Units: []UnitState{
			{Kind: UnitSerf, HomeIndex: -1, HungerTicks: 90},    // halfway to the old meal point
			{Kind: UnitFarmer, HomeIndex: -1, HungerTicks: 500}, // far past the old 180-tick cap
		},
	}
	if err := Save(path, state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	writeVersion(t, path, firstFormatVersion)

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.Version != FormatVersion {
		t.Fatalf("Version after migration = %d, want %d", got.Version, FormatVersion)
	}
	// 90/180 of the old scale becomes 90/180 of the new meal threshold.
	if got, want := got.Units[0].HungerTicks, 90*hunger.MealThresholdTicks/previousHungerTicks; got != want {
		t.Errorf("migrated HungerTicks[0] = %d, want %d", got, want)
	}
	// A value already past the old cap is clamped to it first, so it lands
	// exactly at the new meal threshold -- nowhere near hunger.MaxTicks
	// (death). Scaling onto MaxTicks instead would kill this unit the
	// instant the save loads, which is exactly the bug this guards.
	if got, want := got.Units[1].HungerTicks, hunger.MealThresholdTicks; got != want {
		t.Errorf("migrated HungerTicks[1] = %d, want %d (clamped-then-scaled onto the meal threshold, not death)", got, want)
	}
	if hunger.Dead(got.Units[1].HungerTicks) {
		t.Fatal("migrated unit is already dead on load")
	}
}

// TestPeekName covers the side panel's slot-list use case: reading a save's
// Name without paying for a full versioned Load, and correctly reporting
// "not occupied" for a path that has never been saved to.
func TestPeekName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel_slot_1.json")
	if err := Save(path, GameState{Name: "Моя деревня", GridWidth: 1, GridHeight: 1, Tiles: []world.Tile{{}}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	name, ok := PeekName(path)
	if !ok {
		t.Fatal("PeekName() ok = false for a file that was just saved")
	}
	if name != "Моя деревня" {
		t.Errorf("PeekName() name = %q, want %q", name, "Моя деревня")
	}

	if _, ok := PeekName(filepath.Join(t.TempDir(), "never_saved.json")); ok {
		t.Error("PeekName() ok = true for a path that was never saved to")
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

func TestMigrateLegacyRemovalCounter(t *testing.T) {
	state := GameState{Population: economy.Population{Removed: 9}}

	migrateLegacyRemovalCounter(&state)

	if got := state.Population.BuildingsRemoved; got != 9 {
		t.Errorf("BuildingsRemoved = %d, want legacy total 9", got)
	}
	if got := state.Population.UnitsDismissed; got != 0 {
		t.Errorf("UnitsDismissed = %d, want 0 because old saves cannot split it", got)
	}
	if got := state.Population.Removed; got != 0 {
		t.Errorf("legacy Removed = %d, want cleared", got)
	}
}
