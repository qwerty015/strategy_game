package ui

// SaveSlotInfo is the read-only display state of one save-panel slot, as
// shown in the settings tab's slot list.
type SaveSlotInfo struct {
	Name     string
	Occupied bool
	// Autosave marks this slot as the current periodic-autosave target
	// (see Game.autosave) -- at most one slot is ever true at a time.
	Autosave bool
}

// SettingsSlotAction identifies which half of a save-slot row the cursor is
// over: its Save button or its Load button.
type SettingsSlotAction int

const (
	SettingsSlotNone SettingsSlotAction = iota
	SettingsSlotSave
	SettingsSlotLoad
)

// DialogKind is the active side-panel modal: the settings tab owns save/load
// dialogs, while the inspector owns the generic confirmation before any
// removable unit or building is changed.
type DialogKind int

const (
	DialogNone DialogKind = iota
	DialogNaming
	DialogConfirmOverwrite
	DialogConfirmNewGame
	DialogConfirmRemoval
)

// IsSettingsDialog reports whether dialog belongs in the Settings-tab slot
// area. Inspector confirmation deliberately remains visible over the object
// that is about to be removed.
func IsSettingsDialog(dialog DialogKind) bool {
	switch dialog {
	case DialogNaming, DialogConfirmOverwrite, DialogConfirmNewGame:
		return true
	default:
		return false
	}
}
