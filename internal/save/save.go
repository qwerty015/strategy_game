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
	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// FormatVersion increases whenever GameState's shape changes in a way
// that would break decoding older saves. Load checks it so a stale save
// fails loudly instead of decoding into a half-valid state.
const FormatVersion = 1

// GameState is the full serializable snapshot of a running game.
type GameState struct {
	Version int

	GridWidth  int
	GridHeight int
	Tiles      []world.Tile // row-major, length GridWidth*GridHeight

	Buildings []building.Building

	Stockpile resource.Stockpile

	Population economy.Population

	CameraX float64
	CameraY float64
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
	if state.Version != FormatVersion {
		return GameState{}, fmt.Errorf("save: unsupported save format version %d (want %d)", state.Version, FormatVersion)
	}
	return state, nil
}
