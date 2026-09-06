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
	HireWeaponsmith
)

// HireOption is the read-only state of a card in the hire menu. Limit is zero
// for an unlimited option (currently only serfs).
type HireOption struct {
	Kind      HireKind
	Current   int
	Limit     int
	Available bool

	// GoldCost is the exact price of hiring this unit. It is supplied by the
	// game rules rather than duplicated by the UI, so the card always matches
	// the amount actually deducted when the player clicks it.
	GoldCost int

	// Recommended is a suggested headcount, shown on the card when > 0.
	// Currently only set for HireSerf (see cmd/game's
	// recommendedServeCount), per the user's explicit request: "в меню
	// 'Юниты' рядом со слугами показывать рекомендацию сколько
	// рекомендуется слуг".
	Recommended int
}

// HasEmptyWorkplace reports whether at least one building matching this
// profession stands finished but unstaffed -- per the user's explicit
// request ("во вкладке 'юниты' выделяй красным юнитов которых нет
// (пустые здания)"). A Limit of zero (serfs, the one town-wide unit with
// no dedicated building) never counts as "empty" -- there's no building
// to be missing a worker in.
func (o HireOption) HasEmptyWorkplace() bool {
	return o.Limit > 0 && o.Current < o.Limit
}
