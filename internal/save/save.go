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
// satiety model introduced in version 2. Version 3 additionally persists
// the player's active play time; version 4 separates demolition and unit
// dismissal counters in Population.
const FormatVersion = 4

const (
	firstFormatVersion     = 1
	playTimeFormatVersion  = 2
	separateRemovalStatsV3 = 3
	previousHungerTicks    = 180
)

// GameState is the full serializable snapshot of a running game.
type GameState struct {
	Version int

	// Name is the player-chosen label for a named save slot (see the side
	// panel's save/load UI). Empty for the single quicksave slot, which
	// has no name of its own.
	Name string

	GridWidth  int
	GridHeight int
	Tiles      []world.Tile // row-major, length GridWidth*GridHeight

	Buildings []building.Building

	Stockpile resource.Stockpile

	Population economy.Population

	// PlayedFrames is active play time measured in 60Hz update frames. It is
	// excluded while paused and is independent of the simulation speed, so it
	// represents the time the player actually spent in this settlement.
	PlayedFrames int

	// Units stores the persistent part of every unit. Current routes are
	// intentionally rebuilt after loading, but the roster, position and hunger
	// state stay where the player saved them.
	Units []UnitState

	// BuildingPriority stores every building kind the player gave a
	// non-default supply priority (see logistics.Controller.SetPriority).
	// A kind absent from this list stays at the default priority.
	BuildingPriority []BuildingPriorityState

	// TreeRegrowth stores delayed random respawn attempts after a tree was cut.
	// The queue is kept in the save so loading cannot silently reset the forest
	// cycle or create more trees than the map limit.
	TreeRegrowth []TreeRegrowthState
	TreeSeed     uint32

	// FishRegrowth stores delayed fry spawns after fish are caught. Each entry
	// remembers its original water body, so fish never respawn in another pond.
	FishRegrowth []FishRegrowthState
	FishSeed     uint32

	// StoneSeeded marks that a stone-deposit region has already been
	// generated for this world. Unlike trees/fish, deposits never regrow, so
	// there is no regrowth-queue length to infer this from: a legitimately
	// fully-mined region (every StoneDeposit removed) must not be
	// reseeded on load, and this explicit flag is what tells the two
	// cases apart. Absent (false) in saves from before this feature, which
	// is exactly when a fresh region should be generated.
	StoneSeeded bool

	// OreSeeded is StoneSeeded's counterpart for the Coal/GoldOre/IronOre
	// regions -- one flag for all three, since they're always generated
	// together in the same NewGame call. Same reasoning: there's no
	// regrowth queue to infer "never had this feature" from, so a
	// legitimately fully-mined set of ore regions must not be reseeded.
	OreSeeded bool

	// Meal seeds preserve the pseudo-random choice among foods actually
	// available in a Tavern. Every controller has an independent stream so
	// loading does not silently reintroduce a fixed food preference.
	SerfMealSeed       uint32
	VillagerMealSeed   uint32
	LumberjackMealSeed uint32
	FishermanMealSeed  uint32
	QuarrymanMealSeed  uint32
	BuilderMealSeed    uint32
	MinerMealSeed      uint32

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
	UnitCarpenter  UnitKind = "carpenter"
	UnitQuarryman  UnitKind = "quarryman"
	UnitBuilder    UnitKind = "builder"
	UnitMiner      UnitKind = "miner"
	UnitSmelter    UnitKind = "smelter"
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

	// QuotaIndex/QuotaProgress are meaningful only for UnitMiner: which
	// entry of miner.DefaultQuota this worker is currently cycling
	// through, and how many units into it. Zero (the default) is a
	// perfectly valid state -- "just starting the cycle over" -- so a
	// save from before this field existed loads with no special handling.
	QuotaIndex    int
	QuotaProgress int
}

// TreeRegrowthState is the persistent part of one delayed tree respawn.
// Seed chooses a reproducible random-looking free cell when the delay ends.
// BuildingPriorityState is one entry of a saved supply-priority setting:
// every building of Kind gets Level (see logistics.Controller.SetPriority).
type BuildingPriorityState struct {
	Kind  building.Kind
	Level int
}

// OriginX/OriginY is where the cut tree that made room for this regrowth
// actually stood, so the replacement respawns near there instead of
// anywhere on the map -- see cmd/game's treeRegrowthRadius. Zero-value
// (0,0) in a save from before this field existed is harmless: it's just
// treated as any other origin point, retried later if nothing qualifies
// nearby.
type TreeRegrowthState struct {
	OriginX     int
	OriginY     int
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

// PeekName reports the Name and existence of a save file at path without
// decoding (or version-migrating) the full GameState. Used by the side
// panel's slot list to show five slots' worth of names every frame without
// paying the cost of a full unmarshal for each one.
func PeekName(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var partial struct{ Name string }
	if err := json.Unmarshal(data, &partial); err != nil {
		return "", false
	}
	return partial.Name, true
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
	case firstFormatVersion:
		migrateV1Hunger(&state)
		migrateLegacyRemovalCounter(&state)
		state.Version = FormatVersion
	case playTimeFormatVersion:
		// Version 2 has no play-time field; JSON leaves it at zero.
		migrateLegacyRemovalCounter(&state)
		state.Version = FormatVersion
	case separateRemovalStatsV3:
		migrateLegacyRemovalCounter(&state)
		state.Version = FormatVersion
	case FormatVersion:
		// Current schema needs no migration.
	default:
		return GameState{}, fmt.Errorf("save: unsupported save format version %d (want %d, %d, %d, or %d)", state.Version, firstFormatVersion, playTimeFormatVersion, separateRemovalStatsV3, FormatVersion)
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
// migrateLegacyRemovalCounter preserves the only historical total available in
// pre-v4 saves. They did not distinguish demolition from dismissal, so the
// value is shown as demolished structures rather than discarded silently.
func migrateLegacyRemovalCounter(state *GameState) {
	if state.Population.BuildingsRemoved == 0 && state.Population.UnitsDismissed == 0 {
		state.Population.BuildingsRemoved = state.Population.Removed
	}
	state.Population.Removed = 0
}

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
