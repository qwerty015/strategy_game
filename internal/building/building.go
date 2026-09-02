// Package building defines building types (as data: footprint, terrain
// rules, production recipe) and placed Building instances. Adding a new
// production chain later means adding entries to Types, not new logic.
package building

import (
	"slices"

	"strategy_game/internal/resource"
	"strategy_game/internal/world"
)

// Kind identifies a type of building.
type Kind int

const (
	Farm Kind = iota
	Mill
	Bakery

	// Warehouse is a logistics hub: serfs (see package logistics) deposit
	// collected goods here and draw from it to supply other buildings. The
	// first warehouse is the serf spawn point; additional warehouses share the
	// same town stockpile. It has no Recipe -- it produces nothing itself.
	Warehouse

	// Road is not really a "building" gameplay-wise, but reusing the
	// Building/placement machinery for it (1x1 footprint, buildable
	// anywhere) means the whole grid/overlap/save system already works
	// for roads for free. Serfs may only walk across Road tiles and the
	// footprints of the two buildings they're travelling between --
	// buildings not connected to the road network by an unbroken chain
	// of Road tiles can't be serviced. See package pathfind.
	Road

	// Tavern is where villagers (serfs, farmers, bakers and winemakers -- see
	// package villagers) go to eat. Like Warehouse it has no Recipe.Output (it
	// produces nothing), but it declares an accepted menu so logistics can
	// keep it stocked with any available food -- see Types[Tavern].
	Tavern

	// Tree is a world object rather than a player-buildable structure. It
	// occupies a tile and grows over time. A lumberjack can harvest it; the
	// player still cannot remove it directly.
	Tree

	// LumberjackHut is the workplace and temporary log store for one
	// lumberjack. It is appended after Tree to preserve the numeric Tree value
	// in existing save files.
	LumberjackHut

	// Winery is a vineyard and wine-making workshop. Its eight crop cells
	// occupy the tiles around the building sprite, just like a Farm.
	Winery

	// FisherHut is a one-tile workplace for a fisherman. It must stand on dry
	// land directly beside a water tile, which also becomes the boat launch.
	FisherHut

	// Fish is a persistent water-world object. It grows from fry to a mature
	// catchable fish and is never exposed in the construction palette.
	Fish

	// PigFarm and MeatWorkshop are appended after the pre-existing kinds so
	// their numeric values never shift in older save files, which serialize
	// Building.Kind directly.
	PigFarm
	MeatWorkshop

	// CarpentryWorkshop turns Log into Planks. Appended last for the same
	// save-compatibility reason as PigFarm/MeatWorkshop above.
	CarpentryWorkshop

	// StoneDeposit is a world object like Tree, but it does not grow or
	// regrow: it holds a finite Reserve (see StoneDepositReserve) that only
	// ever decreases. Map generation places a cluster of them as one region;
	// once a deposit's Reserve reaches zero it is removed, leaving ordinary
	// buildable ground behind -- there is no equivalent of Tree's regrowth
	// queue. Appended after CarpentryWorkshop for the same save-compatibility
	// reason as every other addition to this list.
	StoneDeposit

	// QuarryHut is the workplace and temporary stone-block store for one
	// quarryman (see package quarry). Appended last for the same reason.
	QuarryHut

	// CoalDeposit, GoldOreDeposit and IronOreDeposit are world objects like
	// StoneDeposit: a finite Reserve, no regrowth, generated as regions by
	// cmd/game. See resource.Coal/GoldOre/IronOre.
	CoalDeposit
	GoldOreDeposit
	IronOreDeposit

	// MinerHut is the workplace for one Miner (package miner), who cycles
	// between Coal/GoldOre/IronOre deposits on a fixed quota so no single
	// ore dominates just because it's closest -- see package miner's doc
	// comment.
	MinerHut

	// Smeltery turns GoldOre or IronOre, plus Coal, into Gold or Iron. It's
	// the first building with more than one Recipe (see Type.AltRecipes):
	// which one runs is decided per production cycle, not fixed at
	// placement, by whichever ore is actually on hand.
	Smeltery

	// StoneWall and Gate are appended to retain the numeric values written by
	// existing saves. A gate replaces one completed wall segment in place;
	// see WallAxisAt and the placement flow in cmd/game.
	StoneWall
	Gate

	// WatchTower and Barracks are the first defensive buildings. A
	// WatchTower holds one resident Sentry (package sentry) and up to
	// BufferCapacity stone in InputBuffer as ammunition, spent one unit
	// per shot -- see Type.PassiveInputs. A Barracks has no resident of
	// its own; it holds gold (also via PassiveInputs) and is where the
	// player hires a Sentry from its inspector, not from the ordinary
	// left-panel Hire tab -- see cmd/game's Barracks hire button.
	// Appended last for the same save-compatibility reason as every
	// other addition to this list.
	WatchTower
	Barracks

	// Armory is where a Weaponsmith (package villagers) turns Plank/Hide/
	// Iron+Coal into Bow/LeatherArmor/Sword, one item type at a time per
	// player-set queue -- see Building.ProductionQueue. Unlike every other
	// production building this doesn't run through economy.Tick at all
	// (three parallel queued lines don't fit the single-active-recipe
	// model); see cmd/game's tickArmories. Appended last for the same
	// save-compatibility reason as everything above.
	Armory
)

// Recipe describes how a building turns raw resources into a product
// over a number of simulation ticks. An empty Inputs map means the
// building gathers from the land instead of consuming stockpiled goods
// (e.g. a Farm).
type Recipe struct {
	Inputs         map[resource.Type]int
	Output         resource.Type
	OutputAmount   int
	TicksToProduce int

	// ConsumeInputsAtStart reserves and removes the cycle's inputs before
	// progress starts. It models a living animal: a PigFarm must receive all
	// feed before a pig can begin its 600-tick growth cycle. Other recipes
	// retain the normal completion-time consumption behaviour.
	//
	// Not supported in combination with Type.AltRecipes -- a multi-recipe
	// building always consumes at completion (see Building.ActiveRecipe).
	ConsumeInputsAtStart bool

	// SecondaryOutput/SecondaryOutputAmount is a second product added
	// alongside Output in the same cycle -- currently only the PigFarm
	// (Carcass and Hide from one pig, simultaneously, per the user's
	// explicit request). Zero value (SecondaryOutputAmount == 0) means
	// "no second output"; every other recipe is unaffected. See
	// economy.hasOutputRoom/tickRecipe/tickPrepaidRecipe/tickMultiRecipe.
	SecondaryOutput       resource.Type
	SecondaryOutputAmount int
}

// Point is a world-grid coordinate used for building access points.
type Point struct{ X, Y int }

// Type describes a kind of building: how big it is, what terrain it can
// sit on, and what it produces.
type Type struct {
	Kind      Kind
	Name      string
	Footprint int // buildings are Footprint x Footprint tiles

	// AccessX and AccessY identify the single tile where a road must meet
	// the building. For a Farm this is the farmhouse tile; for a 1x1
	// building it is (0, 0).
	AccessX, AccessY int

	// AllowedTerrain lists the terrain types this building may be placed
	// on. An empty slice means "any buildable (non-water) terrain". A type may
	// explicitly list Water for a passive world object such as Fish.
	AllowedTerrain []world.TerrainType

	Recipe Recipe

	// AltRecipes lists additional recipes this building can also run,
	// beyond the primary Recipe -- currently only the Smeltery (smelt gold
	// or smelt iron). Empty for every other building, which keeps their
	// behavior through economy.TickWithConnectivity completely unchanged;
	// see AllRecipes and Building.ActiveRecipe.
	AltRecipes []Recipe

	// RequiresWorker marks production or gathering buildings that pause
	// without their single assigned resident. The simulation layer uses this
	// data to avoid hard-coding profession lists.
	RequiresWorker bool

	// OutputCapacity overrides the normal small production buffer when a
	// building harvests several field cells at once. Zero uses BufferCapacity.
	OutputCapacity int

	// AcceptedResources is used by service buildings that can consume more
	// than one food type. Tavern accepts every current and planned food type;
	// the menu is deliberately not ordered by gameplay value.
	AcceptedResources []resource.Type

	// PassiveInputs are resources this building wants delivered up to
	// BufferCapacity that economy.TickWithConnectivity never touches --
	// unlike Recipe.Inputs, nothing auto-converts them into a production
	// Output on a timer. InputRequirement below folds this in, which is
	// the only thing that makes logistics deliver them at all. Used by
	// the WatchTower's stone ammunition (spent one unit per shot by
	// package sentry) and the Barracks' gold (spent one unit per hire by
	// cmd/game's Barracks hire button) -- both need a steady supply
	// without a Recipe pretending to "produce" something from it.
	PassiveInputs map[resource.Type]int

	// PlankCost/StoneCost/IronCost are the construction units a Builder needs
	// delivered before finishing this building (or, for Road, before
	// finishing one tile of it). Zero for kinds that are never placed
	// through the normal construction flow (Tree, Fish, StoneDeposit).
	PlankCost int
	StoneCost int
	IronCost  int

	// ConstructionFoundationTicks/ConstructionBuildTicks are how long a
	// Builder spends on each of the two construction phases -- see
	// ConstructionStage. The foundation phase needs no materials at all;
	// the build phase needs PlankCost/StoneCost already delivered.
	ConstructionFoundationTicks int
	ConstructionBuildTicks      int
}

// AllRecipes returns every recipe this building kind can run -- the
// primary Recipe (if it has one) followed by AltRecipes, skipping any
// zero-value entry (TicksToProduce <= 0). For every building except the
// Smeltery this is exactly []Recipe{t.Recipe} or empty, so callers written
// against a single recipe keep working unchanged.
func (t Type) AllRecipes() []Recipe {
	var out []Recipe
	if t.Recipe.TicksToProduce > 0 {
		out = append(out, t.Recipe)
	}
	for _, r := range t.AltRecipes {
		if r.TicksToProduce > 0 {
			out = append(out, r)
		}
	}
	return out
}

// InputRequirement reports the largest buffer target for rt across every
// runnable recipe of this type. Logistics uses it instead of looking only at
// Type.Recipe.Inputs, because a multi-recipe building such as the Smeltery
// must receive inputs for its alternative recipe too (IronOre as well as
// GoldOre). The loop is deliberately allocation-free: job search calls it for
// many buildings and idle serfs on a large map.
func (t Type) InputRequirement(rt resource.Type) int {
	need := 0
	if t.Recipe.TicksToProduce > 0 {
		need = t.Recipe.Inputs[rt]
	}
	for _, recipe := range t.AltRecipes {
		if recipe.TicksToProduce > 0 && recipe.Inputs[rt] > need {
			need = recipe.Inputs[rt]
		}
	}
	if t.PassiveInputs[rt] > need {
		need = t.PassiveInputs[rt]
	}
	return need
}

// AccessPoint returns the world tile that serves as this building's door or
// loading point. Roads connected to another part of a multi-tile building do
// not make that building reachable.
func (b *Building) AccessPoint() Point {
	bt := Types[b.Kind]
	return Point{X: b.X + bt.AccessX, Y: b.Y + bt.AccessY}
}

// IsOperationalWarehouse reports whether b is a finished Warehouse that can
// serve as a logistics endpoint. A Warehouse construction site still uses the
// same Kind so it can show the future building's art, but it must not expose
// the shared stockpile or be chosen as a pickup/dropoff point until the
// Builder has completed it.
func (b *Building) IsOperationalWarehouse() bool {
	return b != nil && b.Kind == Warehouse && b.ConstructionStage == ConstructionNone
}

// CanBuildOn reports whether a single tile satisfies this building
// type's terrain requirement. It does not check footprint or overlap;
// see CanPlace for the full placement check.
func (t Type) CanBuildOn(tile world.Tile) bool {
	if !tile.Buildable() {
		return slices.Contains(t.AllowedTerrain, tile.Terrain)
	}
	if len(t.AllowedTerrain) == 0 {
		return true
	}
	return slices.Contains(t.AllowedTerrain, tile.Terrain)
}

// BufferCapacity caps how many units of any single resource type a
// building can hold in its InputBuffer or OutputBuffer. Small on
// purpose: a producer with a full OutputBuffer pauses production until a
// serf clears space, which is what makes logistics (roads + serfs)
// matter instead of being invisible plumbing. 6 matches the reference
// the user asked to follow (wheat/flour/bread and other ordinary production
// buffers hold 6 each; only the Warehouse is unlimited).
const BufferCapacity = 6

// MaxHP is a fully healthy building's HP value, 0-100. See Building.HP's
// doc comment for why 0 in an old save means "not yet migrated", not
// "destroyed".
const MaxHP = 100

// Building is a placed instance of a Type on the grid.
//
// Buildings do not talk to the shared warehouse stockpile directly --
// see AGENTS.md. A producer accumulates its product in OutputBuffer; a
// consumer draws its raw materials from InputBuffer. Moving goods
// between these buffers and the warehouse is entirely the job of serfs
// (package logistics), which is why a building disconnected from the
// road network simply stalls instead of producing.
type Building struct {
	Kind          Kind
	X, Y          int // top-left tile of the footprint
	ProgressTicks int // ticks accumulated toward the current production cycle

	// Owner identifies which faction this building belongs to -- 0 (the
	// zero value) is the human player, 1 is the AI opponent in "1v1 vs
	// AI" mode, leaving room for more factions later. Every building in
	// the ordinary single-player "free map" mode is Owner 0 by construction
	// (nothing ever sets it there), so old saves and every existing
	// single-faction code path need no migration at all. See AGENTS.md's
	// "1×1 против ИИ" notes for the two-faction architecture this enables.
	Owner int

	InputBuffer  map[resource.Type]int
	OutputBuffer map[resource.Type]int

	// Tree growth is kept on the placed object so it survives save/load and
	// each tree can have its own deterministic random-looking lifetime.
	GrowthTicks       int
	GrowthTargetTicks int

	// Reserve is the remaining extractable amount of a finite world resource
	// (currently only StoneDeposit). Unlike GrowthTicks it only ever
	// decreases; there is no regrowth. Zero for every other building kind.
	Reserve int

	// ConstructionStage is ConstructionNone (the zero value) for a
	// finished building -- which is exactly what every pre-existing
	// Building in an old save, and every world object never placed
	// through construction (Tree, Fish, StoneDeposit), already is,
	// without any migration needed. While non-zero the building performs
	// no production and has no resident worker; ProgressTicks counts
	// ticks within the *current* stage (reused, not a second counter),
	// and InputBuffer holds Plank/StoneBlock/Iron delivered so far toward
	// Type.PlankCost/StoneCost -- see AddConstructionMaterial.
	ConstructionStage ConstructionStage

	// ActiveRecipe indexes into Types[Kind].AllRecipes(): which recipe is
	// running the current production cycle (meaningless, always 0, for a
	// building with only one recipe). Also remembers which recipe ran last
	// once a cycle completes, so the next cycle's search for a runnable
	// recipe starts after it instead of always preferring index 0 -- see
	// economy.pickRecipe's doc comment for why that fairness matters.
	ActiveRecipe int

	// GateOpen and GateAuto describe a completed Gate's traffic state. Gates
	// are stored as ordinary Buildings, therefore these fields automatically
	// survive a save/load without a separate migration. GateAxis is fixed on
	// installation, so removing a neighbouring wall segment cannot rotate a
	// gate sprite. GateReplacesWall lets cancellation restore the original
	// finished wall rather than leaving an accidental breach.
	GateOpen         bool
	GateAuto         bool
	GateAxis         WallAxis
	GateReplacesWall bool

	// HP is 0-100, this instance's structural health -- see package combat
	// for how it changes (ApplyDamage/Repair operate on the plain int, so
	// this package needs no dependency on combat). A save from before this
	// field existed leaves it at the JSON zero value on load; see
	// internal/save's migrateBuildingHP, which turns that into MaxHP
	// rather than "destroyed". NewConstructionSite sets it to MaxHP for
	// every newly placed building, so only a load path needs the
	// migration.
	HP int

	// ProductionQueue is how many of each item type the player has asked
	// an Armory to produce (Bow/LeatherArmor/Sword) -- see cmd/game's
	// tickArmories and the queue buttons in internal/ui/panels.go. nil for
	// every other building kind, and for a fresh Armory until the player
	// first queues something (map access on a nil map reads as 0, exactly
	// the right default).
	ProductionQueue map[resource.Type]int
}

// ConstructionStage is where a placed-but-unfinished building or road
// currently stands in package builder's two-phase process: a builder
// starts on-site work immediately (ConstructionFoundation, no materials
// needed yet), then either waits for delivery (ConstructionWaitingMaterials)
// or -- if a serf already got there first -- goes straight to finishing
// (ConstructionFinishing).
type ConstructionStage int

const (
	ConstructionNone ConstructionStage = iota
	ConstructionFoundation
	ConstructionWaitingMaterials
	ConstructionFinishing
)

// NewConstructionSite creates a placed-but-unfinished Building of kind at
// (x, y), starting in the foundation stage. Used by the normal build-palette
// placement flow; Tree/Fish/StoneDeposit use their own dedicated
// constructors and are never "under construction".
func NewConstructionSite(kind Kind, x, y int) *Building {
	return &Building{Kind: kind, X: x, Y: y, ConstructionStage: ConstructionFoundation, HP: MaxHP}
}

// AddConstructionMaterial deposits up to n units of a construction
// material (Plank, StoneBlock or Iron) into a site's InputBuffer, capped by this
// building kind's total requirement -- not the usual BufferCapacity=6, a
// build can need up to 15 planks. Any other resource type is rejected.
// Returns how many units actually fit, like AddInput.
func (b *Building) AddConstructionMaterial(t resource.Type, n int) int {
	limit := b.ConstructionMaterialCost(t)
	if limit <= 0 {
		return 0
	}
	return addCapped(&b.InputBuffer, t, n, limit)
}

// ConstructionMaterialCost returns how many units of t (Plank or
// StoneBlock; zero for any other type) this building kind needs delivered
// in total before construction can finish. Used both to cap
// AddConstructionMaterial and as the "target" passed to
// reservations.Ledger.RoomFor by the logistics job search -- the same
// shape every other consumer's recipe.Inputs[rt] already uses there, just
// sourced from the construction cost instead of the finished recipe.
func (b *Building) ConstructionMaterialCost(t resource.Type) int {
	switch t {
	case resource.Plank:
		return Types[b.Kind].PlankCost
	case resource.StoneBlock:
		return Types[b.Kind].StoneCost
	case resource.Iron:
		return Types[b.Kind].IronCost
	default:
		return 0
	}
}

// ConstructionMaterialsReady reports whether every required Plank/
// StoneBlock unit has already been delivered to this site.
func (b *Building) ConstructionMaterialsReady() bool {
	t := Types[b.Kind]
	return b.InputBuffer[resource.Plank] >= t.PlankCost &&
		b.InputBuffer[resource.StoneBlock] >= t.StoneCost &&
		b.InputBuffer[resource.Iron] >= t.IronCost
}

// ConstructionProgress returns a clamped 0..1 value across both
// construction phases combined, for the progress bar shown on an
// unfinished site -- see render.DrawBuildings and the inspector panel.
func (b *Building) ConstructionProgress() float64 {
	if b == nil || b.ConstructionStage == ConstructionNone {
		return 1
	}
	t := Types[b.Kind]
	total := t.ConstructionFoundationTicks + t.ConstructionBuildTicks
	if total <= 0 {
		return 0
	}
	done := 0
	switch b.ConstructionStage {
	case ConstructionFoundation:
		done = b.ProgressTicks
	case ConstructionWaitingMaterials:
		done = t.ConstructionFoundationTicks
	case ConstructionFinishing:
		done = t.ConstructionFoundationTicks + b.ProgressTicks
	}
	progress := float64(done) / float64(total)
	if progress > 1 {
		return 1
	}
	if progress < 0 {
		return 0
	}
	return progress
}

const (
	// TreeGrowthMinTicks is two minutes at the normal two simulation ticks
	// per second. The extra random-looking part makes a grove grow unevenly.
	TreeGrowthMinTicks       = 240
	TreeGrowthVariationTicks = 240

	// Fish grow on the same deliberately slow 2-4 minute rhythm as trees.
	// Initial fish are seeded at mixed stages so a new fishing hut has some
	// immediate targets instead of waiting through an entirely empty pond.
	FishGrowthMinTicks       = 240
	FishGrowthVariationTicks = 240
)

// NewTree creates an indestructible tree with a stable per-coordinate growth
// target. The target is deterministic, so saving and loading never changes
// how long that particular tree takes to mature.
func NewTree(x, y int) *Building {
	return &Building{
		Kind:              Tree,
		X:                 x,
		Y:                 y,
		GrowthTargetTicks: treeGrowthTarget(x, y),
	}
}

// NewFish creates an uncatchable fry. Its deterministic growth target keeps
// the visual stage stable across save/load just like a tree's stage.
func NewFish(x, y int) *Building {
	return &Building{
		Kind:              Fish,
		X:                 x,
		Y:                 y,
		GrowthTargetTicks: fishGrowthTarget(x, y),
	}
}

// StoneDepositReserve is the fixed starting Reserve of a freshly placed
// stone deposit cell, per the game design. Raised from 10000 to 15000 per
// the user's explicit request ("поместим в 1 клетку не 10к, а 15к
// ресурсов, это позволит нам сократить общее количество клеток"): fewer
// deposit cells on the generated map (see cmd/game's stoneMinPercent etc.),
// each holding more, keeps the total mineable amount roughly the same
// while reducing visual clutter.
const StoneDepositReserve = 15000

// NewStoneDeposit creates a stone deposit at full reserve.
func NewStoneDeposit(x, y int) *Building {
	return &Building{
		Kind:    StoneDeposit,
		X:       x,
		Y:       y,
		Reserve: StoneDepositReserve,
	}
}

// OreDepositReserve is the fixed starting Reserve of a freshly placed
// coal/gold ore/iron ore deposit cell -- the same 15000-per-cell convention
// as stone (see StoneDepositReserve's doc comment), kept as its own
// constant so the two can be tuned independently later without ambiguity
// about which deposits a change affects.
const OreDepositReserve = 15000

// NewOreDeposit creates a CoalDeposit/GoldOreDeposit/IronOreDeposit at full
// reserve. kind must be one of those three; any other value still returns a
// Building (so a caller mistake doesn't panic), just with a Kind nothing
// else in the game recognizes as mineable.
func NewOreDeposit(kind Kind, x, y int) *Building {
	return &Building{
		Kind:    kind,
		X:       x,
		Y:       y,
		Reserve: OreDepositReserve,
	}
}

// TickGrowth advances a tree or fish by one simulation tick. Other buildings
// are ignored so the caller can safely tick the whole building slice.
func (b *Building) TickGrowth() {
	if b == nil || (b.Kind != Tree && b.Kind != Fish) {
		return
	}
	if b.GrowthTargetTicks <= 0 {
		if b.Kind == Fish {
			b.GrowthTargetTicks = fishGrowthTarget(b.X, b.Y)
		} else {
			b.GrowthTargetTicks = treeGrowthTarget(b.X, b.Y)
		}
	}
	if b.GrowthTicks < b.GrowthTargetTicks {
		b.GrowthTicks++
	}
}

// GrowthProgress returns a clamped 0..1 value for a growing world object.
func (b *Building) GrowthProgress() float64 {
	if b == nil || (b.Kind != Tree && b.Kind != Fish) || b.GrowthTargetTicks <= 0 {
		return 0
	}
	progress := float64(b.GrowthTicks) / float64(b.GrowthTargetTicks)
	if progress > 1 {
		return 1
	}
	if progress < 0 {
		return 0
	}
	return progress
}

// GrowthStage returns 0 for a young object and 2 for a mature tree or fish.
func (b *Building) GrowthStage() int {
	progress := b.GrowthProgress()
	switch {
	case progress >= 0.66:
		return 2
	case progress >= 0.33:
		return 1
	default:
		return 0
	}
}

func treeGrowthTarget(x, y int) int {
	seed := uint32(x)*73856093 ^ uint32(y)*19349663 ^ 0x9e3779b9
	return TreeGrowthMinTicks + int(seed%TreeGrowthVariationTicks)
}

func fishGrowthTarget(x, y int) int {
	seed := uint32(x)*83492791 ^ uint32(y)*2654435761 ^ 0x7f4a7c15
	return FishGrowthMinTicks + int(seed%FishGrowthVariationTicks)
}

// WaterAccessPoint returns a cardinal water tile immediately beside b. The
// order is stable, so a hut touching several water cells keeps the same pier
// direction after save/load. It is primarily used by FisherHut.
func WaterAccessPoint(g *world.Grid, b *Building) (Point, bool) {
	if g == nil || b == nil {
		return Point{}, false
	}
	for _, d := range [...]Point{{X: 0, Y: 1}, {X: -1, Y: 0}, {X: 0, Y: -1}, {X: 1, Y: 0}} {
		x, y := b.X+d.X, b.Y+d.Y
		if g.InBounds(x, y) && g.At(x, y).Terrain == world.Water {
			return Point{X: x, Y: y}, true
		}
	}
	return Point{}, false
}

// AddOutput deposits up to n units of t into OutputBuffer, capped by the
// building's output limit, and returns how many units actually fit.
func (b *Building) AddOutput(t resource.Type, n int) int {
	capacity := BufferCapacity
	if b != nil && Types[b.Kind].OutputCapacity > 0 {
		capacity = Types[b.Kind].OutputCapacity
	}
	return addCapped(&b.OutputBuffer, t, n, capacity)
}

// OutputLimit returns the maximum amount of one product this building can
// keep in its output buffer.
func (b *Building) OutputLimit() int {
	if b != nil && Types[b.Kind].OutputCapacity > 0 {
		return Types[b.Kind].OutputCapacity
	}
	return BufferCapacity
}

// TakeOutput removes n units of t from OutputBuffer if available
// (all-or-nothing) and reports whether it succeeded.
func (b *Building) TakeOutput(t resource.Type, n int) bool {
	return takeAvailable(b.OutputBuffer, t, n)
}

// AddInput deposits up to n units of t into InputBuffer, capped by
// BufferCapacity, and returns how many units actually fit.
func (b *Building) AddInput(t resource.Type, n int) int {
	return addCapped(&b.InputBuffer, t, n, BufferCapacity)
}

// TakeInput removes n units of t from InputBuffer if available
// (all-or-nothing) and reports whether it succeeded.
func (b *Building) TakeInput(t resource.Type, n int) bool {
	return takeAvailable(b.InputBuffer, t, n)
}

func addCapped(buf *map[resource.Type]int, t resource.Type, n, capacity int) int {
	if n <= 0 {
		return 0
	}
	if *buf == nil {
		*buf = make(map[resource.Type]int)
	}
	room := capacity - (*buf)[t]
	if room <= 0 {
		return 0
	}
	if n > room {
		n = room
	}
	(*buf)[t] += n
	return n
}

func takeAvailable(buf map[resource.Type]int, t resource.Type, n int) bool {
	if n <= 0 {
		return true
	}
	if buf[t] < n {
		return false
	}
	buf[t] -= n
	return true
}
