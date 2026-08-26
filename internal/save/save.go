// Package save serializes and restores a full game snapshot as JSON.
// It depends only on the plain-Go logic packages (world, building,
// resource, economy), not on ebiten -- a save file is just data.
package save

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"strategy_game/internal/building"
	"strategy_game/internal/economy"
	"strategy_game/internal/hunger"
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// FormatVersion is the current persisted schema. Version 1 remains readable:
// its 180-tick hunger gauge is migrated proportionally to the 1000-tick
// satiety model introduced in version 2.
const FormatVersion = 2

const (
	previousFormatVersion = 1
	previousHungerTicks   = 180
)

// GameState is the full serializable snapshot of a running game.
type GameState struct {
	Version int

	GridWidth  int
	GridHeight int
	Tiles      []world.Tile // row-major, length GridWidth*GridHeight

	Buildings []building.Building

	Stockpile resource.Stockpile

	Population economy.Population

	// Units stores the persistent part of every unit. Current routes are
	// intentionally rebuilt after loading, but the roster, position and hunger
	// state stay where the player saved them.
	Units []UnitState

	// TreeRegrowth stores delayed random respawn attempts after a tree was cut.
	// The queue is kept in the save so loading cannot silently reset the forest
	// cycle or create more trees than the map limit.
	TreeRegrowth []TreeRegrowthState
	TreeSeed     uint32

	// FishRegrowth stores delayed fry spawns after fish are caught. Each entry
	// remembers its original water body, so fish never respawn in another pond.
	FishRegrowth []FishRegrowthState
	FishSeed     uint32

	// Meal seeds preserve the pseudo-random choice among foods actually
	// available in a Tavern. Every controller has an independent stream so
	// loading does not silently reintroduce a fixed food preference.
	SerfMealSeed       uint32
	VillagerMealSeed   uint32
	LumberjackMealSeed uint32
	FishermanMealSeed  uint32

	CameraX    float64
	CameraY    float64
	CameraZoom float64
}

// UnitKind identifies a unit in a save file without coupling the save format
// to the numeric iota values of the simulation packages.
type UnitKind string

const (
	UnitSerf       UnitKind = "serf"
	UnitFarmer     UnitKind = "farmer"
	UnitBaker      UnitKind = "baker"
	UnitLumberjack UnitKind = "lumberjack"
	UnitWinemaker  UnitKind = "winemaker"
	UnitFisherman  UnitKind = "fisherman"
	UnitSwineherd  UnitKind = "swineherd"
	UnitButcher    UnitKind = "butcher"
)

// UnitState is the serializable part of a unit. HomeIndex points into the
// GameState.Buildings slice for resident workers; serfs use -1. State is
// currently meaningful for villagers, whose route can be rebuilt after load.
// Dismissing persists an already requested serf dismissal, so loading cannot
// quietly put a departing serf back into the labour pool.
type UnitState struct {
	Kind        UnitKind
	X           int
	Y           int
	HomeIndex   int
	HungerTicks int
	Starving    bool
	Dismissing  bool
	State       int
	TargetIndex int
	WorkTicks   int
	Cargo       resource.Type
	CargoAmount int
	Meal        resource.Type
}

// TreeRegrowthState is the persistent part of one delayed tree respawn.
// Seed chooses a reproducible random-looking free cell when the delay ends.
type TreeRegrowthState struct {
	Ticks       int
	TargetTicks int
	Seed        uint32
}

// FishRegrowthState is the persistent retry timer for one fish replacement.
// WaterX/WaterY identify the pond or lake component where the fry belongs.
type FishRegrowthState struct {
	WaterX, WaterY int
	Ticks          int
	TargetTicks    int
	Seed           uint32
}

// Save writes state as indented JSON to path, creating any missing
// parent directories.
func Save(path string, state GameState) error {
	state.Version = FormatVersion

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("save: encode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("save: create directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("save: write file: %w", err)
	}
	return nil
}

// Load reads and decodes a previously-saved GameState from path.
func Load(path string) (GameState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GameState{}, fmt.Errorf("save: read file: %w", err)
	}

	var state GameState
	if err := json.Unmarshal(data, &state); err != nil {
		return GameState{}, fmt.Errorf("save: decode: %w", err)
	}
	switch state.Version {
	case previousFormatVersion:
		migrateV1Hunger(&state)
		state.Version = FormatVersion
	case FormatVersion:
		// Current schema needs no migration.
	default:
		return GameState{}, fmt.Errorf("save: unsupported save format version %d (want %d or %d)", state.Version, previousFormatVersion, FormatVersion)
	}
	return state, nil
}

// migrateV1Hunger preserves a saved unit's approximate satiety when moving
// from the old 180-tick hungry timer to the 1000-tick percent scale. Old
// starving timers could grow without bound (there was no death yet), so a
// value is first clamped at the old meal-seeking point (180) -- the
// closest old-scale equivalent of hunger.MealThresholdTicks, not of
// hunger.MaxTicks. Scaling onto MaxTicks instead would put a merely
// hungry unit exactly at the new death threshold, killing it the instant
// the save loads.
func migrateV1Hunger(state *GameState) {
	for i := range state.Units {
		ticks := state.Units[i].HungerTicks
		if ticks < 0 {
			ticks = 0
		}
		if ticks > previousHungerTicks {
			ticks = previousHungerTicks
		}
		state.Units[i].HungerTicks = ticks * hunger.MealThresholdTicks / previousHungerTicks
	}
}
