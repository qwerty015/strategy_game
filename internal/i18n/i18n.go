// Package i18n holds every user-facing string in the game, grouped into
// per-language catalogs (one file per language: ru.go, en.go, ...).
// Adding a new translation later means adding one new file here and
// registering it in an init() -- no other package needs to change.
package i18n

import (
	"strategy_game/internal/building"
	"strategy_game/internal/resource"
)

// Lang identifies a supported language.
type Lang string

const (
	RU Lang = "ru"
	EN Lang = "en"
)

// Default is the language active until SetLang changes it.
const Default = RU

// Catalog holds every translatable string used by the running game.
type Catalog struct {
	WindowTitle string

	Population   string
	DeathsLabel  string
	RemovedLabel string
	ResourceName map[resource.Type]string
	BuildingName map[building.Kind]string

	BuildMenuTitle   string
	BuildTab         string
	HireTab          string
	SettingsTab      string
	InspectorTitle   string
	InspectorHint    string
	MusicVolumeLabel string
	SFXVolumeLabel   string
	SpeedTitle       string
	SpeedPaused      string
	SpeedHalf        string
	SpeedNormal      string
	SpeedDouble      string
	SpeedQuadruple   string
	SpeedOctuple     string
	SpeedSixteenfold string

	StateLabel                  string
	CargoLabel                  string
	RouteLabel                  string
	HungerLabel                 string
	ProfessionLabel             string
	HomeLabel                   string
	BuildingLabel               string
	InputLabel                  string
	OutputLabel                 string
	ContentsLabel               string
	PeopleInsideLabel           string
	PriorityLabel               string
	UnlimitedLabel              string
	GrowthLabel                 string
	HarvestableLabel            string
	StoneReserveLabel           string
	DepositReserveLabel         string
	QuotaLabel                  string
	ConstructionProgressLabel   string
	ConstructionFoundationLabel string
	ConstructionWaitingLabel    string
	ConstructionFinishingLabel  string
	RoadLabel                   string
	Connected                   string
	Disconnected                string
	NoCargo                     string
	NoRoute                     string
	StateIdle                   string
	StateWorking                string
	StateWalking                string
	StateDelivering             string
	StateEating                 string
	StateStarving               string
	StateSearching              string
	StateChopping               string
	StateMining                 string
	StateBuilding               string
	StateWaitingMaterials       string
	StateFishing                string
	StateUnloading              string
	StateLeaving                string
	UnitSerf                    string
	UnitFarmer                  string
	UnitBaker                   string
	UnitLumberjack              string
	UnitWinemaker               string
	UnitFisherman               string
	UnitSwineherd               string
	UnitButcher                 string
	UnitCarpenter               string
	UnitQuarryman               string
	UnitBuilder                 string
	UnitMiner                   string
	UnitSmelter                 string
	HireSerf                    string
	RemoveSelected              string
	DismissSerf                 string
	DemolitionMode              string
	DemolitionModeActive        string
	ConfirmDemolitionModePrompt string
	ConfirmDemolitionModeButton string
	ConfirmRemovalPrompt        string // formatted with the selected object name
	ConfirmRemovalButton        string
	Deleted                     string
	SerfDismissRequested        string
	SerfsTrimmedToRecommended   string // formatted with (dismissed count, recommended headcount)
	TrimServesConfirmPrompt     string // formatted with (current count, recommended headcount)
	ConfirmYesButton            string
	ConfirmNoButton             string
	BuilderDismissRequested     string
	RecommendedServeCountLabel  string // formatted with the recommended headcount, e.g. "реком. %d"

	// Advisor toast (see internal/advisor and cmd/game's advisorTipText).
	// Every AdvisorTip* string is formatted with that tip's own numbers
	// (ticks left, building counts, serf counts) -- see advisorTipText
	// for the exact argument order per kind.
	AdvisorAcknowledgeButton       string
	AdvisorTipFoodRunningOut       string // %d ticks left
	AdvisorTipIdleBuilding         string // %d idle buildings, %d,%d one example's coordinates
	AdvisorTipDisconnectedBuilding string // %d disconnected buildings, %d,%d one example's coordinates
	AdvisorTipServeCountLow        string // %d current, %d recommended
	AdvisorTipServeCountHigh       string // %d current, %d recommended
	AdvisorTipGatherWorkerStuck    string // %d workers stuck, %d,%d one example's coordinates
	CannotDeleteWarehouse          string
	CannotDeleteTree               string
	CannotDeleteFish               string
	CannotDeleteStoneDeposit       string
	CatchableLabel                 string

	CantBuildHere         string
	NotEnoughGold         string
	SaveFailedPrefix      string
	LoadFailedPrefix      string
	Saved                 string
	Loaded                string
	LoadFailedNoWarehouse string

	SaveSlotsLabel      string
	SlotEmptyLabel      string
	SlotSaveButton      string
	SlotLoadButton      string
	SlotNamePrompt      string // formatted with the slot number
	SlotOverwritePrompt string // formatted with the slot number and its existing name
	SlotOverwriteButton string
	SlotCancelButton    string
	SlotDefaultName     string // fallback base name when a slot is saved with an empty typed name
	AutosaveToggle      string // short label on each slot's autosave on/off button
	Autosaved           string // status line after a silent periodic autosave

	NewGameButton        string
	NewGameConfirmPrompt string
	NewGameConfirmButton string
	NewGameStarted       string

	PauseMenuTitle    string
	ResumeButton      string
	HelpButton        string
	ExitButton        string
	ExitConfirmPrompt string
	ExitConfirmButton string
	LanguageLabel     string
}

var catalogs = map[Lang]Catalog{}

// register adds a language's catalog to the registry. Called from each
// language file's init().
func register(l Lang, c Catalog) {
	catalogs[l] = c
}

var current = Default

// SetLang switches the active language. An unrecognized language is
// ignored and the previously active one stays in effect.
func SetLang(l Lang) {
	if _, ok := catalogs[l]; ok {
		current = l
	}
}

// Current returns the active language.
func Current() Lang {
	return current
}

// T returns the catalog for the active language.
func T() Catalog {
	return catalogs[current]
}
