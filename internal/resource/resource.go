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

	// Gold doubles as the game's hiring currency (cmd/game's trySpendGold
	// removes it directly from the Stockpile at the moment a unit is
	// hired) and as an ordinary smelted good: the Smeltery turns GoldOre +
	// Coal into Gold exactly like any other recipe, and serfs haul it like
	// any other output. Nothing else spends it -- production is the only
	// way to replenish it beyond the starting grant.
	Gold

	// Coal, GoldOre and IronOre are finite world resources mined by
	// package miner, the same Reserve-per-cell pattern as StoneDeposit.
	// The ores are raw and unusable on their own; the Smeltery turns
	// GoldOre or IronOre, plus Coal, into Gold or Iron. Coal is also its
	// own resource (not further processed) and, per the roadmap, is meant
	// to have other uses later.
	Coal
	GoldOre
	IronOre
	Iron
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
	Coal:       "coal",
	GoldOre:    "gold_ore",
	IronOre:    "iron_ore",
	Iron:       "iron",
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
	return []Type{Wheat, Flour, Bread, Fish, Wine, Sausage, Carcass, Log, Plank, StoneBlock, Gold, Coal, GoldOre, IronOre, Iron}
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
