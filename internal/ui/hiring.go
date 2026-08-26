package ui

// LeftTab selects the active category in the left-side construction/hiring
// panel. Keeping both actions in one panel prevents population controls from
// being scattered around the window.
type LeftTab int

const (
	BuildTab LeftTab = iota
	HireTab
	SettingsTab
)

// HireKind identifies a unit the player may add through the hire tab.
// Professional workers are constrained by their matching workplaces; serfs
// are the only town-wide unit and deliberately have no building limit.
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
)

// HireOption is the read-only state of a card in the hire menu. Limit is zero
// for an unlimited option (currently only serfs).
type HireOption struct {
	Kind      HireKind
	Current   int
	Limit     int
	Available bool
}
