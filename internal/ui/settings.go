package ui

// SaveSlotInfo is the read-only display state of one Esc pause-menu slot.
type SaveSlotInfo struct {
	Name     string
	Occupied bool
	// Autosave marks this slot as the current periodic-autosave target
	// (see Game.autosave) -- at most one slot is ever true at a time.
	Autosave bool
}

// SaveSlotAction identifies the save or load request for a pause-menu slot.
type SaveSlotAction int

const (
	SaveSlotNone SaveSlotAction = iota
	SaveSlotSave
	SaveSlotLoad
)

// DialogKind identifies the active confirmation or save dialog. The pause menu
// owns save/load flow; the inspector owns removal confirmation.
type DialogKind int

const (
	DialogNone DialogKind = iota
	DialogNaming
	DialogConfirmOverwrite
	DialogConfirmNewGame
	DialogConfirmRemoval
	DialogConfirmDemolitionMode
	DialogConfirmExit
)
