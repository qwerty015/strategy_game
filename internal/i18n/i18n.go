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

	BuildMenuTitle string
	BuildTab       string
	HireTab        string
	SettingsTab    string
	InspectorTitle string
	InspectorHint  string
	SpeedTitle     string
	SpeedPaused    string
	SpeedHalf      string
	SpeedNormal    string
	SpeedDouble    string
	SpeedQuadruple string

	StateLabel               string
	CargoLabel               string
	RouteLabel               string
	HungerLabel              string
	ProfessionLabel          string
	HomeLabel                string
	BuildingLabel            string
	InputLabel               string
	OutputLabel              string
	ContentsLabel            string
	PeopleInsideLabel        string
	PriorityLabel            string
	UnlimitedLabel           string
	GrowthLabel              string
	HarvestableLabel         string
	StoneReserveLabel        string
	RoadLabel                string
	Connected                string
	Disconnected             string
	NoCargo                  string
	NoRoute                  string
	StateIdle                string
	StateWorking             string
	StateWalking             string
	StateDelivering          string
	StateEating              string
	StateStarving            string
	StateSearching           string
	StateChopping            string
	StateMining              string
	StateFishing             string
	StateUnloading           string
	StateLeaving             string
	UnitSerf                 string
	UnitFarmer               string
	UnitBaker                string
	UnitLumberjack           string
	UnitWinemaker            string
	UnitFisherman            string
	UnitSwineherd            string
	UnitButcher              string
	UnitCarpenter            string
	UnitQuarryman            string
	HireSerf                 string
	Deleted                  string
	SerfDismissRequested     string
	CannotDeleteWarehouse    string
	CannotDeleteTree         string
	CannotDeleteFish         string
	CannotDeleteStoneDeposit string
	CatchableLabel           string

	Help                  string
	CantBuildHere         string
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
