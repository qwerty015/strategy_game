// Package resource defines the kinds of goods that flow through the
// economy, and the shared stockpile that holds them.
package resource

import "fmt"

// Type identifies a kind of resource that can sit in a Stockpile.
type Type int

const (
	Wheat Type = iota
	Flour
	Bread
	Fish
	Wine
	Sausage
	Carcass
	Log
	Plank
	StoneBlock

	// Gold is currency, not a hauled good: no building produces or accepts
	// it, and no serf ever carries it. It only ever changes by a direct
	// Stockpile.Remove/Add at the moment a unit is hired -- see
	// cmd/game's trySpendGold.
	Gold
)

var typeNames = map[Type]string{
	Wheat:      "wheat",
	Flour:      "flour",
	Bread:      "bread",
	Fish:       "fish",
	Wine:       "wine",
	Sausage:    "sausage",
	Carcass:    "carcass",
	Log:        "log",
	Plank:      "plank",
	StoneBlock: "stone_block",
	Gold:       "gold",
}

var namesToType = func() map[string]Type {
	m := make(map[string]Type, len(typeNames))
	for t, name := range typeNames {
		m[name] = t
	}
	return m
}()

func (t Type) String() string {
	if name, ok := typeNames[t]; ok {
		return name
	}
	return fmt.Sprintf("Type(%d)", int(t))
}

// MarshalText implements encoding.TextMarshaler so Type values serialize
// as stable names (e.g. "wheat") instead of raw enum numbers. That keeps
// save files (and map keys, which JSON encodes via TextMarshaler) valid
// even if the enum's iota order changes later.
func (t Type) MarshalText() ([]byte, error) {
	if name, ok := typeNames[t]; ok {
		return []byte(name), nil
	}
	return nil, fmt.Errorf("resource: unknown Type %d", int(t))
}

// UnmarshalText implements encoding.TextUnmarshaler, the counterpart to
// MarshalText.
func (t *Type) UnmarshalText(data []byte) error {
	name := string(data)
	got, ok := namesToType[name]
	if !ok {
		return fmt.Errorf("resource: unknown resource name %q", name)
	}
	*t = got
	return nil
}

// AllTypes returns resources in a stable display order. Maps
// are intentionally used for buffers, but UI and service logic must not
// change order from one frame to the next.
func AllTypes() []Type {
	return []Type{Wheat, Flour, Bread, Fish, Wine, Sausage, Carcass, Log, Plank, StoneBlock, Gold}
}

// FoodTypes returns every resource that can feed a unit in a Tavern. The
// order is stable for reproducible job selection, but it is not a gameplay
// priority: bread, fish, wine and sausage are interchangeable meals.
func FoodTypes() []Type {
	return []Type{Bread, Fish, Wine, Sausage}
}

// IsFood reports whether a resource can be consumed as a Tavern meal.
func IsFood(t Type) bool {
	for _, food := range FoodTypes() {
		if food == t {
			return true
		}
	}
	return false
}
