package ui

// SaveSlotInfo is the read-only display state of one save-panel slot, as
// shown in the settings tab's slot list.
type SaveSlotInfo struct {
	Name     string
	Occupied bool
}

// SettingsSlotAction identifies which half of a save-slot row the cursor is
// over: its Save button or its Load button.
type SettingsSlotAction int

const (
	SettingsSlotNone SettingsSlotAction = iota
	SettingsSlotSave
	SettingsSlotLoad
)

// DialogKind is the active side-panel modal for the settings tab's save
// slots: none, typing a name for a slot about to be saved, or confirming an
// overwrite of an already-occupied slot before naming it.
type DialogKind int

const (
	DialogNone DialogKind = iota
	DialogNaming
	DialogConfirmOverwrite
)
