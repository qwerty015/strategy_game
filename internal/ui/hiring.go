package ui

// LeftTab selects the active category in the left-side construction/hiring
// panel. Keeping both actions in one panel prevents population controls from
// being scattered around the window.
type LeftTab int

const (
	BuildTab LeftTab = iota
	HireTab
)

// HireKind identifies a unit the player may add through the hire tab.
// Professional workers are constrained by their matching workplaces; serfs
// are the only town-wide unit and deliberately have no building limit.
// Builder is the one exception in between: town-wide like a serf (no
// dedicated hut), but capped at a flat number (see cmd/game's maxBuilders)
// rather than by building count.
type HireKind int

const (
	HireSerf HireKind = iota
	HireFarmer
	HireBaker
	HireWinemaker
	HireLumberjack
	HireFisherman
	HireSwineherd
	HireButcher
	HireCarpenter
	HireQuarryman
	HireBuilder
	HireMiner
	HireSmelter
)

// HireOption is the read-only state of a card in the hire menu. Limit is zero
// for an unlimited option (currently only serfs).
type HireOption struct {
	Kind      HireKind
	Current   int
	Limit     int
	Available bool
}
