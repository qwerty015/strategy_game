// Package reservations gives every worker controller (logistics,
// villagers, lumberjack) a single shared, same-tick view of resources
// already promised to a unit that has already committed to fetching or
// delivering them. Without it, several idle units -- possibly from
// different controllers, e.g. a serf and a hungry lumberjack -- can each
// independently see the same 6 units, or the Tavern's one remaining
// loaf of Bread, as "available" and all set off for it at once, since
// nothing marks it as spoken for until the first one actually arrives
// (see AGENTS.md).
//
// A Ledger is rebuilt from scratch once per simulation tick, in
// cmd/game's Update loop: every controller first seeds it with its own
// in-flight units via Reserve, then the same Ledger is threaded through
// every controller's Tick call in turn, so a claim made by the first
// controller this tick is already visible to the next one. Nothing
// needs to be released explicitly -- once a unit's trip ends
// (delivered, cancelled, or failed), it simply stops being seeded next
// tick, and the ledger reflects that automatically because it is
// rebuilt fresh every time.
package reservations

import (
	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

// Ledger is the shared same-tick reservation view. The zero value is not
// usable; construct with New.
type Ledger struct {
	pickup  map[*building.Building]map[resource.Type]int
	dropoff map[*building.Building]map[resource.Type]int
	stock   map[resource.Type]int
}

// New creates an empty ledger. Call once per simulation tick, before any
// controller's Reserve or Tick runs.
func New() *Ledger {
	return &Ledger{
		pickup:  make(map[*building.Building]map[resource.Type]int),
		dropoff: make(map[*building.Building]map[resource.Type]int),
		stock:   make(map[resource.Type]int),
	}
}

// ReservePickup marks n units of t at b as already promised to a unit
// that will collect them -- from OutputBuffer for an ordinary producer,
// or from InputBuffer for a building like the Tavern that gets consumed
// from directly. Pickups from any Warehouse draw on the one shared
// stockpile instead of a specific building, since several warehouses
// share it.
func (l *Ledger) ReservePickup(b *building.Building, t resource.Type, n int) {
	if b.Kind == building.Warehouse {
		l.stock[t] += n
		return
	}
	if l.pickup[b] == nil {
		l.pickup[b] = make(map[resource.Type]int)
	}
	l.pickup[b][t] += n
}

// ReserveDropoff marks n units of t as already promised into b's
// InputBuffer. A Warehouse has no capacity limit, so its dropoffs are
// never worth tracking.
func (l *Ledger) ReserveDropoff(b *building.Building, t resource.Type, n int) {
	if b.Kind == building.Warehouse {
		return
	}
	if l.dropoff[b] == nil {
		l.dropoff[b] = make(map[resource.Type]int)
	}
	l.dropoff[b][t] += n
}

// AvailableOutput is how much of t is sitting in b's OutputBuffer and
// not already promised to another unit.
func (l *Ledger) AvailableOutput(b *building.Building, t resource.Type) int {
	return b.OutputBuffer[t] - l.pickup[b][t]
}

// AvailableInput is the same idea for a building that gets consumed from
// directly rather than hauled onward (the Tavern's Bread).
func (l *Ledger) AvailableInput(b *building.Building, t resource.Type) int {
	return b.InputBuffer[t] - l.pickup[b][t]
}

// AvailableStock is how much of t is in the shared warehouse stockpile
// and not already promised to a unit walking to collect it.
func (l *Ledger) AvailableStock(stock *resource.Stockpile, t resource.Type) int {
	if stock == nil {
		return 0
	}
	return stock.Amount(t) - l.stock[t]
}

// RoomFor is how much more of t building b can still be promised, given
// a target level -- a recipe requirement for an ordinary consumer, or
// BufferCapacity for a building (the Tavern) that just wants to stay
// topped up regardless of any single recipe.
func (l *Ledger) RoomFor(b *building.Building, t resource.Type, target int) int {
	return target - b.InputBuffer[t] - l.dropoff[b][t]
}
